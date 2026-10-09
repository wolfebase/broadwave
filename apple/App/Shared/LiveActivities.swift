#if os(iOS)
    import ActivityKit
    import BroadwaveKit
    import Foundation
    import os

    /// Keeps the Live Activities in step with the server while the app runs.
    /// Without a push key on the server they change only while Broadwave is open
    /// or playing; the recording's timer and progress keep moving on their own.
    @MainActor
    final class LiveActivities {
        static let recordingsKey = "LiveActivityRecordings"
        static let gamesKey = "LiveActivityGames"
        private static let log = Logger(subsystem: "com.wolfeup.broadwave", category: "activities")

        private var follows: [TeamFollow] = []
        private var games: [Airing] = []
        private var gamesAt: Date = .distantPast
        private var gamesServer = ""

        /// One pass: reads what is on, then starts, changes, and ends activities to match.
        /// `active` is false in the background, where iOS refuses a new activity.
        /// Returns how long to wait before the next pass.
        func sync(_ store: AppStore, options: LiveActivityFeed.Options, active: Bool) async -> Duration {
            guard ActivityAuthorizationInfo().areActivitiesEnabled else { return .seconds(120) }
            // Forgotten, or the demo: nothing here is anyone's.
            guard let server = store.server, server.id != "demo" else {
                await Self.end(server: nil)
                return .seconds(120)
            }
            // No lineup yet (first launch): every game would look unknown and end.
            guard let api = store.api, !store.channels.isEmpty else { return .seconds(30) }
            let now = Date()
            // A pass that can't read the server changes nothing: an empty answer would end every activity.
            do {
                let recordings = try await api.recordings()
                var scores: [ScoreGame] = []
                if options.games {
                    // The teams and the sports listing change slowly; the scoreboard is what moves.
                    if gamesServer != server.id || now.timeIntervalSince(gamesAt) > 600 {
                        follows = try await api.teams()
                        // From 4 h back, so a game in overtime past its listed end stays.
                        games = follows.isEmpty ? [] : try await api.airings(
                            from: now.addingTimeInterval(-4 * 3600), to: now.addingTimeInterval(3600), sportsOnly: true
                        )
                        gamesAt = now
                        gamesServer = server.id
                    }
                    if !follows.isEmpty {
                        scores = try await api.scoreboard()
                    }
                }
                let snap = TopShelf.Snapshot(channels: store.channels, airings: games, recordings: recordings)
                let wanted = LiveActivityFeed.wanted(snap, follows: follows, scores: scores, now: now, options: options)
                let finals = LiveActivityFeed.finals(snap, follows: follows, scores: scores, options: options)
                await apply(wanted, finals: finals, server: server.id, active: active)
                return wanted.contains { $0.kind == .game } ? .seconds(30) : .seconds(120)
            } catch {
                return .seconds(60)
            }
        }

        private func apply(_ wanted: [LiveActivityFeed.Item], finals: [String: LiveActivityFeed.Content], server: String, active: Bool) async {
            let running = await Self.running(server: server)
            let plan = LiveActivityFeed.plan(running: running.mapValues(\.content), wanted: wanted, finals: finals)
            if active {
                for item in plan.start {
                    let attributes = BroadwaveActivity(id: item.id, server: server, kind: item.kind, channelID: item.channelID)
                    do {
                        _ = try Activity.request(attributes: attributes, content: Self.content(item), pushType: nil)
                    } catch {
                        Self.log.error("live activity \(item.id, privacy: .public): \(error.localizedDescription, privacy: .public)")
                    }
                }
            }
            // A game unchanged but close to stale: the same score again moves its stale date on.
            let changed = Set(plan.update.map(\.id))
            let soon = Date().addingTimeInterval(180)
            let refresh = wanted.filter { item in
                item.kind == .game && !changed.contains(item.id) && running[item.id].map { ($0.stale ?? .distantFuture) < soon } == true
            }
            for item in plan.update + refresh {
                await Self.update(item.id, server: server, Self.content(item))
            }
            for (id, last) in plan.end {
                await Self.end(id, server: server, last: last)
            }
            for id in plan.dismiss {
                await Self.end(id, server: server, last: nil)
            }
        }

        /// A recording goes stale a few minutes after its end; a game ten minutes after the app last asked.
        private nonisolated static func content(_ item: LiveActivityFeed.Item) -> ActivityContent<LiveActivityFeed.Content> {
            let stale: Date? = switch item.kind {
            case .recording: item.content.end.map { $0.addingTimeInterval(300) }
            case .game: .now.addingTimeInterval(600)
            }
            return ActivityContent(state: item.content, staleDate: stale)
        }

        // Activity is not Sendable, so each one is found and used inside one nonisolated call.

        private struct Running: Sendable {
            var content: LiveActivityFeed.Content
            var stale: Date?
        }

        private nonisolated static func showing(_ activity: Activity<BroadwaveActivity>) -> Bool {
            activity.activityState == .active || activity.activityState == .stale
        }

        /// This server's activities on screen. Another server's go at once (a switched server's
        /// "rec-9" is not this one's), and so does a second of one id (a start that raced a relaunch).
        private nonisolated static func running(server: String) async -> [String: Running] {
            var running: [String: Running] = [:]
            for activity in Activity<BroadwaveActivity>.activities where showing(activity) {
                let id = activity.attributes.id
                if activity.attributes.server != server || running[id] != nil {
                    await activity.end(nil, dismissalPolicy: .immediate)
                } else {
                    running[id] = Running(content: activity.content.state, stale: activity.content.staleDate)
                }
            }
            return running
        }

        private nonisolated static func update(_ id: String, server: String, _ content: ActivityContent<LiveActivityFeed.Content>) async {
            for activity in Activity<BroadwaveActivity>.activities where activity.attributes.id == id && showing(activity) {
                if activity.attributes.server == server {
                    await activity.update(content)
                }
            }
        }

        /// With a last word ("Recorded", the final score) it stays a few minutes; without, it goes at once.
        private nonisolated static func end(_ id: String, server: String, last: LiveActivityFeed.Content?) async {
            for activity in Activity<BroadwaveActivity>.activities where activity.attributes.id == id && activity.attributes.server == server {
                if let last {
                    await activity.end(ActivityContent(state: last, staleDate: nil), dismissalPolicy: .after(.now.addingTimeInterval(15 * 60)))
                } else {
                    await activity.end(nil, dismissalPolicy: .immediate)
                }
            }
        }

        /// Every activity (nil), or every one not from `server`.
        private nonisolated static func end(server: String?) async {
            for activity in Activity<BroadwaveActivity>.activities where server == nil || activity.attributes.server != server {
                await activity.end(nil, dismissalPolicy: .immediate)
            }
        }
    }
#endif
