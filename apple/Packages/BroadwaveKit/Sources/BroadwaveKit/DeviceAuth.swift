import AVFoundation
import Foundation

// MARK: Pairing

public extension APIClient {
    /// Trades a code for this device's token. Takes a six-digit code an admin is
    /// showing, or an invite code. The server's message is safe to show on a miss.
    func claimPair(code: String, name: String, kind: String) async throws -> PairGrant {
        struct B: Encodable { var code: String; var name: String; var kind: String }
        let grant: PairGrant = try await send("POST", "/pair/claim", body: B(code: code, name: name, kind: kind), timeout: 15)
        // A grant without a token is not a pairing. Nothing else may stand in for it.
        guard !grant.token.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw APIError(code: "no_token", message: String(localized: "That server did not pair this device.", bundle: .module), status: 0)
        }
        return grant
    }

    /// Asks to be paired. The screen shows the code until an admin approves it.
    func startPair(name: String, kind: String) async throws -> PairTicket {
        struct B: Encodable { var name: String; var kind: String }
        return try await send("POST", "/pair", body: B(name: name, kind: kind), timeout: 15)
    }

    /// One look at a pairing started with `startPair`. `token` comes once, on approval.
    func pollPair(_ ticket: PairTicket) async throws -> PairPoll {
        var parts = URLComponents()
        parts.queryItems = [URLQueryItem(name: "secret", value: ticket.pollSecret ?? "")]
        return try await send("GET", "/pair/\(Self.segment(ticket.id))?\(parts.percentEncodedQuery ?? "")", timeout: 10)
    }

    /// Who this device is to the server: open on the home network, or a paired device.
    func clientMe() async throws -> ClientSeat {
        try await send("GET", "/clients/me")
    }

    private static func segment(_ raw: String) -> String {
        var allowed = CharacterSet.urlPathAllowed
        allowed.remove(charactersIn: "/")
        return raw.addingPercentEncoding(withAllowedCharacters: allowed) ?? raw
    }
}

// MARK: Playback

public extension APIClient {
    /// The cookie the server reads when a request has no header. AVPlayer fetches
    /// each playlist and segment itself, so the token rides along as this.
    var tokenCookie: HTTPCookie? {
        guard let token, let host = base.host() else { return nil }
        return HTTPCookie(properties: [
            .domain: host,
            .path: "/",
            .name: "bw_token",
            .value: token,
        ])
    }

    /// The asset for a playlist on this server. A media link fetched for it earlier
    /// and still fresh plays as is. Otherwise, with a token, every request the
    /// player makes for it carries the cookie.
    func asset(_ path: String) -> AVURLAsset {
        if let linked = MediaLinks.shared.url(for: self, path: path) {
            return AVURLAsset(url: linked)
        }
        guard let cookie = tokenCookie else { return AVURLAsset(url: url(path)) }
        return AVURLAsset(url: url(path), options: [AVURLAssetHTTPCookiesKey: [cookie]])
    }

    /// A player item for a playlist on this server. Use this, not AVPlayerItem(url:).
    /// It does not wait for the network: prefer `linkedPlayerItem` where a call can wait.
    func playerItem(_ path: String) -> AVPlayerItem {
        AVPlayerItem(asset: asset(path))
    }

    /// A player item that also plays on an AirPlay receiver. The receiver fetches
    /// the stream itself, without this app's cookie, so with a token the item
    /// plays from a media link (`mk=`) instead. Without a token, or when the server
    /// gives no link (an older one answers 404), it is the cookie item.
    func linkedPlayerItem(_ path: String) async -> AVPlayerItem {
        if let linked = await mediaLinkURL(path) {
            return AVPlayerItem(asset: AVURLAsset(url: linked))
        }
        return playerItem(path)
    }

    /// A link to a /media/ path that plays without a header or cookie for 24 hours.
    func mediaLink(_ path: String) async throws -> MediaLink {
        struct B: Encodable { var path: String }
        return try await send("POST", "/media/link", body: B(path: path), timeout: 5)
    }

