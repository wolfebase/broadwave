import Foundation

/// How long ago a device answered, for the offline line on its card.
/// Empty when the stamp is missing or not a time. Same buckets as the web.
public enum LastSeen: Sendable {
    public static func phrase(_ iso: String?, now: Date = Date()) -> String {
        guard let iso, !iso.isEmpty, let then = parsed(iso) else { return "" }
        let sec = max(0, Int(now.timeIntervalSince(then).rounded()))
        if sec < 45 {
            return "just now"
        }
        let minutes = Int((Double(sec) / 60).rounded())
        if minutes < 60 {
            return minutes == 1 ? "1 minute ago" : "\(minutes) minutes ago"
        }
        let hours = Int((Double(minutes) / 60).rounded())
        if hours < 24 {
            return hours == 1 ? "1 hour ago" : "\(hours) hours ago"
        }
        let days = Int((Double(hours) / 24).rounded())
        return days == 1 ? "1 day ago" : "\(days) days ago"
    }

    /// The card line. No time when the device has never answered.
    public static func offline(_ when: String) -> String {
        when.isEmpty ? "Offline." : "Offline. Last seen \(when)."
    }

    private static func parsed(_ iso: String) -> Date? {
        if let date = try? Date(iso, strategy: Date.ISO8601FormatStyle(includingFractionalSeconds: true)) {
            return date
        }
        return try? Date(iso, strategy: Date.ISO8601FormatStyle())
    }
}
