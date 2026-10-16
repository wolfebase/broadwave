import AppIntents

/// The phrases Siri knows without setup, on iPhone, iPad, and Apple TV. A phrase can hold an entity, not free text,
/// so a show or a team is asked for after the phrase.
struct BroadwaveShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: WatchChannelIntent(),
            phrases: [
                "Watch \(\.$channel) on \(.applicationName)", "Put on \(\.$channel) in \(.applicationName)",
                "Watch a channel on \(.applicationName)",
            ],
            shortTitle: "Watch a channel",
            systemImageName: "tv"
        )
        AppShortcut(
            intent: WatchGameIntent(),
            phrases: [
                "Watch the \(\.$team) game on \(.applicationName)", "Put the \(\.$team) game on in \(.applicationName)",
                "Watch the game on \(.applicationName)",
            ],
            shortTitle: "Watch a game",
            systemImageName: "sportscourt"
        )
        AppShortcut(
            intent: RecordShowIntent(),
            phrases: ["Record a show in \(.applicationName)", "Record something on \(.applicationName)"],
            shortTitle: "Record a show",
            systemImageName: "record.circle"
        )
        AppShortcut(
            intent: RecordChannelIntent(),
            phrases: [
                "Record \(\.$channel) on \(.applicationName)", "Record what's on \(\.$channel) in \(.applicationName)",
                "Record a channel in \(.applicationName)",
            ],
            shortTitle: "Record a channel",
            systemImageName: "record.circle.fill"
        )
        AppShortcut(
            intent: WhatsOnIntent(),
            phrases: ["What's on \(.applicationName)", "What's on TV in \(.applicationName)"],
            shortTitle: "What's on",
            systemImageName: "list.bullet.rectangle"
        )
        AppShortcut(
            intent: MultiviewIntent(),
            phrases: ["Start multiview in \(.applicationName)"],
            shortTitle: "Start multiview",
            systemImageName: "rectangle.split.2x2"
        )
    }
}
