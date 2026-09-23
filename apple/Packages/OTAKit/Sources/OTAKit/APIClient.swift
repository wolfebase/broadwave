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

    public func airings(hours: Int = 48) async throws -> [Airing] {
        struct R: Decodable { var airings: [Airing] }
        return try await send("GET", "/airings?hours=\(hours)", as: R.self).airings
    }

    public func setFavorite(_ channel: Channel, _ on: Bool) async throws -> Channel {
        struct B: Encodable { var favorite: Bool }
        return try await send("PATCH", "/channels/\(channel.id)", body: B(favorite: on))
    }

    // MARK: Live

    public func watch(channelID: Int64, caps: Caps, prefs: Prefs) async throws -> WatchSession {
        struct B: Encodable { var channelId: Int64; var caps: Caps; var prefs: Prefs }
        return try await send("POST", "/watch", body: B(channelId: channelID, caps: caps, prefs: prefs))
    }

    public func planMultiview(_ channelIDs: [Int64]) async throws -> MultiviewPlan {
        struct B: Encodable { var channelIds: [Int64] }
        return try await send("POST", "/multiview/plan", body: B(channelIds: channelIDs))
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

    public func addPass(title: String, channelID: Int64) async throws {
        struct B: Encodable { var title: String; var channelId: Int64 }
        struct R: Decodable {}
        _ = try await send("POST", "/passes", body: B(title: title, channelId: channelID), as: R.self)
    }

    public func posterURL(recordingID: Int64) -> URL {
        url("/media/poster/\(recordingID)")
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
