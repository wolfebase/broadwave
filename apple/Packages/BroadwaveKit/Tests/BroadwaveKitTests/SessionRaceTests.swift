@testable import BroadwaveKit
import Foundation
import Testing

@MainActor
@Test func aLateReplyDoesNotReplaceTheServerYouJustChose() async throws {
    let store = stallStore()
    defer { retire(store) }
    CatalogStall.gate.hold(19054, "/api/v1/channels")
    CatalogStall.gate.hold(19055, "/api/v1/server")
    store.connect(server(19054, "harbor", "Harbor"))
    #expect(await until { store.info?.id == "harbor" && store.loading && CatalogStall.gate.isParked(19054, "/api/v1/channels") })
    store.connect(server(19055, "inland", "Inland"))
    #expect(await until { CatalogStall.gate.isParked(19055, "/api/v1/server") })
    CatalogStall.gate.release(19054, "/api/v1/channels")
    _ = await until(3) { store.channels.contains { $0.displayName == "Harbor News" } || !store.loading }
    #expect(store.server?.id == "inland")
    #expect(!store.channels.contains { $0.displayName == "Harbor News" })
    #expect(store.loading)

    store.forget()
    CatalogStall.gate.finishAll()
    store.connect(server(19054, "harbor", "Harbor"))
    #expect(await until { store.channels.first?.displayName == "Harbor News" && !store.loading })
    CatalogStall.gate.hold(19054, "/api/v1/recordings")
    CatalogStall.gate.hold(19055, "/api/v1/server")
    let lateRecordings = Task { await store.refreshRecordings() }
    #expect(await until { CatalogStall.gate.isParked(19054, "/api/v1/recordings") })
    store.connect(server(19055, "inland", "Inland"))
    #expect(await until { CatalogStall.gate.isParked(19055, "/api/v1/server") })
    CatalogStall.gate.release(19054, "/api/v1/recordings")
    await lateRecordings.value
    #expect(store.server?.id == "inland")
    #expect(!store.recordings.contains { $0.title == "Harbor News" })

    store.forget()
    CatalogStall.gate.finishAll()
    store.connect(server(19054, "harbor", "Harbor"))
    #expect(await until { store.channels.first?.displayName == "Harbor News" && !store.loading })
    CatalogStall.gate.hold(19054, "/api/v1/passes")
    CatalogStall.gate.hold(19055, "/api/v1/server")
    let latePasses = Task { await store.refreshPasses() }
    #expect(await until { CatalogStall.gate.isParked(19054, "/api/v1/passes") })
    store.connect(server(19055, "inland", "Inland"))
    #expect(await until { CatalogStall.gate.isParked(19055, "/api/v1/server") })
    CatalogStall.gate.release(19054, "/api/v1/passes")
    await latePasses.value
    #expect(store.server?.id == "inland")
    #expect(!store.passes.contains { $0.title == "Harbor News" })

    store.forget()
    CatalogStall.gate.finishAll()
    store.connect(server(19054, "harbor", "Harbor"))
    #expect(await until { store.recordings.first?.title == "Harbor News" && !store.loading })
    let marked = try #require(store.recordings.first)
    CatalogStall.gate.hold(19054, "/api/v1/recordings")
    let staleList = Task { await store.refreshRecordings() }
    #expect(await until { CatalogStall.gate.isParked(19054, "/api/v1/recordings") })
    try await store.setWatched(marked, true)
    #expect(store.recordings.first?.watched == 1)
    CatalogStall.gate.release(19054, "/api/v1/recordings")
    await staleList.value
    #expect(store.recordings.first?.watched == 1)
}

