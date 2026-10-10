import Foundation

/// Which game a multiview should listen to. Matches the web `pickFocus`
/// (`web/src/features/multiview/switcher.ts`) and `sports.PickFocus`.
public enum GameSwitch {
    /// A viewer's sound choice holds the ranking off for this long.
    public static let hold: TimeInterval = 120
    /// The web asks the scoreboard this often while Auto is on.
    public static let poll: TimeInterval = 20

    public struct Team: Equatable, Sendable, Codable {
        public var name: String
        public var short: String?
        public var abbr: String?
        public var score: String?
        public var home: Bool?

        public init(name: String, short: String? = nil, abbr: String? = nil, score: String? = nil, home: Bool? = nil) {
            self.name = name
            self.short = short
            self.abbr = abbr
            self.score = score
            self.home = home
        }
    }

    public struct Game: Equatable, Sendable, Codable {
        public var id: String
        public var league: String?
        public var name: String?
        public var shortName: String?
        public var state: String?
        public var detail: String?
        public var clock: String?
        public var period: Int?
        public var redZone: Bool?
        public var powerPlay: Bool?
        public var teams: [Team]?

        public init(
            id: String,
            league: String? = nil,
            name: String? = nil,
            shortName: String? = nil,
            state: String? = nil,
            detail: String? = nil,
            clock: String? = nil,
            period: Int? = nil,
            redZone: Bool? = nil,
            powerPlay: Bool? = nil,
            teams: [Team]? = nil
        ) {
            self.id = id
            self.league = league
            self.name = name
            self.shortName = shortName
            self.state = state
            self.detail = detail
            self.clock = clock
            self.period = period
            self.redZone = redZone
            self.powerPlay = powerPlay
            self.teams = teams
        }

        public init(_ game: BroadwaveKit.Game) {
            self.init(
                id: game.id,
                league: game.league,
                name: game.name,
                shortName: game.shortName,
                state: game.state,
                detail: game.detail,
                clock: game.clock,
                period: game.period,
                redZone: game.redZone,
                powerPlay: game.powerPlay,
                teams: game.teams?.map { Team(name: $0.name, short: $0.short, abbr: $0.abbr, score: $0.score, home: $0.home) }
            )
        }
    }

    public struct Choice: Equatable, Sendable {
        public var keepManual: Bool
        public var gameId: String
        public var banner: String

        public init(keepManual: Bool, gameId: String, banner: String) {
            self.keepManual = keepManual
            self.gameId = gameId
            self.banner = banner
        }
    }

    /// Auto polls only while it is on and a tile is showing a game. A grid of
    /// ordinary shows never asks the scoreboard.
    public static func shouldPoll(auto: Bool, gameIDs: [String]) -> Bool {
        auto && gameIDs.contains { !$0.isEmpty }
    }

    /// `manualAt` is when the viewer last chose the sound. The distant past, or
    /// any instant before 1970, means nobody has.
    public static func pickFocus(now: Date, manualAt: Date, games: [Game], previous: [Game]) -> Choice {
        if manualAt.timeIntervalSince1970 > 0, now >= manualAt, now.timeIntervalSince(manualAt) < hold {
            return Choice(keepManual: true, gameId: "", banner: "")
        }
        let prev = Dictionary(previous.map { ($0.id, $0) }, uniquingKeysWith: { _, latest in latest })
        var best: Rank = .none
        var chosen: Game?
        var why = ""
        for game in games {
            guard actionable(game) else { continue }
            let earlier = prev[game.id]
            let (label, rank) = classify(game, earlier, earlier != nil)
            if rank > best {
                best = rank
                chosen = game
                why = label
            }
        }
        guard let chosen, best != .none else {
            return Choice(keepManual: false, gameId: "", banner: "")
        }
        return Choice(keepManual: false, gameId: chosen.id, banner: banner(why, chosen))
    }

    private enum Rank: Int, Comparable {
        case none = 0
        case close = 1
        case lead = 2
        case situation = 3

        static func < (lhs: Rank, rhs: Rank) -> Bool {
            lhs.rawValue < rhs.rawValue
        }
    }

