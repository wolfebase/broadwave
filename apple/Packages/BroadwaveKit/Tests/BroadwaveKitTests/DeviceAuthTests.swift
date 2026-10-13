import AVFoundation
@testable import BroadwaveKit
import Foundation
import Testing

/// A paired device's token rides on every request, image link, player, and socket.
/// One stub, so the suite runs one test at a time.
@Suite(.serialized)
struct DeviceAuthTests {
    @Test func everyRequestCarriesTheBearerToken() async throws {
        let api = try authAPI(token: "bw_lab")
        AuthStub.set("GET", "/api/v1/server", body: Data(#"{"id":"lab","name":"Lab","version":"1.0.0","apiVersion":1,"features":[]}"#.utf8))
        AuthStub.set("GET", "/api/v1/health", status: 204, body: Data())
        AuthStub.set("GET", "/live/12/index.m3u8", body: Data("#EXTM3U\n".utf8))
        AuthStub.set("POST", "/api/v1/screens/den/watch", status: 202, body: Data())
        AuthStub.set("POST", "/api/v1/sources", body: Data(#"{"added":1}"#.utf8))
        AuthStub.set("POST", "/api/v1/backup", body: Data("{}".utf8))

        _ = try await api.server()
        _ = await api.reach()
        _ = await api.playlistFound("/live/12/index.m3u8")
        _ = await api.playlistText("/live/12/index.m3u8")
        try await api.sendToScreen(id: "den", channelId: 12, from: "Kitchen")
        _ = try await api.addPlaylistFile(name: "List", fileName: "a.m3u", data: Data("#EXTM3U\n".utf8))
        try await api.restoreCatalog(Data("db".utf8))

        let calls = AuthStub.all()
        #expect(calls.count == 7)
        for call in calls {
            #expect(call.authorization == "Bearer bw_lab", "\(call.method) \(call.path)")
            // The header is enough; the token stays out of request URLs.
            #expect(!(call.query ?? "").contains("access_token"), "\(call.method) \(call.path)")
        }
    }

    @Test func noTokenSendsNoHeader() async throws {
        let api = try authAPI(token: nil)
        AuthStub.set("GET", "/api/v1/server", body: Data(#"{"id":"lab","name":"Lab","version":"1.0.0","apiVersion":1,"features":[]}"#.utf8))
        _ = try await api.server()
        _ = await api.reach()
        #expect(AuthStub.all().allSatisfy { $0.authorization == nil })
        #expect(try APIClient(base: #require(URL(string: "http://lab.invalid")), token: "").token == nil)
    }

    @Test func imageAndDownloadLinksCarryTheTokenInTheQuery() throws {
        let base = try #require(URL(string: "https://tv.example.com"))
        let paired = APIClient(base: base, token: "bw_lab")
        let links = [
            paired.posterURL(recordingID: 44),
            paired.artURL(kind: "airing", id: 44, width: 320),
            paired.frameURL(channelID: 12, width: 480),
            paired.supportURL(),
            paired.backupURL(name: "nightly.db"),
            paired.catalogBackupURL(),
        ]
        for link in links {
            let items = URLComponents(url: link, resolvingAgainstBaseURL: false)?.queryItems ?? []
            #expect(items.first { $0.name == "access_token" }?.value == "bw_lab", "\(link)")
            #expect(link.host() == "tv.example.com")
        }
        #expect(paired.artURL(kind: "airing", id: 44, width: 320).absoluteString == "https://tv.example.com/media/art/airing/44?w=320&access_token=bw_lab")

        let home = APIClient(base: base)
        for link in [home.posterURL(recordingID: 44), home.artURL(kind: "airing", id: 44, width: 320), home.supportURL(), home.catalogBackupURL()] {
            #expect(!link.absoluteString.contains("access_token"), "\(link)")
        }
    }

    @Test func claimSendsTheCodeNameAndKind() async throws {
        let api = try authAPI(token: nil)
        AuthStub.set("POST", "/api/v1/pair/claim", status: 201, body: Data("""
        {"token":"bw_new","device":{"id":"d1","name":"Den TV","kind":"tv","scopes":["watch"],"createdAt":"2026-10-13T12:00:00Z"}}
        """.utf8))
        let grant = try await api.claimPair(code: InviteCode.normalized("k7qm-4xpa 9rtw"), name: "Den TV", kind: "tv")
        #expect(grant.token == "bw_new")
        #expect(grant.device.name == "Den TV")
        let call = try #require(AuthStub.all().last)
        #expect(call.method == "POST")
        #expect(call.path == "/api/v1/pair/claim")
        #expect(call.contentType == "application/json")
        let body = try #require(JSONSerialization.jsonObject(with: call.body) as? [String: Any])
        #expect(Set(body.keys) == ["code", "name", "kind"])
        #expect(body["code"] as? String == "K7QM4XPA9RTW")
        #expect(body["name"] as? String == "Den TV")
        #expect(body["kind"] as? String == "tv")
    }

    @Test func aWrongCodeShowsTheServerWords() async throws {
        let api = try authAPI(token: nil)
        AuthStub.set("POST", "/api/v1/pair/claim", status: 404, body: Data(#"{"code":"not_found","message":"That code does not match."}"#.utf8))
        do {
            _ = try await api.claimPair(code: "123456", name: "Phone", kind: "phone")
            Issue.record("a 404 claim should throw")
        } catch let error as APIError {
            #expect(error.status == 404)
            #expect(error.localizedDescription == "That code does not match.")
            #expect(!error.signedOut)
        }
    }

    @Test func a401IsSignedOut() async throws {
        let api = try authAPI(token: "bw_gone")
        AuthStub.set("GET", "/api/v1/channels", status: 401, body: Data(#"{"code":"unauthorized","message":"Sign in from a paired device."}"#.utf8))
        do {
            _ = try await api.channels()
            Issue.record("a 401 should throw")
        } catch let error as APIError {
            #expect(error.signedOut)
            #expect(error.message == "Sign in from a paired device.")
        }
    }

    @Test func meAndShowACodeHitTheirRoutes() async throws {
        let api = try authAPI(token: "bw_lab")
        AuthStub.set("GET", "/api/v1/clients/me", body: Data("""
        {"auth":"device","device":{"id":"d1","name":"Den TV","kind":"tv","scopes":["watch"],"createdAt":"2026-10-13T12:00:00Z"}}
        """.utf8))
        AuthStub.set("POST", "/api/v1/pair", status: 201, body: Data(#"{"id":"p 1","code":"123456","expiresAt":"2026-10-13T12:10:00Z","pollSecret":"s&1"}"#.utf8))
        AuthStub.set("GET", "/api/v1/pair/p 1", body: Data(#"{"state":"approved","token":"bw_tv"}"#.utf8))
        let me = try await api.clientMe()
        #expect(me.auth == "device")
        #expect(me.device?.name == "Den TV")
        let ticket = try await api.startPair(name: "Den TV", kind: "tv")
        #expect(ticket.code == "123456")
        let poll = try await api.pollPair(ticket)
        #expect(poll.state == "approved")
        #expect(poll.token == "bw_tv")
        let last = try #require(AuthStub.all().last)
        #expect(last.url.hasSuffix("/api/v1/pair/p%201?secret=s%261"))
    }

    @Test func thePlayerAssetCarriesTheCookie() throws {
        let base = try #require(URL(string: "https://tv.example.com"))
        let paired = APIClient(base: base, token: "bw_lab")
        let cookie = try #require(paired.tokenCookie)
        #expect(cookie.name == "bw_token")
        #expect(cookie.value == "bw_lab")
        #expect(cookie.domain == "tv.example.com")
        #expect(cookie.path == "/")
        let asset = paired.asset("/live/12/index.m3u8")
        #expect(asset.url.absoluteString == "https://tv.example.com/live/12/index.m3u8")
        #expect(!asset.url.absoluteString.contains("access_token"))
        #expect(APIClient(base: base).tokenCookie == nil)
        #expect(APIClient(base: base).asset("/live/12/index.m3u8").url == asset.url)
    }

    @Test func theSocketHandshakeCarriesTheToken() throws {
        let url = try #require(URL(string: "wss://tv.example.com/api/v1/ws"))
        #expect(EventSocket.handshake(url, token: "bw_lab").value(forHTTPHeaderField: "Authorization") == "Bearer bw_lab")
        #expect(EventSocket.handshake(url, token: nil).value(forHTTPHeaderField: "Authorization") == nil)
        #expect(EventSocket.handshake(url, token: "").value(forHTTPHeaderField: "Authorization") == nil)
    }

    @Test func addressesKeepHTTPSOn443AndHomeOn8477() {
        // Home network: bare addresses are http on 8477, as before.
        #expect(ServerAddress.url("192.168.1.20")?.absoluteString == "http://192.168.1.20:8477")
        #expect(ServerAddress.url("192.168.1.20:9000")?.absoluteString == "http://192.168.1.20:9000")
        #expect(ServerAddress.url("http://tvbox.local")?.absoluteString == "http://tvbox.local:8477")
        #expect(ServerAddress.url("broadwave.twolfe.io")?.absoluteString == "http://broadwave.twolfe.io:8477")
        // https keeps 443.
        #expect(ServerAddress.url("https://broadwave.twolfe.io")?.absoluteString == "https://broadwave.twolfe.io")
        #expect(ServerAddress.url("https://broadwave.twolfe.io:8443")?.absoluteString == "https://broadwave.twolfe.io:8443")
        #expect(ServerAddress.url(" HTTPS://broadwave.twolfe.io/guide ")?.absoluteString == "https://broadwave.twolfe.io")
        // Join: a bare name is https; something on the home network is still http.
        #expect(ServerAddress.url("broadwave.twolfe.io", remote: true)?.absoluteString == "https://broadwave.twolfe.io")
        #expect(ServerAddress.url("192.168.1.20", remote: true)?.absoluteString == "http://192.168.1.20:8477")
        #expect(ServerAddress.url("tvbox.local", remote: true)?.absoluteString == "http://tvbox.local:8477")
        #expect(ServerAddress.url("tvbox", remote: true)?.absoluteString == "http://tvbox:8477")
        #expect(ServerAddress.url("") == nil)
        #expect(ServerAddress.url("ftp://broadwave.twolfe.io") == nil)
        #expect(ServerAddress.url("https://me:pw@broadwave.twolfe.io") == nil)
    }

    @Test func joinLinksReadServerAndCode() throws {
        let link = try #require(URL(string: "broadwave://join?server=https%3A%2F%2Fbroadwave.twolfe.io&code=K7QM-4XPA-9RTW"))
        #expect(JoinLink(link) == JoinLink(server: "https://broadwave.twolfe.io", code: "K7QM-4XPA-9RTW"))
        #expect(try JoinLink(#require(URL(string: "broadwave://connect?url=http://a"))) == nil)
        #expect(try JoinLink(#require(URL(string: "broadwave://join"))) == nil)
        #expect(InviteCode.normalized("k7qm-4xpa-9rtw") == "K7QM4XPA9RTW")
        #expect(InviteCode.normalized(" 123 456 ") == "123456")
        #expect(try ConnectLink.serverURL(from: #require(URL(string: "broadwave://connect?url=https://broadwave.twolfe.io"))) != nil)
    }

    @Test func tokensAreKeptPerServer() {
        DeviceTokens.save("bw_one", forServer: "auth-test-one")
        DeviceTokens.save("bw_two", forServer: "auth-test-two")
        DeviceTokens.save("bw_demo", forServer: "demo")
        #expect(DeviceTokens.token(forServer: "auth-test-one") == "bw_one")
        #expect(DeviceTokens.token(forServer: "auth-test-two") == "bw_two")
        #expect(DeviceTokens.token(forServer: "demo") == nil)
        DeviceTokens.save(nil, forServer: "auth-test-one")
        DeviceTokens.save(nil, forServer: "auth-test-two")
        #expect(DeviceTokens.token(forServer: "auth-test-one") == nil)
    }

    @Test @MainActor func aStoreConnectsWithTheSavedToken() throws {
        let store = AppStore(session: authSession())
        store.persistServers = false
        defer {
            store.forget()
            DeviceTokens.save(nil, forServer: "auth-store")
        }
        // Nothing answers here, so the socket never opens.
        let url = try #require(URL(string: "http://127.0.0.1:19071"))
        store.connect(FoundServer(id: "auth-store", name: "Lab", url: url), token: "bw_store")
        #expect(store.api?.token == "bw_store")
        store.connect(FoundServer(id: "auth-other", name: "Other", url: url))
        #expect(store.api?.token == nil)
        store.connect(FoundServer(id: "auth-store", name: "Lab", url: url))
        #expect(store.api?.token == "bw_store")
        store.noteSignedOut("Sign in from a paired device.")
        #expect(store.signedOut == "Sign in from a paired device.")
        #expect(store.socket == nil)
        store.connect(FoundServer(id: "auth-store", name: "Lab", url: url), token: "bw_again")
        #expect(store.signedOut == nil)
        #expect(store.api?.token == "bw_again")
    }
}

private func authSession() -> URLSession {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [AuthStub.self]
    return URLSession(configuration: config)
}

private func authAPI(token: String?) throws -> APIClient {
    AuthStub.reset()
    return try APIClient(base: #require(URL(string: "http://auth.invalid")), session: authSession(), token: token)
}

private final class AuthStub: URLProtocol, @unchecked Sendable {
    struct Call: Sendable {
        var method: String
        var url: String
        var path: String
        var query: String?
        var authorization: String?
        var contentType: String?
        var body: Data
    }

    private struct Reply {
        var status: Int
        var body: Data
    }

    private static let lock = NSLock()
    private nonisolated(unsafe) static var calls: [Call] = []
    private nonisolated(unsafe) static var replies: [String: Reply] = [:]

    static func reset() {
        lock.withLock {
            calls = []
            replies = [:]
        }
    }

    static func set(_ method: String, _ path: String, status: Int = 200, body: Data = Data("{}".utf8)) {
        lock.withLock {
            replies["\(method) \(path)"] = Reply(status: status, body: body)
        }
    }

    static func all() -> [Call] {
        lock.withLock { calls }
    }

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        let data = request.httpBody ?? request.httpBodyStream.map(Self.read) ?? Data()
        let method = request.httpMethod ?? "GET"
        let path = request.url?.path(percentEncoded: false) ?? ""
        let call = Call(
            method: method,
            url: request.url?.absoluteString ?? "",
            path: path,
            query: request.url?.query,
            authorization: request.value(forHTTPHeaderField: "Authorization"),
            contentType: request.value(forHTTPHeaderField: "Content-Type"),
            body: data
        )
        let reply = Self.lock.withLock {
            Self.calls.append(call)
            return Self.replies["\(method) \(path)"] ?? Reply(status: 200, body: Data("{}".utf8))
        }
        guard let url = request.url else {
            client?.urlProtocol(self, didFailWithError: URLError(.badURL))
            return
        }
        let response = HTTPURLResponse(url: url, statusCode: reply.status, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: reply.body)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}

    private static func read(_ stream: InputStream) -> Data {
        stream.open()
        defer { stream.close() }
        var out = Data()
        var buf = [UInt8](repeating: 0, count: 4096)
        while stream.hasBytesAvailable {
            let n = stream.read(&buf, maxLength: buf.count)
            guard n > 0 else { break }
            out.append(buf, count: n)
        }
        return out
    }
}
