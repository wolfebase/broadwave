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
    private var probing = false
    private var recovering = false
    private var recovery: PlaybackOutage.Recovery?
    private var lastTime: Double?
    private var playlist: String?
    private var snap: ((Bool, String?) async -> RecoverySnap)?
    private var onMessage: ((OutageDecision) -> Void)?
    private var onRecover: (() -> Void)?
    private let log = Logger(subsystem: "com.wolfeup.broadwave", category: "play")

    func reset() {
        generation += 1
        probing = false
        recovering = false
        recovery = nil
        clock = ServerOutage()
        lastTime = nil
        playlist = nil
    }

    func bind(
        snap: @escaping (Bool, String?) async -> RecoverySnap,
        onMessage: @escaping (OutageDecision) -> Void,
        onRecover: @escaping () -> Void
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
            if let lastTime, abs(time - lastTime) < 0.04 {
                stalled = true
            }
            lastTime = time
        }
        if stalled {
            clock.noteWaiting(at: Date())
        } else {
            clock.notePlaying()
        }
        probe(fatal: false)
    }

    private func present(_ decision: OutageDecision) {
        guard clock.surface(decision.message) != nil else { return }
        recovery = decision.recovery
        report(decision)
        if decision.recovery != nil {
            beginRecover()
        }
    }

    private func probe(fatal: Bool) {
        guard !probing, !recovering else { return }
        guard clock.shouldProbe(at: Date(), fatal: fatal) else { return }
        guard let snap else { return }
        probing = true
        let gen = generation
        let path = playlist
        Task {
            let reading = await snap(false, path)
            guard gen == generation else { return }
            probing = false
            guard let decision = clock.resolve(at: Date(), snap: reading, fatal: fatal) else { return }
            recovery = decision.recovery
            report(decision)
            if decision.recovery != nil {
                beginRecover()
            }
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
                    onRecover()
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
