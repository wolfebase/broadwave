import Foundation

public struct APIError: LocalizedError, Sendable {
    public var code: String
    public var message: String
    public var status: Int

    public var errorDescription: String? {
        message
    }
}

/// Whether /health answered, and whether the device itself still has a connection.
public struct ServerReach: Equatable, Sendable {
    public var up: Bool
    public var online: Bool

    public init(up: Bool, online: Bool) {
        self.up = up
        self.online = online
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

    func send<T: Decodable>(_ method: String, _ path: String, body: (any Encodable)? = nil, timeout: TimeInterval = 30, as _: T.Type = T.self) async throws -> T {
        var req = URLRequest(url: url("/api/v1" + path))
        req.httpMethod = method
        req.timeoutInterval = timeout
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
            throw Self.unreadable(status)
        }
        return try Self.decoder.decode(T.self, from: data)
    }

    // MARK: Server

    public func server() async throws -> ServerInfo {
        try await send("GET", "/server")
    }

    /// True when /health answers. Two seconds, so a dead server does not stall the player.
    public func reachable() async -> Bool {
        await reach().up
    }

    /// `up` is /health. `online` is false only when the device itself has no
    /// connection. A port that does not answer leaves `online` true.
    public func reach() async -> ServerReach {
        var req = URLRequest(url: url("/api/v1/health"))
        req.httpMethod = "GET"
        req.timeoutInterval = 2
        do {
            let (_, res) = try await session.data(for: req)
            let status = (res as? HTTPURLResponse)?.statusCode ?? 0
            return ServerReach(up: (200 ..< 300).contains(status), online: true)
        } catch let error as URLError {
            return ServerReach(up: false, online: PlaybackOutage.deviceOnline(error))
        } catch {
            return ServerReach(up: false, online: true)
        }
    }

    /// Whether the server still serves this playlist. A server that restarted
    /// answers 404 for every picture it had. Nil when the read itself failed.
    public func playlistFound(_ path: String) async -> Bool? {
        var req = URLRequest(url: url(path))
        req.httpMethod = "GET"
        req.timeoutInterval = 2
        guard let (_, res) = try? await session.data(for: req), let status = (res as? HTTPURLResponse)?.statusCode else {
            return nil
        }
        if status == 404 {
            return false
        }
        return (200 ..< 300).contains(status) ? true : nil
    }

