import Foundation
import Security

/// The server address, kept where the app's extensions can read it.
/// The app and its extensions share one keychain group, named in each Info.plist
/// as BroadwaveKeychainGroup, so no App Group is needed.
public enum SharedServer {
    static let service = "com.wolfeup.broadwave.server"

    static var group: String? {
        guard let raw = Bundle.main.object(forInfoDictionaryKey: "BroadwaveKeychainGroup") as? String,
              !raw.isEmpty, !raw.hasPrefix("$(") else { return nil }
        return raw
    }

    private static func query(_ group: String, account: String) -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecAttrAccessGroup as String: group,
        ]
    }

    /// Nil clears it. Does nothing in a bundle without a group (tests, macOS).
    public static func save(_ url: URL?) {
        write(url.map { Data($0.absoluteString.utf8) }, account: "url")
    }

    public static func load() -> URL? {
        guard let data = read(account: "url"), let raw = String(data: data, encoding: .utf8) else { return nil }
        return URL(string: raw)
    }

    static func write(_ data: Data?, account: String) {
        guard let group else { return }
        let base = query(group, account: account)
        guard let data else {
            SecItemDelete(base as CFDictionary)
            return
        }
        // The system runs extensions before anyone opens the app, and an address
        // belongs to this home, so it never moves to another device.
        let update: [String: Any] = [
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        if SecItemUpdate(base as CFDictionary, update as CFDictionary) == errSecItemNotFound {
            SecItemAdd(base.merging(update) { _, new in new } as CFDictionary, nil)
        }
    }

    static func read(account: String) -> Data? {
        guard let group else { return nil }
        var ask = query(group, account: account)
        ask[kSecReturnData as String] = true
        ask[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        guard SecItemCopyMatching(ask as CFDictionary, &out) == errSecSuccess else { return nil }
        return out as? Data
    }
}

/// The lineup the app last saw, kept beside the server address, so a saved
/// Shortcut or a Control Center channel still opens when the server can't be
/// asked. Only what names a channel: id, number, name, network.
public enum SharedLineup {
    public struct Entry: Codable, Sendable, Equatable {
        public var id: Int64
        public var number: String
        public var name: String
        public var network: String?
        public var favorite: Bool
    }

    /// Enabled channels, favorites first, at most 600, so the item stays small.
    public static func entries(_ channels: [Channel]) -> [Entry] {
        let enabled = channels.filter(\.enabled)
        let ordered = enabled.filter(\.favorite) + enabled.filter { !$0.favorite }
        return ordered.prefix(600).map {
            Entry(id: $0.id, number: $0.displayNumber, name: $0.displayName, network: $0.network, favorite: $0.favorite)
        }
    }

    /// Channels built back from entries, enough to match by number or name.
    public static func channels(_ entries: [Entry]) -> [Channel] {
        entries.map {
            Channel(
                id: $0.id, deviceId: "", guideNumber: $0.number, guideName: $0.name,
                displayNumber: $0.number, displayName: $0.name,
                hd: true, favorite: $0.favorite, enabled: true, hidden: false, present: true, network: $0.network
            )
        }
    }

    /// Nil or empty clears it.
    public static func save(_ channels: [Channel]?) {
        let kept = entries(channels ?? [])
        SharedServer.write(kept.isEmpty ? nil : try? JSONEncoder().encode(kept), account: "lineup")
    }

    public static func load() -> [Channel] {
        guard let data = SharedServer.read(account: "lineup"),
              let kept = try? JSONDecoder().decode([Entry].self, from: data) else { return [] }
        return channels(kept)
    }
}
