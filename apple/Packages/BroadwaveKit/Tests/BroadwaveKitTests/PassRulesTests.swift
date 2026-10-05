@testable import BroadwaveKit
import Foundation
import Testing

@Test func addSeriesPassSendsItsRules() async throws {
    let api = try rulesClient("add")
    let rules = NewPass(title: "News", channelId: 0, padBefore: 1, padAfter: 2, matchKind: "category", keepMode: "last", keepCount: 3,
                        limitCount: 0, timeStart: "18:00", timeEnd: "23:00", days: [1, 3])
    let passes = try await api.addSeriesPass(rules)
    #expect(passes.map(\.id) == [7, 3])
    let sent = try RulesStub.sent("add")
    #expect(sent.method == "POST")
    #expect(sent.path == "/api/v1/passes")
    #expect(sent.body["title"] as? String == "News")
    #expect(sent.body["matchKind"] as? String == "category")
    #expect(sent.body["days"] as? [Int] == [1, 3])
    #expect(sent.body["timeStart"] as? String == "18:00")
    #expect(sent.body["timeEnd"] as? String == "23:00")
    #expect(sent.body["keepCount"] as? Int == 3)
    #expect(sent.body["limitCount"] as? Int == 0)
    #expect(sent.body["airingStart"] == nil)
    #expect(sent.body["priority"] == nil)
}

@Test func orderPassesNamesEveryPassFirstHighest() async throws {
    let api = try rulesClient("order")
    _ = try await api.orderPasses([3, 7])
    let sent = try RulesStub.sent("order")
    #expect(sent.method == "PUT")
    #expect(sent.path == "/api/v1/passes/order")
    #expect(sent.body["ids"] as? [Int] == [3, 7])
    #expect(sent.body.count == 1)
}

@Test func previewPassSendsTheRulesAndTheEditedPass() async throws {
    let api = try rulesClient("preview")
    let preview = try await api.previewPass(NewPass(title: "Chiefs", matchKind: "contains"), id: 7)
    #expect(preview.tunerCount == 2)
    #expect(preview.items.count == 1)
    #expect(preview.items.first?.skipped == true)
    #expect(preview.items.first?.conflict == true)
    #expect(preview.bumps.isEmpty)
    var sent = try RulesStub.sent("preview")
    #expect(sent.method == "POST")
    #expect(sent.path == "/api/v1/passes/preview")
    #expect(sent.body["id"] as? Int == 7)
    #expect(sent.body["title"] as? String == "Chiefs")
    #expect(sent.body["matchKind"] as? String == "contains")
    #expect(sent.body["rename"] as? String == "Chiefs")
    #expect(preview.timeZone == "America/Chicago")

    _ = try await api.previewPass(NewPass(title: "Chiefs"))
    sent = try RulesStub.sent("preview")
    #expect(sent.body["id"] == nil)
    #expect(sent.body["rename"] == nil)
}

@Test func updatePassSendsOnlyTheChangedRule() async throws {
    let api = try rulesClient("update")
    _ = try await api.updatePass(Pass(id: 7, title: "Evening news"))
    var sent = try RulesStub.sent("update")
    #expect(sent.method == "PATCH")
    #expect(sent.path == "/api/v1/passes/7")
    #expect(Set(sent.body.keys) == ["id", "title"])
    #expect(sent.body["title"] as? String == "Evening news")

    _ = try await api.updatePass(Pass(id: 7, title: "Evening news", timeStart: "", timeEnd: "", days: []))
    sent = try RulesStub.sent("update")
    #expect(Set(sent.body.keys) == ["id", "title", "timeStart", "timeEnd", "days"])
    #expect(sent.body["timeStart"] as? String == "")
    #expect(sent.body["days"] as? [Int] == [])

    _ = try await api.updatePass(Pass(id: 7, title: "Evening news", padBefore: 5))
    sent = try RulesStub.sent("update")
    #expect(Set(sent.body.keys) == ["id", "title", "padBefore"])

    _ = try await api.renamePass(7, to: "Late news")
    sent = try RulesStub.sent("update")
    #expect(sent.method == "PATCH")
    #expect(sent.body.count == 1)
    #expect(sent.body["rename"] as? String == "Late news")
}

@Test func passLabelsMatchTheWeb() {
    #expect(Pass(id: 1, title: "Jeopardy!", kind: "series", matchKind: "title").label == "Jeopardy!")
    #expect(Pass(id: 1, title: "Chiefs", kind: "series", matchKind: "contains").label == "Titles with “Chiefs”")
    #expect(Pass(id: 1, title: "News", kind: "series", matchKind: "category").label == "News (category)")
    #expect(Pass(id: 1, title: "Chiefs", kind: "team", matchKind: "team").label == "Chiefs games")
}

