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

    private static func query(_ group: String) -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: "url",
            kSecAttrAccessGroup as String: group,
        ]
    }

    /// Nil clears it. Does nothing in a bundle without a group (tests, macOS).
    public static func save(_ url: URL?) {
        guard let group else { return }
        let base = query(group)
        guard let url else {
            SecItemDelete(base as CFDictionary)
            return
        }
        // The system runs extensions before anyone opens the app, and an address
        // belongs to this home, so it never moves to another device.
        let update: [String: Any] = [
            kSecValueData as String: Data(url.absoluteString.utf8),
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        if SecItemUpdate(base as CFDictionary, update as CFDictionary) == errSecItemNotFound {
            SecItemAdd(base.merging(update) { _, new in new } as CFDictionary, nil)
        }
    }

    public static func load() -> URL? {
        guard let group else { return nil }
        var ask = query(group)
        ask[kSecReturnData as String] = true
        ask[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        guard SecItemCopyMatching(ask as CFDictionary, &out) == errSecSuccess,
              let data = out as? Data, let raw = String(data: data, encoding: .utf8) else { return nil }
        return URL(string: raw)
    }
}
