import Foundation

public struct APIError: LocalizedError, Sendable {
    public var code: String
    public var message: String
    public var status: Int

    public var errorDescription: String? {
        message
    }
}

/// Typed access to one server's /api/v1.
public struct APIClient: Sendable {
    public let base: URL
    private let session: URLSession

    public init(base: URL, session: URLSession = .shared) {
        self.base = base
        self.session = session
    }

    static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .custom { decoder in
            let raw = try decoder.singleValueContainer().decode(String.self)
            if let date = ISO8601DateFormatter.fractional.date(from: raw) ?? ISO8601DateFormatter.plain.date(from: raw) {
                return date
            }
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: "Bad date \(raw)"))
        }
        return d
    }()

    public func url(_ path: String) -> URL {
        URL(string: path, relativeTo: base)?.absoluteURL ?? base
    }

    func send<T: Decodable>(_ method: String, _ path: String, body: (any Encodable)? = nil, as _: T.Type = T.self) async throws -> T {
        var req = URLRequest(url: url("/api/v1" + path))
        req.httpMethod = method
        req.timeoutInterval = 30
        if let body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONEncoder().encode(body)
        }
        let (data, res) = try await session.data(for: req)
        let status = (res as? HTTPURLResponse)?.statusCode ?? 0
        guard (200 ..< 300).contains(status) else {
            if let err = try? JSONDecoder().decode(APIErrorBody.self, from: data) {
                throw APIError(code: err.code, message: err.message, status: status)
            }
            throw APIError(code: "http_\(status)", message: "The server answered \(status).", status: status)
        }
        return try Self.decoder.decode(T.self, from: data)
    }

    // MARK: Server

    public func server() async throws -> ServerInfo {
        try await send("GET", "/server")
    }

    public func clock() async throws -> Double {
        struct R: Decodable { var serverTime: Double }
        return try await send("GET", "/clock", as: R.self).serverTime
    }

    // MARK: Lineup and guide

    public func channels() async throws -> [Channel] {
        struct R: Decodable { var channels: [Channel] }
        return try await send("GET", "/channels?guide=1", as: R.self).channels
    }

    public func search(_ query: String) async throws -> SearchResult {
        let q = query.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? ""
        return try await send("GET", "/search?q=\(q)")
    }

    public func airings(hours: Int = 48) async throws -> [Airing] {
        try await airings(path: "/airings?hours=\(hours)")
    }

    /// Listings in a window. The apps paint this first, then ask for the rest.
    public func airings(from: Date, to: Date) async throws -> [Airing] {
        var parts = URLComponents()
        parts.queryItems = [
            URLQueryItem(name: "from", value: ISO8601DateFormatter.plain.string(from: from)),
            URLQueryItem(name: "to", value: ISO8601DateFormatter.plain.string(from: to)),
        ]
        return try await airings(path: "/airings?\(parts.percentEncodedQuery ?? "")")
    }

    private func airings(path: String) async throws -> [Airing] {
        struct R: Decodable { var airings: [Airing] }
        return try await send("GET", path, as: R.self).airings
    }

    public func setFavorite(_ channel: Channel, _ on: Bool) async throws -> Channel {
        struct B: Encodable { var favorite: Bool }
        return try await send("PATCH", "/channels/\(channel.id)", body: B(favorite: on))
    }

    public func setHidden(_ channel: Channel, _ on: Bool) async throws -> Channel {
        struct B: Encodable { var hidden: Bool }
        return try await send("PATCH", "/channels/\(channel.id)", body: B(hidden: on))
    }

    public struct DoctorNote: Decodable, Sendable, Hashable {
        public var id: String
        public var message: String
    }

    public func doctorNotes() async throws -> [DoctorNote] {
        struct R: Decodable { var doctor: [DoctorNote]? }
        return try await send("GET", "/diagnostics", as: R.self).doctor ?? []
    }

    public func settings() async throws -> [String: String] {
        try await send("GET", "/settings")
    }

    public func saveSettings(_ values: [String: String]) async throws {
        _ = try await send("PUT", "/settings", body: values, as: [String: String].self)
    }

    // MARK: Live

    public func watch(channelID: Int64, caps: Caps, prefs: Prefs) async throws -> WatchSession {
        struct B: Encodable { var channelId: Int64; var caps: Caps; var prefs: Prefs }
        return try await send("POST", "/watch", body: B(channelId: channelID, caps: caps, prefs: prefs))
    }

    public func planMultiview(_ channelIDs: [Int64]) async throws -> MultiviewPlan {
        struct B: Encodable { var channelIds: [Int64]; var picker: Bool }
        return try await send("POST", "/multiview/plan", body: B(channelIds: channelIDs, picker: true))
    }

    public func stopWatching(channelID: Int64, rendition: String) async {
        struct B: Encodable { var rendition: String }
        struct Ok: Decodable {}
        _ = try? await send("POST", "/watch/\(channelID)/stop", body: B(rendition: rendition), as: Ok.self)
    }

    // MARK: Recordings

    public func recordings() async throws -> [Recording] {
        struct R: Decodable { var recordings: [Recording] }
        return try await send("GET", "/recordings", as: R.self).recordings
    }

    public func record(channelID: Int64, title: String) async throws -> Recording {
        struct B: Encodable { var channelId: Int64; var minutes: Int; var title: String }
        return try await send("POST", "/recordings", body: B(channelId: channelID, minutes: 0, title: title))
    }

    public func stopRecording(_ id: Int64) async throws {
        struct Ok: Decodable {}
        _ = try await send("POST", "/recordings/\(id)/stop", body: [String: String](), as: Ok.self)
    }

    public func play(recordingID: Int64) async throws -> PlaybackStart {
        try await send("POST", "/recordings/\(recordingID)/play", body: [String: String]())
    }

    public func saveProgress(recordingID: Int64, position: Double) async {
        struct B: Encodable { var position: Double }
        struct R: Decodable {}
        _ = try? await send("PUT", "/recordings/\(recordingID)/progress", body: B(position: position), as: R.self)
    }

    public func scoreboard() async throws -> [ScoreGame] {
        struct R: Decodable { var games: [ScoreGame] }
        return try await send("GET", "/sports/scoreboard", as: R.self).games
    }

    public func teams() async throws -> [TeamFollow] {
        struct R: Decodable { var teams: [TeamFollow] }
        return try await send("GET", "/teams", as: R.self).teams
    }

    public func addPass(title: String, channelID: Int64) async throws {
        struct B: Encodable { var title: String; var channelId: Int64 }
        struct R: Decodable {}
        _ = try await send("POST", "/passes", body: B(title: title, channelId: channelID), as: R.self)
    }

    public func posterURL(recordingID: Int64) -> URL {
        url("/media/poster/\(recordingID)")
    }

    public func artURL(kind: String, id: Int64, width: Int) -> URL {
        url("/media/art/\(kind)/\(id)?w=\(width)")
    }

    public func supportURL() -> URL {
        url("/api/v1/support")
    }

    // MARK: Setup

    public struct FoundHit: Decodable, Sendable, Hashable, Identifiable {
        public var kind: String
        public var name: String
        public var addr: String
        public var deviceID: String?
        public var id: String {
            kind + "-" + addr
        }

        enum CodingKeys: String, CodingKey {
            case kind, name, addr
            case deviceID = "id"
        }
    }

    public struct FreeFeed: Decodable, Sendable, Hashable, Identifiable {
        public var kind: String
        public var name: String
        public var addr: String
        public var playlist: String
        public var guide: String
        public var id: String {
            playlist.isEmpty ? kind + "-" + addr : playlist
        }
    }

    public struct ScanProgress: Decodable, Sendable {
        public var scanning: Bool
        public var found: Int
    }

    public struct StorageInfo: Decodable, Sendable {
        public var freeBytes: Int64
        public var totalBytes: Int64
        public var watermarkGB: Int
    }

    public struct StarredChannel: Decodable, Sendable {
        public var id: Int64
        public var guideName: String?
        public var displayNumber: String?
        public var network: String
    }

    public func devices() async throws -> [Device] {
        struct R: Decodable { var devices: [Device] }
        return try await send("GET", "/devices", as: R.self).devices
    }

    public func discover(ip: String) async throws -> [Device] {
        struct B: Encodable { var ip: String }
        struct R: Decodable { var devices: [Device] }
        return try await send("POST", "/sources/discover", body: B(ip: ip), as: R.self).devices
    }

    public struct HomePlace: Decodable, Sendable, Hashable, Identifiable {
        public var id: String
        public var group: String
        public var kind: String
        public var name: String
        public var addr: String?
        public var action: String
        public var detail: String?
    }

    public struct HomeScan: Decodable, Sendable {
        public var places: [HomePlace]
        public var tunerAddress: String
        public var sharing: Bool
    }

    public func home(fresh: Bool = false) async throws -> HomeScan {
        try await send("GET", fresh ? "/home?fresh=1" : "/home", as: HomeScan.self)
    }

    public func lookHarder() async throws -> [FoundHit] {
        struct R: Decodable { var found: [FoundHit] }
        return try await send("POST", "/sources/look", body: [String: String](), as: R.self).found
    }

    public func freeSources() async throws -> (found: [FreeFeed], guide: String) {
        struct R: Decodable { var found: [FreeFeed]; var guide: String }
        let res = try await send("GET", "/sources/free", as: R.self)
        return (res.found, res.guide)
    }

    public func addFree(kind: String, addr: String, playlist: String, guide: String, name: String) async throws -> String {
        struct B: Encodable { var kind, addr, playlist, guide, name: String }
        struct R: Decodable { var message: String? }
        let res = try await send("POST", "/sources/free", body: B(kind: kind, addr: addr, playlist: playlist, guide: guide, name: name), as: R.self)
        return res.message ?? ""
    }

    public struct SourceAdd: Decodable, Sendable {
        public var pick: Bool?
        public var message: String?
    }

    public func addPlaylist(kind: String, url: String, username: String, password: String) async throws -> SourceAdd {
        struct B: Encodable {
            var kind, name, url, username, password: String
        }
        return try await send("POST", "/sources", body: B(kind: kind, name: "", url: url, username: username, password: password))
    }

    public func startScan(deviceID: String) async throws {
        struct R: Decodable { var scanning: Bool }
        let id = deviceID.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? deviceID
        _ = try await send("POST", "/devices/\(id)/scan", body: [String: String](), as: R.self)
    }

    public func scanStatus(deviceID: String) async throws -> ScanProgress {
        let id = deviceID.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? deviceID
        return try await send("GET", "/devices/\(id)/scan")
    }

    public func storage() async throws -> StorageInfo {
        try await send("GET", "/storage")
    }

    public func guideAirings() async throws -> Int {
        struct G: Decodable { var airings: Int? }
        struct R: Decodable { var guide: G? }
        return try await send("GET", "/diagnostics", as: R.self).guide?.airings ?? 0
    }

    public func refreshGuide() async throws -> Int {
        struct R: Decodable { var airings: Int }
        return try await send("POST", "/guide/refresh", body: [String: String](), as: R.self).airings
    }

    public func starNetworks() async throws -> [StarredChannel] {
        struct R: Decodable { var starred: [StarredChannel] }
        return try await send("POST", "/channels/star", body: [String: String](), as: R.self).starred
    }

    public func startSetupFinish() async throws -> SetupFinish {
        try await send("POST", "/setup/finish", body: [String: String](), as: SetupFinish.self)
    }

    public func setupFinish() async throws -> SetupFinish {
        try await send("GET", "/setup/finish", as: SetupFinish.self)
    }
}

extension ISO8601DateFormatter {
    nonisolated(unsafe) static let fractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    nonisolated(unsafe) static let plain: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()
}
