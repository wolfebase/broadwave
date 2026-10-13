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
    /// The server's words when it wants a paired device: this one has no token
    /// for it, or the owner removed it. Nil while signed in or on the home network.
    public private(set) var signedOut: String?
    /// When the lists on screen last came from the server.
    public private(set) var freshAt: Date?
    /// What the keychain holds for Siri and Control Center.
    @ObservationIgnored private var sharedLineup: [SharedLineup.Entry] = []
    private var offlineWait: Task<Void, Never>?
    /// A device that showed up after the house was already known. Nil when nothing is waiting.
    public private(set) var homeNotice: String?
    private var homeQueue: [String] = []
    /// The game alert on screen, if one is still worth showing.
    public private(set) var gameAlert: GameAlert?
    /// When that alert arrived. The banner drops it ten minutes later.
    public private(set) var gameAlertAt: Date?
    private var gameAlerts = GameAlerts()
    /// A channel another screen sent here, until the shell plays it.
    public private(set) var screenWatch: ScreenWatch?
    /// A remote button pressed for this screen, until the shell acts on it.
    public private(set) var screenRemote: ScreenRemote?
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

    /// Line a new channel up with the other screens before it plays. Off by
    /// default: a channel starts at once and Sync screens lines them up.
    public var lineUpAutomatically: Bool {
        didSet { UserDefaults.standard.set(lineUpAutomatically, forKey: "lineUp") }
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
        // bool(forKey:) also reads a launch argument such as -sync NO.
        syncEnabled = UserDefaults.standard.object(forKey: "sync") == nil || UserDefaults.standard.bool(forKey: "sync")
        lineUpAutomatically = UserDefaults.standard.bool(forKey: "lineUp")
        remembered = Self.load("servers") ?? []
        var resumeDemo: FoundServer?
        #if DEBUG
            let holdConnect = UserDefaults.standard.bool(forKey: "BroadwaveDiscover") || UserDefaults.standard.bool(forKey: "BroadwaveExplain")
            let forcedURL = UserDefaults.standard.string(forKey: "BroadwaveServerURL")
            // UI tests against a server away from home: -BroadwaveToken bw_...
            let forcedToken = UserDefaults.standard.string(forKey: "BroadwaveToken")
        #else
            let holdConnect = false
            let forcedURL: String? = nil
            let forcedToken: String? = nil
        #endif
        if !holdConnect, let raw = forcedURL, let url = URL(string: raw) {
            connect(FoundServer(id: "pending", name: String(localized: "Server", bundle: .module), url: url), token: forcedToken)
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
            SharedServer.saveToken(nil, for: nil)
            SharedLineup.save(nil)
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
            error = String(localized: "The demo did not start.", bundle: .module)
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
            error = String(localized: "The demo did not start.", bundle: .module)
            return
        }
        error = nil
        connect(FoundServer(id: "demo", name: String(localized: "Demo", bundle: .module), url: url))
    }

    /// `token` is this device's token from pairing at exactly this address; an
    /// empty one means none. Without it, only a token issued at this same address
    /// by this same server is used. A server at a new address gets no token.
    public func connect(_ server: FoundServer, token: String? = nil) {
        generation += 1
        let token: String? = if let token {
            token.isEmpty ? nil : token
        } else {
            DeviceTokens.token(for: server.url, server: server.id)
        }
        if let token {
            DeviceTokens.save(token, for: server.url, server: server.id)
        }
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
        clearGameAlerts()
        screenWatch = nil
        screenRemote = nil
        // A new server is not the previous one's outage. The banner waits out a short drop.
        offline = false
        signedOut = nil
        // Down until the first message, so a server that is off at launch gets the banner.
        noteConnection(false)
        announced = false
        self.server = server
        let api = APIClient(base: server.url, session: session, token: token)
        self.api = api
        let socket = EventSocket(base: server.url, token: token)
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
        socket.on("game.alert") { [weak self] data in
            guard let alert = try? JSONDecoder().decode(GameAlert.self, from: data) else { return }
            self?.noteGameAlert(alert)
        }
        socket.on("screen.watch") { [weak self] data in
            guard let sent = try? JSONDecoder().decode(ScreenWatch.self, from: data) else { return }
            self?.noteScreenWatch(sent)
        }
        // A button this app version doesn't know is ignored.
        socket.on("screen.remote") { [weak self] data in
            guard let pressed = try? JSONDecoder().decode(ScreenRemote.self, from: data) else { return }
            self?.noteScreenRemote(pressed)
        }
        socket.connect()
        self.socket = socket
        save(server, "server")
        // The demo lives inside the app, where the Top Shelf cannot reach it.
        SharedServer.save(server.id == "demo" ? nil : server.url)
        SharedServer.saveToken(server.id == "demo" ? nil : token, for: server.url)
        // Channel ids belong to one server; the new lineup is written after its first refresh.
        SharedLineup.save(nil)
        sharedLineup = []
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
        // Only a server on the home network moves. One reached by name away from home stays put.
        guard let saved = server, saved.url.scheme == "http", let key = saved.key, !key.isEmpty else { return }
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

    /// Queues a game alert. A repeat of the same id is ignored.
    public func noteGameAlert(_ alert: GameAlert, now: Date = Date()) {
        gameAlerts.receive(alert, at: now)
        publishGameAlert(now: now)
    }

    /// Drops the one on screen. The next one comes up when it is still fresh.
    public func dismissGameAlert(now: Date = Date()) {
        gameAlerts.dismiss()
        publishGameAlert(now: now)
    }

    /// Drops anything older than ten minutes and publishes what is left.
    public func refreshGameAlerts(now: Date = Date()) {
        publishGameAlert(now: now)
    }

    private func publishGameAlert(now: Date) {
        let item = gameAlerts.current(at: now)
        let next = item?.alert
        if gameAlert != next {
            gameAlert = next
        }
        if gameAlertAt != item?.at {
            gameAlertAt = item?.at
        }
    }

    func noteScreenWatch(_ sent: ScreenWatch) {
        guard sent.channelId > 0 else { return }
        screenWatch = sent
    }

    /// Hands the pressed button to the shell once.
    public func takeScreenRemote() -> ScreenRemote? {
        defer { screenRemote = nil }
        return screenRemote
    }

    func noteScreenRemote(_ pressed: ScreenRemote) {
        screenRemote = pressed
    }

    /// Hands the sent channel to the shell once.
    public func takeScreenWatch() -> ScreenWatch? {
        defer { screenWatch = nil }
        return screenWatch
    }

    private func clearGameAlerts() {
        gameAlerts = GameAlerts()
        gameAlert = nil
        gameAlertAt = nil
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
        signedOut = nil
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
        screenWatch = nil
        screenRemote = nil
        clearGameAlerts()
        #if DEBUG
            if persistServers {
                UserDefaults.standard.removeObject(forKey: "server")
            }
        #else
            UserDefaults.standard.removeObject(forKey: "server")
        #endif
        SharedServer.save(nil)
        SharedServer.saveToken(nil, for: nil)
        SharedLineup.save(nil)
        sharedLineup = []
    }

    /// Writes the keychain only when the lineup changed.
    private func shareLineup() {
        let next = SharedLineup.entries(channels)
        guard next != sharedLineup else { return }
        sharedLineup = next
        SharedLineup.save(channels)
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
            server = FoundServer(id: "preview", name: String(localized: "Preview", bundle: .module), url: url)
            api = APIClient(base: url, session: session)
        }

        /// One library row for a simulator shot. The demo server has no recordings.
        public func previewRecording(_ recording: Recording) {
            recordings = [recording]
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
                error = String(localized: "A different server answered at this address.", bundle: .module)
                socket?.disconnect()
                socket = nil
                self.api = nil
                Task { await self.relocate() }
                return
            }
            self.info = info
            if let current = server, let fixed = RememberedServers.refreshed(current, id: info.id, name: info.name, key: info.discoveryKey) {
                if current.id == "pending" {
                    DeviceTokens.bind(current.url, server: info.id)
                }
                server = fixed
                save(fixed, "server")
                remembered = RememberedServers.upsert(remembered, fixed)
                save(remembered, "servers")
            }
            if !announced, let current = server {
                announced = true
                NSLog("broadwave ready %@ %@", current.id, current.url.absoluteString)
            }
            if lineup || channels.isEmpty {
                // A shown 3.0 channel sorts at its hidden 1.0 twin's number, which only the full list has.
                async let everyChannel = api.allChannels()
                let nextChannels = try await fetchedChannels
                let rows = await (try? everyChannel) ?? nextChannels
                guard sameSession(started, base) else { return }
                channels = nextChannels.sorted { Channel.comesBefore($0, $1, lineup: rows) }
            }
            let window = try await fetchedAirings
            let nextRecordings = try await fetchedRecordings
            guard sameSession(started, base) else { return }
            let horizon = moment.addingTimeInterval(4 * 3600)
            let near = GuideMerge.listings(previous: index.allAirings(), window: window, tail: nil, horizon: horizon)
            index = GuideIndex(near)
            if listedAt == recordingsEpoch {
                recordings = nextRecordings
            }
            now = Date()
            freshAt = now
            error = nil
            if let id = server?.id {
                CatalogCache.save(CatalogSnapshot(channels: channels, airings: near, recordings: recordings), serverID: id)
                if id != "demo" {
                    shareLineup()
                }
            }
            if let rest = try? await api.airings(from: horizon, to: moment.addingTimeInterval(14 * 24 * 3600)) {
                guard sameSession(started, base) else { return }
                let merged = GuideMerge.listings(previous: [], window: window, tail: rest, horizon: horizon)
                index = GuideIndex(merged)
                if let id = server?.id {
                    CatalogCache.save(CatalogSnapshot(channels: channels, airings: merged, recordings: recordings), serverID: id)
                }
            }
        } catch {
            guard sameSession(started, base) else { return }
            if let failure = error as? APIError, failure.signedOut {
                noteSignedOut(failure.message)
                return
            }
            self.error = error.localizedDescription
        }
    }

    /// The server turned this device away. The socket would only be turned away
    /// again, so it closes until a new code is entered.
    func noteSignedOut(_ message: String) {
        let words = message.trimmingCharacters(in: .whitespacesAndNewlines)
        signedOut = words.isEmpty ? String(localized: "Sign in from a paired device.", bundle: .module) : words
        error = nil
        offlineWait?.cancel()
        offline = false
        socket?.disconnect()
        socket = nil
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
        guard let api else { throw APIError(code: "offline", message: String(localized: "Not connected to a server.", bundle: .module), status: 0) }
        let started = generation
        let base = api.base
        // The server does not keep numbers unique, so a stale list could hand out 900 twice.
        let existing = try await api.virtuals()
        guard sameSession(started, base) else { throw APIError(code: "offline", message: String(localized: "Not connected to a server.", bundle: .module), status: 0) }
        virtuals = existing
        let made = try await api.createVirtual(
            number: VirtualChannel.nextNumber(after: virtuals), name: String(localized: "\(recording.title) channel", bundle: .module), recordings: [recording.id]
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

    /// A kept recording is left out of watched clean-up and low-space clean-up.
    /// False clears the flag; the server omits it on the next list.
    public func setKeep(_ rec: Recording, _ keep: Bool) async throws {
        guard let api else { return }
        _ = try await api.setKeep(recordingID: rec.id, keep)
        if let i = recordings.firstIndex(where: { $0.id == rec.id }) {
            recordings[i].keep = keep ? true : nil
        }
    }

    public enum BulkAction: Sendable {
        case watched, unwatched, delete
    }

    /// Marks or deletes each recording in turn, then reads the list once.
    /// Returns the first error and how many failed; the rest still go through.
    @discardableResult
    public func apply(_ action: BulkAction, to recs: [Recording]) async -> (failed: Int, error: Error?) {
        guard let api else { return (recs.count, APIError(code: "offline", message: String(localized: "Not connected to a server.", bundle: .module), status: 0)) }
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
        guard let api else { throw APIError(code: "offline", message: String(localized: "Not connected to a server.", bundle: .module), status: 0) }
        let started = generation
        let base = api.base
        let updated = try await api.patchChannel(id, patch)
        async let guide = api.channels()
        async let everyChannel = api.allChannels()
        if let list = try? await guide {
            let rows = await (try? everyChannel) ?? list
            if sameSession(started, base) {
                channels = list.sorted { Channel.comesBefore($0, $1, lineup: rows) }
            }
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
        let title = RecordAgain.passTitle(recording: rec, airing: airing)
        guard !title.isEmpty else { throw RecordAgain.MissingTitle() }
        let list = try await api.addPass(title: title, channelID: airing.channelId, airingStart: airing.start)
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
