import AVFoundation
import BroadwaveKit
import BroadwaveUI
import SwiftUI

@main
struct BroadwaveApp: App {
    @State private var store = AppStore()

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
    }
}
