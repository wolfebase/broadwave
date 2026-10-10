import Foundation
import Observation
import WatchConnectivity

/// Every call goes through the iPhone app (`WatchRelay`). A watch can't reach a
/// server on the home network itself, and a message wakes the iPhone app.
@MainActor
@Observable
final class PhoneLink: NSObject {
    private(set) var screens: [WatchRelay.Row] = []
    private(set) var error: String?
    private(set) var loading = false
    private var ready = false
    /// One message at a time: a screen that gets three channel presses in the
    /// same moment acts on one of them.
    private var queue: [WatchRelay.Request] = []
    private var sending = false
    private var recheck: Task<Void, Never>?

    override init() {
        super.init()
        guard WCSession.isSupported() else { return }
        WCSession.default.delegate = self
        WCSession.default.activate()
    }

    func refresh() {
        guard !queue.contains(.screens) else { return }
        enqueue(.screens)
    }

    func press(_ press: WatchRelay.Press, on screen: String) {
        enqueue(.press(screen: screen, press))
        guard press == .up || press == .down else { return }
        // The reply lists the old channel; the screen says its new one once it has changed.
        recheck?.cancel()
        recheck = Task {
            try? await Task.sleep(for: .seconds(1.5))
            if !Task.isCancelled {
                refresh()
            }
        }
    }

    func row(_ id: String) -> WatchRelay.Row? {
        screens.first { $0.id == id }
    }

    private func enqueue(_ request: WatchRelay.Request) {
        queue.append(request)
        loading = true
        pump()
    }

    private func pump() {
        guard ready, !sending, !queue.isEmpty else { return }
        guard WCSession.default.isReachable else {
            // Presses are stale by the time the iPhone is back; the list is asked for again then.
            queue = []
            loading = false
            error = "Can't reach your iPhone. Keep it nearby."
            return
        }
        let request = queue.removeFirst()
        sending = true
        // WatchConnectivity calls back on its own queue, so neither block may be the main actor's.
        WCSession.default.sendMessage(WatchRelay.message(request)) { @Sendable message in
            let reply = WatchRelay.reply(message)
            Task { @MainActor in self.take(reply) }
        } errorHandler: { @Sendable failure in
            #if DEBUG
                print("broadwave watch send failed \(failure.localizedDescription)")
            #endif
            Task { @MainActor in self.take(WatchRelay.Reply(error: "Can't reach your iPhone. Keep it nearby.")) }
        }
    }

    private func take(_ reply: WatchRelay.Reply?) {
        sending = false
        defer {
            loading = !queue.isEmpty
            pump()
        }
        guard let reply else {
            error = "Update Broadwave on your iPhone."
            return
        }
        error = reply.error
        #if DEBUG
            if let error = reply.error {
                print("broadwave watch error \(error)")
            }
        #endif
        if reply.error == nil {
            screens = reply.screens
            #if DEBUG
                print("broadwave watch screens \(reply.screens.map { "\($0.name)=\($0.playing)" })")
            #endif
        }
    }

    fileprivate func activated() {
        ready = true
        pump()
    }

    fileprivate func reachable() {
        guard WCSession.default.isReachable, error != nil || screens.isEmpty else { return }
        refresh()
    }
}

extension PhoneLink: WCSessionDelegate {
    nonisolated func session(_: WCSession, activationDidCompleteWith _: WCSessionActivationState, error _: (any Error)?) {
        Task { @MainActor in self.activated() }
    }

    nonisolated func sessionReachabilityDidChange(_: WCSession) {
        Task { @MainActor in self.reachable() }
    }
}
