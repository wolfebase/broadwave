import SwiftUI

/// A recording plays over the whole screen, as the TV app plays a show. Pushed into a
/// tab, an iPhone on its side stacked the More tab's back button over the Recordings one.
@MainActor var recordingCovers: Bool {
    true
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
