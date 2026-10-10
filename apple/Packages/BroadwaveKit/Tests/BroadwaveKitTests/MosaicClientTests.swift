@testable import BroadwaveKit
import Foundation
import Testing

@Test func mosaicAnswerDecodesThePlaylist() throws {
    let json = """
    {"key":"4-12","playlist":"/media/mosaic/4-12/index.m3u8","channelIds":[4,12],"encoder":"libx264","viewers":1}
    """
    let session = try APIClient.decoder.decode(MosaicSession.self, from: Data(json.utf8))
    #expect(session.key == "4-12")
    #expect(session.playlist == "/media/mosaic/4-12/index.m3u8")
    #expect(session.channelIds == [4, 12])
    #expect(session.encoder == "libx264")
    #expect(session.viewers == 1)
}

@Test func watchMosaicSendsTheSoundChannelFirstAndStopNamesTheKey() async throws {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [MosaicStub.self]
    let api = try APIClient(base: #require(URL(string: "http://stub.invalid")), session: URLSession(configuration: config))
    let session = try await api.watchMosaic(channelIDs: [4, 12])
    #expect(session.key == "4-12")
    #expect(session.playlist == "/media/mosaic/4-12/index.m3u8")
    #expect(MosaicStub.lastPath == "/api/v1/mosaic")
    let body = try #require(MosaicStub.lastBody)
    let sent = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
    let ids = (sent["channelIds"] as? [NSNumber])?.map(\.int64Value)
    #expect(ids == [4, 12])

    await api.stopMosaic(key: session.key)
    #expect(MosaicStub.lastPath == "/api/v1/mosaic/4-12/stop")
}

private final class MosaicStub: URLProtocol, @unchecked Sendable {
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
        let json = request.url?.path.hasSuffix("/stop") == true
            ? #"{"ok":true}"#
            : #"{"key":"4-12","playlist":"/media/mosaic/4-12/index.m3u8","channelIds":[4,12],"encoder":"libx264","viewers":1}"#
        let res = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: res, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(json.utf8))
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
