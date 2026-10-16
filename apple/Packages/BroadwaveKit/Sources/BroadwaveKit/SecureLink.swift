import Foundation
import Network

/// The server's secure address on the home network: `secureUrl` in GET /server,
/// such as https://lan.abcdefghij.direct.wolfeup.com:8443. Same server, same
/// handler, but HTTPS with a trusted certificate and HTTP/2, which low-latency
/// HLS needs. The app prefers it when it answers and quietly stays on the home
/// address when it does not: a router that refuses private answers to public
/// names, a stale address, or a phone away from home.
///
/// A token is kept per address (`DeviceTokens`), and a token never goes to an
/// address the server did not prove. The token kept for the home address is
/// copied to the secure one only when all of these hold:
/// - the secure URL came from GET /server at the address the token was issued
///   at, and that /server named the same server id the token is bound to;
/// - the URL has the direct service's shape (https, `lan.<label>.` under a
///   known direct domain, no path), so a server cannot send a token to an
///   arbitrary site;
/// - GET /server at the secure URL, over TLS, answers with the same id and the
///   same discovery key the app already holds for that server.
/// Anyone able to rewrite the home address's plain HTTP reply could already
/// read the token sent over it, so this gives such a party nothing new, and a
/// party that cannot gets nothing. The home entry stays, for falling back.
public enum SecureLink {
    /// Domains under which the direct service hands out names.
    static let domains = ["direct.wolfeup.com"]

    /// The offered URL, normalized, when it has the direct service's shape and
    /// is not the home address itself.
    public static func candidate(_ raw: String?, offeredBy home: URL) -> URL? {
        guard let raw, let comps = URLComponents(string: raw), comps.scheme?.lowercased() == "https",
              comps.user == nil, comps.password == nil, comps.query == nil, comps.fragment == nil,
              comps.path.isEmpty || comps.path == "/",
              let host = comps.host?.lowercased(), ServerOrigin.of(home) != nil else { return nil }
        guard domains.contains(where: { host.hasSuffix("." + $0) && validLabels(String(host.dropLast($0.count + 1))) }) else { return nil }
        var out = URLComponents()
        out.scheme = "https"
        out.host = host
        if let port = comps.port, port != 443 {
            guard (1 ... 65535).contains(port) else { return nil }
            out.port = port
        }
        guard let url = out.url, !ServerOrigin.same(url, home) else { return nil }
        return url
    }

    /// "lan.<10 base32 letters>"
    private static func validLabels(_ head: String) -> Bool {
        let parts = head.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count == 2, parts[0] == "lan", parts[1].count == 10 else { return false }
        return parts[1].allSatisfy { ("a" ... "z").contains($0) || ("2" ... "7").contains($0) }
    }

    /// Copies the token kept for `home` to `secure`, for the same server, and
    /// returns it. Nil when there is no token bound to that server at home.
    /// Call only after `probe` proved the secure address.
    public static func handOver(from home: URL, to secure: URL, server id: String) -> String? {
        guard !id.isEmpty, id != "pending", id != "demo", !ServerOrigin.same(home, secure),
              let origin = ServerOrigin.of(home), let entry = DeviceTokens.all()[origin], entry.server == id else { return nil }
        DeviceTokens.save(entry.token, for: secure, server: id, device: entry.device)
        return entry.token
    }

    /// True when GET /server at `url` answers quickly, over a trusted
    /// connection, as this same server. A known discovery key must match too.
    public static func probe(_ url: URL, id: String, key: String?, session: URLSession = .shared, timeout: TimeInterval = 3) async -> Bool {
        guard url.scheme == "https" else { return false }
        var req = URLRequest(url: url.appending(path: "api/v1/server"))
        req.timeoutInterval = timeout
        req.cachePolicy = .reloadIgnoringLocalCacheData
        guard let (data, res) = try? await session.data(for: req),
              (res as? HTTPURLResponse)?.statusCode == 200,
              let info = try? JSONDecoder().decode(ServerInfo.self, from: data) else { return false }
        guard info.id == id else { return false }
        if let key, !key.isEmpty, info.discoveryKey != key {
            return false
        }
        return true
    }
}

/// The secure address that last answered, per server id, so the next launch
/// starts on it. Removed when it stops answering.
enum SecureMemory {
    static let key = "secureLinks"

    static func url(for id: String) -> URL? {
        let list = UserDefaults.standard.dictionary(forKey: key) as? [String: String] ?? [:]
        return list[id].flatMap(URL.init(string:))
    }

    static func save(_ url: URL?, for id: String) {
        var list = UserDefaults.standard.dictionary(forKey: key) as? [String: String] ?? [:]
        guard list[id] != url?.absoluteString else { return }
        list[id] = url?.absoluteString
        UserDefaults.standard.set(list, forKey: key)
    }
}

/// Calls back when the device's network changes, such as a new Wi-Fi.
final class PathWatch: @unchecked Sendable {
    private let monitor = NWPathMonitor()
    private var last: String?
    private let lock = NSLock()

    init(_ changed: @escaping @Sendable () -> Void) {
        monitor.pathUpdateHandler = { [weak self] path in
            guard let self else { return }
            let now = "\(path.status) \(path.availableInterfaces.map(\.name).sorted()) \(path.gateways)"
            let prev = lock.withLock {
                defer { last = now }
                return last
            }
            if let prev, prev != now, path.status == .satisfied {
                changed()
            }
        }
        monitor.start(queue: DispatchQueue(label: "broadwave.path"))
    }

    deinit {
        monitor.cancel()
    }
}
