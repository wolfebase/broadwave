@testable import BroadwaveKit
import Foundation
import Testing

@Test func theScreenIDIsMadeOnceAndTheServerTakesIt() throws {
    let defaults = try #require(UserDefaults(suiteName: "screen-id-\(UUID().uuidString)"))
    let first = ScreenID.current(defaults)
    #expect(ScreenID.valid(first))
    #expect(first.count <= 64)
    #expect(ScreenID.current(defaults) == first)
    #expect(defaults.string(forKey: ScreenID.key) == first)

    // One the server would refuse is replaced, and the new one is kept.
    defaults.set("den ipad!", forKey: ScreenID.key)
    let fresh = ScreenID.current(defaults)
    #expect(fresh != "den ipad!")
    #expect(ScreenID.valid(fresh))
    #expect(ScreenID.current(defaults) == fresh)
}

@Test func theVendorIDWinsWhenThereIsOne() throws {
    let defaults = try #require(UserDefaults(suiteName: "screen-vendor-\(UUID().uuidString)"))
    let vendor = "6F1C2D3E-0000-4A4B-8C8D-112233445566"
    #expect(ScreenID.current(defaults, vendor: vendor) == vendor)
    #expect(ScreenID.current(defaults, vendor: vendor) == vendor)
    // No vendor id (or one the server would refuse): the kept one, made once.
    let kept = ScreenID.current(defaults, vendor: nil)
    #expect(kept != vendor)
    #expect(ScreenID.valid(kept))
    #expect(ScreenID.current(defaults, vendor: "not ok") == kept)
}

@Test func screenIDsFollowTheServersRule() {
    #expect(ScreenID.valid("a"))
    #expect(ScreenID.valid("7F3A-b2"))
    #expect(ScreenID.valid(String(repeating: "x", count: 64)))
    #expect(!ScreenID.valid(""))
    #expect(!ScreenID.valid(String(repeating: "x", count: 65)))
    #expect(!ScreenID.valid("den ipad"))
    #expect(!ScreenID.valid("den_ipad"))
    #expect(!ScreenID.valid("café"))
}

@Test func hereCarriesTheIDOnlyWhenTheServerTakesIt() {
    #expect(EventSocket.here(id: "abc-1", name: "Den", kind: "appletv") == ["id": "abc-1", "name": "Den", "kind": "appletv"])
    #expect(EventSocket.here(id: "", name: "Den", kind: "appletv") == ["name": "Den", "kind": "appletv"])
    #expect(EventSocket.here(id: "not ok", name: "Den", kind: "appletv") == ["name": "Den", "kind": "appletv"])
}

@Test func theListLeavesOutThisScreen() {
    let screens = [
        Screen(id: "me", name: "Den iPhone", kind: "iphone", channelId: 3),
        Screen(id: "tv", name: "Living Room", kind: "appletv"),
        Screen(id: "", name: "Old app", kind: "ipad"),
    ]
    #expect(ScreenID.others(screens, except: "me").map(\.id) == ["tv"])
}

