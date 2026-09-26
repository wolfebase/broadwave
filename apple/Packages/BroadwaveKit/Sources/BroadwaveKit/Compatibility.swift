import Foundation

/// What this app can ask of a server, and what the server asks of the app.
public enum Compatibility {
    /// Matches the App Store version when the bundle does not say.
    public static let appVersion = "1.1"
    public static let updateServer = "Update your Broadwave server to use this"
    public static let updateApp = "Update Broadwave to use this server"

    /// The sentence to show in place of a control, or nil when the server can do it.
    /// No server info yet means leave the control up.
    public static func gateFeature(_ info: ServerInfo?, _ feature: String) -> String? {
        guard let features = info?.features else { return nil }
        return features.contains(feature) ? nil : updateServer
    }

    /// Full-screen block when this app is older than `minAppVersion`.
    /// A newer `apiVersion` alone does not block. The server's version is not the minimum.
    public static func gateApp(_ info: ServerInfo?, app: String = appVersion) -> String? {
        guard let need = info?.minAppVersion?.trimmingCharacters(in: .whitespacesAndNewlines), !need.isEmpty else { return nil }
        return compare(app, need) < 0 ? updateApp : nil
    }

    /// Missing numeric pieces count as 0, so 1.0 and 1.0.0 are the same app.
    public static func compare(_ lhs: String, _ rhs: String) -> Int {
        let a = parts(lhs)
        let b = parts(rhs)
        let n = max(a.count, b.count)
        for i in 0 ..< n {
            let da = i < a.count ? a[i] : 0
            let db = i < b.count ? b[i] : 0
            if da != db {
                return da < db ? -1 : 1
            }
        }
        return 0
    }

    private static func parts(_ version: String) -> [Int] {
        version.split(separator: ".").map { piece in
            let digits = piece.prefix { $0.isNumber }
            return Int(digits) ?? 0
        }
    }
}
