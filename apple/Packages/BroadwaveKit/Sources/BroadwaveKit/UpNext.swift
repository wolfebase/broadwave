import Foundation

public extension Recording {
    /// Where the show's intro plays, in seconds into the file, when the server found one.
    /// The server leaves out an intro start of 0.
    var intro: ClosedRange<Double>? {
        let start = introStart ?? 0
        guard let introEnd, introEnd > start else { return nil }
        return start ... introEnd
    }

    /// Skip intro is offered inside the intro, but not in its last second, where it would do nothing.
    func showsSkipIntro(at time: Double) -> Bool {
        guard let intro else { return false }
        return time >= intro.lowerBound && time < intro.upperBound - 1
    }

    /// How the Up next card names an episode: "S2 E5", the episode's name, or the show's.
    var upNextLabel: String {
        if let tag = episodeTag {
            return tag
        }
        if let subtitle, !subtitle.isEmpty {
            return subtitle
        }
        return title
    }
}

/// When a recording offers its show's next episode, and when it plays it.
/// Fed the playhead on every tick of the file player; times are seconds into the file.
public struct UpNext: Sendable {
    public enum Step: Equatable, Sendable {
        case none
        /// The card is up. `left` counts down to playing the next episode; nil when autoplay is off.
        case card(left: Int?)
        /// Play the next episode now.
        case play
    }

    /// Seconds the card counts down before the next episode plays.
    public static let countdown = 10.0

    /// The "Play the next episode" setting.
    public var autoplay: Bool
    /// The viewer chose Not now: no card, and nothing plays at the end.
    public private(set) var dismissed = false
    /// The next episode was started once; nothing more happens for this recording.
    public private(set) var done = false
    private var shownAt: Double?

    public init(autoplay: Bool = true) {
        self.autoplay = autoplay
    }

    /// The end titles when the server found them inside the file, else ten seconds before the end.
    public static func cardTime(duration: Double, creditsStart: Double?) -> Double? {
        // A clip of twenty seconds or less is too short to count down over.
        guard duration.isFinite, duration > countdown * 2 else { return nil }
        if let creditsStart, creditsStart > 0, creditsStart < duration {
            return creditsStart
        }
        return max(0, duration - countdown)
    }

    public mutating func observe(_ time: Double, duration: Double, creditsStart: Double?, hasNext: Bool) -> Step {
        guard hasNext, !dismissed, !done, time.isFinite,
              let from = Self.cardTime(duration: duration, creditsStart: creditsStart), time >= from
        else {
            shownAt = nil
            return .none
        }
        guard autoplay else {
            shownAt = nil
            return .card(left: nil)
        }
        let start = shownAt ?? time
        shownAt = start
        let left = Self.countdown - (time - start)
        guard left > 0 else {
            done = true
            return .play
        }
        return .card(left: Int(min(Self.countdown, left).rounded(.up)))
    }

    /// Not now.
    public mutating func dismiss() {
        dismissed = true
        shownAt = nil
    }

    /// The file played to its end: whether the next episode plays.
    public mutating func ended(hasNext: Bool) -> Bool {
        guard hasNext, autoplay, !dismissed, !done else { return false }
        done = true
        return true
    }

    /// Play now.
    public mutating func played() {
        done = true
    }
}

/// Whether a recording's playlist reaches the place to resume. A seek past its end plays from 0.
public enum ResumeReach {
    public static func reached(seekableEnd: Double?, position: Double) -> Bool {
        guard let end = seekableEnd, end.isFinite else { return false }
        return end + 0.25 >= position
    }
}
