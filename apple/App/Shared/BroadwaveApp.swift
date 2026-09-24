import AVFoundation
import BroadwaveKit
import BroadwaveUI
import SwiftUI

@main
struct BroadwaveApp: App {
    @State private var store = AppStore()

    init() {
        #if os(iOS)
            try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback)
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
