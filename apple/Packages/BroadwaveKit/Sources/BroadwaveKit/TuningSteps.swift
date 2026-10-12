import Foundation

/// What the player says while a channel starts. The words and times match the web player.
public enum TuningSteps {
    static let steps: [(at: TimeInterval, text: String)] = [
        (0, String(localized: "Tuning the antenna", bundle: .module)),
        (3, String(localized: "Starting the picture", bundle: .module)),
        (7, String(localized: "Lining up with live", bundle: .module)),
        (18, String(localized: "Still tuning. A weak signal can take longer", bundle: .module)),
    ]

    public static func text(after elapsed: TimeInterval) -> String {
        steps.last { elapsed >= $0.at }?.text ?? steps[0].text
    }
}
