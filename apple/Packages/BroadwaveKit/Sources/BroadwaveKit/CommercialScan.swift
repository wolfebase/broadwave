import Foundation

/// What Find commercials says, the same words as the web player.
public enum CommercialScan: Sendable {
    public static let finding = String(localized: "Finding commercials…", bundle: .module)
    public static let failed = String(localized: "Could not look for commercials. Try again.", bundle: .module)

    public static func found(_ count: Int) -> String {
        switch count {
        case ...0: String(localized: "No breaks found.", bundle: .module)
        case 1: String(localized: "Found 1 break.", bundle: .module)
        default: String(localized: "Found \(String(count)) breaks.", bundle: .module)
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
