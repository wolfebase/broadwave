import Foundation

/// Each tile joins its own room. A shared multiview room steps every tile
/// back when any of them stalls, so a dead stream pauses the pictures that
/// are fine. The server accepts a multiview room of at most 64 characters.
public enum MultiviewRooms {
    public static let limit = 64

    /// `base` is `multiview:<id>`. The channel keeps this tile off the others.
    public static func tile(_ base: String, channelID: Int64) -> String {
        "\(base):\(channelID)"
    }
}

/// Pause and play go to every tile. Tiles do not share a room, so the server
/// will not deliver one command to the rest. A later bind for the same channel
/// replaces that tile's sender.
@MainActor
public final class TileCommands {
    private var sends: [Int64: (String) -> Void] = [:]

    public init() {}

    public func bind(_ channelID: Int64, _ send: @escaping (String) -> Void, whenPaused: Bool = false) {
        sends[channelID] = send
        if whenPaused {
            send("pause")
        }
    }

    public func unbind(_ channelID: Int64) {
        sends[channelID] = nil
    }

    public func send(_ action: String) {
        // Copy first. A sender that binds or unbinds must not mutate this map
        // while it is being walked.
        for send in Array(sends.values) {
            send(action)
        }
    }
}

/// A tile whose player refused the first start stays at rate 0 with no program
/// date. The sync engine will not call play until that date exists, so the tile
/// asks again. A viewer's pause, a failed item, and a player already waiting on
/// its buffer are not this.
public struct TilePlaybackSnap: Equatable, Sendable {
    public var syncWaiting = false
    public var rate = 0.0
    public var itemFailed = false
    public var hasItem = false
    public var waitingToPlay = false
    public var viewerPaused = false
    public var stuckFor: TimeInterval = 0
    /// Seconds of media loaded past the playhead, and whether this item ever had any.
    public var buffered = 0.0
    public var primed = false
    /// The item posted failedToPlayToEndTime. It holds media and never plays it.
    public var ended = false
    public var deadFor: TimeInterval = 0
    public var reloads = 0

    public init() {}
}

public enum TilePlayback {
    public static let replayAfter: TimeInterval = 3

    /// Rate is still 0 and the engine has nothing to follow. The clock starts here.
    public static func isStuck(_ snap: TilePlaybackSnap) -> Bool {
        snap.hasItem && snap.syncWaiting && snap.rate == 0 && !snap.itemFailed && !snap.waitingToPlay && !snap.viewerPaused
    }

    public static func shouldReplay(_ snap: TilePlaybackSnap) -> Bool {
        isStuck(snap) && snap.stuckFor >= replayAfter
    }

    public static let reloadAfter: TimeInterval = 5
    public static let endedAfter: TimeInterval = 1
    public static let maxReloads = 3

    /// AVPlayer gave up on the item (a playlist it refused, often inside the
    /// join hold), or it once had media, has none, and the engine has nothing
    /// to follow. Play alone never opens it again, and a tile that never moved
    /// never reaches the outage clock, which waits for a picture that played.
    /// The clock starts here.
    public static func isDead(_ snap: TilePlaybackSnap) -> Bool {
        guard snap.hasItem, !snap.itemFailed, !snap.viewerPaused else { return false }
        return snap.ended || snap.syncWaiting && snap.primed && snap.buffered < 0.5
    }

    /// A new item opens at the live edge, and the engine places it.
    public static func shouldReload(_ snap: TilePlaybackSnap) -> Bool {
        isDead(snap) && snap.deadFor >= wait(snap) && snap.reloads < maxReloads
    }

    public static func wait(_ snap: TilePlaybackSnap) -> TimeInterval {
        snap.ended ? endedAfter : reloadAfter
    }

    /// What a tile says once its picture has died. The same decision as the
    /// one-channel player: a lost signal keeps the no-signal sentence and waits
    /// for the antenna, and a picture with no named cause stays on the quiet retry.
    public static func outage(_ snap: RecoverySnap) -> OutageDecision {
        PlaybackOutage.classify(snap)
    }
}
