import Foundation

/// Up on the remote, or a swipe up, moves to the previous channel.
/// Down moves to the next. The channel list order is the order on screen.
public enum ChannelStep {
    public static func offset(up: Bool) -> Int {
        up ? -1 : 1
    }
}

/// Where Start over can find the beginning of the show on now.
public enum StartOver: Equatable, Sendable {
    /// The picture's seekable window still includes the show's start.
    case live
    /// A recording of this showing began at or before the start.
    case recording

    /// Nothing when the show's start is unknown, the window begins later, or the
    /// recording started after the show. The live picture wins when both hold it.
    public static func choice(showStart: Date?, seekableFrom: Date?, recordingStarted: Date?) -> StartOver? {
        guard let showStart else { return nil }
        if let seekableFrom, seekableFrom <= showStart {
            return .live
        }
        if let recordingStarted, recordingStarted <= showStart {
            return .recording
        }
        return nil
    }
}

/// A recording holds this showing's start when the file begins at or before it
/// and the take is this showing, not an earlier one with the same name.
/// The longest ring is 4 hours, so a file that started further back is another showing.
public enum ShowRecording {
    public static func holdingStart(of airing: Airing, in recordings: [Recording]) -> Recording? {
        let earliest = airing.start.addingTimeInterval(-4 * 60 * 60)
        return recordings
            .filter { rec in
                guard rec.channelId == airing.channelId, rec.title.caseInsensitiveCompare(airing.title) == .orderedSame else {
                    return false
                }
                if let ended = rec.endedAt, ended < airing.start {
                    return false
                }
                if let recID = rec.programId, !recID.isEmpty, let showID = airing.programId, !showID.isEmpty, recID != showID {
                    return false
                }
                return rec.startedAt <= airing.start && rec.startedAt >= earliest
            }
            .min { $0.startedAt < $1.startedAt }
    }
}

/// The episode line under a title. A guide label wins, then a season and episode, then the subtitle.
public enum ProgramLine {
    public static func episode(label: String?, season: Int?, episode: Int?, subtitle: String?) -> String? {
        if let label = text(label) {
            return label
        }
        if let season, let episode, season > 0, episode > 0 {
            return "Season \(season), episode \(episode)"
        }
        return text(subtitle)
    }

    private static func text(_ value: String?) -> String? {
        let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return trimmed.isEmpty ? nil : trimmed
    }
}

/// The earliest time the player can jump to, from the frame on screen and the seekable window.
public enum SeekWindow {
    public static func start(current: Date, currentSeconds: Double, earliestSeconds: Double) -> Date? {
        guard currentSeconds.isFinite, earliestSeconds.isFinite else { return nil }
        return current.addingTimeInterval(earliestSeconds - currentSeconds)
    }
}

/// What the Stream panel says. A missing piece stays "Waiting" so a row is never blank.
public struct StreamFacts: Equatable, Sendable {
    public var rendition: String
    public var encoder: String
    public var bitrate: String
    public var dropped: String
    public var drift: String

    public struct Sync: Equatable, Sendable {
        public var milliseconds: Double?
        public var state: String?

        public init(milliseconds: Double? = nil, state: String? = nil) {
            self.milliseconds = milliseconds
            self.state = state
        }
    }

    public static func make(rendition: String?, encoder: String?, bitrate: String?, dropped: Int, sync: Sync) -> StreamFacts {
        StreamFacts(
            rendition: text(rendition),
            encoder: text(encoder),
            bitrate: rate(bitrate),
            dropped: "\(max(0, dropped))",
            drift: drift(sync.milliseconds, state: sync.state)
        )
    }

    private static func text(_ value: String?) -> String {
        let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return trimmed.isEmpty ? "Waiting" : trimmed
    }

    private static func rate(_ value: String?) -> String {
        let raw = text(value)
        guard raw != "Waiting" else { return raw }
        if raw.hasSuffix("M") {
            return String(raw.dropLast()) + " Mb/s"
        }
        if raw.hasSuffix("k") {
            return String(raw.dropLast()) + " kb/s"
        }
        return raw
    }

    private static func drift(_ ms: Double?, state: String?) -> String {
        guard let state, !state.isEmpty, state != "off" else { return "Off" }
        let word = state.prefix(1).uppercased() + state.dropFirst()
        return "\(word) · \(Int((ms ?? 0).rounded())) ms"
    }
}
