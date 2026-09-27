import Foundation

/// What the player says when live TV cannot keep going, and when a stall is
/// long enough to ask. A short stall is the player's to ride out. The words
/// match the web player.
public enum PlaybackOutage {
    public static let serverStopped = "The server stopped. It will try again when it's back."
    public static let pictureStopped = "The picture stopped. Trying again usually fixes it."
    /// A stall is named only after this long, and only when the server is gone.
    public static let stallSeconds: TimeInterval = 8
}

/// Clock for one playback. A stall starts when the picture stops advancing.
/// Eight seconds later the caller asks whether the server is still up.
public struct ServerOutage: Equatable, Sendable {
    public enum Phase: Equatable, Sendable {
        case playing
        case waiting(since: Date)
        case surfaced
    }

    public private(set) var phase: Phase = .playing

    public init() {}

    public mutating func notePlaying() {
        if case .surfaced = phase {
            return
        }
        phase = .playing
    }

    /// The first stalled sample starts the clock. Later ones leave it alone.
    public mutating func noteWaiting(at now: Date) {
        if case .playing = phase {
            phase = .waiting(since: now)
        }
    }

    public func shouldProbe(at now: Date, fatal: Bool) -> Bool {
        if case .surfaced = phase {
            return false
        }
        if fatal {
            return true
        }
        if case let .waiting(since) = phase {
            return now.timeIntervalSince(since) >= PlaybackOutage.stallSeconds
        }
        return false
    }

    /// `health` is true when the server answered. A stall while it is up stays
    /// with the player, and the clock starts over. A dead server, or a fatal
    /// error while the server is up, is named once.
    public mutating func resolve(at now: Date, health: Bool, fatal: Bool) -> String? {
        if case .surfaced = phase {
            return nil
        }
        if !health {
            phase = .surfaced
            return PlaybackOutage.serverStopped
        }
        if fatal {
            phase = .surfaced
            return PlaybackOutage.pictureStopped
        }
        phase = .waiting(since: now)
        return nil
    }

    /// A start that never reached the server. Once, until the clock is reset.
    public mutating func surface(_ message: String) -> String? {
        if case .surfaced = phase {
            return nil
        }
        phase = .surfaced
        return message
    }
}
