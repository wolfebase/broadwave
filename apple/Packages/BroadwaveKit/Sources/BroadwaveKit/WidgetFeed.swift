import Foundation

/// What the iPhone and iPad widgets show. Each function is one widget's list.
public enum WidgetFeed {
    public struct Row: Equatable, Sendable, Identifiable {
        public var id: String
        /// Channel number, or empty for a row with no channel.
        public var number: String
        public var title: String
        /// The second line: a score, "Recording", a start time.
        public var detail: String
        public var start: Date?
        public var end: Date?
        public var link: URL
        /// Set when the row can be recorded from the widget.
        public var record: RecordAsk?

        public init(
            id: String, number: String, title: String, detail: String = "",
            start: Date? = nil, end: Date? = nil, link: URL, record: RecordAsk? = nil
        ) {
            self.id = id
            self.number = number
            self.title = title
            self.detail = detail
            self.start = start
            self.end = end
            self.link = link
            self.record = record
        }
    }

    /// Records one airing, as the app's Record once does.
    public struct RecordAsk: Equatable, Sendable {
        public var channelID: Int64
        public var title: String
        public var start: Date
    }

    /// Favorite channels with what they show now. With no favorites, every channel with a listing on now.
    public static func onNow(_ snap: TopShelf.Snapshot, now: Date, limit: Int = 4) -> [Row] {
        let shown = snap.channels.filter { $0.enabled && !$0.hidden }
        let current = onAir(snap.airings, now: now)
        let favorites = shown.filter(\.favorite)
        let pick = favorites.isEmpty ? shown.filter { current[$0.id] != nil } : favorites
        return pick.prefix(limit).map { channel in
            let airing = current[channel.id]
            var row = Row(
                id: "now-\(channel.id)", number: channel.displayNumber, title: airing?.title ?? channel.displayName,
                start: airing?.start, end: airing?.end, link: TopShelf.watchLink(channel.id)
            )
            if let airing {
                mark(&row, airing, snap)
            }
            return row
        }
    }

    /// Games of the teams the viewer follows, on now first, then the next to start.
    /// A game the scoreboard still has on stays past its listed end (overtime).
    public static func teams(
        _ snap: TopShelf.Snapshot, follows: [TeamFollow], scores: [ScoreGame], now: Date, limit: Int = 4
    ) -> [Row] {
        let byID = Dictionary(scores.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        let channels = Dictionary(snap.channels.filter { $0.enabled && !$0.hidden }.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        let games = snap.airings
            .filter { airing in
                (airing.end > now || airing.gameId.flatMap { byID[$0]?.state } == "in") && airing.kind == .sports
                    && channels[airing.channelId] != nil && followed(airing, follows)
            }
            .sorted { $0.start < $1.start }
        return games.prefix(limit).compactMap { airing in
            guard let channel = channels[airing.channelId] else { return nil }
            var row = Row(
                id: "game-\(airing.id)", number: channel.displayNumber, title: airing.title,
                start: airing.start, end: airing.end, link: TopShelf.watchLink(channel.id)
            )
            mark(&row, airing, snap)
            if let score = airing.gameId.flatMap({ byID[$0]?.line }) {
                row.detail = row.detail.isEmpty ? score : "\(score) · \(row.detail)"
            }
            return row
        }
    }

    /// Recording, set to record, or a Record button. The button adds a once pass,
    /// so after a tap the reloaded widget reads "Set to record".
    private static func mark(_ row: inout Row, _ airing: Airing, _ snap: TopShelf.Snapshot) {
        let recording = snap.recordings.contains { rec in
            rec.isRecording && rec.channelId == airing.channelId && (rec.endsAt ?? .distantFuture) > airing.start
                && rec.startedAt < airing.end
        }
        let planned = snap.plan.contains { item in
            !item.skipped && item.airing.channelId == airing.channelId && item.airing.start == airing.start
        }
        if recording {
            row.detail = "Recording"
        } else if planned {
            row.detail = "Set to record"
        } else {
            row.record = RecordAsk(channelID: airing.channelId, title: airing.title, start: airing.start)
        }
    }

    /// The Home shelf's rule: a followed team's short name (4 letters or more) in the title or subtitle.
    public static func followed(_ airing: Airing, _ follows: [TeamFollow]) -> Bool {
        follows.contains { team in
            let name = team.short.flatMap { $0.isEmpty ? nil : $0 } ?? team.name
            return name.count >= 4 && (airing.title.localizedCaseInsensitiveContains(name)
                || (airing.subtitle ?? "").localizedCaseInsensitiveContains(name))
        }
    }

    public static func recordingNow(_ recordings: [Recording], limit: Int = 4) -> [Row] {
        recordings.filter(\.isRecording).sorted { $0.startedAt < $1.startedAt }.prefix(limit).map { rec in
            Row(
                id: "rec-\(rec.id)", number: rec.guideNumber, title: rec.title, detail: rec.subtitle ?? "",
                start: rec.startedAt, end: rec.endsAt, link: TopShelf.watchLink(rec.channelId)
            )
        }
    }

    /// What records next. A conflict stays in the list: the server marks it skipped,
    /// and a show that will not record for lack of a tuner is what the viewer needs to see.
    public static func upNext(_ items: [PlannedAiring], channels: [Channel], now: Date, limit: Int = 4) -> [Row] {
        let numbers = Dictionary(channels.map { ($0.id, $0.displayNumber) }, uniquingKeysWith: { first, _ in first })
        return items.filter { (!$0.skipped || $0.conflict) && $0.airing.start > now }
            .sorted { $0.airing.start < $1.airing.start }
            .prefix(limit).map { item in
                let a = item.airing
                return Row(
                    id: "next-\(a.id)", number: numbers[a.channelId] ?? a.guideNumber ?? "", title: a.title,
                    detail: item.conflict ? "Conflict" : (a.subtitle ?? ""), start: a.start, end: a.end,
                    link: URL(string: "broadwave://recordings")!
                )
            }
    }

    private static func onAir(_ airings: [Airing], now: Date) -> [Int64: Airing] {
        var current: [Int64: Airing] = [:]
        for airing in airings where airing.isOn(at: now) {
            current[airing.channelId] = airing
        }
        return current
    }

    /// When the widget should ask again: the next listing that ends or starts, at most 15 minutes out.
    public static func refresh(after rows: [Row], now: Date) -> Date {
        let soon = now.addingTimeInterval(15 * 60)
        let edges = rows.flatMap { [$0.start, $0.end] }.compactMap(\.self).filter { $0 > now }
        return min(edges.min() ?? soon, soon)
    }
}
