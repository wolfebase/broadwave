import Foundation

/// What the iPhone's Live Activities show: each recording in progress and each
/// followed team's game on now. The app runs the activities; this decides them.
public enum LiveActivityFeed {
    public enum Kind: String, Codable, Sendable {
        case recording, game
    }

    /// What an activity shows. It changes while the activity runs.
    public struct Content: Codable, Hashable, Sendable {
        public var title: String
        /// Channel number.
        public var number: String
        /// The score, the episode, or empty.
        public var detail: String
        public var start: Date
        public var end: Date?
        /// False once the recording stopped or the game is final.
        public var live: Bool

        public init(title: String, number: String, detail: String = "", start: Date, end: Date? = nil, live: Bool = true) {
            self.title = title
            self.number = number
            self.detail = detail
            self.start = start
            self.end = end
            self.live = live
        }
    }

    public struct Item: Equatable, Sendable, Identifiable {
        /// Stable across refreshes: "rec-<id>" or "game-<airing id>".
        public var id: String
        public var kind: Kind
        public var channelID: Int64
        public var content: Content

        public var link: URL {
            TopShelf.watchLink(channelID)
        }
    }

    /// iOS shows a few at once at most; past that the oldest are the ones that matter.
    public static let limit = 3

    public struct Options: Equatable, Sendable {
        public var recordings: Bool
        public var games: Bool

        public init(recordings: Bool = true, games: Bool = true) {
            self.recordings = recordings
            self.games = games
        }
    }

    /// A game the house records gets no activity of its own: the recording's
    /// shows it without the score, so a viewer saving it for later sees no spoiler.
    /// A game on two channels is one activity.
    public static func wanted(
        _ snap: TopShelf.Snapshot, follows: [TeamFollow], scores: [ScoreGame], now: Date, options: Options = Options()
    ) -> [Item] {
        var items: [Item] = []
        if options.recordings {
            items += snap.recordings.filter(\.isRecording).sorted { $0.startedAt < $1.startedAt }.map { rec in
                Item(id: "rec-\(rec.id)", kind: .recording, channelID: rec.channelId, content: content(rec, live: true))
            }
        }
        if options.games {
            items += games(snap, follows: follows, scores: scores) { airing, state in
                // On the scoreboard's word when it has the game: overtime runs past the listing.
                state.map { $0 == "in" } ?? airing.isOn(at: now)
            }
        }
        return Array(items.prefix(limit))
    }

    /// What an activity that is over shows last: a recording that finished, a game the scoreboard calls final.
    /// Anything else no longer wanted (a setting turned off, a failed recording, a game the house
    /// started recording) has no last word and goes at once.
    public static func finals(
        _ snap: TopShelf.Snapshot, follows: [TeamFollow], scores: [ScoreGame], options: Options = Options()
    ) -> [String: Content] {
        var out: [String: Content] = [:]
        if options.recordings {
            for rec in snap.recordings where rec.status == "complete" {
                out["rec-\(rec.id)"] = content(rec, live: false)
            }
        }
        if options.games {
            for var item in games(snap, follows: follows, scores: scores, when: { _, state in state == "post" }) {
                item.content.live = false
                out[item.id] = item.content
            }
        }
        return out
    }

    private static func content(_ rec: Recording, live: Bool) -> Content {
        Content(title: rec.title, number: rec.guideNumber, detail: rec.subtitle ?? "", start: rec.startedAt, end: rec.endsAt, live: live)
    }

    private static func games(
        _ snap: TopShelf.Snapshot, follows: [TeamFollow], scores: [ScoreGame], when: (Airing, String?) -> Bool
    ) -> [Item] {
        guard !follows.isEmpty else { return [] }
        let byID = Dictionary(scores.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        let shown = Dictionary(snap.channels.filter { $0.enabled && !$0.hidden }.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        var seen = Set<String>()
        var items: [Item] = []
        for airing in snap.airings.sorted(by: { $0.start < $1.start }) {
            guard airing.kind == .sports, let channel = shown[airing.channelId], WidgetFeed.followed(airing, follows),
                  when(airing, airing.gameId.flatMap { byID[$0]?.state }), !recorded(airing, snap.recordings) else { continue }
            let id = "game-\(airing.gameId ?? String(airing.id))"
            guard seen.insert(id).inserted else { continue }
            let score = airing.gameId.flatMap { byID[$0]?.line } ?? ""
            items.append(Item(
                id: id, kind: .game, channelID: channel.id,
                content: Content(title: airing.title, number: channel.displayNumber, detail: score, start: airing.start, end: airing.end)
            ))
        }
        return items
    }

    /// A recording of the same game (any channel, done or not), or one in progress over this airing.
    private static func recorded(_ airing: Airing, _ recordings: [Recording]) -> Bool {
        recordings.contains { rec in
            if let game = airing.gameId, rec.gameId == game, rec.missing != true {
                return true
            }
            return rec.isRecording && rec.channelId == airing.channelId && rec.startedAt < airing.end
                && (rec.endsAt ?? .distantFuture) > airing.start
        }
    }

    public struct Plan: Equatable, Sendable {
        public var start: [Item] = []
        public var update: [Item] = []
        /// Over, each with what it shows last for a few minutes.
        public var end: [String: Content] = [:]
        /// Over with nothing to say: gone at once.
        public var dismiss: Set<String> = []
    }

    /// What to start, change, and end, given what runs now (id to content).
    public static func plan(running: [String: Content], wanted: [Item], finals: [String: Content] = [:]) -> Plan {
        var plan = Plan()
        let ids = Set(wanted.map(\.id))
        for item in wanted {
            if let now = running[item.id] {
                if now != item.content {
                    plan.update.append(item)
                }
            } else {
                plan.start.append(item)
            }
        }
        for id in running.keys where !ids.contains(id) {
            if let last = finals[id] {
                plan.end[id] = last
            } else {
                plan.dismiss.insert(id)
            }
        }
        return plan
    }
}
