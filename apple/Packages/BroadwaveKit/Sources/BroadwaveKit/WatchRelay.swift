import Foundation

/// What the watch app and the iPhone app say to each other. A watch can't call
/// a server on the home network by itself (watchOS keeps plain local HTTP for
/// a few kinds of app), so the iPhone makes every call and answers the watch.
///
/// Foundation only: the watch target compiles this file on its own, without the rest of the kit.
public enum WatchRelay {
    static let key = "json"

    /// A remote button. The same names as `screen.remote` on the server.
    public enum Press: String, Codable, Sendable, CaseIterable {
        case up, down, pause, play, record
    }

    public enum Request: Codable, Sendable, Equatable {
        case screens
        case press(screen: String, Press)
    }

    /// A screen the watch can work, with what it plays in words.
    public struct Row: Codable, Sendable, Equatable, Identifiable {
        public var id: String
        public var name: String
        public var kind: String
        /// "4.1 Big Buck Bunny", or empty when it plays nothing a remote can change.
        public var playing: String

        public init(id: String, name: String, kind: String, playing: String) {
            self.id = id
            self.name = name
            self.kind = kind
            self.playing = playing
        }
    }

    public struct Reply: Codable, Sendable, Equatable {
        public var screens: [Row]
        /// Safe to show as is.
        public var error: String?

        public init(screens: [Row] = [], error: String? = nil) {
            self.screens = screens
            self.error = error
        }
    }

    public static func message(_ request: Request) -> [String: Any] {
        [key: (try? JSONEncoder().encode(request)) ?? Data()]
    }

    public static func request(_ message: [String: Any]) -> Request? {
        guard let data = message[key] as? Data else { return nil }
        return try? JSONDecoder().decode(Request.self, from: data)
    }

    public static func message(_ reply: Reply) -> [String: Any] {
        [key: (try? JSONEncoder().encode(reply)) ?? Data()]
    }

    public static func reply(_ message: [String: Any]) -> Reply? {
        guard let data = message[key] as? Data else { return nil }
        return try? JSONDecoder().decode(Reply.self, from: data)
    }

    /// Phones and tablets first would bury the televisions people want to work.
    public static func ordered(_ rows: [Row], without own: String?) -> [Row] {
        let rank = ["appletv": 0, "web": 1, "ipad": 2, "iphone": 3]
        return rows.filter { $0.id != own }.sorted {
            let a = rank[$0.kind] ?? 1, b = rank[$1.kind] ?? 1
            return a != b ? a < b : $0.name.localizedStandardCompare($1.name) == .orderedAscending
        }
    }

    /// The crown settled after turning from `from` to `to`, one channel per detent.
    /// Turning up is channel up. A fast spin is held to a few channels.
    public static func crownSteps(from: Double, to: Double, most: Int = 5) -> (Press, Int)? {
        let steps = Int((to - from).rounded())
        guard steps != 0 else { return nil }
        return (steps > 0 ? .up : .down, min(abs(steps), most))
    }
}
