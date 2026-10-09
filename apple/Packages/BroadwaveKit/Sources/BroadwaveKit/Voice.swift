import Foundation

/// What Siri, Shortcuts, and Spotlight ask for, matched against the lineup and the guide.
public enum Voice {
    /// "9", "9.1", "channel 9 1", "4 point 1", "41", "KMBC", "ABC": the channel a viewer means.
    /// A bare number is its first subchannel. An encrypted 3.0 station plays as its clear twin.
    public static func channel(_ spoken: String, in channels: [Channel]) -> Channel? {
        let byID = Dictionary(channels.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        let shown = channels.filter { $0.enabled && !$0.hidden }
        guard let hit = match(spoken, shown) ?? match(spoken, channels.filter(\.enabled)) else { return nil }
        return hit.playsAs.flatMap { byID[$0] } ?? hit
    }

    private static func match(_ spoken: String, _ channels: [Channel]) -> Channel? {
        var said = " " + spoken.lowercased().trimmingCharacters(in: .whitespacesAndNewlines) + " "
        for word in [" point ", " dot "] {
            said = said.replacingOccurrences(of: word, with: ".")
        }
        said = said.trimmingCharacters(in: .whitespaces)
        for lead in ["channel ", "ch "] where said.hasPrefix(lead) {
            said = String(said.dropFirst(lead.count))
        }
        if let hit = numbered(said, channels) {
            return hit
        }
        let names: [(Channel) -> String?] = [\.displayName, \.guideName, \.network]
        for name in names {
            if let hit = channels.first(where: { name($0)?.lowercased() == said }) {
                return hit
            }
        }
        return said.count >= 3 ? channels.first { $0.displayName.lowercased().contains(said) } : nil
    }

    private static func numbers(_ text: String) -> [Int]? {
        let parts = text.split(whereSeparator: { " .-".contains($0) })
        let values = parts.compactMap { Int($0) }
        return values.isEmpty || values.count != parts.count ? nil : values
    }

    private static func numbered(_ said: String, _ channels: [Channel]) -> Channel? {
        guard let asked = numbers(said), asked.count <= 2 else { return nil }
        let listed = channels.compactMap { channel in numbers(channel.displayNumber).map { (channel, $0) } }
        func find(_ major: Int, _ minor: Int?) -> Channel? {
            listed.filter { $0.1[0] == major && (minor == nil || $0.1.dropFirst().first == minor) }
                .min { ($0.1.dropFirst().first ?? 0) < ($1.1.dropFirst().first ?? 0) }?.0
        }
        if let hit = find(asked[0], asked.count == 2 ? asked[1] : nil) {
            return hit
        }
        // Siri hears "four one" as 41: with no channel 41, try 4.1.
        if asked.count == 1, asked[0] >= 10 {
            return find(asked[0] / 10, asked[0] % 10)
        }
        return nil
    }

    /// The team in "the Chiefs game": a leading "the" and a trailing "game" go.
    public static func team(_ spoken: String) -> String {
        var words = tokens(spoken)
        if words.first == "the" {
            words.removeFirst()
        }
        if words.last == "game" {
            words.removeLast()
        }
        return words.joined(separator: " ")
    }

    /// A team's game on now, else its next one in the next 12 hours. The team's words
    /// must appear whole in the title or subtitle: "Kings" is not the Vikings.
    public static func game(_ spoken: String, airings: [Airing], channels: [Channel], now: Date) -> Airing? {
        let name = tokens(team(spoken))
        guard name.joined().count >= 3 else { return nil }
        let shown = Set(channels.filter { $0.enabled && !$0.hidden }.map(\.id))
        let games = airings.filter { airing in
            airing.kind == .sports && shown.contains(airing.channelId) && airing.end > now
                && airing.start < now.addingTimeInterval(12 * 3600)
                && (has(tokens(airing.title), name) || has(tokens(airing.subtitle ?? ""), name))
        }
        return games.first { $0.isOn(at: now) } ?? games.min { $0.start < $1.start }
    }

    /// The show to record, airing soonest: a title that is the words (punctuation aside),
    /// else one that has them. `exact` is false for the second kind, which is worth a confirmation.
    public static func show(_ name: String, in airings: [Airing], now: Date) -> (airing: Airing, exact: Bool)? {
        let words = tokens(name)
        guard !words.isEmpty else { return nil }
        let ahead = airings.filter { $0.end > now }.sorted { $0.start < $1.start }
        if let hit = ahead.first(where: { tokens($0.title) == words }) {
            return (hit, true)
        }
        return ahead.first { has(tokens($0.title), words) }.map { ($0, false) }
    }

    /// Lowercased words, punctuation dropped: "Jeopardy!" is "jeopardy".
    public static func tokens(_ text: String) -> [String] {
        text.lowercased().split(whereSeparator: { !$0.isLetter && !$0.isNumber }).map(String.init)
    }

    private static func has(_ words: [String], _ part: [String]) -> Bool {
        guard !part.isEmpty, words.count >= part.count else { return false }
        return (0 ... words.count - part.count).contains { Array(words[$0 ..< $0 + part.count]) == part }
    }

    /// What Siri says for "What's on": favorites, or every channel when no favorite has a listing now.
    public static func onNow(_ snap: TopShelf.Snapshot, now: Date, limit: Int = 4) -> String {
        var rows = WidgetFeed.onNow(snap, now: now, limit: limit).filter { $0.start != nil }
        if rows.isEmpty {
            var everyone = snap
            everyone.channels = snap.channels.map { channel in
                var channel = channel
                channel.favorite = false
                return channel
            }
            rows = WidgetFeed.onNow(everyone, now: now, limit: limit).filter { $0.start != nil }
        }
        guard !rows.isEmpty else { return "Nothing in the guide right now." }
        return rows.map { "\($0.number), \($0.title)" }.joined(separator: ". ") + "."
    }
}
