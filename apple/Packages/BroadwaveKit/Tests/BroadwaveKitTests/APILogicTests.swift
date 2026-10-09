@testable import BroadwaveKit
import Foundation
import Testing

private func fixture(_ name: String) throws -> Data {
    var url = URL(fileURLWithPath: #filePath)
    for _ in 0 ..< 6 {
        url.deleteLastPathComponent()
    }
    return try Data(contentsOf: url.appendingPathComponent("api/fixtures/\(name).json"))
}

@Test func aScanLineNamesTheCountOrThePercent() {
    #expect(APIClient.ScanProgress(scanning: true, found: 0, progress: 40).line == "40% done.")
    #expect(APIClient.ScanProgress(scanning: true, found: 0, progress: 0).line == "0 channels found.")
    #expect(APIClient.ScanProgress(scanning: false, found: 0, progress: nil).line == "0 channels found.")
    #expect(APIClient.ScanProgress(scanning: false, found: 1, progress: 100).line == "1 channel found.")
    #expect(APIClient.ScanProgress(scanning: false, found: 12, progress: nil).line == "12 channels found.")
}

@Test func aZeroLengthAiringHasNoProgressAndTheNextStartsOnTime() {
    let start = Date(timeIntervalSince1970: 1000)
    let empty = Airing(id: 1, channelId: 1, title: "News", start: start, end: start)
    #expect(empty.progress(at: start) == 0)
    #expect(!empty.isOn(at: start))

    let hour = Airing(id: 2, channelId: 1, title: "News", start: start, end: start.addingTimeInterval(3600))
    #expect(hour.progress(at: hour.end) == 1)
    #expect(!hour.isOn(at: hour.end))

    let later = Airing(id: 3, channelId: 1, title: "Game", start: hour.end, end: hour.end.addingTimeInterval(1800))
    let index = GuideIndex([later, hour])
    #expect(index.on(1, at: start.addingTimeInterval(10))?.id == 2)
    #expect(index.next(1, after: hour.end)?.id == 3)
    #expect(index.next(1, after: later.end) == nil)
    #expect(index.on(9, at: start) == nil)
}

@Test func savedPrefsAndTheServerAddressRoundTrip() throws {
    let chosen = Prefs(quality: .tile360, audio: .surround, picture: "film", track: "described", even: true)
    let data = try JSONEncoder().encode(chosen)
    let back = try JSONDecoder().decode(Prefs.self, from: data)
    #expect(back == chosen)

    let quiet = try JSONEncoder().encode(Prefs())
    let object = try #require(JSONSerialization.jsonObject(with: quiet) as? [String: Any])
    #expect(object["even"] == nil)
    #expect(object["picture"] == nil)
    #expect(object["quality"] as? String == "auto")
    let restored = try JSONDecoder().decode(Prefs.self, from: quiet)
    #expect(restored == Prefs())

    let server = try FoundServer(
        id: "home",
        name: "Home",
        url: #require(URL(string: "http://127.0.0.1:8477")),
        key: "public-key",
        signature: "sig",
        nonce: Data([1, 2, 3]),
        signedURL: "http://127.0.0.1:8477"
    )
    let stored = try JSONEncoder().encode(server)
    let loaded = try JSONDecoder().decode(FoundServer.self, from: stored)
    #expect(loaded.id == "home")
    #expect(loaded.key == "public-key")
    #expect(loaded.url.port == 8477)
    #expect(loaded.signature == nil)
    #expect(loaded.nonce == nil)
    #expect(loaded.signedURL == nil)
}

@Test func aClockSampleKeepsTheShortestRoundTrip() {
    var best = Double.infinity
    #expect(EventSocket.clockOffset(t0: 1000, t1: 5000, t2: 1100, best: &best) == 3950)
    #expect(best == 100 * 1.01)

    let rejected = EventSocket.clockOffset(t0: 2000, t1: 9000, t2: 1800, best: &best)
    #expect(rejected == nil)
    #expect(best == 100 * 1.01 * 1.01)

    let huge = EventSocket.clockOffset(t0: 0, t1: 1, t2: 10000, best: &best)
    #expect(huge == nil)

    var fresh = Double.infinity
    let first = EventSocket.clockOffset(t0: 0, t1: 250, t2: 50, best: &fresh)
    #expect(first == 225)
    let worse = EventSocket.clockOffset(t0: 0, t1: 999, t2: 200, best: &fresh)
    #expect(worse == nil)

    // A zero round trip is the clock not moving. Keeping it would pin best at
    // zero, and the next real sample could never replace it.
    var pinned = Double.infinity
    #expect(EventSocket.clockOffset(t0: 1000, t1: 4000, t2: 1000, best: &pinned) == nil)
    #expect(pinned.isInfinite)
    #expect(EventSocket.clockOffset(t0: 2000, t1: 5000, t2: 2004, best: &pinned) == 2998)
}

@Test func theClientReadsErrorsPlaylistsAndSettings() async throws {
    APILogicStub.reset()
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [APILogicStub.self]
    let api = try APIClient(base: #require(URL(string: "http://stub.invalid")), session: URLSession(configuration: config))

    APILogicStub.set("/api/v1/health", status: 204)
    let up = await api.reach()
    #expect(up == ServerReach(up: true, online: true))
    #expect(await api.reachable())

    APILogicStub.set("/api/v1/health", status: 503, body: Data("no".utf8))
    let down = await api.reach()
    #expect(down == ServerReach(up: false, online: true))
    #expect(await !api.reachable())

    APILogicStub.set("/api/v1/health", fail: .notConnectedToInternet)
    let offline = await api.reach()
    #expect(offline == ServerReach(up: false, online: false))

    APILogicStub.set("/api/v1/health", fail: .cannotConnectToHost)
    let closed = await api.reach()
    #expect(closed == ServerReach(up: false, online: true))

    APILogicStub.set("/api/v1/health", fail: .timedOut)
    let timed = await api.reach()
    #expect(timed == ServerReach(up: false, online: true))

    APILogicStub.set("/live/4/index.m3u8", body: Data("#EXTM3U\n#EXTINF:2,\nseg.m4s\n".utf8))
    #expect(await api.playlistFound("/live/4/index.m3u8") == true)
    #expect(await api.playlistText("/live/4/index.m3u8") == "#EXTM3U\n#EXTINF:2,\nseg.m4s\n")

    APILogicStub.set("/live/4/index.m3u8", status: 404, body: Data("{}".utf8))
    #expect(await api.playlistFound("/live/4/index.m3u8") == false)
    #expect(await api.playlistText("/live/4/index.m3u8") == nil)

    APILogicStub.set("/live/4/index.m3u8", status: 500, body: Data("upstream".utf8))
    #expect(await api.playlistFound("/live/4/index.m3u8") == nil)

    APILogicStub.set("/live/4/index.m3u8", fail: .networkConnectionLost)
    #expect(await api.playlistFound("/live/4/index.m3u8") == nil)
    #expect(await api.playlistText("/live/4/index.m3u8") == nil)

    APILogicStub.set("/api/v1/channels", status: 409, body: Data(#"{"code":"tuner_refused","message":"The tuner would not start this channel. Try again."}"#.utf8))
    do {
        _ = try await api.allChannels()
        Issue.record("a refused tuner returned channels")
    } catch let error as APIError {
        #expect(error.code == "tuner_refused")
        #expect(error.status == 409)
        #expect(error.message == "The tuner would not start this channel. Try again.")
        #expect(error.errorDescription == error.message)
    }

    APILogicStub.set("/api/v1/channels", status: 502, body: Data("<html>bad gateway</html>".utf8))
    do {
        _ = try await api.lineup()
        Issue.record("a bad gateway returned channels")
    } catch let error as APIError {
        #expect(error.code == "http_502")
        #expect(error.status == 502)
        #expect(error.message == PlaybackOutage.requestFailed)
    }

    APILogicStub.set("/api/v1/channels", body: Data("{".utf8))
    await #expect(throws: DecodingError.self) {
        try await api.channels()
    }

    let settingsBody = try fixture("settings")
    APILogicStub.set("/api/v1/settings", body: settingsBody)
    let read = try await api.settings()
    #expect(read["pictureMode"] == "broadcast")
    #expect(read["bufferMinutes"] == "60")

    try APILogicStub.set("/api/v1/settings", body: fixture("settings-save"))
    try await api.saveSettings(["hideScores": "1"])
    let put = try #require(APILogicStub.requests().last { $0.method == "PUT" })
    #expect(put.url.contains("/api/v1/settings"))
    let sent = try #require(JSONSerialization.jsonObject(with: put.body) as? [String: String])
    #expect(sent == ["hideScores": "1"])

    try APILogicStub.set("/api/v1/diagnostics", body: fixture("diagnostics"))
    #expect(try await api.doctorNotes().isEmpty)
    let depth = try #require(await api.guideDepth())
    #expect(depth.channels == 3)
    #expect(depth.channelsWithListings == 1)
    #expect(depth.airings == 1)
    #expect(try await api.guideAirings() == 1)

    APILogicStub.set("/api/v1/diagnostics", body: Data(#"{"doctor":[{"id":"guide","message":"The guide is a day behind."}]}"#.utf8))
    let notes = try await api.doctorNotes()
    #expect(notes.map(\.id) == ["guide"])
    #expect(notes.first?.message == "The guide is a day behind.")
    #expect(try await api.guideDepth() == nil)
    #expect(try await api.guideAirings() == 0)

    try APILogicStub.set("/api/v1/virtuals/schedule", body: fixture("virtual-schedule"))
    let scheduled = try await api.virtualSchedule()
    #expect(scheduled.first?.number == "9000")
    #expect((scheduled.first?.slots.count ?? 0) > 20)

    try APILogicStub.set("/api/v1/storage", body: fixture("storage"))
    let storage = try await api.storage()
    #expect(storage.freeBytes == 0)
    #expect(storage.totalBytes == 0)

    try APILogicStub.set("/api/v1/signals", body: fixture("signals"))
    let signals = try await api.signals()
    #expect(!signals.running)
    #expect(signals.channels.count == 3)

    let from = Date(timeIntervalSince1970: 1_790_262_000)
    let to = from.addingTimeInterval(3600)
    try APILogicStub.set("/api/v1/airings", body: fixture("airings"))
    _ = try await api.airings(from: from, to: to)
    let window = try #require(APILogicStub.requests().last { $0.url.contains("/api/v1/airings") })
    let fromText = ISO8601DateFormatter.plain.string(from: from)
    #expect(window.url.contains("from="))
    #expect(window.url.contains("to="))
    #expect(!window.url.contains("kind="))
    #expect(window.url.contains(fromText.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? fromText))

    _ = try await api.airings(from: from, to: to, sportsOnly: true)
    let games = try #require(APILogicStub.requests().last { $0.url.contains("/api/v1/airings") })
    #expect(games.url.contains("kind=sports"))
    #expect(games.url.contains(fromText.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? fromText))

    try APILogicStub.set("/api/v1/search", body: fixture("search"))
    let found = try await api.search("a & b")
    #expect(found.query == "Jeopardy")
    let query = try #require(APILogicStub.requests().last { $0.url.contains("/api/v1/search") })
    #expect(query.url.contains("q=a%20%26%20b"))

    // A slash in a device id must stay one path segment. urlPathAllowed keeps it,
    // and URL then resolves ".." onto a different route.
    APILogicStub.set("/api/v1/devices/a/../b/scan", body: Data(#"{"scanning":true,"found":1,"progress":10}"#.utf8))
    let progress = try await api.scanStatus(deviceID: "a/../b")
    #expect(progress.line == "1 channel found.")
    let scan = try #require(APILogicStub.requests().last { $0.method == "GET" && $0.url.contains("/scan") })
    #expect(scan.url.contains("/api/v1/devices/a%2F..%2Fb/scan"))
    #expect(!scan.url.contains("/../"))

    APILogicStub.set("/api/v1/devices/a/../b/scan", body: Data(#"{"scanning":true}"#.utf8))
    try await api.startScan(deviceID: "a/../b")
    let started = try #require(APILogicStub.requests().last { $0.method == "POST" })
    #expect(started.url.contains("/api/v1/devices/a%2F..%2Fb/scan"))

    // "." and ".." are whole segments. Left alone, URL resolves them and the
    // scan request leaves /devices.
    APILogicStub.set("/api/v1/devices/../scan", body: Data(#"{"scanning":false,"found":0}"#.utf8))
    APILogicStub.set("/api/v1/scan", body: Data(#"{"scanning":true,"found":9,"progress":1}"#.utf8))
    let dotdot = try await api.scanStatus(deviceID: "..")
    #expect(dotdot.found == 0)
    let dotdotURL = try #require(APILogicStub.requests().last { $0.url.contains("scan") })
    #expect(dotdotURL.url.contains("/devices/%2E%2E/scan"))
    #expect(!dotdotURL.url.contains("/api/v1/scan"))

    #expect(api.backupURL(name: "..").absoluteString.contains("/backups/%2E%2E"))
    #expect(api.backupURL(name: ".").absoluteString.contains("/backups/%2E"))
    #expect(!api.backupURL(name: ".").absoluteString.hasSuffix("/backups"))
    #expect(api.backupURL(name: "nightly.db").absoluteString.hasSuffix("/backups/nightly.db"))
}

private final class APILogicStub: URLProtocol, @unchecked Sendable {
    struct Reply: Sendable {
        var status: Int
        var body: Data
        var fail: URLError.Code?
    }

    struct Call: Sendable {
        var method: String
        var url: String
        var body: Data
    }

    private static let lock = NSLock()
    private nonisolated(unsafe) static var replies: [String: Reply] = [:]
    private nonisolated(unsafe) static var seen: [Call] = []

    static func reset() {
        lock.lock()
        replies = [:]
        seen = []
        lock.unlock()
    }

    static func set(_ path: String, status: Int = 200, body: Data = Data("{}".utf8), fail: URLError.Code? = nil) {
        lock.lock()
        replies[path] = Reply(status: status, body: body, fail: fail)
        lock.unlock()
    }

    static func requests() -> [Call] {
        lock.lock()
        defer { lock.unlock() }
        return seen
    }

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        let path = request.url?.path ?? ""
        let method = request.httpMethod ?? "GET"
        let url = request.url?.absoluteString ?? ""
        let body = request.httpBody ?? request.httpBodyStream.map(Self.read) ?? Data()
        Self.lock.lock()
        Self.seen.append(Call(method: method, url: url, body: body))
        let reply = Self.replies[path]
        Self.lock.unlock()
        guard let reply else {
            client?.urlProtocol(self, didFailWithError: URLError(.badURL))
            return
        }
        if let fail = reply.fail {
            client?.urlProtocol(self, didFailWithError: URLError(fail))
            return
        }
        let response = HTTPURLResponse(url: request.url!, statusCode: reply.status, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
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
            if n <= 0 {
                break
            }
            out.append(buf, count: n)
        }
        return out
    }
}