@MainActor @Test func aSentChannelIsHandedOverOnce() throws {
    let data = Data(#"{"channelId":5,"from":"Test iPhone"}"#.utf8)
    let sent = try JSONDecoder().decode(ScreenWatch.self, from: data)
    #expect(sent.channelId == 5)
    #expect(sent.note == "From Test iPhone")
    #expect(ScreenWatch(channelId: 5, from: "  ").note == nil)
    let nameless = try JSONDecoder().decode(ScreenWatch.self, from: Data(#"{"channelId":6}"#.utf8))
    #expect(nameless.channelId == 6)
    #expect(nameless.note == nil)

    let store = AppStore()
    store.noteScreenWatch(sent)
    #expect(store.screenWatch?.channelId == 5)
    #expect(store.takeScreenWatch()?.from == "Test iPhone")
    #expect(store.screenWatch == nil)
    #expect(store.takeScreenWatch() == nil)

    // The same channel again is a new send.
    let again = try JSONDecoder().decode(ScreenWatch.self, from: data)
    #expect(again != sent)
    store.noteScreenWatch(ScreenWatch(channelId: 0, from: "x"))
    #expect(store.screenWatch == nil)
}

@Test func aMoveCountsOnlyWhenTheOtherScreenPlaysTheChannel() {
    let screens = [
        Screen(id: "tv", name: "Living Room", kind: "appletv", channelId: 3),
        Screen(id: "me", name: "Den iPhone", kind: "iphone", channelId: 5),
    ]
    #expect(MoveCheck.started(screens, id: "tv", channel: 3))
    #expect(!MoveCheck.started(screens, id: "tv", channel: 5))
    #expect(!MoveCheck.started(screens, id: "gone", channel: 3))
    #expect(!MoveCheck.started([Screen(id: "tv", name: "Living Room", kind: "appletv")], id: "tv", channel: 3))
}

@MainActor @Test func theSenderWaitsForTheOtherScreenThenGivesUp() async {
    var reads = 0
    let late = await MoveCheck.confirm(id: "tv", channel: 3, every: .milliseconds(1)) {
        reads += 1
        // Asleep for two reads, a failed read, then playing.
        if reads == 3 {
            return nil
        }
        return [Screen(id: "tv", name: "Living Room", kind: "appletv", channelId: reads >= 4 ? 3 : nil)]
    }
    #expect(late)
    #expect(reads == 4)

    reads = 0
    let asleep = await MoveCheck.confirm(id: "tv", channel: 3, every: .milliseconds(1)) {
        reads += 1
        return [Screen(id: "tv", name: "Living Room", kind: "appletv")]
    }
    #expect(!asleep)
    #expect(reads == MoveCheck.attempts)
}

@Test func handoffOpensTheWebPlayerOnlyOverHTTP() throws {
    let base = try #require(URL(string: "http://192.168.1.20:8477"))
    #expect(WatchHandoff.webpageURL(base: base, channelID: 7)?.absoluteString == "http://192.168.1.20:8477/watch?channel=7")
    let slash = try #require(URL(string: "https://tv.example.com/"))
    #expect(WatchHandoff.webpageURL(base: slash, channelID: 12)?.absoluteString == "https://tv.example.com/watch?channel=12")
    let demo = try #require(URL(string: "broadwave-demo://local"))
    #expect(WatchHandoff.webpageURL(base: demo, channelID: 7) == nil)

    #expect(WatchHandoff.channel(["channelId": Int64(7), "serverId": "s1"], server: "s1") == 7)
    #expect(WatchHandoff.channel(["channelId": 7], server: "s1") == 7)
    #expect(WatchHandoff.channel(["channelId": NSNumber(value: 9)], server: nil) == 9)
    #expect(WatchHandoff.channel(["channelId": Int64(7), "serverId": "s2"], server: "s1") == nil)
    #expect(WatchHandoff.channel(["channelId": 0], server: "s1") == nil)
    #expect(WatchHandoff.channel(nil, server: "s1") == nil)
}

@Test func screensListsAndSendsAndShowsTheServersRefusal() async throws {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [ScreensStub.self]
    let api = try APIClient(base: #require(URL(string: "http://stub.invalid")), session: URLSession(configuration: config))

    ScreensStub.status = 200
    ScreensStub.payload = Data(#"""
    {"screens":[{"id":"tv-1","name":"Living Room","kind":"appletv","channelId":4},{"id":"ph-2","name":"Test iPhone","kind":"iphone"}]}
    """#.utf8)
    let screens = try await api.screens()
    #expect(screens.map(\.id) == ["tv-1", "ph-2"])
    #expect(screens[0].channelId == 4)
    #expect(screens[1].channelId == nil)
    #expect(ScreensStub.lastMethod == "GET")
    #expect(ScreensStub.lastPath == "/api/v1/screens")

    ScreensStub.status = 202
    ScreensStub.payload = Data()
    try await api.sendToScreen(id: "tv-1", channelId: 4, from: "Test iPhone")
    #expect(ScreensStub.lastMethod == "POST")
    #expect(ScreensStub.lastPath == "/api/v1/screens/tv-1/watch")
    let body = try JSONSerialization.jsonObject(with: #require(ScreensStub.lastBody)) as? [String: Any]
    #expect(body?["channelId"] as? Int == 4)
    #expect(body?["from"] as? String == "Test iPhone")

    ScreensStub.status = 404
    ScreensStub.payload = Data(#"{"code":"not_found","message":"That screen isn't open right now."}"#.utf8)
    do {
        try await api.sendToScreen(id: "tv-1", channelId: 4, from: "Test iPhone")
        Issue.record("a closed screen took the channel")
    } catch let error as APIError {
        #expect(error.status == 404)
        #expect(error.localizedDescription == "That screen isn't open right now.")
    }
}

private final class ScreensStub: URLProtocol, @unchecked Sendable {
    nonisolated(unsafe) static var payload = Data()
    nonisolated(unsafe) static var status = 200
    nonisolated(unsafe) static var lastMethod: String?
    nonisolated(unsafe) static var lastPath: String?
    nonisolated(unsafe) static var lastBody: Data?

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        Self.lastMethod = request.httpMethod
        Self.lastPath = request.url?.path
        Self.lastBody = request.httpBody ?? request.httpBodyStream.map(Self.read)
        let res = HTTPURLResponse(url: request.url!, statusCode: Self.status, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: res, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Self.payload)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}

    private static func read(_ stream: InputStream) -> Data {
        stream.open()
        defer { stream.close() }
        var data = Data()
        var buffer = [UInt8](repeating: 0, count: 4096)
        while stream.hasBytesAvailable {
            let n = stream.read(&buffer, maxLength: buffer.count)
            guard n > 0 else { break }
            data.append(buffer, count: n)
        }
        return data
    }
}

@MainActor @Test func aRemoteButtonIsHandedOverOnce() throws {
    let data = Data(#"{"action":"down","from":"Sam's Watch"}"#.utf8)
    let pressed = try JSONDecoder().decode(ScreenRemote.self, from: data)
    #expect(pressed.action == .down)
    #expect(pressed.from == "Sam's Watch")
    #expect(try JSONDecoder().decode(ScreenRemote.self, from: Data(#"{"action":"pause"}"#.utf8)).from == "")
    // A newer server's button is not this app's to guess at.
    #expect(throws: (any Error).self) { try JSONDecoder().decode(ScreenRemote.self, from: Data(#"{"action":"eject"}"#.utf8)) }

    let store = AppStore()
    store.noteScreenRemote(pressed)
    #expect(store.takeScreenRemote()?.action == .down)
    #expect(store.screenRemote == nil)
    #expect(store.takeScreenRemote() == nil)
    // The same button again is a new press.
    #expect(try JSONDecoder().decode(ScreenRemote.self, from: data) != pressed)
}