    private static let sport: [String: String] = [
        "nfl": "football", "ncaaf": "football",
        "nba": "basketball", "wnba": "basketball", "ncaab": "basketball",
        "mlb": "baseball",
        "nhl": "hockey",
        "mls": "soccer", "nwsl": "soccer", "epl": "soccer",
    ]

    private static func sportOf(_ league: String) -> String {
        sport[league] ?? ""
    }

    private static func actionable(_ game: Game) -> Bool {
        game.state == "in" && !betweenPeriods(game.detail ?? "")
    }

    private static func classify(_ game: Game, _ prev: Game?, _ hasPrev: Bool) -> (String, Rank) {
        if game.redZone == true, sportOf(game.league ?? "") == "football" {
            return ("Red zone", .situation)
        }
        if game.powerPlay == true, sportOf(game.league ?? "") == "hockey" {
            return ("Power play", .situation)
        }
        if hasPrev, let prev, leadChanged(game, prev) {
            return ("Lead change", .lead)
        }
        if closeFinish(game) {
            return ("Final minutes", .close)
        }
        return ("", .none)
    }

    private static func banner(_ why: String, _ game: Game) -> String {
        let who = matchup(game)
        return who.isEmpty ? why : "\(why): \(who)"
    }

    private static func matchup(_ game: Game) -> String {
        let teams = game.teams ?? []
        let home = teams.first { $0.home == true }
        let away = home.flatMap { home in teams.first { $0.abbr != home.abbr || $0.name != home.name } }
        if let home, let away {
            return "\(label(away)) at \(label(home))"
        }
        if let short = game.shortName, !short.isEmpty {
            return short.replacingOccurrences(of: " @ ", with: " at ")
        }
        return game.name ?? ""
    }

    private static func label(_ team: Team) -> String {
        if let abbr = team.abbr, !abbr.isEmpty {
            return abbr
        }
        if let short = team.short, !short.isEmpty {
            return short
        }
        return team.name
    }

    private static func leadChanged(_ cur: Game, _ prev: Game) -> Bool {
        let (prevLead, prevOK) = leaderKey(prev)
        let (curLead, curOK) = leaderKey(cur)
        return prevOK && curOK && !prevLead.isEmpty && !curLead.isEmpty && prevLead != curLead
    }

    private static func leaderKey(_ game: Game) -> (String, Bool) {
        let teams = game.teams ?? []
        guard teams.count >= 2 else { return ("", false) }
        var best = 0
        var key = ""
        var tied = false
        var seen = 0
        for team in teams {
            guard let n = intScore(team.score) else { return ("", false) }
            if seen == 0 || n > best {
                best = n
                key = label(team)
                tied = false
            } else if n == best {
                tied = true
            }
            seen += 1
        }
        if seen < 2 {
            return ("", false)
        }
        if tied {
            return ("", true)
        }
        return (key, true)
    }

    private static func closeFinish(_ game: Game) -> Bool {
        guard let margin = closeMargin(game.league ?? ""), inFinalMinutes(game) else { return false }
        guard let gap = scoreGap(game) else { return false }
        return gap <= margin
    }

    private static func closeMargin(_ league: String) -> Int? {
        switch sportOf(league) {
        case "football", "basketball": 8
        case "hockey", "soccer": 1
        default: nil
        }
    }

    private static func scoreGap(_ game: Game) -> Int? {
        let teams = game.teams ?? []
        guard teams.count >= 2 else { return nil }
        var minScore = 0
        var maxScore = 0
        for (index, team) in teams.enumerated() {
            guard let n = intScore(team.score) else { return nil }
            if index == 0 || n < minScore {
                minScore = n
            }
            if index == 0 || n > maxScore {
                maxScore = n
            }
        }
        return maxScore - minScore
    }

