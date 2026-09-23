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
            connect(saved)
        }
        clock = Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.now = Date() }
        }
    }

    public var connected: Bool { api != nil }

    public func connect(_ server: FoundServer) {
        socket?.disconnect()
        self.server = server
        let api = APIClient(base: server.url)
        self.api = api
        let socket = EventSocket(base: server.url)
        socket.on("activity") { [weak self] _ in Task { await self?.refresh(lineup: false) } }
        socket.on("live.changed") { [weak self] _ in Task { await self?.refreshRecordings() } }
        socket.connect()
        self.socket = socket
        save(server, "server")
        Task { await refresh() }
    }

    public func forget() {
        socket?.disconnect()
        socket = nil
        api = nil
        server = nil
        info = nil
        channels = []
        recordings = []
        UserDefaults.standard.removeObject(forKey: "server")
    }

    public func refresh(lineup: Bool = true) async {
        guard let api else { return }
        loading = true
        defer { loading = false }
        do {
            async let info = api.server()
            async let channels = api.channels()
            async let airings = api.airings(hours: 36)
            async let recordings = api.recordings()
            self.info = try await info
            if lineup || self.channels.isEmpty {
                self.channels = try await channels.sorted(by: Channel.guideOrder)
            }
            self.index = GuideIndex(try await airings)
            self.recordings = try await recordings
            self.now = Date()
            error = nil
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
        if let i = channels.firstIndex(where: { $0.id == updated.id }) { channels[i] = updated }
    }

    public func recordSeries(_ airing: Airing) async {
        guard let api else { return }
        try? await api.addPass(title: airing.title, channelID: airing.channelId)
    }

    /// What most likely deserves the big spot: sports first, then favorites.
    public func featured() -> (Channel, Airing?)? {
        let scored = channels.map { c -> (Channel, Airing?, Double) in
            let a = index.on(c.id, at: now)
            var s = 0.0
            if a?.kind == .sports { s += 4 }
            if c.favorite { s += 2 }
            if a != nil { s += 1 }
            if c.hd { s += 0.5 }
            return (c, a, s)
        }
        guard let best = scored.max(by: { $0.2 < $1.2 }) else { return nil }
        return (best.0, best.1)
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

    private func save<T: Encodable>(_ value: T, _ key: String) {
        if let data = try? JSONEncoder().encode(value) { UserDefaults.standard.set(data, forKey: key) }
    }

    private static func load<T: Decodable>(_ key: String) -> T? {
        guard let data = UserDefaults.standard.data(forKey: key) else { return nil }
        return try? JSONDecoder().decode(T.self, from: data)
    }
}
