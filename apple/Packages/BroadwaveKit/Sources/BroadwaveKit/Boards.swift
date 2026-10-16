import Foundation

/// The Sports page lists games, not shows about sports.
public enum SportsBoard {
    /// One game, on the channel to show it on, and the other channels carrying the same game.
    public struct Entry: Sendable, Identifiable {
        public let channel: Channel
        public let airing: Airing
        public let also: [Channel]

        public var id: Int64 {
            airing.id
        }
    }

    /// A game the server matched to a scoreboard, or a sports event the guide lists as
    /// live or as one side against another. Sitcoms about football ("The Game"), talk
    /// shows, and magazines are left out.
    public static func isGame(_ airing: Airing) -> Bool {
        if let id = airing.gameId, !id.isEmpty {
            return true
        }
        let genres = (airing.category ?? "")
            .split(separator: ",")
            .map { $0.trimmingCharacters(in: .whitespaces).lowercased() }
        guard let first = genres.first, first == "sport" || first == "sports" || first == "sports event" else { return false }
        if genres.contains(where: { $0.contains("talk") || $0 == "sports non-event" }) {
            return false
        }
        return airing.matchup != nil || airing.live == true
    }

    /// Games only, each once. The same game on two stations (or a 3.0 station and its
    /// 1.0 twin) is one entry on the first channel in guide order, a favorite first.
    public static func games(_ pairs: [(Channel, Airing)]) -> [Entry] {
        var order: [String] = []
        var found: [String: [(Channel, Airing)]] = [:]
        for pair in pairs where isGame(pair.1) {
            let key = sameGame(pair.1)
            if found[key] == nil {
                order.append(key)
            }
            found[key, default: []].append(pair)
        }
        return order.compactMap { key in
            guard let list = found[key], !list.isEmpty else { return nil }
            let first = list.first { $0.0.favorite } ?? list[0]
            var seen: Set<Int64> = [first.0.id]
            var also: [Channel] = []
            for (channel, _) in list where seen.insert(channel.id).inserted {
                if let twin = channel.twinId, twin == first.0.id {
                    continue
                }
                also.append(channel)
            }
            return Entry(channel: first.0, airing: first.1, also: also)
        }
    }

    /// What makes two listings the same game: the scoreboard id, else the matchup (a
    /// later airing of it is a replay), else title and start.
    static func sameGame(_ airing: Airing) -> String {
        if let id = airing.gameId, !id.isEmpty {
            return "game:\(id)"
        }
        let title = airing.title.trimmingCharacters(in: .whitespaces).lowercased()
        let sub = (airing.subtitle ?? "").trimmingCharacters(in: .whitespaces).lowercased()
        if let pair = airing.matchup {
            return "match:\(pair.0.lowercased())|\(pair.1.lowercased())"
        }
        return "\(title)|\(sub)|\(Int(airing.start.timeIntervalSince1970))"
    }

    /// The league as a short badge when the listing names one: "NFL", "College Football".
    public static func league(_ airing: Airing) -> String? {
        let title = airing.title.trimmingCharacters(in: .whitespaces)
        if airing.matchup != nil, let sub = airing.subtitle, !sub.isEmpty, sub != title {
            return title
        }
        if let colon = title.firstIndex(of: ":") {
            let head = title[..<colon].trimmingCharacters(in: .whitespaces)
            return head.isEmpty ? nil : head
        }
        return nil
    }

    /// "Q4 9:28", "Halftime", "Final" from the scoreboard's own words.
    public static func status(_ game: Game) -> String {
        if game.state == "post" || game.completed == true {
            return String(localized: "Final", bundle: .module)
        }
        if let detail = game.detail, !detail.isEmpty {
            return detail
        }
        return ""
    }
}

/// Search results the way the TV shows them: on now first, then later, each airing once.
public enum SearchBoard {
    public struct Groups: Sendable {
        public var onNow: [Airing]
        public var later: [Airing]

        public init(onNow: [Airing] = [], later: [Airing] = []) {
            self.onNow = onNow
            self.later = later
        }

        public var isEmpty: Bool {
            onNow.isEmpty && later.isEmpty
        }
    }

    /// A 3.0 station and its clear twin list the same airing twice; the one on a channel
    /// the guide shows wins. Ended airings are dropped.
    public static func group(_ airings: [Airing], visible: Set<Int64>, now: Date) -> Groups {
        var picked: [String: Airing] = [:]
        var order: [String] = []
        for airing in airings where airing.end > now {
            let key = SportsBoard.sameGame(airing)
            if let held = picked[key] {
                if !visible.contains(held.channelId), visible.contains(airing.channelId) {
                    picked[key] = airing
                }
                continue
            }
            picked[key] = airing
            order.append(key)
        }
        let list = order.compactMap { picked[$0] }
        return Groups(
            onNow: list.filter { $0.isOn(at: now) },
            later: list.filter { !$0.isOn(at: now) }.sorted { $0.start < $1.start }
        )
    }

