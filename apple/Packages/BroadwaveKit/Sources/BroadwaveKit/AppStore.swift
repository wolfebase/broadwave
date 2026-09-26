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
    public private(set) var loading = false
    public var error: String?
    public var now = Date()
    /// A device that showed up after the house was already known. Nil when nothing is waiting.
    public private(set) var homeNotice: String?
    private var homeQueue: [String] = []
    /// Settings asks the shell to show setup again. A used catalog never sets needsSetup.
    public var presentSetup = false

    public var prefs: Prefs {
        didSet { save(prefs, "prefs") }
    }

    public var syncEnabled: Bool {
        didSet { UserDefaults.standard.set(syncEnabled, forKey: "sync") }
    }

    private var clock: Timer?
    private var announced = false
    private var relocateAfter = Date.distantPast

    public init() {
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
            }
            if saved.id == "demo" {
                server = saved
                api = APIClient(base: saved.url)
                resumeDemo = saved
            } else {
                connect(saved)
            }
        }
        if remembered.isEmpty, let saved: FoundServer = Self.load("server") {
            remembered = RememberedServers.upsert(remembered, saved)
        }
        clock = Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.now = Date() }
        }
        if let resumeDemo {
            Task { @MainActor in
                guard await DemoServer.shared.prepare() != nil else {
                    self.error = "The demo did not start."
                    self.forget()
                    return
                }
                self.connect(resumeDemo)
            }
        }
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
        socket?.disconnect()
        announced = false
        self.server = server
        let api = APIClient(base: server.url)
        self.api = api
        let socket = EventSocket(base: server.url)
        socket.onFailure = { [weak self] in
            Task { await self?.relocate() }
        }
        socket.on("activity") { [weak self] data in
            if let note = try? JSONDecoder().decode(HomeNote.self, from: data), note.kind == "home" {
                self?.noteHome(note.message)
            }
            Task { await self?.refresh(lineup: false) }
        }
        socket.on("live.changed") { [weak self] _ in Task { await self?.refreshRecordings() } }
        socket.connect()
        self.socket = socket
        save(server, "server")
        remembered = RememberedServers.upsert(remembered, server)
        save(remembered, "servers")
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
        let found = await Task.detached { LANProbe.collect(timeout: 1.2) }.value
        guard server?.id == saved.id, let next = ServerFollow.updated(saved, found: found) else { return }
        guard let proven = await proven(next, key: key) else { return }
        NSLog("broadwave followed %@", proven.url.absoluteString)
        connect(proven)
    }

    /// GET /server at the candidate must return the same id and the stored key
    /// before any event socket or saved settings go there.
    private func proven(_ candidate: FoundServer, key: String) async -> FoundServer? {
        guard FinderPacket.isLocal(candidate.url) else { return nil }
        guard let info = try? await APIClient(base: candidate.url).server() else { return nil }
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
        guard !message.isEmpty else { return }
        if homeNotice == nil {
            homeNotice = message
        } else {
            homeQueue.append(message)
        }
    }

    public func forget() {
        socket?.disconnect()
        socket = nil
        api = nil
        server = nil
        info = nil
        channels = []
        recordings = []
        homeNotice = nil
        homeQueue = []
        UserDefaults.standard.removeObject(forKey: "server")
    }

    #if DEBUG
        /// Offline tiles for the tvOS remote test. It does not open a socket or a tuner.
        public func previewLineup(_ channels: [Channel]) {
            self.channels = channels
            let url = URL(string: "http://127.0.0.1:9")!
            server = FoundServer(id: "preview", name: "Preview", url: url)
            api = APIClient(base: url)
        }
    #endif

    public func refresh(lineup: Bool = true) async {
        #if DEBUG
            if UserDefaults.standard.bool(forKey: "BroadwaveMultiviewTest") {
                return
            }
        #endif
        guard let api else { return }
        let base = api.base
        loading = true
        defer { loading = false }
        do {
            let moment = Date()
            async let fetchedInfo = api.server()
            async let fetchedChannels = api.channels()
            async let fetchedAirings = api.airings(from: moment.addingTimeInterval(-30 * 60), to: moment.addingTimeInterval(4 * 3600))
            async let fetchedRecordings = api.recordings()
            let info = try await fetchedInfo
            guard self.api?.base == base else { return }
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
                channels = try await fetchedChannels.sorted(by: Channel.guideOrder)
            }
            guard self.api?.base == base else { return }
            let window = try await fetchedAirings
            index = GuideIndex(window)
            recordings = try await fetchedRecordings
            guard self.api?.base == base else { return }
            now = Date()
            error = nil
            if let id = server?.id {
                CatalogCache.save(CatalogSnapshot(channels: channels, airings: window, recordings: recordings), serverID: id)
            }
            if let rest = try? await api.airings(from: moment.addingTimeInterval(4 * 3600), to: moment.addingTimeInterval(14 * 24 * 3600)) {
                guard self.api?.base == base else { return }
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
            guard self.api?.base == base else { return }
            self.error = error.localizedDescription
        }
    }

    public func refreshRecordings() async {
        guard let api, let list = try? await api.recordings() else { return }
        recordings = list
    }

    public func activeRecording(on channel: Channel) -> Recording? {
        recordings.first { $0.isRecording && $0.channelId == channel.id }
    }

    public func toggleRecord(_ channel: Channel) async {
        guard let api else { return }
        do {
            if let active = activeRecording(on: channel) {
                try await api.stopRecording(active.id)
            } else {
                _ = try await api.record(channelID: channel.id, title: index.on(channel.id, at: Date())?.title ?? channel.displayName)
            }
            await refreshRecordings()
        } catch {
            self.error = error.localizedDescription
        }
    }

    public func toggleFavorite(_ channel: Channel) async {
        guard let api, let updated = try? await api.setFavorite(channel, !channel.favorite) else { return }
        if let i = channels.firstIndex(where: { $0.id == updated.id }) {
            channels[i] = updated
        }
    }

    public func recordSeries(_ airing: Airing) async {
        guard let api else { return }
        try? await api.addPass(title: airing.title, channelID: airing.channelId)
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
