import Foundation

/// What the player says while a channel starts. The words and times match the web player.
public enum TuningSteps {
    static let steps: [(at: TimeInterval, text: String)] = [
        (0, "Tuning the antenna"),
        (3, "Starting the picture"),
        (7, "Lining up with live"),
        (18, "Still tuning. A weak signal can take longer"),
    ]

    public static func text(after elapsed: TimeInterval) -> String {
        steps.last { elapsed >= $0.at }?.text ?? steps[0].text
    }
}