    private static func intScore(_ raw: String?) -> Int? {
        guard let raw else { return nil }
        let text = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard text.range(of: #"^[+-]?\d+$"#, options: .regularExpression) != nil else { return nil }
        return Int(text)
    }

    private static func inFinalMinutes(_ game: Game) -> Bool {
        guard game.state == "in", !betweenPeriods(game.detail ?? "") else { return false }
        if soccerClockLate(game) {
            return true
        }
        let need = finalPeriod(game.league ?? "")
        guard need > 0 else { return false }
        let late = (game.period ?? 0) >= need || detailIsLastPeriod(game)
        guard late else { return false }
        guard let left = remainingClock(game) else { return false }
        return left < 300
    }

    private static func finalPeriod(_ league: String) -> Int {
        if league == "ncaab" {
            return 2
        }
        switch sportOf(league) {
        case "hockey": return 3
        case "soccer": return 2
        case "football", "basketball": return 4
        default: return 0
        }
    }

    private static func betweenPeriods(_ detail: String) -> Bool {
        let text = detail.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if text.isEmpty {
            return false
        }
        if text == "ht" || text == "halftime" || text == "half time" {
            return true
        }
        return text.contains("halftime") || text.contains("half time") || text.contains("end of")
            || text.contains("intermission") || text.contains("delay") || text.contains("suspended")
    }

    private static func detailIsLastPeriod(_ game: Game) -> Bool {
        let text = game.detail ?? ""
        let sport = sportOf(game.league ?? "")
        if sport == "hockey" {
            return matches(#"\b(?:3rd|third|ot|overtime)\b"#, text)
        }
        // College basketball is two halves. It is checked before the quarter sports.
        if game.league == "ncaab" || sport == "soccer" {
            return matches(#"\b(?:2nd half|second half|2h)\b"#, text) || matches(#"\b(?:ot|overtime|extra time|et)\b"#, text)
        }
        if sport == "football" || sport == "basketball" {
            return matches(#"\b(?:4th|fourth|ot|overtime)\b"#, text)
        }
        return false
    }

    private static func soccerClockLate(_ game: Game) -> Bool {
        guard sportOf(game.league ?? "") == "soccer" else { return false }
        let text = "\(game.detail ?? "") \(game.clock ?? "")"
        if matches(#"\b(?:extra time|stoppage|penalties|aet|et)\b"#, text) {
            return true
        }
        for src in [game.clock ?? "", game.detail ?? ""] {
            if let minute = soccerMinute(src), minute >= 85 {
                return true
            }
        }
        return false
    }

    private static func remainingClock(_ game: Game) -> Int? {
        clockSeconds(game.clock ?? "") ?? clockSeconds(game.detail ?? "")
    }

    private static func clockSeconds(_ raw: String) -> Int? {
        if let direct = parseClock(raw.trimmingCharacters(in: .whitespacesAndNewlines)) {
            return direct
        }
        guard let found = firstMatch(#"(?:^|[^0-9])(\d{1,2}):(\d{2})(?:[^0-9]|$)"#, raw), found.count == 2 else { return nil }
        return parseClock("\(found[0]):\(found[1])")
    }

    private static func parseClock(_ raw: String) -> Int? {
        let parts = raw.split(separator: ":", omittingEmptySubsequences: false)
        guard parts.count == 2 else { return nil }
        var secText = String(parts[1])
        if let cut = secText.firstIndex(where: { $0 == "." || $0 == " " }) {
            secText = String(secText[..<cut])
        }
        let minText = parts[0].trimmingCharacters(in: .whitespacesAndNewlines)
        guard minText.allSatisfy(\.isNumber), secText.allSatisfy(\.isNumber) else { return nil }
        guard let min = Int(minText), let sec = Int(secText), min >= 0, sec >= 0, sec <= 59 else { return nil }
        return min * 60 + sec
    }

    private static func soccerMinute(_ src: String) -> Int? {
        guard let found = firstMatch(#"(\d{1,3})(?:\s*\+\s*\d{1,2})?\s*'"#, src), let minute = found.first else { return nil }
        return Int(minute)
    }

    private static func matches(_ pattern: String, _ text: String) -> Bool {
        firstMatch(pattern, text) != nil
    }

    private static func firstMatch(_ pattern: String, _ text: String) -> [String]? {
        guard let re = try? NSRegularExpression(pattern: pattern, options: [.caseInsensitive]) else { return nil }
        let range = NSRange(text.startIndex..., in: text)
        guard let match = re.firstMatch(in: text, range: range) else { return nil }
        var groups: [String] = []
        for index in 1 ..< match.numberOfRanges {
            guard let slice = Range(match.range(at: index), in: text) else { return nil }
            groups.append(String(text[slice]))
        }
        return groups
    }
}