    /// Titles from the guide that start with, then contain, what was typed.
    public static func suggestions(_ query: String, titles: [String], limit: Int = 6) -> [String] {
        let typed = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard typed.count >= 2 else { return [] }
        var seen = Set<String>()
        var starts: [String] = []
        var inside: [String] = []
        for title in titles {
            let key = title.lowercased()
            guard key != typed.lowercased(), seen.insert(key).inserted else { continue }
            if title.range(of: typed, options: [.caseInsensitive, .anchored, .diacriticInsensitive]) != nil {
                starts.append(title)
            } else if title.range(of: typed, options: [.caseInsensitive, .diacriticInsensitive]) != nil {
                inside.append(title)
            }
        }
        let byLength = { (a: String, b: String) in a.count == b.count ? a < b : a.count < b.count }
        return Array((starts.sorted(by: byLength) + inside.sorted(by: byLength)).prefix(limit))
    }
}

/// What this screen searched for, newest first. Kept on the device.
public struct RecentSearches: Sendable {
    public static let key = "recentSearches"
    public static let limit = 8

    public private(set) var items: [String]

    public init(_ items: [String] = []) {
        self.items = Array(items.prefix(Self.limit))
    }

    public static func load(_ defaults: UserDefaults = .standard) -> RecentSearches {
        RecentSearches(defaults.stringArray(forKey: key) ?? [])
    }

    public func save(_ defaults: UserDefaults = .standard) {
        defaults.set(items, forKey: Self.key)
    }

    /// Moves a search to the front. Blank and one-letter searches are not kept.
    public mutating func note(_ query: String) {
        let typed = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard typed.count >= 2 else { return }
        items.removeAll { $0.caseInsensitiveCompare(typed) == .orderedSame }
        items.insert(typed, at: 0)
        items = Array(items.prefix(Self.limit))
    }

    public mutating func clear() {
        items = []
    }
}

public extension SearchBoard {
    /// One show and its airings: on now first, then by start.
    struct Show: Sendable, Identifiable {
        public let id: String
        public let airings: [Airing]
        /// The airing on at the time the list was made.
        public let live: Airing?

        /// What the card leads with: the airing on now, else the next one.
        public var lead: Airing {
            live ?? airings[0]
        }

        /// The first airing that has not started.
        public func next(after now: Date) -> Airing? {
            airings.first { $0.start > now }
        }
    }

    /// Results as one card per show. Every airing of a series is one card; a game is its
    /// own card, so two different College Football games stay apart. A twin's copy is dropped
    /// as `group` drops it. Shows on now lead; the rest follow by their next airing.
    static func shows(_ airings: [Airing], visible: Set<Int64>, now: Date) -> [Show] {
        let grouped = group(airings, visible: visible, now: now)
        var order: [String] = []
        var found: [String: [Airing]] = [:]
        for airing in grouped.onNow + grouped.later {
            let key = showKey(airing)
            if found[key] == nil {
                order.append(key)
            }
            found[key, default: []].append(airing)
        }
        return order.compactMap { key -> Show? in
            guard let list = found[key], !list.isEmpty else { return nil }
            let live = list.first { $0.isOn(at: now) }
            let rest = list.filter { !$0.isOn(at: now) }.sorted { $0.start < $1.start }
            return Show(id: key, airings: (live.map { [$0] } ?? []) + rest, live: live)
        }
        .sorted { a, b in
            if (a.live != nil) != (b.live != nil) {
                return a.live != nil
            }
            return a.lead.start < b.lead.start
        }
    }

    internal static func showKey(_ airing: Airing) -> String {
        if (airing.gameId ?? "").isEmpty == false || airing.matchup != nil {
            return SportsBoard.sameGame(airing)
        }
        return "show:" + airing.title.trimmingCharacters(in: .whitespaces).lowercased()
    }
}

/// Home's lead: something worth watching now, not a show in its last minutes.
public enum HomeBoard {
    /// The channel and program the hero shows. A live game leads, then a favorite channel,
    /// then a program with art, a new episode, or one just starting. A program with under
    /// ten minutes left, paid programming, and shopping sink to the bottom.
    public static func hero(_ pairs: [(Channel, Airing?)], now: Date) -> (Channel, Airing?)? {
        var best: (Channel, Airing?)?
        var bestScore = -Double.infinity
        for (channel, airing) in pairs {
            let s = score(channel, airing, now: now)
            if s > bestScore {
                best = (channel, airing)
                bestScore = s
            }
        }
        return best
    }

    static func score(_ channel: Channel, _ airing: Airing?, now: Date) -> Double {
        var s = channel.favorite ? 2.0 : 0
        if channel.hd {
            s += 0.5
        }
        guard let airing, airing.isOn(at: now) else { return s - 6 }
        let title = airing.title.lowercased()
        let category = (airing.category ?? "").lowercased()
        if title.contains("paid program") || title.contains("infomercial") || category.contains("shopping") {
            return s - 20
        }
        s += 1
        if SportsBoard.isGame(airing) {
            s += 5
        }
        if let image = airing.imageUrl, !image.isEmpty {
            s += 1.5
        }
        if airing.new == true || airing.premiere == true {
            s += 1
        }
        let left = airing.end.timeIntervalSince(now) / 60
        if left < 10 {
            s -= 5
        } else if left < 20 {
            s -= 1.5
        }
        if now.timeIntervalSince(airing.start) < 15 * 60 {
            s += 0.5
        }
        return s
    }
}
