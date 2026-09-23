import AVFoundation
import OTAKit
import OTAUI
import SwiftUI

@main
struct OTAViewerApp: App {
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