@Test func passDetailsNameEveryRule() {
    let pass = Pass(id: 1, title: "News", kind: "series", padBefore: 1, padAfter: 2, episodes: "new", keepMode: "last", keepCount: 3,
                    limitCount: 4, timeStart: "18:00", timeEnd: "23:30", matchKind: "category", days: [5, 1, 2, 3, 4])
    let parts = pass.details(channel: nil).components(separatedBy: " · ")
    let window = "\(Pass.clockLabel("18:00"))–\(Pass.clockLabel("23:30"))"
    #expect(parts == ["Any channel", "Weekdays", window, "New only", "Keeps the newest 3", "Stops at 4 unwatched", "1 min early, 2 after"])
    #expect(Pass(id: 1, title: "x", channelId: 4).details(channel: "4.1 WDAF") == "4.1 WDAF · 0 min early, 0 after")
    #expect(Pass(id: 1, title: "x", kind: "once", padBefore: 1, padAfter: 2).details(channel: "4.1 WDAF") == "1 min early, 2 after")
}

@Test func daysLabelNamesCommonSets() {
    #expect(Pass.daysLabel([]) == "Every day")
    #expect(Pass.daysLabel([0, 1, 2, 3, 4, 5, 6]) == "Every day")
    #expect(Pass.daysLabel([5, 4, 3, 2, 1]) == "Weekdays")
    #expect(Pass.daysLabel([6, 0]) == "Weekends")
    let names = Pass.dayNames
    #expect(Pass.daysLabel([3, 1]) == "\(names[1]), \(names[3])")
    #expect(Pass.clockLabel("bad") == "bad")
}

@Test func rulesKeepWhatEachKindTakes() {
    let once = Pass(id: 1, title: "Jeopardy!", channelId: 4, kind: "once", padBefore: 3, padAfter: 4, keepMode: "last", days: [1])
    let onceRules = once.rules
    #expect(onceRules.padBefore == 3)
    #expect(onceRules.days == nil)
    #expect(onceRules.matchKind == nil)
    #expect(onceRules.channelId == nil)

    let team = Pass(id: 2, title: "Chiefs", kind: "team", matchKind: "team").rules
    #expect(team.matchKind == nil)
    #expect(team.title == "Chiefs")

    let series = Pass(id: 3, title: " News ", kind: "series", keepMode: "all", keepCount: 4, matchKind: "category", days: [3, 1]).rules
    #expect(series.title == "News")
    #expect(series.keepCount == 0)
    #expect(series.days == [1, 3])
    #expect(series.channelId == 0)
    #expect(series.timeStart == "")
}

@Test func wordAndCategoryPassesAreNotTheTitlesSeriesPass() {
    let airing = Airing(id: 1, channelId: 4, title: "News", start: Date(), end: Date().addingTimeInterval(1800))
    let category = Pass(id: 1, title: "News", kind: "series", matchKind: "category")
    let words = Pass(id: 2, title: "News", kind: "series", matchKind: "contains")
    let title = Pass(id: 3, title: "news", kind: "series", matchKind: "title")
    #expect(Pass.series(in: [category, words], for: airing) == nil)
    #expect(Pass.series(in: [category, words, title], for: airing)?.id == 3)
}

@Test func demoAnswersOrderAndPreview() async throws {
    let server = DemoServer()
    let port = UInt16.random(in: 20000 ... 45000)
    let origin = try #require(await server.prepare(port: port))
    defer { server.stop() }
    let client = APIClient(base: origin)
    #expect(try await client.orderPasses([]).isEmpty)
    let preview = try await client.previewPass(NewPass(title: "News", matchKind: "category"))
    #expect(preview.items.isEmpty)
    #expect(preview.bumps.isEmpty)
}

private func rulesClient(_ host: String) throws -> APIClient {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [RulesStub.self]
    return try APIClient(base: #require(URL(string: "http://\(host).invalid")), session: URLSession(configuration: config))
}

/// Answers every request and keeps the last one per host, so tests running at once don't read each other's.
private final class RulesStub: URLProtocol, @unchecked Sendable {
    struct Sent {
        var method: String
        var path: String
        var body: [String: Any]
    }

    private static let lock = NSLock()
    private nonisolated(unsafe) static var last: [String: URLRequest] = [:]
    private nonisolated(unsafe) static var bodies: [String: Data] = [:]

    static func sent(_ host: String) throws -> Sent {
        let key = "\(host).invalid"
        let (hit, data) = lock.withLock { (last[key], bodies[key] ?? Data()) }
        let req = try #require(hit)
        let body = data.isEmpty ? [:] : try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        return Sent(method: req.httpMethod ?? "GET", path: req.url?.path ?? "", body: body)
    }

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        let url = request.url!
        let body = request.httpBody ?? request.httpBodyStream.map(Self.read) ?? Data()
        Self.lock.withLock {
            Self.last[url.host ?? ""] = request
            Self.bodies[url.host ?? ""] = body
        }
        let json = url.path.hasSuffix("/preview")
            ? #"{"tunerCount":2,"items":[{"passId":7,"airing":{"id":1,"channelId":4,"title":"Chiefs at Bills","start":"2026-10-11T17:00:00Z","end":"2026-10-11T20:00:00Z"},"priority":1,"padBefore":1,"padAfter":2,"conflict":true,"skipped":true}],"bumps":[],"timeZone":"America/Chicago","utcOffset":-18000}"#
            : #"{"passes":[{"id":7,"title":"News","kind":"series","matchKind":"category"},{"id":3,"title":"Jeopardy!","kind":"series"}]}"#
        let res = HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
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
