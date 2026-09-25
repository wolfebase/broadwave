import Foundation
import Observation

/// Everything the apps show about one server, kept fresh by the event socket.
@MainActor
@Observable
public final class AppStore {
    public private(set) var server: FoundServer?
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

    public init() {
        prefs = Self.load("prefs") ?? Prefs()
        syncEnabled = UserDefaults.standard.object(forKey: "sync") as? Bool ?? true
        if let saved: FoundServer = Self.load("server") {
            if let snap = CatalogCache.load(serverID: saved.id) {
                channels = snap.channels
                index = GuideIndex(snap.airings)
                recordings = snap.recordings
            }
            connect(saved)
        }
        clock = Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.now = Date() }
        }
    }

    public var connected: Bool {
        api != nil
    }

    public func connect(_ server: FoundServer) {
        socket?.disconnect()
        self.server = server
        let api = APIClient(base: server.url)
        self.api = api
        let socket = EventSocket(base: server.url)
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
        Task { await refresh() }
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
        loading = true
        defer { loading = false }
        do {
            let moment = Date()
            async let info = api.server()
            async let channels = api.channels()
            async let airings = api.airings(from: moment.addingTimeInterval(-30 * 60), to: moment.addingTimeInterval(4 * 3600))
            async let recordings = api.recordings()
            self.info = try await info
            if lineup || self.channels.isEmpty {
                self.channels = try await channels.sorted(by: Channel.guideOrder)
            }
            let window = try await airings
            index = GuideIndex(window)
            self.recordings = try await recordings
            now = Date()
            error = nil
            if let id = server?.id {
                CatalogCache.save(CatalogSnapshot(channels: self.channels, airings: window, recordings: self.recordings), serverID: id)
            }
            if let rest = try? await api.airings(from: moment.addingTimeInterval(4 * 3600), to: moment.addingTimeInterval(14 * 24 * 3600)) {
                var seen = Set(window.map(\.id))
                var merged = window
                for airing in rest where !seen.contains(airing.id) {
                    seen.insert(airing.id)
                    merged.append(airing)
                }
                index = GuideIndex(merged)
                if let id = server?.id {
                    CatalogCache.save(CatalogSnapshot(channels: self.channels, airings: merged, recordings: self.recordings), serverID: id)
                }
            }
        } catch {
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
        guard airing.imageUrl != nil else { return nil }
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
