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
        case auto, original, high, medium, saver, tile
        case tile360 = "360"
    }

    public enum Sound: String, Codable, Sendable, CaseIterable { case auto, surround, stereo, none }

    public var quality: Quality
    public var audio: Sound
    public var picture: String?

    public init(quality: Quality = .auto, audio: Sound = .auto, picture: String? = nil) {
        self.quality = quality
        self.audio = audio
        self.picture = picture
    }
}

public struct PlaybackStart: Codable, Sendable, Hashable {
    public var playlist: String
    public var position: Double
    public var growing: Bool
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

public extension Recording {
    var isRecording: Bool {
        status == "recording"
    }
}

public extension RoomState {
    /// Media time (Unix ms of the frame on screen) the room shows at a server time (Unix ms).
    func target(atServer now: Double) -> Double {
        anchorMedia + (now - anchorServer) * rate
    }
}
