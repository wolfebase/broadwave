import SwiftUI

#if os(iOS)
    /// Command-number changes the page. Invisible buttons, as in `PlayerKeyLayer`:
    /// the same shortcuts as scene `Commands` never fired on iPad.
    struct PageKeyLayer: View {
        let go: (AppTab) -> Void

        var body: some View {
            ZStack {
                key("Home", .home, "1")
                key("Guide", .guide, "2")
                key("Sports", .sports, "3")
                key("Recordings", .recordings, "4")
                key("Search", .search, "f")
                key("Settings", .settings, ",")
            }
            .frame(width: 0, height: 0)
            .opacity(0)
            .allowsHitTesting(false)
            .accessibilityHidden(true)
        }

        private func key(_ title: String, _ tab: AppTab, _ key: KeyEquivalent) -> some View {
            Button(title) { go(tab) }
                .keyboardShortcut(key)
        }
    }

    /// The player's keys, the same letters as the web player. A nil action has no key.
    struct PlayerKeys {
        var channels: (() -> Void)?
        var multiview: (() -> Void)?
        var record: (() -> Void)?
        var recording = false
        var info: (() -> Void)?
        var leave: () -> Void
        var leaveTitle = "Minimize"
    }

    /// Keys for a full-screen player. Its buttons are invisible because the visible
    /// controls hide with the chrome. The one-channel player's controller also takes
    /// the arrows and Escape, which need priority over scrolling and focus.
    struct PlayerKeyLayer: View {
        let keys: PlayerKeys

        var body: some View {
            ZStack {
                if let channels = keys.channels {
                    key("Channels", "g", channels)
                }
                if let multiview = keys.multiview {
                    key("Multiview", "m", multiview)
                }
                if let record = keys.record {
                    key(keys.recording ? "Stop recording" : "Record", "r", record)
                }
                if let info = keys.info {
                    key("Stream details", "i", info)
                }
                key(keys.leaveTitle, .escape, keys.leave)
                // Command-period is Escape on a keyboard without an Escape key.
                Button(keys.leaveTitle, action: keys.leave)
                    .keyboardShortcut(".", modifiers: .command)
            }
            .frame(width: 0, height: 0)
            .opacity(0)
            .allowsHitTesting(false)
            .accessibilityHidden(true)
        }

        private func key(_ title: String, _ key: KeyEquivalent, _ action: @escaping () -> Void) -> some View {
            Button(title, action: action)
                .keyboardShortcut(key, modifiers: [])
        }
    }
#endif