    /// The media link for this path, from the cache or the server. Nil without a
    /// token, or when the server gives none.
    func mediaLinkURL(_ path: String, now: Date = Date()) async -> URL? {
        guard token != nil else { return nil }
        if let cached = MediaLinks.shared.url(for: self, path: path, now: now) {
            return cached
        }
        guard MediaLinks.shared.worthAsking(self, now: now) else { return nil }
        do {
            let link = try await mediaLink(path)
            // The link is a path on this server. Anything that resolves elsewhere is not used.
            let resolved = url(link.url)
            guard ServerOrigin.same(resolved, base), link.expiresAt > now else { return nil }
            MediaLinks.shared.keep(resolved, until: link.expiresAt, for: self, path: path)
            return resolved
        } catch let error as APIError where error.status == 404 || error.status == 405 {
            MediaLinks.shared.noLinks(self, now: now)
            return nil
        } catch {
            return nil
        }
    }
}

/// Media links by server address, token, and path, kept until shortly before they
/// run out. A server with no /media/link is not asked again for a while.
final class MediaLinks: @unchecked Sendable {
    static let shared = MediaLinks()

    /// A link is reused only while this much of it is left, so a stream started
    /// on it does not run out partway through an evening.
    static let margin: TimeInterval = 2 * 3600
    /// How long an older server's 404 holds before asking again.
    static let quiet: TimeInterval = 10 * 60

    private struct Kept {
        var url: URL
        var expiresAt: Date
    }

    private let lock = NSLock()
    private var links: [String: Kept] = [:]
    private var unsupported: [String: Date] = [:]

    private static func server(_ api: APIClient) -> String? {
        guard let token = api.token, let origin = ServerOrigin.of(api.base) else { return nil }
        return origin + "\n" + token
    }

    func url(for api: APIClient, path: String, now: Date = Date()) -> URL? {
        guard let server = Self.server(api) else { return nil }
        return lock.withLock {
            guard let kept = links[server + "\n" + path] else { return nil }
            guard kept.expiresAt.timeIntervalSince(now) > Self.margin else {
                links[server + "\n" + path] = nil
                return nil
            }
            return kept.url
        }
    }

    func keep(_ url: URL, until: Date, for api: APIClient, path: String) {
        guard let server = Self.server(api) else { return }
        lock.withLock {
            links = links.filter { $0.value.expiresAt > Date() }
            links[server + "\n" + path] = Kept(url: url, expiresAt: until)
            unsupported[server] = nil
        }
    }

    func worthAsking(_ api: APIClient, now: Date = Date()) -> Bool {
        guard let server = Self.server(api) else { return false }
        return lock.withLock { (unsupported[server] ?? .distantPast) <= now }
    }

    func noLinks(_ api: APIClient, now: Date = Date()) {
        guard let server = Self.server(api) else { return }
        lock.withLock { unsupported[server] = now.addingTimeInterval(Self.quiet) }
    }

    func reset() {
        lock.withLock {
            links = [:]
            unsupported = [:]
        }
    }
}

// MARK: Addresses

/// What someone types for a server address.
public enum ServerAddress {
    /// The port Broadwave listens on at home.
    public static let homePort = 8477

    /// A typed address as a server URL. Without a scheme it is http on the home
    /// network, or https when `remote` (the Join screen). Only http gets the home
    /// port; https without a port stays on 443.
    public static func url(_ typed: String, remote: Bool = false) -> URL? {
        var raw = typed.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !raw.isEmpty else { return nil }
        if !raw.contains("://") {
            raw = (remote && !looksLocal(raw) ? "https://" : "http://") + raw
        }
        guard var comps = URLComponents(string: raw), comps.user == nil, comps.password == nil,
              let scheme = comps.scheme?.lowercased(), scheme == "http" || scheme == "https",
              let host = comps.host, !host.isEmpty else { return nil }
        comps.scheme = scheme
        if comps.port == nil, scheme == "http" {
            comps.port = homePort
        }
        // A pasted link to the web app is still the server.
        comps.path = ""
        comps.query = nil
        comps.fragment = nil
        return comps.url
    }

