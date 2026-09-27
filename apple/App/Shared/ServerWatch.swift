import BroadwaveKit
import Foundation
import os

/// Watches one player for a server that has gone away. The decision lives in
/// `ServerOutage`. This type only asks `/health` and asks the player to start again.
@MainActor
final class ServerWatch {
    private var clock = ServerOutage()
    private var generation = 0
    private var probing = false
    private var recovering = false
    private var lastTime: Double?
    private var health: (() async -> Bool)?
    private var onMessage: ((String) -> Void)?
    private var onRecover: (() -> Void)?
    private let log = Logger(subsystem: "com.wolfeup.broadwave", category: "play")

    func reset() {
        generation += 1
        probing = false
        recovering = false
        clock = ServerOutage()
        lastTime = nil
    }

    func bind(health: @escaping () async -> Bool, onMessage: @escaping (String) -> Void, onRecover: @escaping () -> Void) {
        self.health = health
        self.onMessage = onMessage
        self.onRecover = onRecover
    }

    /// The watch request itself could not reach the server.
    func failToReach() {
        guard let shown = clock.surface(PlaybackOutage.serverStopped) else { return }
        report(shown)
        beginRecover()
    }

    /// `waiting` is the player's own waiting state. A picture that does not
    /// advance counts too, because a stalled item can stay on "playing".
    func note(time: Double?, waiting: Bool, failed: Bool) {
        if failed {
            probe(fatal: true)
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

    private func probe(fatal: Bool) {
        guard !probing, !recovering else { return }
        guard clock.shouldProbe(at: Date(), fatal: fatal) else { return }
        guard let health else { return }
        probing = true
        let gen = generation
        Task {
            let up = await health()
            guard gen == generation else { return }
            probing = false
            guard let message = clock.resolve(at: Date(), health: up, fatal: fatal) else { return }
            report(message)
            if message == PlaybackOutage.serverStopped {
                beginRecover()
            }
        }
    }

    private func beginRecover() {
        guard !recovering, let health, let onRecover else { return }
        recovering = true
        let gen = generation
        Task {
            while gen == generation {
                try? await Task.sleep(for: .seconds(1))
                guard gen == generation else { return }
                if await health() {
                    guard gen == generation else { return }
                    log.info("recovered")
                    print("broadwave recovered")
                    fflush(stdout)
                    onRecover()
                    return
                }
            }
        }
    }

    private func report(_ message: String) {
        log.info("\(message, privacy: .public)")
        print("broadwave outage \(message)")
        fflush(stdout)
        onMessage?(message)
    }
}
