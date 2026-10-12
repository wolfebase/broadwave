import Foundation

/// How long ago a device answered, for the offline line on its card.
/// Empty when the stamp is missing or not a time. Same buckets as the web.
public enum LastSeen: Sendable {
    public static func phrase(_ iso: String?, now: Date = Date()) -> String {
        guard let iso, !iso.isEmpty, let then = parsed(iso) else { return "" }
        let sec = max(0, Int(now.timeIntervalSince(then).rounded()))
        if sec < 45 {
            return String(localized: "just now", bundle: .module)
        }
        let minutes = Int((Double(sec) / 60).rounded())
        if minutes < 60 {
            return minutes == 1
                ? String(localized: "1 minute ago", bundle: .module)
                : String(localized: "\(String(minutes)) minutes ago", bundle: .module)
        }
        let hours = Int((Double(minutes) / 60).rounded())
        if hours < 24 {
            return hours == 1
                ? String(localized: "1 hour ago", bundle: .module)
                : String(localized: "\(String(hours)) hours ago", bundle: .module)
        }
        let days = Int((Double(hours) / 24).rounded())
        return days == 1
            ? String(localized: "1 day ago", bundle: .module)
            : String(localized: "\(String(days)) days ago", bundle: .module)
    }

    /// The card line. No time when the device has never answered.
    public static func offline(_ when: String) -> String {
        when.isEmpty
            ? String(localized: "Offline.", bundle: .module)
            : String(localized: "Offline. Last seen \(when).", bundle: .module)
    }

    private static func parsed(_ iso: String) -> Date? {
        if let date = try? Date(iso, strategy: Date.ISO8601FormatStyle(includingFractionalSeconds: true)) {
            return date
        }
        return try? Date(iso, strategy: Date.ISO8601FormatStyle())
    }
}
