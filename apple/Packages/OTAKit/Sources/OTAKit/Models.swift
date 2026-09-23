import Foundation

// Mirrors of the schemas in api/openapi.yaml.

public struct ServerInfo: Codable, Sendable, Hashable {
    public var id: String
    public var name: String
    public var version: String
    public var apiVersion: Int
    public var encoder: String?
    public var tunerCount: Int?
    public var features: [String]
}

public struct Channel: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var deviceId: String
    public var guideNumber: String
    public var guideName: String
    public var displayNumber: String
    public var displayName: String
    public var videoCodec: String?
    public var audioCodec: String?
    public var hd: Bool
    public var favorite: Bool
    public var enabled: Bool
    public var hidden: Bool
    public var present: Bool
    public var artUrl: String?
}

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

public struct Airing: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var channelId: Int64
    public var title: String
    public var subtitle: String?
    public var description: String?
    public var category: String?
    public var programId: String?
    public var new: Bool?
    public var imageUrl: String?
    public var season: Int?
    public var episode: Int?
    public var episodeLabel: String?
    public var originalAir: String?
    public var seriesId: String?
    public var live: Bool?
    public var premiere: Bool?
    public var finale: Bool?
    public var rating: String?
    public var cast: String?
    public var guideNumber: String?
    public var channelName: String?
    public var start: Date
    public var end: Date

    public func isOn(at date: Date) -> Bool {
        start <= date && end > date
    }

    public func progress(at date: Date) -> Double {
        let span = end.timeIntervalSince(start)
        guard span > 0 else { return 0 }
        return min(1, max(0, date.timeIntervalSince(start) / span))
    }
}

public struct Recording: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var channelId: Int64
    public var guideNumber: String
    public var title: String
    public var subtitle: String?
    public var description: String?
    public var category: String?
    public var status: String
    public var startedAt: Date
    public var endsAt: Date?
    public var position: Double?
    public var durationSec: Double?
    public var watched: Int?

    public var isRecording: Bool {
        status == "recording"
    }
}

public struct Pass: Codable, Sendable, Hashable, Identifiable {
    public var id: Int64
    public var title: String
    public var channelId: Int64
}

public struct Caps: Codable, Sendable, Hashable {
    public var platform: String
    public var video: [String]
    public var audio: [String]
    public var maxHeight: Int?
    public var network: String?

    public init(platform: String, video: [String], audio: [String], maxHeight: Int? = nil, network: String? = nil) {
        self.platform = platform
        self.video = video
        self.audio = audio
        self.maxHeight = maxHeight
        self.network = network
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

public struct StreamInfo: Codable, Sendable, Hashable {
    public var rendition: String
    public var video: String
    public var audio: String
    public var mode: String?
    public var reason: String
    public var sourceVideo: String?
    public var sourceAudio: String?
    public var encoder: String?
}

public struct WatchSession: Codable, Sendable, Hashable {
    public var channelId: Int64
    public var playlist: String
    public var rendition: String
    public var stream: StreamInfo
    public var shared: Bool
    public var viewers: Int
}

public struct MultiviewPlan: Codable, Sendable, Hashable {
    public struct Playable: Codable, Sendable, Hashable {
        public var channelId: Int64
        public var frequencyHz: Int
        public var shared: Bool
    }

    public struct Blocked: Codable, Sendable, Hashable {
        public var channelId: Int64
        public var reason: String
        public var holders: [String]
    }

    public var playable: [Playable]
    public var blocked: [Blocked]
    public var tunersNeeded: Int
    public var tunersFree: Int
    public var note: String?
}

public struct PlaybackStart: Codable, Sendable, Hashable {
    public var playlist: String
    public var position: Double
    public var growing: Bool
}

public struct RoomState: Codable, Sendable, Hashable {
    public var room: String
    public var channelId: Int64?
    public var mode: String
    public var anchorServer: Double
    public var anchorMedia: Double
    public var rate: Double
    public var latency: String
    public var version: Int
    public var members: Int

    /// Media time (Unix ms of the frame on screen) the room shows at a server time (Unix ms).
    public func target(atServer now: Double) -> Double {
        anchorMedia + (now - anchorServer) * rate
    }
}

public struct APIErrorBody: Codable, Sendable {
    public var code: String
    public var message: String
}
