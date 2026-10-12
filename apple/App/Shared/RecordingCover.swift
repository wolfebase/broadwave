import SwiftUI
#if os(iOS)
    import UIKit
#endif

/// iPhone pushes a recording into the tab. iPad and Apple TV play it over the picture, so the sidebar stays off it.
@MainActor var recordingCovers: Bool {
    #if os(iOS)
        UIDevice.current.userInterfaceIdiom == .pad
    #else
        true
    #endif
}

/// A recording played over the picture. On iPad the stack gives Start over, Next, Breaks, and Close a bar.
/// Apple TV has no bar in a cover; its menu is the player transport.
struct RecordingCover<Content: View>: View {
    @ViewBuilder var content: () -> Content

    var body: some View {
        #if os(iOS)
            NavigationStack {
                content()
            }
            .toolbar(removing: .sidebarToggle)
        #else
            content()
        #endif
    }
}
