import Foundation

/// Which multiview tile asks for a picture first. A server that cannot play
/// every tile gives the picture to whoever asks first, so the tile with the
/// sound asks before the quiet ones. A tile that had no watch when the server
/// came back asks after the ones that did.
public enum PictureOrder {
    /// A quiet tile stops waiting if the sound tile's watch has not answered.
    public static let soundWait: TimeInterval = 8
    /// A tile with no watch asks this long after the tiles that had one.
    public static let lateAsk: TimeInterval = 3

    public struct Round: Equatable, Sendable {
        public var soundID: Int64
        /// The sound tile's watch has come back, or the wait was given up.
        public var answered: Bool
        /// When this round began. The wait is measured from here.
        public var since: Date

        public init(soundID: Int64, answered: Bool = false, since: Date) {
            self.soundID = soundID
            self.answered = answered
            self.since = since
        }
    }

    public static func begin(_ soundID: Int64, now: Date) -> Round {
        Round(soundID: soundID, since: now)
    }

    /// A quiet tile holds its start. The sound tile never does. No sound tile
    /// yet means nothing is held, so a grid does not wait forever.
    public static func holdQuiet(_ round: Round, channelID: Int64, now: Date) -> Bool {
        if round.soundID == 0 || channelID == round.soundID || round.answered {
            return false
        }
        return now.timeIntervalSince(round.since) < soundWait
    }

    /// The socket dropped or the server restarted. Nobody has answered in this round.
    public static func beginAgain(_ round: Round, now: Date) -> Round {
        Round(soundID: round.soundID, answered: false, since: now)
    }

    /// The sound moved to another tile. That tile asks first, and the wait starts over.
    public static func soundChanged(_ round: Round, soundID: Int64, now: Date) -> Round {
        if round.soundID == soundID {
            return round
        }
        return Round(soundID: soundID, answered: false, since: now)
    }

    /// The sound tile's watch answered: a picture, or a reason it cannot start.
    /// An answer from any other tile leaves the round alone.
    public static func soundDidAnswer(_ round: Round, channelID: Int64) -> Round {
        guard channelID == round.soundID, channelID != 0 else { return round }
        var next = round
        next.answered = true
        return next
    }

    /// How long a tile waits after it is allowed to start. One that already had
    /// a watch asks at once. One that did not, after the server came back, waits
    /// so it does not take a picture from those.
    public static func askDelay(hadWatch: Bool, serverCameBack: Bool) -> TimeInterval {
        if hadWatch || !serverCameBack {
            return 0
        }
        return lateAsk
    }
}
