import Foundation

// Behavior on top of the generated API types. Prefs stays here because the
// apps use its enums directly.

public struct SearchResult: Codable, Sendable {
    public var query: String?
    public var airings: [Airing]
    public var recordings: [Recording]

    public init(query: String? = nil, airings: [Airing] = [], recordings: [Recording] = []) {
        self.query = query
        self.airings = airings
        self.recordings = recordings
    }
}

public struct ScoreTeam: Codable, Sendable, Hashable {
    public var name: String
    public var short: String?
    public var abbr: String?
    public var score: String?
    public var home: Bool?
}

public struct ScoreGame: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var state: String?
    public var teams: [ScoreTeam]?

    public var line: String? {
        guard state != "pre", let teams else { return nil }
        guard let home = teams.first(where: { $0.home == true }),
              let away = teams.first(where: { $0.home != true }),
              let homeScore = home.score, !homeScore.isEmpty,
              let awayScore = away.score, !awayScore.isEmpty else { return nil }
        let awayName = away.abbr ?? away.short ?? away.name
        let homeName = home.abbr ?? home.short ?? home.name
        return "\(awayName) \(awayScore) · \(homeName) \(homeScore)"
    }
}

public struct Prefs: Codable, Sendable, Hashable {
    public enum Quality: String, Codable, Sendable, CaseIterable {
        case auto, original, high, medium, saver, tile, focus
        case tile360 = "360"
    }

    public enum Sound: String, Codable, Sendable, CaseIterable { case auto, surround, stereo, none }

    public var quality: Quality
    public var audio: Sound
    public var picture: String?
    /// main, language, or described. Empty plays the main mix.
    public var track: String?
    /// Levels the volume. Off keeps the original mix.
    public var even: Bool

    public init(quality: Quality = .auto, audio: Sound = .auto, picture: String? = nil, track: String? = nil, even: Bool = false) {
        self.quality = quality
        self.audio = audio
        self.picture = picture
        self.track = track
        self.even = even
    }

    private enum CodingKeys: String, CodingKey { case quality, audio, picture, track, even }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        quality = try c.decodeIfPresent(Quality.self, forKey: .quality) ?? .auto
        audio = try c.decodeIfPresent(Sound.self, forKey: .audio) ?? .auto
        picture = try c.decodeIfPresent(String.self, forKey: .picture)
        track = try c.decodeIfPresent(String.self, forKey: .track)
        even = try c.decodeIfPresent(Bool.self, forKey: .even) ?? false
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(quality, forKey: .quality)
        try c.encode(audio, forKey: .audio)
        try c.encodeIfPresent(picture, forKey: .picture)
        try c.encodeIfPresent(track, forKey: .track)
        if even {
            try c.encode(true, forKey: .even)
        }
    }
}

public struct PlaybackStart: Codable, Sendable, Hashable {
    public var playlist: String
    public var position: Double
    public var growing: Bool
    public var markers: [Marker]?
}

public struct APIErrorBody: Codable, Sendable {
    public var code: String
    public var message: String
}

public extension Airing {
    func isOn(at date: Date) -> Bool {
        start <= date && end > date
    }

    func progress(at date: Date) -> Double {
        let span = end.timeIntervalSince(start)
        guard span > 0 else { return 0 }
        return min(1, max(0, date.timeIntervalSince(start) / span))
    }
}

public extension Pass {
    var isOnce: Bool {
        kind == "once"
    }

    /// The once pass that records exactly this airing. The server keeps its start to the second.
    static func once(in passes: [Pass], for airing: Airing) -> Pass? {
        passes.first { p in
            guard p.isOnce, p.channelId == airing.channelId, let start = p.airingStart else { return false }
            return abs(start.timeIntervalSince(airing.start)) < 1
        }
    }

    /// A series or team pass for the title. A once pass does not count.
    static func series(in passes: [Pass], for airing: Airing) -> Pass? {
        passes.first { !$0.isOnce && $0.title.caseInsensitiveCompare(airing.title) == .orderedSame }
    }

    /// "Title · Mon 11:00 AM only" for a once pass, the title otherwise.
    var label: String {
        guard isOnce, let start = airingStart else { return title }
        return "\(title) · \(start.formatted(.dateTime.weekday(.abbreviated).hour().minute())) only"
    }
}

