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

    public static func save(_ snapshot: CatalogSnapshot, serverID: String, directory: URL? = nil) {
        guard let data = try? encoder.encode(snapshot) else { return }
        let url = file(serverID, directory)
        try? FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try? data.write(to: url, options: .atomic)
    }

    private static func file(_ serverID: String, _ directory: URL?) -> URL {
        let dir = directory ?? FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
        let safe = String(serverID.filter { $0.isLetter || $0.isNumber })
        return dir.appendingPathComponent("broadwave-\(safe).json")
    }

    private static let encoder: JSONEncoder = {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .custom { date, enc in
            var box = enc.singleValueContainer()
            try box.encode(ISO8601DateFormatter.plain.string(from: date))
        }
        return encoder
    }()
}
