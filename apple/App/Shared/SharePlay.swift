import BroadwaveKit
import GroupActivities
import SwiftUI

/// The SharePlay activity: which channel, on which server.
struct WatchTogetherActivity: GroupActivity {
    static let activityIdentifier = "com.wolfeup.broadwave.together"
    var invite: WatchTogether

    var metadata: GroupActivityMetadata {
        var metadata = GroupActivityMetadata()
        metadata.title = invite.channelName
        metadata.subtitle = "Live TV · \(invite.serverName)"
        metadata.type = .watchTogether
        metadata.supportsContinuationOnTV = true
        return metadata
    }
}

/// SharePlay in a FaceTime call. The session names the channel and nothing else:
/// each device plays it from the server and joins the channel's room, so the
/// screens stay on one frame the way they do at home. The playback coordinator
/// is not used; it would fight the sync engine's own rate and seeks.
/// Off unless the build has the Group Activities entitlement (`BroadwaveSharePlay`).
@MainActor
@Observable
final class SharePlay {
    static let enabled = Bundle.main.object(forInfoDictionaryKey: "BroadwaveSharePlay") as? Bool ?? false

    /// In a call that can start SharePlay.
    private(set) var eligible = false
    /// Joined to a session.
    private(set) var active = false
    /// Why an invite was not followed. The shell shows it and clears it.
    var note: String?

    @ObservationIgnored private let observer = GroupStateObserver()
    @ObservationIgnored private var session: GroupSession<WatchTogetherActivity>?
    @ObservationIgnored private var watching: [Task<Void, Never>] = []

    /// Follows sessions for as long as the app runs. `open` plays a channel on the
    /// current server; `force` plays it even when a channel with that id is on.
    func run(store: AppStore, open: @escaping (_ id: Int64, _ force: Bool) -> Void) async {
        guard Self.enabled else { return }
        let eligibility = Task { [observer] in
            for await value in observer.$isEligibleForGroupSession.values {
                self.eligible = value
            }
        }
        defer {
            eligibility.cancel()
            leave()
        }
        for await next in WatchTogetherActivity.sessions() {
            follow(next, store: store, open: open)
        }
    }

    /// Starts SharePlay on the channel, or moves the session to it when one is going.
    func share(_ invite: WatchTogether) async {
        if let session {
            if session.activity.invite != invite {
                session.activity = WatchTogetherActivity(invite: invite)
            }
            return
        }
        do {
            _ = try await WatchTogetherActivity(invite: invite).activate()
        } catch {
            note = "SharePlay didn't start. \(error.localizedDescription)"
        }
    }

    /// The channel playing here changed. Everyone in the session follows it.
    func playing(_ invite: WatchTogether?) {
        guard let session, let invite, invite.serverID == session.activity.invite.serverID,
              invite.channelID != session.activity.invite.channelID else { return }
        session.activity = WatchTogetherActivity(invite: invite)
    }

    func leave() {
        session?.leave()
        end()
    }

    private func follow(_ next: GroupSession<WatchTogetherActivity>, store: AppStore, open: @escaping (Int64, Bool) -> Void) {
        leave()
        if case let .cannot(why) = TogetherJoin.decide(next.activity.invite, server: store.server, remembered: store.remembered) {
            note = why
            next.leave()
            return
        }
        session = next
        active = true
        watching = [
            // The first value is the invite itself, so joining opens the channel.
            Task {
                for await activity in next.$activity.values {
                    self.apply(activity.invite, store: store, open: open)
                }
            },
            Task {
                for await state in next.$state.values {
                    if case .invalidated = state, self.session === next {
                        self.end()
                    }
                }
            },
        ]
        next.join()
    }

    private func apply(_ invite: WatchTogether, store: AppStore, open: @escaping (Int64, Bool) -> Void) {
        switch TogetherJoin.decide(invite, server: store.server, remembered: store.remembered) {
        case let .watch(id):
            open(id, false)
        case let .connect(server, id):
            // Channel ids belong to one server, and the lineup stays the old
            // server's until a refresh from the new one lands.
            let before = store.freshAt
            store.connect(server)
            Task {
                await store.refresh()
                guard store.server?.id == invite.serverID, let at = store.freshAt, at != before else {
                    note = "Couldn't reach \(server.name). Try again when it's on."
                    leave()
                    return
                }
                open(id, true)
            }
        case let .cannot(why):
            note = why
            leave()
        }
    }

    private func end() {
        watching.forEach { $0.cancel() }
        watching = []
        session = nil
        active = false
    }
}
