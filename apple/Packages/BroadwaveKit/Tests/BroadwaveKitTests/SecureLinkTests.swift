@testable import BroadwaveKit
import Foundation
import Testing

private let label = "abcdefgh23"

@Test func theSecureURLMustHaveTheDirectServiceShape() throws {
    let home = try #require(URL(string: "http://192.168.1.2:8477"))
    let good = "https://lan.\(label).direct.wolfeup.com:8443"
    #expect(SecureLink.candidate(good, offeredBy: home)?.absoluteString == good)
    #expect(SecureLink.candidate("HTTPS://LAN.\(label.uppercased()).Direct.WolfeUp.com:8443/", offeredBy: home)?.absoluteString == good)
    #expect(SecureLink.candidate("https://lan.\(label).direct.wolfeup.com", offeredBy: home)?.absoluteString == "https://lan.\(label).direct.wolfeup.com")
    for bad in [
        nil, "", "http://lan.\(label).direct.wolfeup.com:8443",
        "https://evil.example.com:8443",
        "https://lan.\(label).direct.wolfeup.com.evil.com",
        "https://lan.\(label).evil-direct.wolfeup.com",
        "https://xlan.\(label).direct.wolfeup.com",
        "https://lan.short.direct.wolfeup.com",
        "https://lan.abcdefgh01.direct.wolfeup.com",
        "https://a.lan.\(label).direct.wolfeup.com",
        "https://lan.\(label).direct.wolfeup.com:8443/steal",
        "https://lan.\(label).direct.wolfeup.com:8443?x=1",
        "https://user:pw@lan.\(label).direct.wolfeup.com:8443",
        "https://direct.wolfeup.com",
    ] {
        #expect(SecureLink.candidate(bad, offeredBy: home) == nil, "\(bad ?? "nil")")
    }
}

@Test func aTokenMovesOnlyToTheSameServer() throws {
    let home = try #require(URL(string: "http://192.168.77.2:8477"))
    let secure = try #require(URL(string: "https://lan.\(label).direct.wolfeup.com:8443"))
    DeviceTokens.save("bw_home", for: home, server: "srv-move", device: "dev-1")
    defer {
        DeviceTokens.save(nil, for: home, server: "srv-move")
        DeviceTokens.save(nil, for: secure, server: "srv-move")
    }
    #expect(SecureLink.handOver(from: home, to: secure, server: "srv-other") == nil)
    #expect(DeviceTokens.token(for: secure, server: "srv-move") == nil)
    #expect(SecureLink.handOver(from: home, to: secure, server: "pending") == nil)
    #expect(SecureLink.handOver(from: home, to: home, server: "srv-move") == nil)
    #expect(SecureLink.handOver(from: home, to: secure, server: "srv-move") == "bw_home")
    #expect(DeviceTokens.token(for: secure, server: "srv-move") == "bw_home")
    #expect(DeviceTokens.device(for: secure) == "dev-1")
    // The home address keeps its own copy for falling back.
    #expect(DeviceTokens.token(for: home, server: "srv-move") == "bw_home")
    // Another server claiming the id at the secure address still gets nothing back from it.
    #expect(DeviceTokens.token(for: secure, server: "srv-impostor") == nil)
}

@Test func aTokenNotYetBoundToAServerDoesNotMove() throws {
    let home = try #require(URL(string: "http://192.168.77.3:8477"))
    let secure = try #require(URL(string: "https://lan.\(label).direct.wolfeup.com:8444"))
    DeviceTokens.save("bw_unbound", for: home, server: "pending")
    defer { DeviceTokens.save(nil, for: home, server: "pending") }
    #expect(SecureLink.handOver(from: home, to: secure, server: "srv-unbound") == nil)
    #expect(DeviceTokens.token(for: secure, server: "srv-unbound") == nil)
}

