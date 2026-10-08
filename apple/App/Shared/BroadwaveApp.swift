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

    init() {
        #if os(iOS)
            // longFormVideo is what lets Home start Picture in Picture and AirPlay offer a television.
            try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback, policy: .longFormVideo)
        #endif
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(store)
                .preferredColorScheme(.dark)
                .tint(Tokens.ColorToken.accent)
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
