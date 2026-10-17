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
                let show = airing.title.trimmingCharacters(in: .whitespacesAndNewlines)
                let name = rec.title.trimmingCharacters(in: .whitespacesAndNewlines)
                guard rec.channelId == airing.channelId, !rec.isMissing, name.caseInsensitiveCompare(show) == .orderedSame else {
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
            return String(localized: "Season \(String(season)), episode \(String(episode))", bundle: .module)
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

    private static var waiting: String {
        String(localized: "Waiting", bundle: .module)
    }

    private static func text(_ value: String?) -> String {
        let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return trimmed.isEmpty ? waiting : trimmed
    }

    private static func rate(_ value: String?) -> String {
        let raw = text(value)
        guard raw != waiting else { return raw }
        if raw.hasSuffix("M") {
            let number = String(raw.dropLast())
            return String(localized: "\(number) Mb/s", bundle: .module)
        }
        if raw.hasSuffix("k") {
            let number = String(raw.dropLast())
            return String(localized: "\(number) kb/s", bundle: .module)
        }
        return raw
    }

    private static func drift(_ ms: Double?, state: String?) -> String {
        guard let state, !state.isEmpty, state != "off" else { return String(localized: "Off", bundle: .module) }
        let word = state.prefix(1).uppercased() + state.dropFirst()
        let rounded = Int((ms ?? 0).rounded())
        return String(localized: "\(word) · \(String(rounded)) ms", bundle: .module)
    }
}

/// Where a live picture sits in what the server still holds: its oldest frame,
/// the frame on screen, and the live edge, as station clock times. The player's
/// timeline draws from this; LIVE is the right end.
public struct LiveWindow: Equatable, Sendable {
    public var start: Date
    public var position: Date
    public var edge: Date

    /// A player in a room sits about 13-18 s behind the edge (balanced or stable).
    /// Within this it reads as live.
    public static let liveSlack: TimeInterval = 25

    public init(start: Date, position: Date, edge: Date) {
        self.start = min(start, position)
        self.position = position
        self.edge = max(edge, position)
    }

    /// From the item's clock: its date now, its time in seconds, and its seekable ranges in seconds.
    public static func make(currentDate: Date?, currentSeconds: Double, seekable: [ClosedRange<Double>]) -> LiveWindow? {
        guard let currentDate, currentSeconds.isFinite, let first = seekable.map(\.lowerBound).min(),
              let last = seekable.map(\.upperBound).max(), first.isFinite, last.isFinite
        else { return nil }
        return LiveWindow(
            start: currentDate.addingTimeInterval(first - currentSeconds),
            position: currentDate,
            edge: currentDate.addingTimeInterval(last - currentSeconds)
        )
    }

    public var span: TimeInterval {
        edge.timeIntervalSince(start)
    }

    /// 0 at the oldest frame, 1 at the live edge.
    public var fraction: Double {
        span > 0 ? min(1, max(0, position.timeIntervalSince(start) / span)) : 1
    }

    public var behind: TimeInterval {
        edge.timeIntervalSince(position)
    }

    public var isLive: Bool {
        behind <= Self.liveSlack
    }

    /// The time a point on the bar stands for.
    public func date(at fraction: Double) -> Date {
        start.addingTimeInterval(span * min(1, max(0, fraction)))
    }

    /// Back or forward by `seconds`, never past either end.
    public func moved(by seconds: TimeInterval) -> Date {
        min(edge, max(start, position.addingTimeInterval(seconds)))
    }

    /// Whether the window holds a moment, such as a show's start.
    public func holds(_ date: Date) -> Bool {
        date >= start && date <= edge
    }

    /// "−4:05" or "−1:02:09": how far behind live, as the TV app writes it.
    public static func behindText(_ seconds: TimeInterval) -> String {
        let total = max(0, Int(seconds.rounded()))
        let h = total / 3600, m = (total % 3600) / 60, s = total % 60
        return h > 0 ? String(format: "\u{2212}%d:%02d:%02d", h, m, s) : String(format: "\u{2212}%d:%02d", m, s)
    }
}