@Test func theProbeWantsTheSameServerAndKey() async throws {
    let session = SecureStub.session()
    let url = try #require(URL(string: "https://lan.aaaaaaaaaa.direct.wolfeup.com:8443"))
    SecureStub.set(host: "lan.aaaaaaaaaa.direct.wolfeup.com", id: "srv-probe", key: "KEY")
    #expect(await SecureLink.probe(url, id: "srv-probe", key: "KEY", session: session))
    #expect(await SecureLink.probe(url, id: "srv-probe", key: nil, session: session))
    #expect(await !SecureLink.probe(url, id: "srv-else", key: "KEY", session: session))
    #expect(await !SecureLink.probe(url, id: "srv-probe", key: "OTHER", session: session))
    SecureStub.down(host: "lan.aaaaaaaaaa.direct.wolfeup.com")
    #expect(await !SecureLink.probe(url, id: "srv-probe", key: "KEY", session: session))
    let plain = try #require(URL(string: "http://lan.aaaaaaaaaa.direct.wolfeup.com:8443"))
    #expect(await !SecureLink.probe(plain, id: "srv-probe", key: nil, session: session))
}

@MainActor
@Test func theAppMovesToTheSecureAddressAndBack() async throws {
    let id = "srv-upgrade"
    let host = "lan.bbbbbbbbbb.direct.wolfeup.com"
    let homeHost = "192.168.77.10"
    let secure = try #require(URL(string: "https://\(host):8443"))
    let home = try #require(URL(string: "http://\(homeHost):8477"))
    SecureStub.set(host: homeHost, id: id, key: "K", secureUrl: secure.absoluteString)
    SecureStub.set(host: host, id: id, key: "K", secureUrl: secure.absoluteString)
    DeviceTokens.save("bw_up", for: home, server: id, device: "dev-up")
    defer {
        DeviceTokens.save(nil, for: home, server: id)
        DeviceTokens.save(nil, for: secure, server: id)
        SecureMemory.save(nil, for: id)
    }
    let store = AppStore(session: SecureStub.session())
    store.persistServers = false
    defer { store.forget() }
    store.connect(FoundServer(id: id, name: "Up", url: home, key: "K"))
    #expect(await until { store.api?.base == secure })
    #expect(store.secureBase == secure)
    #expect(store.server?.url == home)
    #expect(store.api?.token == "bw_up")
    #expect(DeviceTokens.token(for: secure, server: id) == "bw_up")
    #expect(SecureMemory.url(for: id) == secure)
    // Calls on the secure address carry the token; the server saw it there.
    #expect(SecureStub.calls(host: host).contains { $0.path == "/api/v1/server" })

    // The next launch starts on the secure address at once.
    let next = AppStore(session: SecureStub.session())
    next.persistServers = false
    next.connect(FoundServer(id: id, name: "Up", url: home, key: "K"))
    #expect(next.api?.base == secure)
    // It stops answering: back home, and it is forgotten.
    SecureStub.down(host: host)
    await next.refresh()
    #expect(await until { next.api?.base == home })
    #expect(next.secureBase == nil)
    #expect(next.api?.token == "bw_up")
    #expect(SecureMemory.url(for: id) == nil)
    next.forget()
}

@MainActor
@Test func aSecureAddressThatAnswersAsAnotherServerIsNotUsed() async throws {
    let id = "srv-impostor-test"
    let host = "lan.cccccccccc.direct.wolfeup.com"
    let homeHost = "192.168.77.11"
    let secure = try #require(URL(string: "https://\(host):8443"))
    let home = try #require(URL(string: "http://\(homeHost):8477"))
    SecureStub.set(host: homeHost, id: id, key: "K", secureUrl: secure.absoluteString)
    SecureStub.set(host: host, id: "someone-else", key: "K", secureUrl: secure.absoluteString)
    DeviceTokens.save("bw_keep", for: home, server: id)
    defer {
        DeviceTokens.save(nil, for: home, server: id)
        DeviceTokens.save(nil, for: secure, server: id)
        SecureMemory.save(nil, for: id)
    }
    let store = AppStore(session: SecureStub.session())
    store.persistServers = false
    defer { store.forget() }
    store.connect(FoundServer(id: id, name: "Home", url: home, key: "K"))
    #expect(await until { store.info?.id == id && !store.loading })
    #expect(await until(0.5) { store.api?.base != home } == false)
    #expect(DeviceTokens.token(for: secure, server: id) == nil)
    #expect(!SecureStub.calls(host: host).contains { $0.authorization != nil })
}

