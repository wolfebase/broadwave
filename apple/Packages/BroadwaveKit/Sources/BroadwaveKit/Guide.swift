import Foundation

public enum Category: String, Sendable, CaseIterable {
    case sports, news, movies, kids, series, other

    public var label: String {
        switch self {
        case .sports: "Sports"
        case .news: "News"
        case .movies: "Movies"
        case .kids: "Kids"
        case .series: "Shows"
        case .other: "Other"
        }
    }
}

private let sportsWords = regex(#"\b(sports?|football|basketball|baseball|hockey|soccer|golf|tennis|racing|nascar|motorsports?|boxing|mma|ufc|wrestling|olympics?|nfl|nba|mlb|nhl|mls|wnba|ncaa|college (football|basketball)|bowl|playoffs?|pregame|postgame|game day)\b"#)
private let versus = regex(#"\b(vs\.?|at|@)\b"#)

private func regex(_ pattern: String) -> NSRegularExpression {
    // Patterns above are fixed; a mistake is a programmer error, not a listing error.
    guard let re = try? NSRegularExpression(pattern: pattern, options: [.caseInsensitive]) else {
        fatalError("bad pattern")
    }
    return re
}

private func matches(_ re: NSRegularExpression, _ s: String) -> Bool {
    re.firstMatch(in: s, range: NSRange(s.startIndex..., in: s)) != nil
}

public extension Airing {
    /// The same rules as the web guide, so both clients tint and filter alike.
    var kind: Category {
        let c = (category ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        if matches(sportsWords, c) || (matches(sportsWords, title) && matches(versus, title)) {
            return .sports
        }
        if c.localizedCaseInsensitiveContains("news") || title.range(of: #"\bnews\b"#, options: [.regularExpression, .caseInsensitive]) != nil {
            return .news
        }
        if c.localizedCaseInsensitiveContains("movie") || c.localizedCaseInsensitiveContains("film") {
            return .movies
        }
        if ["child", "kids", "animat", "family", "educational"].contains(where: { c.localizedCaseInsensitiveContains($0) }) {
            return .kids
        }
        return c.isEmpty ? .other : .series
    }

    /// "Harbor at Valley" as a matchup, when the listing names one.
    /// A league prefix before a colon is dropped on either side, as the web does.
    var matchup: (String, String)? {
        let text = (subtitle.map { $0.range(of: #" (at|vs\.?|@) "#, options: [.regularExpression, .caseInsensitive]) != nil } ?? false) ? subtitle! : title
        guard let re = try? NSRegularExpression(pattern: #"^(.*?)\s+(?:at|vs\.?|@)\s+(.*)$"#, options: [.caseInsensitive]),
              let hit = re.firstMatch(in: text, range: NSRange(text.startIndex..., in: text)),
              hit.numberOfRanges == 3,
              let left = Range(hit.range(at: 1), in: text),
              let right = Range(hit.range(at: 2), in: text)
        else { return nil }
        return (Self.side(String(text[left])), Self.side(String(text[right])))
    }

    /// Drops a "NFL: " style prefix. The first colon wins, so a second one stays in the name.
    private static func side(_ raw: String) -> String {
        guard let range = raw.range(of: #"^.*?:\s*"#, options: .regularExpression) else {
            return raw.trimmingCharacters(in: .whitespaces)
        }
        return String(raw[range.upperBound...]).trimmingCharacters(in: .whitespaces)
    }

    func minutesLeft(at date: Date) -> String {
        let m = max(0, Int((end.timeIntervalSince(date) / 60).rounded()))
        return m >= 60 ? "\(m / 60)h \(m % 60)m left" : "\(m)m left"
    }
}

/// Listings by channel, sorted, for fast "what's on" lookups.
public struct GuideIndex: Sendable {
    private var byChannel: [Int64: [Airing]] = [:]

    public init(_ airings: [Airing]) {
        for a in airings {
            byChannel[a.channelId, default: []].append(a)
        }
        for key in byChannel.keys {
            byChannel[key]?.sort { $0.start < $1.start }
        }
    }

    public func airings(_ channel: Int64) -> [Airing] {
        byChannel[channel] ?? []
    }

    public func on(_ channel: Int64, at date: Date) -> Airing? {
        airings(channel).first { $0.isOn(at: date) }
    }

    public func next(_ channel: Int64, after date: Date) -> Airing? {
        airings(channel).first { $0.start >= date }
    }

    /// Program art for a recording: the same program when it has a picture, otherwise the closest listing with the same title.
    public func artAiring(for recording: Recording) -> Airing? {
        let list = airings(recording.channelId)
        if let pid = recording.programId, !pid.isEmpty {
            let matched = list.filter { $0.programId == pid && hasArt($0) }
            if let hit = closest(matched, to: recording.startedAt) {
                return hit
            }
        }
        let titled = list.filter { $0.title == recording.title && hasArt($0) }
        return closest(titled, to: recording.startedAt)
    }
}

private func hasArt(_ airing: Airing) -> Bool {
    guard let image = airing.imageUrl else { return false }
    return !image.isEmpty
}

private func closest(_ list: [Airing], to date: Date) -> Airing? {
    list.min { abs($0.start.timeIntervalSince(date)) < abs($1.start.timeIntervalSince(date)) }
}

public extension Channel {
    /// An ATSC 3.0 broadcast.
    var isATSC3: Bool {
        standard == "atsc3"
    }

    /// Channel numbers sort as numbers: 14.2 before 14.10.
    static func guideOrder(_ a: Channel, _ b: Channel) -> Bool {
        let x = a.displayNumber.split(separator: ".").map { Int($0) ?? 0 }
        let y = b.displayNumber.split(separator: ".").map { Int($0) ?? 0 }
        for i in 0 ..< max(x.count, y.count) {
            let l = i < x.count ? x[i] : 0
            let r = i < y.count ? y[i] : 0
            if l != r {
                return l < r
            }
        }
        return a.displayName < b.displayName
    }
}

/// Which channel to play when a viewer picks a row, including an encrypted 3.0 station.
public struct PlaybackChoice: Equatable, Sendable {
    public var channel: Channel
    /// Set when the row was an encrypted 3.0 station and the clear broadcast plays instead.
    public var note: String?

    public init(channel: Channel, note: String? = nil) {
        self.channel = channel
        self.note = note
    }
}

public enum ClearBroadcast {
    public static let encryptedNote = "The 3.0 version is encrypted. Showing the regular broadcast."

    /// A channel on the guide plays as itself. An encrypted 3.0 station plays its clear 1.0 twin.
    /// A hidden half of a 1.0/3.0 pair plays as the half that is on the guide.
    public static func play(id: Int64, visible: [Channel], lineup: [Channel], depth: Int = 0) -> PlaybackChoice? {
        if let shown = visible.first(where: { $0.id == id }) {
            return PlaybackChoice(channel: shown)
        }
        guard let asked = lineup.first(where: { $0.id == id }) else { return nil }
        if let twin = asked.playsAs, let clear = visible.first(where: { $0.id == twin }) {
            return PlaybackChoice(channel: clear, note: encryptedNote)
        }
        if asked.protected != true, let twin = asked.twinId, let other = visible.first(where: { $0.id == twin }) {
            return PlaybackChoice(channel: other)
        }
        // Another tuner's copy plays as the row on the guide, which can itself
        // be the hidden half of a pair.
        if let row = asked.sameAs, row != id, depth < 2 {
            return play(id: row, visible: visible, lineup: lineup, depth: depth + 1)
        }
        return nil
    }

    /// The channels a multiview link, a saved set, or a deep link should open.
    /// Each id goes through `play`. The first choice for a channel wins, so a
    /// later encrypted id for a channel already on the grid does not add the note.
    public static func grid(ids: [Int64], visible: [Channel], lineup: [Channel]) -> PlaybackGrid {
        var channels: [Channel] = []
        var standIns = Set<Int64>()
        var seen = Set<Int64>()
        for id in ids {
            guard let choice = play(id: id, visible: visible, lineup: lineup) else { continue }
            if !seen.insert(choice.channel.id).inserted {
                continue
            }
            channels.append(choice.channel)
            if choice.note != nil {
                standIns.insert(choice.channel.id)
            }
        }
        return PlaybackGrid(channels: channels, standIns: standIns)
    }
}

/// Channels a multiview should show, and which of them stand in for an encrypted 3.0 station.
public struct PlaybackGrid: Equatable, Sendable {
    public var channels: [Channel]
    public var standIns: Set<Int64>

    public init(channels: [Channel], standIns: Set<Int64>) {
        self.channels = channels
        self.standIns = standIns
    }
}

/// Where the guide grid starts: the half hour before the one `now` is in, so the
/// program on the air and the now line are always on screen.
/// `Calendar.date(bySetting:)` searches forward, so it would pick the next half hour.
public func guideOrigin(for now: Date, calendar: Calendar = .current) -> Date {
    var parts = calendar.dateComponents([.era, .year, .month, .day, .hour, .minute], from: now)
    parts.minute = (parts.minute ?? 0) < 30 ? 0 : 30
    let floored = calendar.date(from: parts) ?? now
    return floored.addingTimeInterval(-30 * 60)
}