    /// Names routers hand out under a public top-level domain. Same list as the server.
    static let homeDomains = ["home.arpa", "fritz.box", "attlocal.net"]

    /// Top-level names no registry sells, so only a home network answers them.
    /// The server asks the public suffix list; this is the usual part of it.
    static let privateTLDs: Set = [
        "local", "lan", "home", "internal", "localdomain", "intranet", "corp", "private",
        "priv", "router", "gateway", "localhost", "test", "invalid", "example", "domain", "workgroup",
    ]

    /// Something on the home network, by the server's rules: an IP address, a
    /// single word, a router's home domain, or a name under no public registry.
    static func looksLocal(_ raw: String) -> Bool {
        let host = raw.split(separator: "/").first.map(String.init) ?? raw
        if host.hasPrefix("[") || host.filter({ $0 == ":" }).count > 1 {
            return true
        }
        let name = String(host.split(separator: ":").first ?? "").lowercased().trimmingCharacters(in: CharacterSet(charactersIn: "."))
        if !name.contains(".") || name.allSatisfy({ $0.isNumber || $0 == "." }) {
            return true
        }
        if homeDomains.contains(where: { name == $0 || name.hasSuffix("." + $0) }) {
            return true
        }
        return privateTLDs.contains(String(name.split(separator: ".").last ?? ""))
    }
}

/// An invite code as the server compares it: letters and digits, upper case.
public enum InviteCode {
    public static func normalized(_ typed: String) -> String {
        String(typed.uppercased().filter { $0.isASCII && ($0.isLetter || $0.isNumber) })
    }
}

/// broadwave://join?server=https%3A%2F%2Ftv.example.com&code=ABCD-EFGH-JKMN
public struct JoinLink: Equatable, Sendable, Identifiable {
    public var server: String
    public var code: String
    public var id: String {
        server + "|" + code
    }

    public init(server: String, code: String) {
        self.server = server
        self.code = code
    }

    public init?(_ url: URL) {
        guard url.scheme == "broadwave", url.host() == "join" else { return nil }
        let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems ?? []
        let server = items.first { $0.name == "server" }?.value ?? ""
        let code = items.first { $0.name == "code" }?.value ?? ""
        guard !server.isEmpty || !code.isEmpty else { return nil }
        self.init(server: server, code: code)
    }
}

// MARK: Tokens

/// Where a token may go: scheme, host, and port, with the default port spelled out.
public enum ServerOrigin {
    public static func of(_ url: URL) -> String? {
        guard let scheme = url.scheme?.lowercased(), scheme == "http" || scheme == "https",
              let host = url.host(percentEncoded: false)?.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased(),
              !host.isEmpty else { return nil }
        let port = url.port ?? (scheme == "https" ? 443 : 80)
        return "\(scheme)://\(host):\(port)"
    }

    public static func same(_ a: URL, _ b: URL) -> Bool {
        guard let left = of(a) else { return false }
        return left == of(b)
    }
}

/// Each token is kept under the address it was issued at, with the server id
/// that answered there. A token goes only to that same address: a server at
/// another address that claims the same id gets nothing, and a server that moves
/// is paired again. The keychain is the one the extensions share.
public enum DeviceTokens {
    struct Entry: Codable, Equatable {
        var token: String
        /// Empty until the server's id is known (a test launch with a token).
        var server: String
        /// This device's id on that server, from the claim or GET /clients/me.
        /// A `client.revoked` naming it signs this device out.
        var device: String?
    }

    static let account = "device-tokens"
    private static let lock = NSLock()
    /// Kept in memory too, for a bundle with no keychain group (tests, previews).
    private nonisolated(unsafe) static var memory: [String: Entry] = [:]

