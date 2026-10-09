import Foundation
import Observation

/// Everything the apps show about one server, kept fresh by the event socket.
@MainActor
@Observable
public final class AppStore {
    public private(set) var server: FoundServer?
    /// Servers this device has connected to, newest first. The demo is not kept.
    public private(set) var remembered: [FoundServer] = []
    public private(set) var info: ServerInfo?
    public private(set) var api: APIClient?
    public private(set) var socket: EventSocket?

    public private(set) var channels: [Channel] = []
    public private(set) var index = GuideIndex([])
    public private(set) var recordings: [Recording] = []
    /// Library channels: recordings played around the clock without a tuner.
    public private(set) var virtuals: [VirtualChannel] = []
    public private(set) var passes: [Pass] = []
    /// Channel ids whose preview JPEG is already on the server.
    public private(set) var frameIDs: Set<Int64> = []
    public private(set) var loading = false
    public var error: String?
    public var now = Date()
    /// The event socket has been down for a few seconds, so the lists on screen are the saved copy.
    public private(set) var offline = false
    /// When the lists on screen last came from the server.
    public private(set) var freshAt: Date?
    private var offlineWait: Task<Void, Never>?
    /// A device that showed up after the house was already known. Nil when nothing is waiting.
    public private(set) var homeNotice: String?
    private var homeQueue: [String] = []
    /// This screen's name as it announces itself. The server tells every screen
    /// about a new one, the new one included.
    public var screenName = ""
    /// Settings asks the shell to show setup again. A used catalog never sets needsSetup.
    public var presentSetup = false

    public var prefs: Prefs {
        didSet { save(prefs, "prefs") }
    }

    public var syncEnabled: Bool {
        didSet { UserDefaults.standard.set(syncEnabled, forKey: "sync") }
    }

    private var clock: Timer?
    private var frameTick = 0
    private var announced = false
    private var relocateAfter = Date.distantPast
    /// Bumped on connect and forget, so a move that started earlier cannot undo them.
    private var generation = 0
    /// Bumped when this screen changes a recording, so an older list does not undo it.
    private var recordingsEpoch = 0
    /// Bumped when this screen changes a pass, so an older list does not undo it.
    private var passesEpoch = 0
    /// Lineup and event calls share this session. Tests pass their own.
    private let session: URLSession
    #if DEBUG
        /// When set, a saved demo waits on this instead of the loopback player.
        var prepareDemo: (@MainActor () async -> URL?)?
        /// A test connect must not write the process-wide server list.
        var persistServers = true
    #endif

    public convenience init() {
        self.init(session: .shared)
    }

