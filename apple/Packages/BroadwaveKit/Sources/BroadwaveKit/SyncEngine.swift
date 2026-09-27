import AVFoundation
import Foundation
import Observation
import os
#if canImport(UIKit)
    import UIKit
#endif

/// Holds an AVPlayer on a Whole-Home Sync room's timeline.
///
/// Segments carry EXT-X-PROGRAM-DATE-TIME on the channel's shared timeline, so
/// `currentDate()` is the same wall time for the same frame on every screen.
/// Small drift is trimmed with rate, a screen that is ahead pauses for exactly
/// its drift, and a screen that is behind seeks forward by date.
@MainActor
@Observable
public final class SyncEngine {
    public enum State: String, Sendable { case off, waiting, syncing, locked }

    public private(set) var state: State = .off
    public private(set) var drift: Double = 0
    public private(set) var members = 0

    /// When set, a frame counts only after the layer is showing it.
    /// Presentation size arrives first, and pausing then freezes the tile.
    public var displayedFrame: (@MainActor () -> Bool)?

    private let player: AVPlayer
    private let socket: EventSocket
    private let room: String
    private let channelID: Int64
    private var room_: RoomState?
    private var handler: UUID?
    private var timer: Timer?
    private var holdUntil = Date.distantPast
    private var lastSeek = Date.distantPast
    /// `-BroadwaveSyncLog 1` logs the frame on screen once a second so screens
    /// on one Mac can be lined up against the same wall clock.
    private let logs = UserDefaults.standard.bool(forKey: "BroadwaveSyncLog")
    private var ticks = 0
    private var rateSets = 0
    private var trim = Trim.none
    private var trimEnded = Date.distantPast
    private var fastFrom: (at: Date, drift: Double)?
    private var canSpeedUp = true
    private var speedUpOff = Date.distantPast
    private var lastMove: SyncMove?
    private var easeVersion: Int?
    /// The viewer paused this screen. It keeps its own place, time-shifted,
    /// and the engine stops pulling it back to the room.
    public private(set) var detached = false
    private var inactive = false
    private var seenPlaying = false
    /// The player went to rate 0 through a pause or rate call. AVPlayer also
    /// drops to rate 0 on its own, at a discontinuity; that is not the viewer.
    private var pausedByCall = false
    private var activeSince = Date()
    private var lifecycle: [NSObjectProtocol] = []
    private var rateObserver: NSObjectProtocol?
    private static let log = Logger(subsystem: "com.wolfeup.broadwave", category: "sync")

    /// A trim starts past trimMS and ends inside lockMS, and the next one waits
    /// quietSeconds. Every rate change on AVPlayer holds a frame, and a trim
    /// recomputed from each tick's drift changed it four times a second.
    static let trimMS = 25.0
    static let lockMS = 10.0
    static let seekMS = 400.0
    static let trimStep: Float = 0.02
    static let quietSeconds = 3.0

    public enum Trim: Sendable { case none, slow, fast }

    /// What one sync tick should do. A pause before the first decoded frame
    /// leaves the layer black, and a seek into a date the playlist does not
    /// hold stalls the same way.
    enum SyncMove: Equatable {
        case wait
        case pause(resumeAfter: Double?, seekToTarget: Bool)
        case seek
        /// Play at the room's rate with this trim.
        case play(Trim, locked: Bool)
    }

    /// `trim` is the trim in force. `quiet` is false until quietSeconds after
    /// the last trim ended. `canSpeedUp` is false once a fast trim has failed
    /// to gain on the room; a live item can ignore a rate above 1, and then
    /// only a seek catches up.
    static func decide(
        hasFrame: Bool,
        driftMS: Double,
        roomRate: Double,
        canSeek: Bool,
        forwardBuffer: Double = 2,
        trim: Trim = .none,
        quiet: Bool = true,
        canSpeedUp: Bool = true
    ) -> SyncMove {
        guard hasFrame else { return .wait }
        if roomRate == 0 {
            return .pause(resumeAfter: nil, seekToTarget: canSeek && abs(driftMS) > trimMS)
        }
        // Behind a target the item cannot seek to is behind its live edge less
        // hold-back. Speeding up only runs into that edge and stalls.
        if driftMS < -trimMS, !canSeek {
            return .play(.none, locked: false)
        }
        if abs(driftMS) > seekMS {
            if driftMS > 0 {
                return .pause(resumeAfter: driftMS / 1000, seekToTarget: false)
            }
            // Chasing a target the buffer does not hold lands past the live edge.
            if !canSeek {
                return .wait
            }
            if forwardBuffer < 1.5 {
                return .play(.none, locked: false)
            }
            return .seek
        }
        let roomy = forwardBuffer >= 1.5
        switch trim {
        case .slow where driftMS > lockMS:
            return .play(.slow, locked: false)
        case .fast where driftMS < -lockMS && roomy && canSpeedUp:
            return .play(.fast, locked: false)
        default:
            break
        }
        if abs(driftMS) <= trimMS {
            return .play(.none, locked: true)
        }
        if trim == .none, !quiet {
            return .play(.none, locked: false)
        }
        if driftMS > 0 {
            return .play(.slow, locked: false)
        }
        if canSpeedUp {
            return .play(roomy ? .fast : .none, locked: false)
        }
        return canSeek && roomy ? .seek : .play(.none, locked: false)
    }