    /// The token issued at this address, when the server there is the one it was issued by.
    public static func token(for url: URL, server id: String) -> String? {
        guard let origin = ServerOrigin.of(url), let entry = all()[origin] else { return nil }
        guard entry.server.isEmpty || id == "pending" || entry.server == id else { return nil }
        return entry.token
    }

    /// Keeps a token for this address. Nil or empty forgets it. Another address
    /// keeps its own token, even when it reported the same server id. `device` is
    /// this device's id there; nil keeps the one known for this same token.
    public static func save(_ token: String?, for url: URL, server id: String, device: String? = nil) {
        guard id != "demo", let origin = ServerOrigin.of(url) else { return }
        var list = all()
        var next = token.flatMap { $0.isEmpty ? nil : Entry(token: $0, server: id == "pending" ? "" : id) }
        if next != nil {
            let known = list[origin].flatMap { $0.token == next?.token ? $0.device : nil }
            next?.device = device.flatMap { $0.isEmpty ? nil : $0 } ?? known
        }
        guard list[origin] != next else { return }
        list[origin] = next
        store(list)
    }

    /// Names the server for a token kept before its id was known.
    public static func bind(_ url: URL, server id: String) {
        guard !id.isEmpty, id != "pending", let origin = ServerOrigin.of(url) else { return }
        var list = all()
        guard var entry = list[origin], entry.server.isEmpty else { return }
        entry.server = id
        list[origin] = entry
        store(list)
    }

    /// This device's id at this address, while a token is kept for it.
    public static func device(for url: URL) -> String? {
        guard let origin = ServerOrigin.of(url) else { return nil }
        return all()[origin]?.device
    }

    /// Names this device for the token kept at this address. Only for that same token.
    public static func noteDevice(_ device: String, for url: URL, token: String) {
        guard !device.isEmpty, let origin = ServerOrigin.of(url) else { return }
        var list = all()
        guard var entry = list[origin], entry.token == token, entry.device != device else { return }
        entry.device = device
        list[origin] = entry
        store(list)
    }

    private static func store(_ list: [String: Entry]) {
        lock.withLock { memory = list }
        SharedServer.write(list.isEmpty ? nil : try? JSONEncoder().encode(list), account: account)
    }

    static func all() -> [String: Entry] {
        if let data = SharedServer.read(account: account), let list = try? JSONDecoder().decode([String: Entry].self, from: data) {
            return list
        }
        return lock.withLock { memory }
    }
}

public extension SharedServer {
    private struct Current: Codable {
        var origin: String
        var token: String
    }

    /// The current server's token for the extensions, tied to its address. Nil clears it.
    static func saveToken(_ token: String?, for url: URL?) {
        guard let token, !token.isEmpty, let url, let origin = ServerOrigin.of(url) else {
            write(nil, account: "token")
            return
        }
        write(try? JSONEncoder().encode(Current(origin: origin, token: token)), account: "token")
    }

    /// The token, only when it was issued at this address.
    static func loadToken(for url: URL) -> String? {
        guard let data = read(account: "token"), let current = try? JSONDecoder().decode(Current.self, from: data),
              current.origin == ServerOrigin.of(url), !current.token.isEmpty else { return nil }
        return current.token
    }
}

// MARK: Redirects

/// A redirect to another address loses the Authorization header, so a token
/// never follows a server's redirect somewhere else.
final class RedirectGuard: NSObject, URLSessionTaskDelegate, Sendable {
    static let shared = RedirectGuard()

    func urlSession(
        _: URLSession, task: URLSessionTask, willPerformHTTPRedirection _: HTTPURLResponse, newRequest request: URLRequest
    ) async -> URLRequest? {
        Self.follow(from: task.originalRequest?.url, to: request)
    }

    static func follow(from original: URL?, to next: URLRequest) -> URLRequest {
        guard next.value(forHTTPHeaderField: "Authorization") != nil else { return next }
        if let original, let target = next.url, ServerOrigin.same(original, target) {
            return next
        }
        var stripped = next
        stripped.setValue(nil, forHTTPHeaderField: "Authorization")
        return stripped
    }
}
