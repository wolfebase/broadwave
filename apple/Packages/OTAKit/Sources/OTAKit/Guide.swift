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

private let sportsWords = regex(#"\b(sports?|football|basketball|baseball|hockey|soccer|golf|tennis|racing|nascar|motorsports?|boxing|mma|ufc|wrestling|olympics?|nfl|nba|mlb|nhl|mls|wnba|ncaa|bowl|playoffs?|pregame|postgame)\b"#)
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
        let c = category ?? ""
        if matches(sportsWords, c) || (matches(sportsWords, title) && matches(versus, title)) {
            return .sports
        }
        if c.localizedCaseInsensitiveContains("news") || title.localizedCaseInsensitiveContains("news") {
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

    /// "Chiefs at Bills" as a matchup, when the listing names one.
    var matchup: (String, String)? {
        let text = (subtitle.map { $0.range(of: #" (at|vs\.?|@) "#, options: [.regularExpression, .caseInsensitive]) != nil } ?? false) ? subtitle! : title
        guard let r = text.range(of: #"\s+(at|vs\.?|@)\s+"#, options: [.regularExpression, .caseInsensitive]) else { return nil }
        let clean: (Substring) -> String = { s in
            let str = String(s)
            if let colon = str.lastIndex(of: ":") {
                return str[str.index(after: colon)...].trimmingCharacters(in: .whitespaces)
            }
            return str.trimmingCharacters(in: .whitespaces)
        }
        return (clean(text[..<r.lowerBound]), clean(text[r.upperBound...]))
    }

    func minutesLeft(at date: Date) -> String {
        let m = max(0, Int(end.timeIntervalSince(date) / 60))
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
}

public extension Channel {
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