    init(session: URLSession) {
        self.session = session
        prefs = Self.load("prefs") ?? Prefs()
        syncEnabled = UserDefaults.standard.object(forKey: "sync") as? Bool ?? true
        remembered = Self.load("servers") ?? []
        var resumeDemo: FoundServer?
        #if DEBUG
            let holdConnect = UserDefaults.standard.bool(forKey: "BroadwaveDiscover") || UserDefaults.standard.bool(forKey: "BroadwaveExplain")
            let forcedURL = UserDefaults.standard.string(forKey: "BroadwaveServerURL")
        #else
            let holdConnect = false
            let forcedURL: String? = nil
        #endif
        if !holdConnect, let raw = forcedURL, let url = URL(string: raw) {
            connect(FoundServer(id: "pending", name: "Server", url: url))
        } else if !holdConnect, let saved: FoundServer = Self.load("server") {
            if let snap = CatalogCache.load(serverID: saved.id) {
                channels = snap.channels
                index = GuideIndex(snap.airings)
                recordings = snap.recordings
                freshAt = CatalogCache.savedAt(serverID: saved.id)
            }
            if saved.id == "demo" {
                server = saved
                api = APIClient(base: saved.url, session: session)
                resumeDemo = saved
            } else {
                connect(saved)
            }
        }
        if remembered.isEmpty, let saved: FoundServer = Self.load("server") {
            remembered = RememberedServers.upsert(remembered, saved)
        }
        // Keychain items outlive the app, so a reinstall must not leave the old address for the Top Shelf.
        if server == nil || server?.id == "demo" {
            SharedServer.save(nil)
        }
        clock = Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in
            Task { @MainActor in
                guard let self else { return }
                self.now = Date()
                self.frameTick += 1
                if self.frameTick >= 4 {
                    self.frameTick = 0
                    await self.refreshFrames()
                }
            }
        }
        if let resumeDemo {
            let started = generation
            Task { await self.resumeSavedDemo(resumeDemo, started: started) }
        }
    }

    /// The saved demo starts after the player is up. A server chosen in that gap wins.
    func resumeSavedDemo(_ server: FoundServer, started: Int) async {
        let url: URL?
        #if DEBUG
            if let prepareDemo {
                url = await prepareDemo()
            } else {
                url = await DemoServer.shared.prepare()
            }
        #else
            url = await DemoServer.shared.prepare()
        #endif
        // Forget or a real server during prepare must stick. A failed player
        // only wipes the demo that was waiting, not the server chosen since.
        guard generation == started else { return }
        guard url != nil else {
            forget()
            error = "The demo did not start."
            return
        }
        connect(server)
    }

    public var connected: Bool {
        api != nil
    }

    /// True while this screen is playing the bundled films.
    public var demo: Bool {
        server?.id == "demo"
    }

    /// Starts the loopback demo, or joins one already running on this Mac.
    public func startDemo() async {
        guard let url = await DemoServer.shared.prepare() else {
            error = "The demo did not start."
            return
        }
        error = nil
        connect(FoundServer(id: "demo", name: "Demo", url: url))
    }

    public func connect(_ server: FoundServer) {
        generation += 1
        if Self.dropCatalog(previousID: self.server?.id, nextID: server.id) {
            channels = []
            index = GuideIndex([])
            recordings = []
            passes = []
            virtuals = []
            freshAt = nil
            error = nil
        }
        socket?.disconnect()
        // A new server is not the previous one's outage. The banner waits out a short drop.
        offline = false
        // Down until the first message, so a server that is off at launch gets the banner.
        noteConnection(false)
        announced = false
        self.server = server
        let api = APIClient(base: server.url, session: session)
        self.api = api
        let socket = EventSocket(base: server.url)
        socket.onFailure = { [weak self] in
            Task { await self?.relocate() }
        }
        socket.onConnected = { [weak self] up in
            self?.noteConnection(up)
        }
        // Events sent while this screen was away are gone, so read everything again.
        socket.on("reconnected") { [weak self] _ in
            Task { await self?.refresh() }
        }
        socket.on("activity") { [weak self] data in
            if let note = try? JSONDecoder().decode(HomeNote.self, from: data), note.kind == "home" {
                self?.noteHome(note.message)
            }
            Task { await self?.refresh(lineup: false) }
        }
        socket.on("live.changed") { [weak self] _ in
            Task {
                await self?.refreshRecordings()
                await self?.refreshFrames()
            }
        }
        socket.connect()
        self.socket = socket
        save(server, "server")
        // The demo lives inside the app, where the Top Shelf cannot reach it.
        SharedServer.save(server.id == "demo" ? nil : server.url)
        remembered = RememberedServers.upsert(remembered, server)
        save(remembered, "servers")
        frameIDs = []
        Task { await refresh() }
    }

    /// Looks on the local network for a saved server whose address changed.
    /// Nil until the stored key verifies the reply and GET /server agrees.
    public func locate(_ id: String) async -> FoundServer? {
        let saved = remembered.first { $0.id == id } ?? (server?.id == id ? server : nil)
        guard let saved, let key = saved.key, !key.isEmpty else { return nil }
        let found = await Task.detached { LANProbe.collect(timeout: 1.2) }.value
        guard let next = ServerFollow.updated(saved, found: found) else { return nil }
        return await proven(next, key: key)
    }

    /// Connects to the saved server when a probe finds it at a new address.
    public func relocate() async {
        guard Date() >= relocateAfter else { return }
        relocateAfter = Date().addingTimeInterval(5)
        guard let saved = server, let key = saved.key, !key.isEmpty else { return }
        let savedID = saved.id
        let started = generation
        let found = await Task.detached { LANProbe.collect(timeout: 1.2) }.value
        guard Self.shouldApplyMove(savedID: savedID, currentID: server?.id, epoch: generation, captured: started),
              let next = ServerFollow.updated(saved, found: found) else { return }
        guard let proven = await proven(next, key: key) else { return }
        guard Self.shouldApplyMove(savedID: savedID, currentID: server?.id, epoch: generation, captured: started) else { return }
        NSLog("broadwave followed %@", proven.url.absoluteString)
        connect(proven)
    }

    /// GET /server at the candidate must return the same id and the stored key
    /// before any event socket or saved settings go there.
    private func proven(_ candidate: FoundServer, key: String) async -> FoundServer? {
        guard FinderPacket.isLocal(candidate.url) else { return nil }
        guard let info = try? await APIClient(base: candidate.url, session: session).server() else { return nil }
        guard info.id == candidate.id, info.discoveryKey == key else { return nil }
        let name = info.name.isEmpty ? candidate.name : info.name
        return FoundServer(id: info.id, name: name, url: candidate.url, key: key)
    }

    /// Shows the next arrival. One line stays up until it is dismissed.
    public func dismissHome() {
        if homeQueue.isEmpty {
            homeNotice = nil
            return
        }
        homeNotice = homeQueue.removeFirst()
    }

    func noteHome(_ message: String) {
        let message = message.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !message.isEmpty, !Self.namesScreen(message, screenName) else { return }
        if homeNotice == nil {
            homeNotice = message
        } else {
            homeQueue.append(message)
        }
    }

    /// "New iPad found: Den iPad." on the screen named Den iPad.
    nonisolated static func namesScreen(_ message: String, _ name: String) -> Bool {
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        return !name.isEmpty && message.hasPrefix("New ") && message.hasSuffix(" found: \(name).")
    }

    public func forget() {
        generation += 1
        offlineWait?.cancel()
        offlineWait = nil
        offline = false
        loading = false
        socket?.disconnect()
        socket = nil
        api = nil
        server = nil
        info = nil
        channels = []
        index = GuideIndex([])
        recordings = []
        passes = []
        virtuals = []
        frameIDs = []
        freshAt = nil
        error = nil
        homeNotice = nil
        homeQueue = []
        #if DEBUG
            if persistServers {
                UserDefaults.standard.removeObject(forKey: "server")
            }
        #else
            UserDefaults.standard.removeObject(forKey: "server")
        #endif
        SharedServer.save(nil)
    }

    /// A fetch that started on another server, or before Forget, must not land here.
    private func sameSession(_ started: Int, _ base: URL) -> Bool {
        generation == started && api?.base == base
    }

    /// A probe that started before Forget, or before another server was chosen, must not connect.
    public nonisolated static func shouldApplyMove(savedID: String, currentID: String?, epoch: Int, captured: Int) -> Bool {
        currentID == savedID && epoch == captured
    }

    /// A cold launch has no previous server, so the cached lineup stays until refresh.
    public nonisolated static func dropCatalog(previousID: String?, nextID: String) -> Bool {
        guard let previousID else { return false }
        return previousID != nextID
    }

    #if DEBUG
        /// Simulator testing: shows an arrival line as if the server had sent it.
        public func showHomeNotice(_ message: String) {
            noteHome(message)
        }

        /// Offline tiles for the tvOS remote test. It does not open a socket or a tuner.
        public func previewLineup(_ channels: [Channel]) {
            self.channels = channels
            let url = URL(string: "http://127.0.0.1:9")!
            server = FoundServer(id: "preview", name: "Preview", url: url)
            api = APIClient(base: url, session: session)
        }
    #endif

    public func refresh(lineup: Bool = true) async {
        #if DEBUG
            if UserDefaults.standard.bool(forKey: "BroadwaveMultiviewTest") {
                return
            }
        #endif
        guard let api else { return }
        let started = generation
        let base = api.base
        let listedAt = recordingsEpoch
        await refreshFrames()
        guard sameSession(started, base) else { return }
        loading = true
        defer {
            if generation == started {
                loading = false
            }
        }
        do {
            let moment = Date()
            async let fetchedInfo = api.server()
            async let fetchedChannels = api.channels()
            async let fetchedAirings = api.airings(from: moment.addingTimeInterval(-30 * 60), to: moment.addingTimeInterval(4 * 3600))
            async let fetchedRecordings = api.recordings()
            let info = try await fetchedInfo
            guard sameSession(started, base) else { return }
            if let current = server, current.id != "pending", current.id != "demo", !current.id.isEmpty, current.id != info.id {
                error = "A different server answered at this address."
                socket?.disconnect()
                socket = nil
                self.api = nil
                Task { await self.relocate() }
                return
            }
            self.info = info
            if let current = server {
                let pending = current.id == "pending" || current.id.isEmpty
                let same = current.id == info.id
                let needKey = current.key?.isEmpty != false && info.discoveryKey?.isEmpty == false
                if pending || same, pending || needKey {
                    let fixed = FoundServer(
                        id: info.id,
                        name: info.name.isEmpty ? current.name : info.name,
                        url: current.url,
                        key: info.discoveryKey ?? current.key
                    )
                    server = fixed
                    save(fixed, "server")
                    remembered = RememberedServers.upsert(remembered, fixed)
                    save(remembered, "servers")
                }
            }
            if !announced, let current = server {
                announced = true
                NSLog("broadwave ready %@ %@", current.id, current.url.absoluteString)
            }
            if lineup || channels.isEmpty {
                let nextChannels = try await fetchedChannels
                guard sameSession(started, base) else { return }
                channels = nextChannels.sorted(by: Channel.guideOrder)
            }
            let window = try await fetchedAirings
            let nextRecordings = try await fetchedRecordings
            guard sameSession(started, base) else { return }
            index = GuideIndex(window)
            if listedAt == recordingsEpoch {
                recordings = nextRecordings
            }
            now = Date()
            freshAt = now
            error = nil
            if let id = server?.id {
                CatalogCache.save(CatalogSnapshot(channels: channels, airings: window, recordings: recordings), serverID: id)
            }
            if let rest = try? await api.airings(from: moment.addingTimeInterval(4 * 3600), to: moment.addingTimeInterval(14 * 24 * 3600)) {
                guard sameSession(started, base) else { return }
                var seen = Set(window.map(\.id))
                var merged = window
                for airing in rest where !seen.contains(airing.id) {
                    seen.insert(airing.id)
                    merged.append(airing)
                }
                index = GuideIndex(merged)
                if let id = server?.id {
                    CatalogCache.save(CatalogSnapshot(channels: channels, airings: merged, recordings: recordings), serverID: id)
                }
            }
        } catch {
            guard sameSession(started, base) else { return }
            self.error = error.localizedDescription
        }
    }

    /// A drop that reconnects within a few seconds is not worth a word.
    private func noteConnection(_ up: Bool) {
        offlineWait?.cancel()
        if up {
            offline = false
            return
        }
        let started = generation
        offlineWait = Task { [weak self] in
            try? await Task.sleep(for: .seconds(3))
            guard !Task.isCancelled, self?.generation == started else { return }
            self?.offline = true
        }
    }

    public func refreshRecordings() async {
        guard let api else { return }
        let started = generation
        let base = api.base
        let listedAt = recordingsEpoch
        guard let list = try? await api.recordings(), sameSession(started, base), listedAt == recordingsEpoch else { return }
        recordings = list
    }

    public func refreshVirtuals() async {
        guard let api else { return }
        let started = generation
        let base = api.base
        guard let list = try? await api.virtuals(), sameSession(started, base) else { return }
        virtuals = list
    }

    /// Makes a library channel that plays one recording, numbered like the web does it.
    public func makeChannel(from recording: Recording) async throws -> VirtualChannel {
        guard let api else { throw APIError(code: "offline", message: "Not connected to a server.", status: 0) }
        let started = generation
        let base = api.base
        // The server does not keep numbers unique, so a stale list could hand out 900 twice.
        let existing = try await api.virtuals()
        guard sameSession(started, base) else { throw APIError(code: "offline", message: "Not connected to a server.", status: 0) }
        virtuals = existing
        let made = try await api.createVirtual(
            number: VirtualChannel.nextNumber(after: virtuals), name: "\(recording.title) channel", recordings: [recording.id]
        )
        guard sameSession(started, base) else { return made }
        await refreshVirtuals()
        return made
    }

    /// Channels with a preview newer than ten minutes. A miss keeps the last list.
    func refreshFrames() async {
        guard let api else { return }
        let started = generation
        let base = api.base
        guard let list = try? await api.frames(), sameSession(started, base) else { return }
        frameIDs = Set(list.channels)
    }

    public func activeRecording(on channel: Channel) -> Recording? {
        recordings.first { $0.isRecording && $0.channelId == channel.id }
    }

    public func toggleRecord(_ channel: Channel) async {
        guard let api else { return }
        let started = generation
        let base = api.base
        do {
            if let active = activeRecording(on: channel) {
                try await api.stopRecording(active.id)
            } else {
                _ = try await api.record(channelID: channel.id, title: index.on(channel.id, at: Date())?.title ?? channel.displayName)
            }
            guard sameSession(started, base) else { return }
            recordingsEpoch += 1
            await refreshRecordings()
        } catch {
            guard sameSession(started, base) else { return }
            self.error = error.localizedDescription
        }
    }

    /// Deletes the file and its markers on the server, then reloads the list.
    public func deleteRecording(_ rec: Recording) async throws {
        guard let api else { return }
        let started = generation
        let base = api.base
        try await api.deleteRecording(rec.id)
        guard sameSession(started, base) else { return }
        recordingsEpoch += 1
        recordings.removeAll { $0.id == rec.id }
        await refreshRecordings()
    }

    public func setWatched(_ rec: Recording, _ watched: Bool) async throws {
        guard let api else { return }
        let started = generation
        let base = api.base
        try await api.setWatched(recordingID: rec.id, watched)
        guard sameSession(started, base) else { return }
        recordingsEpoch += 1
        if let i = recordings.firstIndex(where: { $0.id == rec.id }) {
            recordings[i].watched = watched ? 1 : 2
        }
    }

    public enum BulkAction: Sendable {
        case watched, unwatched, delete
    }

    /// Marks or deletes each recording in turn, then reads the list once.
    /// Returns the first error and how many failed; the rest still go through.
    @discardableResult
    public func apply(_ action: BulkAction, to recs: [Recording]) async -> (failed: Int, error: Error?) {
        guard let api else { return (recs.count, APIError(code: "offline", message: "Not connected to a server.", status: 0)) }
        let started = generation
        let base = api.base
        recordingsEpoch += 1
        var failed = 0
        var first: Error?
        for rec in recs where !rec.isRecording {
            guard sameSession(started, base) else { break }
            do {
                switch action {
                case .watched, .unwatched:
                    try await api.setWatched(recordingID: rec.id, action == .watched)
                    guard sameSession(started, base) else { break }
                    if let i = recordings.firstIndex(where: { $0.id == rec.id }) {
                        recordings[i].watched = action == .watched ? 1 : 2
                    }
                case .delete:
                    try await api.deleteRecording(rec.id)
                    guard sameSession(started, base) else { break }
                    recordings.removeAll { $0.id == rec.id }
                }
            } catch {
                failed += 1
                first = first ?? error
            }
        }
        await refreshRecordings()
        return (failed, first)
    }

    public func stopRecording(_ rec: Recording) async throws {
        guard let api else { return }
        let started = generation
        let base = api.base
        try await api.stopRecording(rec.id)
        guard sameSession(started, base) else { return }
        recordingsEpoch += 1
        await refreshRecordings()
    }

    public func toggleFavorite(_ channel: Channel) async {
        guard let api else { return }
        let started = generation
        let base = api.base
        do {
            let updated = try await api.setFavorite(channel, !channel.favorite)
            guard sameSession(started, base) else { return }
            if let i = channels.firstIndex(where: { $0.id == updated.id }) {
                channels[i] = updated
            }
        } catch {
            guard sameSession(started, base) else { return }
            self.error = error.localizedDescription
        }
    }

    /// Saves a channel edit, then reloads the guide's lineup so hiding or
    /// renaming shows everywhere at once.
    public func editChannel(_ id: Int64, _ patch: ChannelPatch) async throws -> Channel {
        guard let api else { throw APIError(code: "offline", message: "Not connected to a server.", status: 0) }
        let started = generation
        let base = api.base
        let updated = try await api.patchChannel(id, patch)
        if let list = try? await api.channels(), sameSession(started, base) {
            channels = list.sorted(by: Channel.guideOrder)
        }
        return updated
    }

    public func refreshPasses() async {
        guard let api else { return }
        let started = generation
        let base = api.base
        let listedAt = passesEpoch
        guard let list = try? await api.passes(), sameSession(started, base), listedAt == passesEpoch else { return }
        passes = list
    }

    public func recordSeries(_ airing: Airing) async throws {
        guard let api else { return }
        let started = generation
        let base = api.base
        let list = try await api.addPass(title: airing.title, channelID: airing.channelId)
        guard sameSession(started, base) else { return }
        passesEpoch += 1
        passes = list
    }

    public func recordOnce(_ airing: Airing) async throws {
        guard let api else { return }
        let started = generation
        let base = api.base
        let list = try await api.addPass(title: airing.title, channelID: airing.channelId, airingStart: airing.start)
        guard sameSession(started, base) else { return }
        passesEpoch += 1
        passes = list
    }

    /// Records the next airing of a damaged recording's episode. Nil when the guide has none.
    public func recordAgain(_ rec: Recording) async throws -> Airing? {
        guard let api else { return nil }
        let started = generation
        let base = api.base
        guard let airing = try await api.recordAgain(recordingID: rec.id) else { return nil }
        guard sameSession(started, base) else { return airing }
        let list = try await api.addPass(title: rec.title, channelID: airing.channelId, airingStart: airing.start)
        guard sameSession(started, base) else { return airing }
        passesEpoch += 1
        passes = list
        return airing
    }

    public func removePass(_ id: Int64) async throws {
        guard let api else { return }
        let started = generation
        let base = api.base
        let list = try await api.deletePass(id)
        guard sameSession(started, base) else { return }
        passesEpoch += 1
        passes = list
    }

    /// What most likely deserves the big spot: sports first, then favorites.
    /// The cached program picture for an airing, or nil when the guide has none.
    public func artURL(_ airing: Airing, width: Int) -> URL? {
        guard let image = airing.imageUrl, !image.isEmpty else { return nil }
        return api?.artURL(kind: "airing", id: airing.id, width: width)
    }

    public func featured() -> (Channel, Airing?)? {
        struct Pick {
            var channel: Channel
            var airing: Airing?
            var score: Double
        }
        let scored = channels.map { c -> Pick in
            let a = index.on(c.id, at: now)
            var s = 0.0
            if a?.kind == .sports {
                s += 4
            }
            if c.favorite {
                s += 2
            }
            if a != nil {
                s += 1
            }
            if c.hd {
                s += 0.5
            }
            return Pick(channel: c, airing: a, score: s)
        }
        guard let best = scored.max(by: { $0.score < $1.score }) else { return nil }
        return (best.channel, best.airing)
    }

    public func sports(hours: Double = 48) -> [(Channel, Airing)] {
        let until = now.addingTimeInterval(hours * 3600)
        var out: [(Channel, Airing)] = []
        for c in channels {
            for a in index.airings(c.id) where a.end > now && a.start < until && a.kind == .sports {
                out.append((c, a))
            }
        }
        return out.sorted { $0.1.start < $1.1.start }
    }

    private func save(_ value: some Encodable, _ key: String) {
        #if DEBUG
            if !persistServers {
                return
            }
        #endif
        if let data = try? JSONEncoder().encode(value) {
            UserDefaults.standard.set(data, forKey: key)
        }
    }

    private static func load<T: Decodable>(_ key: String) -> T? {
        guard let data = UserDefaults.standard.data(forKey: key) else { return nil }
        return try? JSONDecoder().decode(T.self, from: data)
    }

    private struct HomeNote: Decodable {
        var kind: String
        var message: String
    }
}
