#if os(iOS)
    import BroadwaveKit
    import Foundation
    import UIKit
    import WatchConnectivity

    /// Answers the watch app. The watch can't reach the server on the home network,
    /// so it asks here and this app makes the calls (`WatchRelay`). A message wakes
    /// the app in the background; the store has its saved server by then.
    @MainActor
    final class WatchBridge: NSObject {
        static let shared = WatchBridge()
        private weak var store: AppStore?

        func start(_ store: AppStore) {
            self.store = store
            guard WCSession.isSupported() else { return }
            WCSession.default.delegate = self
            WCSession.default.activate()
        }

        func answer(_ request: WatchRelay.Request) async -> WatchRelay.Reply {
            guard let store, let api = store.api else {
                return WatchRelay.Reply(error: "Open Broadwave on your iPhone and pick your server.")
            }
            do {
                // The watch waits on this reply, so a server that is off has to fail fast.
                if case let .press(screen, press) = request {
                    try await api.pressRemote(screenID: screen, action: press.rawValue, from: "Apple Watch", timeout: 5)
                }
                let screens = try await api.screens(timeout: 5)
                let names = Dictionary(store.channels.map { ($0.id, "\($0.displayNumber) \($0.displayName)") }) { a, _ in a }
                let rows = screens.map { screen in
                    WatchRelay.Row(id: screen.id, name: screen.name, kind: screen.kind, playing: screen.channelId.flatMap { names[$0] } ?? "")
                }
                return WatchRelay.Reply(screens: WatchRelay.ordered(rows, without: ScreenIdentity.id))
            } catch let error as APIError {
                return WatchRelay.Reply(error: error.message)
            } catch {
                return WatchRelay.Reply(error: "Your server isn't answering. Check that it's on.")
            }
        }
    }

    /// WatchConnectivity's reply block, carried to the main actor and called once.
    private struct ReplyOnce: @unchecked Sendable {
        let send: ([String: Any]) -> Void
    }

    /// A message can wake this app in the background; it keeps running until the reply is sent.
    @MainActor
    private final class Awake {
        private var task = UIBackgroundTaskIdentifier.invalid

        init() {
            task = UIApplication.shared.beginBackgroundTask(withName: "Apple Watch") { [weak self] in
                self?.end()
            }
        }

        func end() {
            guard task != .invalid else { return }
            UIApplication.shared.endBackgroundTask(task)
            task = .invalid
        }
    }

    extension WatchBridge: WCSessionDelegate {
        nonisolated func session(_: WCSession, activationDidCompleteWith _: WCSessionActivationState, error _: (any Error)?) {}

        nonisolated func sessionDidBecomeInactive(_: WCSession) {}

        /// A different watch was paired. Its session starts after this one ends.
        nonisolated func sessionDidDeactivate(_ session: WCSession) {
            session.activate()
        }

        nonisolated func session(_: WCSession, didReceiveMessage message: [String: Any], replyHandler: @escaping ([String: Any]) -> Void) {
            let request = WatchRelay.request(message)
            let reply = ReplyOnce(send: replyHandler)
            Task { @MainActor in
                let awake = Awake()
                defer { awake.end() }
                let answer = if let request {
                    await self.answer(request)
                } else {
                    WatchRelay.Reply(error: "Update Broadwave on your iPhone.")
                }
                reply.send(WatchRelay.message(answer))
            }
        }
    }
#endif