    /// The playlist's text, or nil when it did not answer 200.
    public func playlistText(_ path: String) async -> String? {
        var req = URLRequest(url: url(path))
        req.timeoutInterval = 2
        guard let (data, res) = try? await session.data(for: req), (res as? HTTPURLResponse)?.statusCode == 200 else {
            return nil
        }
        return String(data: data, encoding: .utf8)
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

    /// Every channel, including hidden, off, and off-air ones.
    public func lineup() async throws -> [Channel] {
        struct R: Decodable { var channels: [Channel] }
        return try await send("GET", "/channels", as: R.self).channels
    }

    public func patchChannel(_ id: Int64, _ patch: ChannelPatch) async throws -> Channel {
        try await send("PATCH", "/channels/\(id)", body: patch)
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

    public func watch(channelID: Int64, caps: Caps, prefs: Prefs, confirmLive: Bool = false) async throws -> WatchSession {
        struct B: Encodable { var channelId: Int64; var caps: Caps; var prefs: Prefs; var confirmLive: Bool }
        return try await send("POST", "/watch", body: B(channelId: channelID, caps: caps, prefs: prefs, confirmLive: confirmLive))
    }

    public func tuners() async throws -> [Tuner] {
        struct R: Decodable { var tuners: [Tuner] }
        return try await send("GET", "/tuners", as: R.self).tuners
    }

    public func planMultiview(_ channelIDs: [Int64]) async throws -> MultiviewPlan {
        struct B: Encodable { var channelIds: [Int64]; var picker: Bool }
        return try await send("POST", "/multiview/plan", body: B(channelIds: channelIDs, picker: true))
    }

    /// `boot` is the process from the watch answer. A restarted server ignores a
    /// stop that names another process, so a stop always goes out.
    public func stopWatching(channelID: Int64, rendition: String, boot: String) async {
        struct B: Encodable { var rendition: String; var boot: String }
        struct Ok: Decodable {}
        _ = try? await send("POST", "/watch/\(channelID)/stop", body: B(rendition: rendition, boot: boot), as: Ok.self)
    }

    public func stopWatching(_ session: WatchSession) async {
        await stopWatching(channelID: session.channelId, rendition: session.rendition, boot: session.boot ?? "")
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

    public func deleteRecording(_ id: Int64) async throws {
        struct Ok: Decodable {}
        _ = try await send("DELETE", "/recordings/\(id)", as: Ok.self)
    }

    public func setWatched(recordingID: Int64, _ watched: Bool) async throws {
        struct B: Encodable { var watched: Bool }
        struct Ok: Decodable {}
        _ = try await send("PUT", "/recordings/\(recordingID)/watched", body: B(watched: watched), as: Ok.self)
    }

    /// Runs commercial detection and returns every marker the recording has after it.
    /// The server answers when detection ends, which takes minutes for a long recording.
    public func detectBreaks(recordingID: Int64) async throws -> [Marker] {
        struct R: Decodable { var markers: [Marker] }
        return try await send("POST", "/recordings/\(recordingID)/detect", body: [String: String](), timeout: 600, as: R.self).markers
    }

    public func addMarker(recordingID: Int64, start: Double, end: Double) async throws -> Marker {
        struct B: Encodable { var start: Double; var end: Double }
        return try await send("POST", "/recordings/\(recordingID)/markers", body: B(start: start, end: end))
    }

    public func deleteMarker(_ id: Int64) async throws {
        struct Ok: Decodable {}
        _ = try await send("DELETE", "/markers/\(id)", as: Ok.self)
    }

    // MARK: Library channels

    public func virtuals() async throws -> [VirtualChannel] {
        struct R: Decodable { var virtuals: [VirtualChannel] }
        return try await send("GET", "/virtuals", as: R.self).virtuals
    }

    public func createVirtual(number: String, name: String, recordings: [Int64]) async throws -> VirtualChannel {
        struct B: Encodable { var number: String; var name: String; var recordings: [Int64] }
        return try await send("POST", "/virtuals", body: B(number: number, name: name, recordings: recordings))
    }

    public func playVirtual(_ id: Int64, index: Int) async throws -> VirtualPlayback {
        struct B: Encodable { var index: Int }
        return try await send("POST", "/virtuals/\(id)/play", body: B(index: index))
    }

    public func saveProgress(recordingID: Int64, position: Double) async {
        struct B: Encodable { var position: Double }
        struct R: Decodable {}
        _ = try? await send("PUT", "/recordings/\(recordingID)/progress", body: B(position: position), as: R.self)
    }

    public func schedule() async throws -> SchedulePlan {
        try await send("GET", "/schedule")
    }

    /// Records the later airing instead of a skipped one and returns the schedule after it.
    public func fixSchedule(_ item: PlannedAiring, later: Suggestion) async throws -> SchedulePlan {
        struct B: Encodable {
            var passId: Int64
            var channelId: Int64
            var start: String
            var suggestionChannelId: Int64
            var suggestionStart: String
        }
        let format = ISO8601DateFormatter.plain
        let body = B(
            passId: item.passId, channelId: item.airing.channelId, start: format.string(from: item.airing.start),
            suggestionChannelId: later.channelId, suggestionStart: format.string(from: later.start)
        )
        return try await send("POST", "/schedule/fix", body: body)
    }

    public func events() async throws -> [Event] {
        struct R: Decodable { var events: [Event] }
        return try await send("GET", "/events", as: R.self).events
    }

    public func scoreboard() async throws -> [ScoreGame] {
        struct R: Decodable { var games: [ScoreGame] }
        return try await send("GET", "/sports/scoreboard", as: R.self).games
    }

    public func teams() async throws -> [TeamFollow] {
        struct R: Decodable { var teams: [TeamFollow] }
        return try await send("GET", "/teams", as: R.self).teams
    }

    /// With `airingStart`, records only the airing that starts then on the channel.
    @discardableResult
    public func addPass(title: String, channelID: Int64, airingStart: Date? = nil) async throws -> [Pass] {
        struct B: Encodable { var title: String; var channelId: Int64; var airingStart: String? }
        let start = airingStart.map { ISO8601DateFormatter.plain.string(from: $0) }
        return try await send("POST", "/passes", body: B(title: title, channelId: channelID, airingStart: start), as: PassList.self).passes
    }

    public func passes() async throws -> [Pass] {
        try await send("GET", "/passes", as: PassList.self).passes
    }

    public func updatePass(_ pass: Pass) async throws -> [Pass] {
        try await send("PATCH", "/passes/\(pass.id)", body: pass, as: PassList.self).passes
    }

    public func deletePass(_ id: Int64) async throws -> [Pass] {
        try await send("DELETE", "/passes/\(id)", as: PassList.self).passes
    }

    public func posterURL(recordingID: Int64) -> URL {
        url("/media/poster/\(recordingID)")
    }

    public func artURL(kind: String, id: Int64, width: Int) -> URL {
        url("/media/art/\(kind)/\(id)?w=\(width)")
    }

    /// A still from a mux that is already tuned. The route keeps a 480 and a 1280 JPEG.
    public func frameURL(channelID: Int64, width: Int) -> URL {
        let w = width >= 1280 ? 1280 : 480
        return url("/api/v1/channels/\(channelID)/frame?w=\(w)")
    }

    /// Nil when this channel has no fresh preview, so the app does not ask for one.
    public func frameURL(channelID: Int64, width: Int, listed: Set<Int64>) -> URL? {
        guard listed.contains(channelID) else { return nil }
        return frameURL(channelID: channelID, width: width)
    }

    public func frames() async throws -> FrameList {
        try await send("GET", "/frames")
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
        public var progress: Int?

        public var line: String {
            if found == 0, let progress, progress > 0 {
                return "\(progress)% done."
            }
            return found == 1 ? "1 channel found." : "\(found) channels found."
        }
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
        public var added: Int?
        public var groups: [String]?
        public var channels: [PickChannel]?

        /// What a long playlist offers to keep: its groups, or its channels when it has none.
        public var options: [String] {
            if let groups, !groups.isEmpty {
                return groups
            }
            return (channels ?? []).map(\.name)
        }

        /// The groups or keep value for the chosen option positions. A channel goes by
        /// its tvg-id when it has one, so two channels with one name stay apart.
        public func chosen(_ picks: Set<Int>) -> (groups: String, keep: String) {
            let order = picks.sorted()
            if let groups, !groups.isEmpty {
                return (order.filter { $0 < groups.count }.map { groups[$0] }.joined(separator: ", "), "")
            }
            let list = channels ?? []
            let keys = order.filter { $0 < list.count }.map { list[$0].id?.isEmpty == false ? list[$0].id! : list[$0].name }
            return ("", keys.joined(separator: ", "))
        }
    }

    public struct PickChannel: Decodable, Sendable, Hashable {
        public var name: String
        public var id: String?
        public var number: String?
    }

    /// `groups` filters by group ("News, -Shopping"). `keep` names the channels to keep when a playlist has no groups.
    public func addSource(kind: String, name: String = "", url: String, username: String = "", password: String = "", groups: String = "", keep: String = "") async throws -> SourceAdd {
        struct B: Encodable {
            var kind, name, url, username, password, groups, keep: String
        }
        return try await send("POST", "/sources", body: B(kind: kind, name: name, url: url, username: username, password: password, groups: groups, keep: keep))
    }

    /// Uploads a playlist file from this device.
    public func addPlaylistFile(name: String, fileName: String, data: Data, groups: String = "", keep: String = "") async throws -> SourceAdd {
        let boundary = "broadwave-\(UUID().uuidString)"
        var body = Data()
        func part(_ field: String, _ value: String) {
            body.append(Data("--\(boundary)\r\nContent-Disposition: form-data; name=\"\(field)\"\r\n\r\n\(value)\r\n".utf8))
        }
        part("name", name)
        part("groups", groups)
        part("keep", keep)
        let safeName = fileName.replacingOccurrences(of: "\"", with: "")
        body.append(Data("--\(boundary)\r\nContent-Disposition: form-data; name=\"file\"; filename=\"\(safeName)\"\r\nContent-Type: application/octet-stream\r\n\r\n".utf8))
        body.append(data)
        body.append(Data("\r\n--\(boundary)--\r\n".utf8))
        var req = URLRequest(url: url("/api/v1/sources"))
        req.httpMethod = "POST"
        req.timeoutInterval = 60
        req.setValue("multipart/form-data; boundary=\(boundary)", forHTTPHeaderField: "Content-Type")
        req.httpBody = body
        let (reply, res) = try await session.data(for: req)
        let status = (res as? HTTPURLResponse)?.statusCode ?? 0
        guard (200 ..< 300).contains(status) else {
            if let err = try? JSONDecoder().decode(APIErrorBody.self, from: reply) {
                throw APIError(code: err.code, message: err.message, status: status)
            }
            throw Self.unreadable(status)
        }
        return try Self.decoder.decode(SourceAdd.self, from: reply)
    }

    public func sources() async throws -> [Source] {
        struct R: Decodable { var sources: [Source] }
        return try await send("GET", "/sources", as: R.self).sources
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

    public struct GuideDepth: Decodable, Sendable {
        public var channels: Int
        public var channelsWithListings: Int
        public var airings: Int
    }

    public func guideDepth() async throws -> GuideDepth? {
        struct R: Decodable { var guide: GuideDepth? }
        return try await send("GET", "/diagnostics", as: R.self).guide
    }

    public func deviceHealth() async throws -> [DeviceHealth] {
        struct R: Decodable { var devices: [DeviceHealth] }
        return try await send("GET", "/devices/health", as: R.self).devices
    }

    public func signals() async throws -> (channels: [ChannelSignal], running: Bool) {
        struct R: Decodable { var channels: [ChannelSignal]; var running: Bool }
        let res = try await send("GET", "/signals", as: R.self)
        return (res.channels, res.running)
    }

    /// Starts an antenna pass. The server tunes each frequency on an idle tuner and stops if someone watches.
    public func checkSignals() async throws -> String {
        struct R: Decodable { var running: Bool; var message: String? }
        let res = try await send("POST", "/signals/check", body: [String: String](), as: R.self)
        return res.message ?? ""
    }

    public func backups() async throws -> [CatalogBackup] {
        struct R: Decodable { var backups: [CatalogBackup] }
        return try await send("GET", "/backups", as: R.self).backups
    }

    public func restoreBackup(name: String) async throws {
        struct Ok: Decodable {}
        _ = try await send("POST", "/backups/\(Self.pathSegment(name))/restore", body: [String: String](), as: Ok.self)
    }

    public func backupURL(name: String) -> URL {
        url("/api/v1/backups/\(Self.pathSegment(name))")
    }

    public func catalogBackupURL() -> URL {
        url("/api/v1/backup")
    }

    /// Puts a catalog file back. The body is the database, not JSON.
    public func restoreCatalog(_ data: Data) async throws {
        var req = URLRequest(url: catalogBackupURL())
        req.httpMethod = "POST"
        req.timeoutInterval = 60
        req.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
        req.httpBody = data
        let (body, res) = try await session.data(for: req)
        let status = (res as? HTTPURLResponse)?.statusCode ?? 0
        guard (200 ..< 300).contains(status) else {
            if let err = try? JSONDecoder().decode(APIErrorBody.self, from: body) {
                throw APIError(code: err.code, message: err.message, status: status)
            }
            throw Self.unreadable(status)
        }
    }

    /// A body that is not the error envelope. The words match the web client.
    private static func unreadable(_ status: Int) -> APIError {
        APIError(code: "http_\(status)", message: PlaybackOutage.requestFailed, status: status)
    }

    /// One URL path segment. Backup names are filenames; a slash would address a different route.
    private static func pathSegment(_ name: String) -> String {
        var allowed = CharacterSet.urlPathAllowed
        allowed.remove(charactersIn: "/")
        return name.addingPercentEncoding(withAllowedCharacters: allowed) ?? name
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
