import Foundation

/// This install's own id on the event socket, so another screen can send it a channel.
public enum ScreenID {
    public static let key = "broadwave-screen-id"

    /// The device's vendor id when there is one, so a device restored from another's
    /// backup does not answer to its id. Otherwise one made once and kept. One the
    /// server would refuse is replaced.
    public static func current(_ defaults: UserDefaults = .standard, vendor: String? = nil) -> String {
        if let vendor, valid(vendor) {
            return vendor
        }
        if let saved = defaults.string(forKey: key), valid(saved) {
            return saved
        }
        let id = UUID().uuidString
        defaults.set(id, forKey: key)
        return id
    }

    /// 1-64 letters, digits, and dashes, the same rule as the server.
    public static func valid(_ id: String) -> Bool {
        guard (1 ... 64).contains(id.count) else { return false }
        return id.unicodeScalars.allSatisfy { $0 == "-" || ("0" ... "9").contains($0) || ("a" ... "z").contains($0) || ("A" ... "Z").contains($0) }
    }

    /// The screens another one can move to: open, with an id, and not itself.
    public static func others(_ screens: [Screen], except id: String) -> [Screen] {
        screens.filter { !$0.id.isEmpty && $0.id != id }
    }
}

/// A channel another screen sent here (`screen.watch`).
public struct ScreenWatch: Codable, Sendable, Equatable {
    public var channelId: Int64
    public var from: String
    /// Each send is its own, so the same channel sent twice plays twice.
    public var received = UUID()

    private enum CodingKeys: String, CodingKey { case channelId, from }

    public init(channelId: Int64, from: String) {
        self.channelId = channelId
        self.from = from
    }

    /// A sender that gave no name still plays.
    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        channelId = try c.decode(Int64.self, forKey: .channelId)
        from = try c.decodeIfPresent(String.self, forKey: .from) ?? ""
    }

    /// "From Kitchen iPad", or nil when the sender gave no name.
    public var note: String? {
        let name = from.trimmingCharacters(in: .whitespacesAndNewlines)
        return name.isEmpty ? nil : "From \(name)"
    }
}

/// A 202 only means the other screen's socket took the message. A phone that
/// went to sleep keeps its socket for a while and never plays, so the sender
/// stops only once the server sees that screen on the channel.
public enum MoveCheck {
    public static let attempts = 12
    public static let every = Duration.seconds(1)

    /// The screen reports the sent channel as the one it is watching.
    public static func started(_ screens: [Screen], id: String, channel: Int64) -> Bool {
        screens.contains { $0.id == id && $0.channelId == channel }
    }

    /// Reads the screens once a second, 12 times at most. False when the screen
    /// never started the channel or the wait was cancelled. A failed read counts as not yet.
    @MainActor
    public static func confirm(
        id: String, channel: Int64, attempts: Int = attempts, every: Duration = every,
        screens: @MainActor () async -> [Screen]?
    ) async -> Bool {
        for attempt in 0 ..< attempts {
            if attempt > 0 {
                guard await (try? Task.sleep(for: every)) != nil else { return false }
            }
            if let list = await screens(), started(list, id: id, channel: channel) {
                return true
            }
        }
        return false
    }
}

/// Handoff of a live channel to another iPhone, iPad, or Mac.
public enum WatchHandoff {
    public static let activityType = "com.wolfeup.broadwave.watch"
    public static let channelKey = "channelId"
    public static let serverKey = "serverId"

    /// The server's web player, so a Mac picks up in its browser. Nil unless http or https.
    public static func webpageURL(base: URL, channelID: Int64) -> URL? {
        guard let scheme = base.scheme?.lowercased(), scheme == "http" || scheme == "https", base.host() != nil else { return nil }
        guard var parts = URLComponents(url: base, resolvingAgainstBaseURL: false) else { return nil }
        let path = parts.path.hasSuffix("/") ? String(parts.path.dropLast()) : parts.path
        parts.path = path + "/watch"
        parts.queryItems = [URLQueryItem(name: "channel", value: String(channelID))]
        parts.fragment = nil
        return parts.url
    }

    /// The channel a continued activity names. Nil when it came from another server.
    public static func channel(_ userInfo: [AnyHashable: Any]?, server: String?) -> Int64? {
        guard let info = userInfo else { return nil }
        let id = (info[channelKey] as? Int64) ?? (info[channelKey] as? Int).map(Int64.init) ?? (info[channelKey] as? NSNumber)?.int64Value
        guard let id, id > 0 else { return nil }
        if let sent = info[serverKey] as? String, !sent.isEmpty, let server, !server.isEmpty, sent != server {
            return nil
        }
        return id
    }
}

/// A remote button another device pressed for this screen (`screen.remote`).
public struct ScreenRemote: Codable, Sendable, Equatable {
    public enum Action: String, Codable, Sendable, CaseIterable {
        case up, down, pause, play, record
    }

    public var action: Action
    public var from: String
    /// Each press is its own, so the same button twice acts twice.
    public var received = UUID()

    private enum CodingKeys: String, CodingKey { case action, from }

    public init(action: Action, from: String = "") {
        self.action = action
        self.from = from
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        action = try c.decode(Action.self, forKey: .action)
        from = try c.decodeIfPresent(String.self, forKey: .from) ?? ""
    }
}
