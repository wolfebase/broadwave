import Foundation

/// The last lineup the app showed, so launch can paint before the network answers.
public struct CatalogSnapshot: Codable, Sendable {
    public var channels: [Channel]
    public var airings: [Airing]
    public var recordings: [Recording]

    public init(channels: [Channel], airings: [Airing], recordings: [Recording]) {
        self.channels = channels
        self.airings = airings
        self.recordings = recordings
    }
}

public enum CatalogCache {
    public static func load(serverID: String, directory: URL? = nil) -> CatalogSnapshot? {
        guard let data = try? Data(contentsOf: file(serverID, directory)) else { return nil }
        return try? APIClient.decoder.decode(CatalogSnapshot.self, from: data)
    }

    /// When the saved copy was written.
    public static func savedAt(serverID: String, directory: URL? = nil) -> Date? {
        let attrs = try? FileManager.default.attributesOfItem(atPath: file(serverID, directory).path)
        return attrs?[.modificationDate] as? Date
    }

    public static func save(_ snapshot: CatalogSnapshot, serverID: String, directory: URL? = nil) {
        guard let data = try? encoder.encode(snapshot) else { return }
        let url = file(serverID, directory)
        try? FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try? data.write(to: url, options: .atomic)
    }

    private static func file(_ serverID: String, _ directory: URL?) -> URL {
        let dir = directory ?? FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
        // Letters and digits alone map "abc-123" and "abc123" onto one file,
        // and a case-insensitive volume does the same for "Ab" and "ab".
        let safe = Data(serverID.utf8).map { String(format: "%02x", $0) }.joined()
        return dir.appendingPathComponent("broadwave-\(safe).json")
    }

    private static let encoder: JSONEncoder = {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .custom { date, enc in
            var box = enc.singleValueContainer()
            try box.encode(ISO8601DateFormatter.plainString(from: date))
        }
        return encoder
    }()
}