@MainActor
@Test func noSecureURLMeansTheHomeAddress() async throws {
    let id = "srv-plain"
    let homeHost = "192.168.77.12"
    let home = try #require(URL(string: "http://\(homeHost):8477"))
    SecureStub.set(host: homeHost, id: id, key: nil, secureUrl: nil)
    SecureMemory.save(URL(string: "https://lan.dddddddddd.direct.wolfeup.com:8443"), for: id)
    defer { SecureMemory.save(nil, for: id) }
    let store = AppStore(session: SecureStub.session())
    store.persistServers = false
    defer { store.forget() }
    store.connect(FoundServer(id: id, name: "Plain", url: home))
    // Remembered, with no token needed, so it starts there; it does not answer.
    #expect(await until { store.api?.base == home && store.info?.id == id && !store.loading })
    #expect(store.secureBase == nil)
    // The server no longer offers one: nothing is remembered.
    await store.refresh()
    #expect(SecureMemory.url(for: id) == nil)
}

@MainActor
private func until(_ timeout: Double = 3, _ body: @MainActor () -> Bool) async -> Bool {
    let deadline = Date().addingTimeInterval(timeout)
    while Date() < deadline {
        if body() {
            return true
        }
        try? await Task.sleep(for: .milliseconds(20))
    }
    return body()
}

/// Serves /api/v1 for any host it was told about. A host that is down fails
/// like a name that does not resolve.
private final class SecureStub: URLProtocol, @unchecked Sendable {
    struct Host {
        var id: String
        var key: String?
        var secureUrl: String?
        var up = true
    }

    struct Call {
        var path: String
        var authorization: String?
    }

    private static let lock = NSLock()
    private nonisolated(unsafe) static var hosts: [String: Host] = [:]
    private nonisolated(unsafe) static var seen: [String: [Call]] = [:]

    static func session() -> URLSession {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [SecureStub.self]
        return URLSession(configuration: config)
    }

    static func set(host: String, id: String, key: String?, secureUrl: String? = nil) {
        lock.withLock { hosts[host] = Host(id: id, key: key, secureUrl: secureUrl) }
    }

    static func down(host: String) {
        lock.withLock { hosts[host]?.up = false }
    }

    static func calls(host: String) -> [Call] {
        lock.withLock { seen[host] ?? [] }
    }

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        guard let url = request.url, let name = url.host() else {
            client?.urlProtocol(self, didFailWithError: URLError(.badURL))
            return
        }
        let path = url.path(percentEncoded: false)
        let host = Self.lock.withLock {
            Self.seen[name, default: []].append(Call(path: path, authorization: request.value(forHTTPHeaderField: "Authorization")))
            return Self.hosts[name]
        }
        guard let host, host.up else {
            client?.urlProtocol(self, didFailWithError: URLError(.cannotFindHost))
            return
        }
        var body: [String: Any] = [:]
        switch path {
        case "/api/v1/server":
            body = ["id": host.id, "name": "Test", "version": "dev", "apiVersion": 1, "features": [String]()]
            if let key = host.key {
                body["discoveryKey"] = key
            }
            if let secureUrl = host.secureUrl {
                body["secureUrl"] = secureUrl
            }
        case "/api/v1/channels":
            body = ["channels": [Any]()]
        case "/api/v1/airings":
            body = ["airings": [Any]()]
        case "/api/v1/recordings":
            body = ["recordings": [Any]()]
        case "/api/v1/clients/me":
            body = ["device": ["id": "dev"]]
        default:
            body = [:]
        }
        let data = (try? JSONSerialization.data(withJSONObject: body)) ?? Data()
        let response = HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: data)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}