    /// Player rate for a room rate and a trim.
    static func rate(room: Double, trim: Trim) -> Float {
        let base = Float(room)
        switch trim {
        case .none: return base
        case .slow: return base * (1 - trimStep)
        case .fast: return base * (1 + trimStep)
        }
    }

    /// The room is moving and this player is not. AVPlayer can report
    /// `.playing` with rate 0, and that state never paints the next frame.
    /// A paused player the engine did not pause, with picture to play, in a
    /// room that is moving: the viewer pressed pause. A group room pauses
    /// through the room instead, and a stuck player has nothing buffered.
    /// Before the first frame plays, a paused player has not started yet.
    static func viewerPaused(
        roomRate: Double, paused: Bool, forwardBuffer: Double, sinceHold: Double,
        followRoom: Bool = true, sinceActive: Double = .infinity, seenPlaying: Bool = true, byCall: Bool = true
    ) -> Bool {
        seenPlaying && byCall && followRoom && roomRate != 0 && paused && forwardBuffer >= 1 && sinceHold > 1 && sinceActive > 3
    }

    /// A follow room eases back from its first frame at a little under 1x, closer
    /// to live than AVPlayer can seek. Each Apple screen then held wherever it
    /// started, about 100 ms from the next. Aiming at the room's latency, where
    /// the ease ends, puts them on one frame now; the room joins them at 1x.
    /// A screen keeps that aim until the room changes (`aiming`).
    static func easeTarget(room st: RoomState, serverNow: Double, reachable: Bool, aiming: Bool) -> Double? {
        guard st.mode == "follow", st.rate > 0, st.rate < 1, let latency = st.latencyMs, latency > 0 else { return nil }
        guard aiming || !reachable else { return nil }
        return serverNow - latency
    }

    static func shouldKeepPlaying(roomRate: Double, paused: Bool, rate: Float) -> Bool {
        roomRate != 0 && (paused || rate == 0)
    }

    public init(player: AVPlayer, socket: EventSocket, room: String, channelID: Int64) {
        self.player = player
        self.socket = socket
        self.room = room
        self.channelID = channelID
    }