@MainActor
@Test func aDemoResumeLosesToTheServerChosenWhileItStarts() async throws {
    let store = quietStore()
    defer { store.forget() }
    let box = PauseBox()
    store.prepareDemo = {
        await box.wait()
        return URL(string: "http://127.0.0.1:19056")
    }
    let demo = try FoundServer(id: "demo", name: "Demo", url: #require(URL(string: "http://127.0.0.1:19056")))
    let resume = Task { await store.resumeSavedDemo(demo, started: 0) }
    await box.untilEntered()
    store.connect(server(19055, "inland", "Inland"))
    box.go()
    await resume.value
    #expect(store.server?.id == "inland")
    #expect(store.error != "The demo did not start.")
}

@MainActor
@Test func aDemoThatDoesNotStartSaysSo() async throws {
    let store = quietStore()
    defer { store.forget() }
    store.prepareDemo = { nil }
    let demo = try FoundServer(id: "demo", name: "Demo", url: #require(URL(string: "http://127.0.0.1:19056")))
    await store.resumeSavedDemo(demo, started: 0)
    #expect(store.server == nil)
    #expect(store.error == "The demo did not start.")
}

@MainActor
@Test func leavingAServerClearsTheOfflineBanner() async throws {
    let store = quietStore()
    defer { store.forget() }
    try store.connect(FoundServer(id: "harbor", name: "Harbor", url: #require(URL(string: "http://127.0.0.1:19058"))))
    #expect(await until(6) { store.offline })
    store.forget()
    try store.connect(FoundServer(id: "inland", name: "Inland", url: #require(URL(string: "http://127.0.0.1:19059"))))
    #expect(store.connected)
    #expect(!store.offline)
}

@MainActor
private func stallStore() -> AppStore {
    CatalogStall.gate.reset()
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [CatalogStall.self]
    let store = AppStore(session: URLSession(configuration: config))
    store.persistServers = false
    return store
}

@MainActor
private func quietStore() -> AppStore {
    let store = AppStore(session: URLSession(configuration: .ephemeral))
    store.persistServers = false
    return store
}

@MainActor
private func retire(_ store: AppStore) {
    store.forget()
    CatalogStall.gate.finishAll()
    for id in ["harbor", "inland", "demo"] {
        let safe = String(id.filter { $0.isLetter || $0.isNumber })
        let dir = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
        try? FileManager.default.removeItem(at: dir.appendingPathComponent("broadwave-\(safe).json"))
    }
}

private func server(_ port: Int, _ id: String, _ name: String) -> FoundServer {
    FoundServer(id: id, name: name, url: URL(string: "http://127.0.0.1:\(port)")!)
}

@MainActor
private func until(_ timeout: Double = 2, _ body: @MainActor () -> Bool) async -> Bool {
    let deadline = Date().addingTimeInterval(timeout)
    while Date() < deadline {
        if body() {
            return true
        }
        try? await Task.sleep(for: .milliseconds(20))
    }
    return body()
}

@MainActor
private final class PauseBox {
    private var resume: (() -> Void)?
    private var entered = false

    func wait() async {
        await withCheckedContinuation { (cont: CheckedContinuation<Void, Never>) in
            resume = { cont.resume() }
            entered = true
        }
    }

    func untilEntered() async {
        while !entered {
            try? await Task.sleep(for: .milliseconds(10))
        }
    }

    func go() {
        resume?()
        resume = nil
    }
}

private final class CatalogGate: @unchecked Sendable {
    let lock = NSLock()
    var held: Set<String> = []
    var parked: [String: [CatalogStall]] = [:]

    func key(_ port: Int, _ path: String) -> String {
        "\(port) \(path)"
    }

    func hold(_ port: Int, _ path: String) {
        lock.lock()
        held.insert(key(port, path))
        lock.unlock()
    }

    func isParked(_ port: Int, _ path: String) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return !(parked[key(port, path)] ?? []).isEmpty
    }

    func park(_ stub: CatalogStall, port: Int, path: String) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        let name = key(port, path)
        guard held.contains(name) else { return false }
        parked[name, default: []].append(stub)
        return true
    }

    func release(_ port: Int, _ path: String) {
        let stubs = take(key(port, path))
        for stub in stubs {
            stub.finish()
        }
    }

    func finishAll() {
        lock.lock()
        held.removeAll()
        let waiting = parked
        parked.removeAll()
        lock.unlock()
        for stubs in waiting.values {
            for stub in stubs {
                stub.finish()
            }
        }
    }

    func reset() {
        finishAll()
    }

    private func take(_ name: String) -> [CatalogStall] {
        lock.lock()
        held.remove(name)
        let stubs = parked.removeValue(forKey: name) ?? []
        lock.unlock()
        return stubs
    }
}

private final class CatalogStall: URLProtocol, @unchecked Sendable {
    static let gate = CatalogGate()
    private let state = NSLock()
    private var stopped = false

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        let url = request.url
        let port = url?.port ?? 0
        let path = url?.path ?? ""
        if Self.gate.park(self, port: port, path: path) {
            return
        }
        finish()
    }

    override func stopLoading() {
        state.lock()
        stopped = true
        state.unlock()
    }

    func finish() {
        state.lock()
        let done = stopped
        if !done {
            stopped = true
        }
        state.unlock()
        guard !done, let url = request.url else { return }
        let body = Self.body(port: url.port ?? 0, path: url.path)
        let response = HTTPURLResponse(url: url, statusCode: 200, httpVersion: "HTTP/1.1", headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(body.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    private static func body(port: Int, path: String) -> String {
        let harbor = port == 19054
        let name = harbor ? "Harbor News" : "Inland Weather"
        let id = harbor ? "harbor" : "inland"
        let channel = harbor ? 1 : 2
        if path.hasSuffix("/server") {
            return #"{"id":"\#(id)","name":"\#(name)","version":"1","apiVersion":1,"features":[]}"#
        }
        if path.hasSuffix("/channels") {
            return #"{"channels":[{"id":\#(channel),"deviceId":"lab","guideNumber":"11.1","guideName":"HBR","displayNumber":"11.1","displayName":"\#(name)","hd":true,"favorite":false,"enabled":true,"hidden":false,"present":true}]}"#
        }
        if path.hasSuffix("/recordings") {
            return #"{"recordings":[{"id":\#(channel),"channelId":\#(channel),"guideNumber":"11.1","title":"\#(name)","status":"finished","startedAt":"2026-10-09T00:00:00Z"}]}"#
        }
        if path.hasSuffix("/passes") {
            return #"{"passes":[{"id":\#(channel),"title":"\#(name)"}]}"#
        }
        if path.hasSuffix("/frames") || path.hasSuffix("/airings") {
            return path.hasSuffix("/frames") ? #"{"channels":[]}"# : #"{"airings":[]}"#
        }
        return "{}"
    }
}
