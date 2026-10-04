@testable import BroadwaveKit
import Foundation
import Testing

@Test func watchAnswerKeepsTheServerProcess() throws {
    let json = """
    {"channelId":4,"playlist":"/live/4/1080.aac2.broadcast/index.m3u8","rendition":"1080.aac2.broadcast","stream":{"rendition":"1080.aac2.broadcast","video":"1080","audio":"aac2","reason":"Original picture"},"encoder":"copy","shared":true,"viewers":1,"boot":"process-a"}
    """
    let session = try APIClient.decoder.decode(WatchSession.self, from: Data(json.utf8))
    #expect(session.boot == "process-a")
    #expect(session.rendition == "1080.aac2.broadcast")
    #expect(session.channelId == 4)
}

@Test func stopWatchingNamesTheServerProcess() async throws {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [WatchStopStub.self]
    let api = try APIClient(base: #require(URL(string: "http://stub.invalid")), session: URLSession(configuration: config))
    let session = WatchSession(
        channelId: 4,
        playlist: "/live/4/index.m3u8",
        rendition: "720.aac2",
        stream: StreamInfo(rendition: "720.aac2", video: "720", audio: "aac2", reason: "Original picture"),
        encoder: "copy",
        shared: false,
        viewers: 1,
        boot: "process-a"
    )
    await api.stopWatching(session)
    let body = try #require(WatchStopStub.lastBody)
    let sent = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
    #expect(sent["rendition"] as? String == "720.aac2")
    #expect(sent["boot"] as? String == "process-a")
    #expect(WatchStopStub.lastPath == "/api/v1/watch/4/stop")

    await api.stopWatching(channelID: 4, rendition: "720.aac2", boot: "")
    let emptyBody = try #require(WatchStopStub.lastBody)
    let empty = try #require(JSONSerialization.jsonObject(with: emptyBody) as? [String: Any])
    #expect(empty["boot"] as? String == "")
    #expect(empty["rendition"] as? String == "720.aac2")
}

@Test func warmSendsThePlayersCapabilities() async throws {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [WarmStub.self]
    let api = try APIClient(base: #require(URL(string: "http://stub.invalid")), session: URLSession(configuration: config))
    let started = await api.warm(channelID: 7, caps: Capabilities.current(), prefs: Prefs(quality: .high))
    #expect(started)
    #expect(WarmStub.lastPath == "/api/v1/watch/7/warm")
    let body = try #require(WarmStub.lastBody)
    let sent = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
    let caps = try #require(sent["caps"] as? [String: Any])
    #expect(caps["alternates"] as? Bool == true)
    #expect((sent["prefs"] as? [String: Any])?["quality"] as? String == "high")
}

@Test func watchNamesItsRoomOnlyWhenItSyncs() async throws {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [WatchRoomStub.self]
    let api = try APIClient(base: #require(URL(string: "http://stub.invalid")), session: URLSession(configuration: config))
    _ = try await api.watch(channelID: 4, caps: Capabilities.current(), prefs: Prefs(), room: "channel:4")
    let body = try #require(WatchRoomStub.lastBody)
    let sent = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
    #expect(sent["room"] as? String == "channel:4")
    #expect(WatchRoomStub.lastPath == "/api/v1/watch")

    _ = try await api.watch(channelID: 4, caps: Capabilities.current(), prefs: Prefs())
    let aloneBody = try #require(WatchRoomStub.lastBody)
    let alone = try #require(JSONSerialization.jsonObject(with: aloneBody) as? [String: Any])
    #expect(alone["room"] == nil)
}

private final class WatchStopStub: URLProtocol, @unchecked Sendable {
    nonisolated(unsafe) static var lastBody: Data?
    nonisolated(unsafe) static var lastPath: String?

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        Self.lastBody = request.httpBody ?? request.httpBodyStream.map(Self.read)
        Self.lastPath = request.url?.path
        let res = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: res, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data("{}".utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}

    fileprivate static func read(_ stream: InputStream) -> Data {
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

/// Its own statics: Swift Testing runs the stop test beside this one.
private final class WarmStub: URLProtocol, @unchecked Sendable {
    nonisolated(unsafe) static var lastBody: Data?
    nonisolated(unsafe) static var lastPath: String?

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        Self.lastBody = request.httpBody ?? request.httpBodyStream.map(WatchStopStub.read)
        Self.lastPath = request.url?.path
        let res = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: res, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(#"{"warm":true}"#.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

private final class WatchRoomStub: URLProtocol, @unchecked Sendable {
    nonisolated(unsafe) static var lastBody: Data?
    nonisolated(unsafe) static var lastPath: String?

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        Self.lastBody = request.httpBody ?? request.httpBodyStream.map(WatchStopStub.read)
        Self.lastPath = request.url?.path
        let json = #"{"channelId":4,"playlist":"/live/4/index.m3u8","rendition":"720.aac2","stream":{"rendition":"720.aac2","video":"720","audio":"aac2","reason":"Original picture"},"encoder":"copy","shared":false,"viewers":1}"#
        let res = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: res, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(json.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}
