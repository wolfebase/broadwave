import AppIntents
import SwiftUI
import WidgetKit

/// A Control Center button that opens a chosen channel, or the guide.
struct WatchControl: ControlWidget {
    var body: some ControlWidgetConfiguration {
        AppIntentControlConfiguration(kind: "watch", intent: WatchControlIntent.self) { config in
            ControlWidgetButton(action: OpenChannelIntent(channel: config.channel)) {
                Label(config.channel.map { "\($0.number) \($0.name)" } ?? "Guide", systemImage: "tv")
            }
        }
        .displayName("Watch")
        .description("Opens a channel live, or the guide.")
    }
}
