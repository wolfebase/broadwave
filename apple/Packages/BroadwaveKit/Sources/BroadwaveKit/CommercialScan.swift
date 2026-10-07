import Foundation

/// What Find commercials says, the same words as the web player.
public enum CommercialScan: Sendable {
    public static let finding = "Finding commercials…"
    public static let failed = "Could not look for commercials. Try again."

    public static func found(_ count: Int) -> String {
        switch count {
        case ...0: "No breaks found."
        case 1: "Found 1 break."
        default: "Found \(count) breaks."
        }
    }

    /// True the first time. A second press while this recording is still scanning does not start another.
    public static func start(_ id: Int64, running: inout Set<Int64>) -> Bool {
        if running.contains(id) {
            return false
        }
        running.insert(id)
        return true
    }
}