    public func start() {
        handler = socket.on("sync.state") { [weak self] data in
            guard let self, let st = try? JSONDecoder().decode(RoomState.self, from: data), st.room == self.room else { return }
            room_ = st
            members = st.members
            apply()
        }
        socket.join(room: room, channelID: channelID)
        state = .waiting
        if let cached = socket.roomState(room), let st = try? JSONDecoder().decode(RoomState.self, from: cached), st.room == room {
            room_ = st
            members = st.members
            apply()
        }
        timer = Timer.scheduledTimer(withTimeInterval: 0.25, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.apply() }
        }
        // Noted before the next tick, or the tick would play a viewer's pause
        // again as if AVPlayer had stopped on its own.
        rateObserver = NotificationCenter.default.addObserver(
            forName: AVPlayer.rateDidChangeNotification, object: player, queue: .main
        ) { [weak self] note in
            let called = (note.userInfo?[AVPlayer.rateDidChangeReasonKey] as? AVPlayer.RateDidChangeReason) == .setRateCalled
            MainActor.assumeIsolated {
                guard let self else { return }
                self.pausedByCall = self.player.rate == 0 && called
                if self.logs, self.player.rate == 0 {
                    Self.log.notice("sync rate 0 by \(called ? "a call" : "the player", privacy: .public)")
                }
            }
        }
        #if canImport(UIKit)
            // The system pauses the player when the app leaves the screen. That
            // pause is not the viewer's.
            let center = NotificationCenter.default
            lifecycle = [
                center.addObserver(forName: UIApplication.willResignActiveNotification, object: nil, queue: .main) { [weak self] _ in
                    Task { @MainActor in self?.inactive = true }
                },
                center.addObserver(forName: UIApplication.didBecomeActiveNotification, object: nil, queue: .main) { [weak self] _ in
                    Task { @MainActor in
                        self?.inactive = false
                        self?.activeSince = Date()
                    }
                },
            ]
        #endif
    }

    public func stop() {
        timer?.invalidate()
        lifecycle.forEach(NotificationCenter.default.removeObserver)
        lifecycle = []
        if let rateObserver {
            NotificationCenter.default.removeObserver(rateObserver)
            self.rateObserver = nil
        }
        if let handler {
            socket.off("sync.state", handler)
        }
        socket.leave(room: room)
        player.rate = player.rate == 0 ? 0 : 1
        state = .off
    }

    /// Puts a screen the viewer paused back on the room's frame.
    public func rejoin() {
        guard detached else { return }
        detached = false
        seenPlaying = false
        holdUntil = Date().addingTimeInterval(1)
        player.play()
        state = .syncing
    }

    public func command(_ action: String, mediaTime: Double? = nil) {
        socket.command(room: room, action: action, mediaTime: mediaTime)
    }

    /// Program date-time of the frame on screen, Unix ms.
    public func mediaNow() -> Double? {
        guard let date = player.currentItem?.currentDate() else { return nil }
        return date.timeIntervalSince1970 * 1000
    }

    private func apply() {
        guard let st = room_, Date() >= holdUntil else { return }
        guard let item = player.currentItem, item.status == .readyToPlay, let local = mediaNow() else {
            state = .waiting
            return
        }
        let serverNow = socket.serverNow()
        var target = st.rate == 0 ? st.anchorMedia : st.target(atServer: serverNow)
        var roomRate = st.rate
        if let settled = Self.easeTarget(
            room: st, serverNow: serverNow, reachable: canSeek(to: target, item: item), aiming: easeVersion == st.version
        ) {
            target = settled
            roomRate = 1
            easeVersion = st.version
        }
        let d = local - target
        drift = d
        if logs {
            ticks += 1
            if ticks % 4 == 0 {
                let wall = Int(Date().timeIntervalSince1970 * 1000), media = Int(local), rate = player.rate
                let ahead = bufferedAhead(item), room = room, members = members, state = state.rawValue, sets = rateSets, trim = "\(trim)"
                let events = item.accessLog()?.events ?? []
                let dropped = events.reduce(0) { $0 + max(0, $1.numberOfDroppedVideoFrames) }
                let stalls = events.reduce(0) { $0 + max(0, $1.numberOfStalls) }
                let tc = player.timeControlStatus.rawValue, why = player.reasonForWaitingToPlay?.rawValue ?? "-"
                let itemStatus = item.status.rawValue, err = item.errorLog()?.events.last?.errorComment ?? "-"
                Self.log.notice("sync room=\(room, privacy: .public) wall=\(wall) media=\(media) drift=\(Int(d)) rate=\(rate) sets=\(sets) trim=\(trim, privacy: .public) dropped=\(dropped) stalls=\(stalls) buffer=\(ahead) members=\(members) state=\(state, privacy: .public) tc=\(tc) why=\(why, privacy: .public) item=\(itemStatus) err=\(err, privacy: .public)")
            }
        }
        let sized = item.presentationSize.width > 0 && item.presentationSize.height > 0
        let hasFrame = displayedFrame?() ?? sized
        if detached {
            state = .off
            return
        }
        let paused = player.timeControlStatus == .paused
        if player.timeControlStatus == .playing {
            seenPlaying = true
        }
        if Self.viewerPaused(
            roomRate: st.rate, paused: paused, forwardBuffer: bufferedAhead(item), sinceHold: Date().timeIntervalSince(holdUntil),
            followRoom: room.hasPrefix("channel:"), sinceActive: inactive ? 0 : Date().timeIntervalSince(activeSince),
            seenPlaying: seenPlaying, byCall: pausedByCall
        ) {
            detached = true
            setTrim(.none)
            state = .off
            if logs {
                Self.log.notice("sync detached: the viewer paused")
            }
            return
        }
        // Pausing again on the tick that restarts a stuck player puts rate
        // straight back to 0, and the tile stays on one frame.
        if Self.shouldKeepPlaying(roomRate: st.rate, paused: player.timeControlStatus == .paused, rate: player.rate) {
            player.play()
            state = hasFrame ? .syncing : .waiting
            return
        }
        let quiet = Date().timeIntervalSince(trimEnded) >= Self.quietSeconds
        checkSpeedUp(drift: d)
        let move = Self.decide(
            hasFrame: hasFrame, driftMS: d, roomRate: roomRate, canSeek: canSeek(to: target, item: item),
            forwardBuffer: bufferedAhead(item), trim: trim, quiet: quiet, canSpeedUp: canSpeedUp
        )
        if logs, move != lastMove {
            let status = player.timeControlStatus.rawValue, ahead = bufferedAhead(item), what = String(describing: move)
            Self.log.notice("sync move \(what, privacy: .public) drift=\(Int(d)) status=\(status) buffer=\(ahead)")
        }
        lastMove = move
        switch move {
        case .wait:
            state = .waiting
        case let .pause(resumeAfter, seekToTarget):
            setTrim(.none)
            player.pause()
            if seekToTarget {
                seek(to: target)
            }
            if let resumeAfter {
                holdUntil = Date().addingTimeInterval(resumeAfter)
                DispatchQueue.main.asyncAfter(deadline: .now() + resumeAfter) { [weak self] in
                    self?.player.play()
                }
                state = .syncing
            } else {
                state = .locked
            }
        case .seek:
            setTrim(.none)
            if abs(d) > Self.seekMS {
                seek(to: target)
            } else {
                nudge(by: -d, item: item)
            }
            state = .syncing
        case let .play(wanted, locked):
            // A live item resumed straight into a trimmed rate stayed frozen on
            // tvOS. Trim only a player that is already moving.
            let next = player.timeControlStatus == .playing ? wanted : .none
            setTrim(next, drift: d)
            let rate = Self.rate(room: roomRate, trim: next)
            if player.rate != rate {
                player.rate = rate
                rateSets += 1
            }
            state = locked ? .locked : .syncing
        }
    }

    private func setTrim(_ next: Trim, drift: Double = 0) {
        guard next != trim else { return }
        if trim != .none {
            trimEnded = Date()
        }
        fastFrom = next == .fast ? (Date(), drift) : nil
        trim = next
    }

    /// A fast trim should gain about trimStep of wall time. A live item that
    /// ignores the rate gains nothing, so behind is then fixed with a seek.
    private func checkSpeedUp(drift: Double) {
        if !canSpeedUp, Date().timeIntervalSince(speedUpOff) > 300 {
            canSpeedUp = true
        }
        guard trim == .fast, let from = fastFrom else { return }
        // A stall or rebuffer during the trim is not the item ignoring the rate.
        guard player.timeControlStatus == .playing else {
            fastFrom = (Date(), drift)
            return
        }
        let elapsed = Date().timeIntervalSince(from.at)
        guard elapsed >= 4 else { return }
        let expected = elapsed * 1000 * Double(Self.trimStep)
        if drift - from.drift < expected * 0.4 {
            canSpeedUp = false
            speedUpOff = Date()
            setTrim(.none)
        } else {
            fastFrom = nil
        }
    }

    /// Seconds of media loaded past the playhead.
    private func bufferedAhead(_ item: AVPlayerItem) -> Double {
        let now = CMTimeGetSeconds(item.currentTime())
        if !now.isFinite {
            return 0
        }
        var ahead = 0.0
        for value in item.loadedTimeRanges {
            let range = value.timeRangeValue
            let start = CMTimeGetSeconds(range.start)
            let dur = CMTimeGetSeconds(range.duration)
            if !start.isFinite || !dur.isFinite {
                continue
            }
            let end = start + dur
            if now >= start - 0.05, now <= end {
                ahead = max(ahead, end - now)
            }
        }
        return ahead
    }

    /// The target date has to fall in a range the item can already play.
    /// Seeking earlier than the first segment, or past the live edge, stalls.
    private func canSeek(to mediaMS: Double, item: AVPlayerItem) -> Bool {
        guard let current = item.currentDate() else { return false }
        let delta = mediaMS / 1000 - current.timeIntervalSince1970
        let target = CMTimeAdd(item.currentTime(), CMTime(seconds: delta, preferredTimescale: 90000))
        return item.seekableTimeRanges.contains { CMTimeRangeContainsTime($0.timeRangeValue, time: target) }
    }

    /// A small exact seek forward inside the buffer, at most every 15 s.
    private func nudge(by ms: Double, item: AVPlayerItem) {
        guard Date().timeIntervalSince(lastSeek) > 15 else { return }
        lastSeek = Date()
        if logs {
            Self.log.notice("sync nudge \(Int(ms)) ms")
        }
        let to = CMTimeAdd(item.currentTime(), CMTime(seconds: ms / 1000, preferredTimescale: 90000))
        item.seek(to: to, toleranceBefore: .zero, toleranceAfter: .zero) { _ in }
    }

    private func seek(to media: Double) {
        guard Date().timeIntervalSince(lastSeek) > 2, let item = player.currentItem else { return }
        guard canSeek(to: media, item: item) else { return }
        lastSeek = Date()
        if logs {
            Self.log.notice("sync seek to \(Int(media))")
        }
        item.seek(to: Date(timeIntervalSince1970: media / 1000)) { _ in }
    }
}
