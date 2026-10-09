import Foundation

/// Game alerts from the server (`game.alert`). One stays up for 12 seconds
/// once it can be seen, a repeat of the same id is dropped, and one older
/// than ten minutes is dropped. The player holds the queue while it is up.
public struct GameAlerts: Sendable {
    public struct Item: Equatable, Sendable {
        public var alert: GameAlert
        public var at: Date

        public init(alert: GameAlert, at: Date) {
            self.alert = alert
            self.at = at
        }
    }

    /// How long an alert stays worth showing. A close finish is over soon.
    public static let freshFor: TimeInterval = 10 * 60
    /// How long the one on screen stays once it can be seen.
    public static let visibleFor: TimeInterval = 12

    private var shown: Set<String> = []
    private var queue: [Item] = []

    public init() {}

    /// False when the alert has no id, no channel, or this screen already took it.
    @discardableResult
    public mutating func receive(_ alert: GameAlert, at: Date) -> Bool {
        guard !alert.id.isEmpty, alert.channelId != 0, !shown.contains(alert.id) else { return false }
        shown.insert(alert.id)
        queue.append(Item(alert: alert, at: at))
        return true
    }

    /// The alert to show, after anything older than ten minutes is dropped.
    public mutating func current(at now: Date) -> Item? {
        while let first = queue.first, now.timeIntervalSince(first.at) >= Self.freshFor {
            queue.removeFirst()
        }
        return queue.first
    }

    /// The viewer passed on the one that is showing. The next one, if any, comes up.
    public mutating func dismiss() {
        if !queue.isEmpty {
            queue.removeFirst()
        }
    }
}
