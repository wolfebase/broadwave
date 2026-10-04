import BroadwaveKit
import Foundation
import os

/// One look at health, tuners, the channel, and the player's own playlist. A
/// failed read is nil, which the decision treats as not fixed.
func playbackSnap(api: APIClient, channelID: Int64, assumeLost: Bool, playlist: String?) async -> RecoverySnap {
    let reach = await api.reach()
    let facts = RecoveryFacts(health: reach.up, online: reach.online, channelID: channelID, assumeLost: assumeLost && reach.up)
    if !reach.up {
        return PlaybackOutage.snap(facts)
    }
    let tuners = try? await api.tuners()
    let devices = try? await api.deviceHealth()
    let signals = try? await api.signals()
    var found: Bool?
    if let playlist {
        found = await api.playlistFound(playlist)
    }
    let lists = RecoveryLists(tuners: tuners, devices: devices, signals: signals?.channels, playlistFound: found)
    return PlaybackOutage.snap(facts, lists: lists)
}

/// Watches one player. The decision lives in `ServerOutage`. This type asks
/// the server what is wrong and asks the player to start again.
@MainActor
final class ServerWatch {
    private var clock = ServerOutage()
    private var generation = 0
    private var probeGen = 0
    private var probing = false
    private var recovering = false
    private var recovery: PlaybackOutage.Recovery?
    private var lastTime: Double?
    private var playlist: String?
    /// True once this viewing's picture has moved. A stall before that is startup.
    private var played = false
    /// When the unnamed picture-stopped message first showed. Nil once the
    /// picture moves, the viewer leaves, or a named cause takes over.
    private var pictureSince: Date?
    private var pictureGen = 0
    /// A quiet watch that is still starting or holding for its first picture.
    /// A fresh tune can hold longer than one turn of the clock, so the turn is
    /// skipped rather than starting that watch over.
    private var retryPending = false
    private var snap: ((Bool, String?) async -> RecoverySnap)?
    private var onMessage: ((OutageDecision) -> Void)?
    /// `true` is a quiet retry of a stopped picture. The message stays up.
    private var onRecover: ((Bool) -> Void)?
    private let log = Logger(subsystem: "com.wolfeup.broadwave", category: "play")

    /// Drops the stall clock. A quiet picture retry keeps its own clock.
    func resetStall() {
        generation += 1
        probeGen += 1
        probing = false
        recovering = false
        recovery = nil
        clock = ServerOutage()
        lastTime = nil
        playlist = nil
    }

    func reset() {
        resetStall()
        endPictureRetry()
        played = false
    }

    /// A reload replaced the item. A probe that started for the old one, often
    /// fatal for an item AVPlayer gave up on, would name an outage over the new
    /// picture. The stall clock keeps its place.
    func newItem() {
        probeGen += 1
        probing = false
    }

    func endPictureRetry() {
        pictureSince = nil
        pictureGen += 1
        retryPending = false
    }

    /// The quiet watch failed with nothing named. The next turn may start another.
    func pictureRetryFailed() {
        retryPending = false
    }

    func bind(
        snap: @escaping (Bool, String?) async -> RecoverySnap,
        onMessage: @escaping (OutageDecision) -> Void,
        onRecover: @escaping (Bool) -> Void
    ) {
        self.snap = snap
        self.onMessage = onMessage
        self.onRecover = onRecover
    }

    /// The playlist the player is on. A stall checks that the server still has it.
    func watching(_ playlist: String) {
        self.playlist = playlist
    }

    /// The watch request itself could not reach the server.
    func failToReach(online: Bool) {
        present(PlaybackOutage.unreachable(online: online))
    }

    /// The watch request came back with a reason the viewer can wait out.
    func fail(_ decision: OutageDecision) {
        present(decision)
    }

    /// `waiting` is the player's own waiting state. A picture that does not
    /// advance counts too, because a stalled item can stay on "playing".
    /// A pause by the viewer or the sync engine is not a stall: a long one
    /// can outlast the server's hold on the picture.
    func note(time: Double?, waiting: Bool, paused: Bool, failed: Bool) {
        if failed {
            probe(fatal: true)
            return
        }
        if paused {
            lastTime = nil
            clock.notePlaying()
            return
        }
        var stalled = waiting
        if let time, time.isFinite {
            let jump = lastTime.map { abs(time - $0) }
            lastTime = time
            // A seek or a reloaded item is not the picture playing. The stall
            // clock keeps its place, so a reload does not hide what is wrong.
            if let jump, jump >= 3 {
                probe(fatal: false)
                return
            }
            if let jump, jump < 0.04 {
                stalled = true
            }
        }
        if stalled {
            clock.noteWaiting(at: Date())
        } else {
            if let time, time.isFinite {
                played = true
            }
            clock.notePlaying()
        }
        probe(fatal: false)
    }

    private func present(_ decision: OutageDecision) {
        guard clock.surface(decision.message) != nil else { return }
        recovery = decision.recovery
        report(decision)
        follow(decision)
    }

    private func probe(fatal: Bool) {
        guard !probing, !recovering else { return }
        guard clock.shouldProbe(at: Date(), fatal: fatal) else { return }
        guard let snap else { return }
        probing = true
        let gen = probeGen
        let path = playlist
        Task {
            let reading = await snap(false, path)
            guard gen == probeGen else { return }
            probing = false
            guard let decision = clock.resolve(at: Date(), snap: reading, fatal: fatal, afterPicture: played) else { return }
            recovery = decision.recovery
            report(decision)
            follow(decision)
        }
    }

    /// A named cause waits for that cause. A stopped picture with no name
    /// starts a new watch on its own clock.
    private func follow(_ decision: OutageDecision) {
        if decision.recovery != nil {
            endPictureRetry()
            beginRecover()
            return
        }
        // The quiet watch started and then stopped too.
        retryPending = false
        armPictureRetry(decision)
    }

    private func armPictureRetry(_ decision: OutageDecision) {
        guard pictureSince == nil else { return }
        guard PlaybackOutage.pictureRetryDelay(message: decision.message, recovery: decision.recovery, elapsed: 0) != nil else { return }
        let since = Date()
        pictureSince = since
        let gen = pictureGen
        Task {
            await self.runPictureRetry(gen: gen, since: since)
        }
    }

    private func runPictureRetry(gen: Int, since: Date) async {
        while gen == pictureGen {
            let elapsed = Date().timeIntervalSince(since)
            guard let wait = PlaybackOutage.pictureRetryDelay(
                message: PlaybackOutage.pictureStopped,
                recovery: nil,
                elapsed: elapsed
            ) else { return }
            try? await Task.sleep(for: .seconds(wait))
            guard gen == pictureGen, !retryPending else { continue }
            retryPending = true
            log.info("picture retry")
            print("broadwave picture-retry")
            fflush(stdout)
            onRecover?(true)
        }
    }

    private func beginRecover() {
        guard !recovering, let snap, let onRecover, let kind = recovery else { return }
        recovering = true
        let gen = generation
        Task {
            while gen == generation {
                let reading = await snap(kind == .signal, nil)
                guard gen == generation else { return }
                if PlaybackOutage.recoveryReady(kind, reading) {
                    log.info("recovered")
                    print("broadwave recovered")
                    fflush(stdout)
                    onRecover(false)
                    return
                }
                try? await Task.sleep(for: .seconds(1))
            }
        }
    }

    private func report(_ decision: OutageDecision) {
        let message = decision.message
        log.info("\(message, privacy: .public)")
        print("broadwave outage \(message)")
        fflush(stdout)
        onMessage?(decision)
    }
}
