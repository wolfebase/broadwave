import Foundation

/// How far behind live a channel plays. The first screen on a channel sets it
/// for everyone there. AVPlayer holds back about 13 s behind live, so an Apple
/// screen offers Balanced and Stable only, and the server keeps a room with one
/// in it off Lowest.
public enum LiveDelay: String, CaseIterable, Identifiable, Sendable {
    case balanced
    case stable

    public var id: String {
        rawValue
    }

    public var title: String {
        self == .balanced ? "Balanced" : "Stable"
    }

    static let key = "BroadwaveLiveDelay"

    /// This device's default, sent with every join.
    public static var saved: LiveDelay {
        get { UserDefaults.standard.string(forKey: key).flatMap(LiveDelay.init) ?? .balanced }
        set { UserDefaults.standard.set(newValue.rawValue, forKey: key) }
    }
}
