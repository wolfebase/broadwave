@testable import BroadwaveKit
import Foundation
import Testing

private let start = Date(timeIntervalSince1970: 1_790_000_000)

@Test func oncePassMatchesOnlyItsAiring() {
    let airing = Airing(id: 1, channelId: 4, title: "Jeopardy!", start: start, end: start.addingTimeInterval(1800))
    let later = Airing(id: 2, channelId: 4, title: "Jeopardy!", start: start.addingTimeInterval(86400), end: start.addingTimeInterval(88200))
    let elsewhere = Airing(id: 3, channelId: 5, title: "Jeopardy!", start: start, end: start.addingTimeInterval(1800))
    let once = Pass(id: 9, title: "Jeopardy!", channelId: 4, kind: "once", airingStart: start)
    #expect(Pass.once(in: [once], for: airing)?.id == 9)
    #expect(Pass.once(in: [once], for: later) == nil)
    #expect(Pass.once(in: [once], for: elsewhere) == nil)
    #expect(Pass.series(in: [once], for: airing) == nil)
}

@Test func seriesPassIgnoresCaseAndOncePasses() {
    let airing = Airing(id: 1, channelId: 4, title: "Jeopardy!", start: start, end: start.addingTimeInterval(1800))
    let series = Pass(id: 3, title: "jeopardy!", channelId: 4, kind: "series")
    let legacy = Pass(id: 4, title: "Jeopardy!")
    #expect(Pass.series(in: [series], for: airing)?.id == 3)
    #expect(Pass.series(in: [legacy], for: airing)?.id == 4)
    #expect(Pass.once(in: [series, legacy], for: airing) == nil)
}

@Test func oncePassLabelNamesTheAiring() {
    let once = Pass(id: 9, title: "Jeopardy!", channelId: 4, kind: "once", airingStart: start)
    let when = start.formatted(.dateTime.weekday(.abbreviated).hour().minute())
    #expect(once.label == "Jeopardy! · \(when) only")
    #expect(Pass(id: 3, title: "Jeopardy!", kind: "series").label == "Jeopardy!")
}

@Test func addPassSendsTheAiringStartAsAnInstant() async throws {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [PassStub.self]
    let api = try APIClient(base: #require(URL(string: "http://stub.invalid")), session: URLSession(configuration: config))
    let passes = try await api.addPass(title: "Jeopardy!", channelID: 4, airingStart: start)
    #expect(passes.first?.kind == "once")
    let body = try #require(PassStub.lastBody)
    let sent = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
    #expect(sent["airingStart"] as? String == ISO8601DateFormatter.plain.string(from: start))
    #expect(sent["channelId"] as? Int == 4)

    _ = try await api.addPass(title: "Jeopardy!", channelID: 4)
    let seriesBody = try #require(PassStub.lastBody)
    let series = try #require(JSONSerialization.jsonObject(with: seriesBody) as? [String: Any])
    #expect(series["airingStart"] == nil)
}

private final class PassStub: URLProtocol, @unchecked Sendable {
    nonisolated(unsafe) static var lastBody: Data?

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        Self.lastBody = request.httpBody ?? request.httpBodyStream.map(Self.read)
        let json = #"{"passes":[{"id":9,"title":"Jeopardy!","channelId":4,"kind":"once","airingStart":"2026-09-21T13:33:20Z"}]}"#
        let res = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: res, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(json.utf8))
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