public extension Recording {
    var isRecording: Bool {
        status == "recording"
    }

    /// Same rule as the web library: a mark wins (1 watched, 2 unwatched),
    /// otherwise the last 15 s or 90% has played.
    var isWatched: Bool {
        if watched == 1 {
            return true
        }
        if watched == 2 {
            return false
        }
        let pos = position ?? 0
        let dur = durationSec ?? 0
        guard dur >= 10, pos >= 1 else { return false }
        return pos >= dur - 15 || pos / dur >= 0.9
    }

    /// Nothing for a finished recording; the others say what happened.
    var statusLabel: String? {
        switch status {
        case "recording": "Recording"
        case "failed": "Failed"
        case "stopped": "Stopped early"
        case "imported": "Imported"
        default: nil
        }
    }

    var isMovie: Bool {
        (category ?? "").lowercased().contains("movie")
    }
}

public struct SchedulePlan: Codable, Sendable, Hashable {
    public var tunerCount: Int
    public var items: [PlannedAiring]
}

public extension PlannedAiring {
    /// One planned airing of one pass.
    var key: String {
        "\(passId)-\(airing.channelId)-\(airing.start.timeIntervalSince1970)"
    }

    /// The tuner's window: the listing plus the pass's early and after minutes.
    var recordWindow: ClosedRange<Date> {
        airing.start.addingTimeInterval(-Double(padBefore) * 60) ... airing.end.addingTimeInterval(Double(padAfter) * 60)
    }

    /// "Will record 7:59 PM–9:02 PM", or why it will not, as the web Schedule reads.
    func statusLine(tuners: Int) -> String {
        if skipped {
            if let reason, !reason.isEmpty {
                return reason
            }
            return tuners == 1 ? "Lower priority · 1 tuner" : "Lower priority · \(tuners) tuners"
        }
        let window = recordWindow
        return "Will record \(window.lowerBound.formatted(date: .omitted, time: .shortened))–\(window.upperBound.formatted(date: .omitted, time: .shortened))"
    }

    func laterLine(_ later: Suggestion) -> String {
        let when = later.start.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day().hour().minute())
        if let number = later.guideNumber, !number.isEmpty {
            return "Later on \(when) on \(number)."
        }
        return "Later on \(when)."
    }
}

/// What a recording does at a commercial break. Kept per device, like the web.
public enum BreakSkip: String, CaseIterable, Sendable {
    case auto
    case button
    case manual

    public static let key = "breakSkip"

    public var label: String {
        switch self {
        case .auto: "Skip them"
        case .button: "Show a Skip button"
        case .manual: "Play them"
        }
    }

    /// The break a player at `time` is inside, if any. The last 50 ms does not
    /// count, so a seek to a break's end is not caught by the same break again.
    public static func marker(in markers: [Marker], at time: Double) -> Marker? {
        markers.first { time >= $0.start && time < $0.end - 0.05 }
    }
}

public extension Marker {
    /// "12:30–13:05", or with hours past the first hour, as the break list reads it.
    var span: String {
        "\(Self.clock(start))–\(Self.clock(end))"
    }

    /// "12:30", or "1:02:05" past the first hour.
    static func clock(_ seconds: Double) -> String {
        let total = Int(max(0, seconds).rounded())
        let (h, m, s) = (total / 3600, total / 60 % 60, total % 60)
        return h > 0 ? String(format: "%d:%02d:%02d", h, m, s) : String(format: "%d:%02d", m, s)
    }
}

/// One recording on a library channel, ready to play. It never uses a tuner.
public struct VirtualPlayback: Codable, Sendable, Hashable {
    public var usesTuner: Bool?
    public var index: Int
    public var count: Int
    public var playlist: String
    public var number: String
    public var name: String
    public var recording: Recording
    public var markers: [Marker]?
}

public extension VirtualChannel {
    /// The first free number from 900, as the web picks it.
    static func nextNumber(after taken: [VirtualChannel]) -> String {
        let used = Set(taken.map(\.number))
        var n = 900
        while used.contains(String(n)) {
            n += 1
        }
        return String(n)
    }
}

public extension RoomState {
    /// Media time (Unix ms of the frame on screen) the room shows at a server time (Unix ms).
    func target(atServer now: Double) -> Double {
        anchorMedia + (now - anchorServer) * rate
    }
}
