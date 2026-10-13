import AVFoundation
import Foundation

// MARK: Pairing

public extension APIClient {
    /// Trades a code for this device's token. Takes a six-digit code an admin is
    /// showing, or an invite code. The server's message is safe to show on a miss.
    func claimPair(code: String, name: String, kind: String) async throws -> PairGrant {
        struct B: Encodable { var code: String; var name: String; var kind: String }
        return try await send("POST", "/pair/claim", body: B(code: code, name: name, kind: kind), timeout: 15)
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

    /// The asset for a playlist on this server. With a token, every request the
    /// player makes for it carries the cookie.
    func asset(_ path: String) -> AVURLAsset {
        guard let cookie = tokenCookie else { return AVURLAsset(url: url(path)) }
        return AVURLAsset(url: url(path), options: [AVURLAssetHTTPCookiesKey: [cookie]])
    }

    /// A player item for a playlist on this server. Use this, not AVPlayerItem(url:).
    func playerItem(_ path: String) -> AVPlayerItem {
        AVPlayerItem(asset: asset(path))
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

    /// An IP address, a .local name, or a single word: something on the home network.
    static func looksLocal(_ raw: String) -> Bool {
        let host = raw.split(separator: "/").first.map(String.init) ?? raw
        let name = host.hasPrefix("[") ? host : String(host.split(separator: ":").first ?? "")
        if name.hasPrefix("[") || name.lowercased().hasSuffix(".local") || !name.contains(".") {
            return true
        }
        return name.allSatisfy { $0.isNumber || $0 == "." }
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

/// Each server's device token, in the keychain the extensions share. The current
/// server's token is also kept on its own so the Top Shelf, widgets, and Shortcuts
/// can use it away from home.
public enum DeviceTokens {
    private static let lock = NSLock()
    /// Kept in memory too, for a bundle with no keychain group (tests, previews).
    private nonisolated(unsafe) static var memory: [String: String] = [:]

    public static func token(forServer id: String) -> String? {
        guard !id.isEmpty else { return nil }
        return all()[id]
    }

    /// Nil forgets it.
    public static func save(_ token: String?, forServer id: String) {
        guard !id.isEmpty, id != "demo" else { return }
        var list = all()
        let next = token?.isEmpty == false ? token : nil
        guard list[id] != next else { return }
        list[id] = next
        lock.withLock { memory = list }
        SharedServer.write(list.isEmpty ? nil : try? JSONEncoder().encode(list), account: "tokens")
    }

    static func all() -> [String: String] {
        if let data = SharedServer.read(account: "tokens"), let list = try? JSONDecoder().decode([String: String].self, from: data) {
            return list
        }
        return lock.withLock { memory }
    }
}

public extension SharedServer {
    /// The token for the address in `load()`. Nil clears it.
    static func saveToken(_ token: String?) {
        write(token.flatMap { $0.isEmpty ? nil : Data($0.utf8) }, account: "token")
    }

    static func loadToken() -> String? {
        guard let data = read(account: "token"), let raw = String(data: data, encoding: .utf8), !raw.isEmpty else { return nil }
        return raw
    }
}
