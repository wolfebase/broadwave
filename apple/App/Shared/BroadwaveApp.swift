import AVFoundation
import BroadwaveKit
import BroadwaveUI
import SwiftUI
#if os(tvOS)
    import TVServices
#else
    import WidgetKit
#endif

@main
struct BroadwaveApp: App {
    @State private var store = AppStore()
    @Environment(\.scenePhase) private var scenePhase
    #if os(iOS)
        @State private var activities = LiveActivities()
        @AppStorage(LiveActivities.recordingsKey) private var activityRecordings = true
        @AppStorage(LiveActivities.gamesKey) private var activityGames = true
    #endif

    init() {
        #if os(iOS)
            // longFormVideo is what lets Home start Picture in Picture and AirPlay offer a television.
            try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback, policy: .longFormVideo)
        #endif
    }

    #if os(iOS)
        private var activityKey: String {
            let live = store.recordings.filter(\.isRecording).map { String($0.id) }.joined(separator: ",")
            return "\(store.server?.id ?? "") \(store.channels.isEmpty) \(live) \(activityRecordings) \(activityGames) \(scenePhase == .active)"
        }
    #endif

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(store)
                .preferredColorScheme(.dark)
                .tint(Tokens.ColorToken.accent)
            #if os(iOS)
                // A recording that starts or stops, a changed setting, or coming back to the app runs a pass now.
                .task(id: activityKey) {
                    let options = LiveActivityFeed.Options(recordings: activityRecordings, games: activityGames)
                    while !Task.isCancelled {
                        let wait = await activities.sync(store, options: options, active: scenePhase == .active)
                        try? await Task.sleep(for: wait)
                    }
                }
            #endif
        }
        // What is on now has changed by the time the viewer is back on the home screen.
        .onChange(of: scenePhase) { _, phase in
            if phase != .active {
                #if os(tvOS)
                    TVTopShelfContentProvider.topShelfContentDidChange()
                #else
                    WidgetCenter.shared.reloadAllTimelines()
                #endif
            }
        }
    }
}
