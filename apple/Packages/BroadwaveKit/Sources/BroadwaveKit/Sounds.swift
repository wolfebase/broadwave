import Foundation

/// A sound track as the Audio menu names it: the broadcast's main mix, a
/// second language, or described video.
public struct SoundTrack: Equatable, Sendable {
    public var role: String
    public var name: String
    /// Position of the option in the player's audible group.
    public var index: Int

    public init(role: String, name: String, index: Int) {
        self.role = role
        self.name = name
        self.index = index
    }

    /// An audible option as the player lists it.
    public struct Option: Sendable {
        public var name: String
        public var isDefault: Bool
        public var describes: Bool

        public init(name: String, isDefault: Bool, describes: Bool) {
            self.name = name
            self.isDefault = isDefault
            self.describes = describes
        }
    }

    /// One sound per role from a master's audible options. The server marks the
    /// main mix as the default and described video by its characteristic; the
    /// first other option is the second language. The same rule as the web.
    public static func roles(_ options: [Option]) -> [SoundTrack] {
        var out: [SoundTrack] = []
        for (index, option) in options.enumerated() {
            let role = option.describes ? "described" : option.isDefault ? "main" : "language"
            if !out.contains(where: { $0.role == role }) {
                out.append(SoundTrack(role: role, name: option.name, index: index))
            }
        }
        let order = ["main", "language", "described"]
        return out.sorted { order.firstIndex(of: $0.role) ?? 0 < order.firstIndex(of: $1.role) ?? 0 }
    }

    /// The sound for a role, or the main mix when the channel has none.
    public static func pick(_ sounds: [SoundTrack], role: String?) -> SoundTrack? {
        sounds.first { $0.role == (role ?? "main") } ?? sounds.first { $0.role == "main" }
    }
}
