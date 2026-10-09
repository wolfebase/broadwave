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

/// `warm()` reads this bool. The contract file was never decoded.
@Test func warmAnswerDecodesTheFixture() throws {
    struct Warm: Decodable { var warm: Bool }
    let warm = try APIClient.decoder.decode(Warm.self, from: fixture("watch-warm"))
    #expect(!warm.warm)
}

/// `addFree` keeps only the note. The generated `Source` type does not fit this body.
@Test func freeAddKeepsTheNoteTheClientReads() throws {
    struct Note: Decodable { var message: String? }
    let note = try APIClient.decoder.decode(Note.self, from: fixture("free-add"))
    #expect(note.message == "")
    let added = try APIClient.decoder.decode(APIClient.SourceAdd.self, from: fixture("free-add"))
    #expect(added.message == "")
    #expect(added.pick == nil)
}

/// The socket reads `boot` from hello and `t0`/`t1` from clock. Tests only checked the type string.
@Test func helloAndClockFramesCarryWhatTheSocketReads() throws {
    struct Hello: Decodable {
        struct Body: Decodable { var boot: String }
        var type: String
        var data: Body
    }
    struct Clock: Decodable {
        struct Body: Decodable { var t0: Double; var t1: Double }
        var type: String
        var data: Body
    }
    let hello = try APIClient.decoder.decode(Hello.self, from: fixture("ws-hello"))
    #expect(hello.type == "hello")
    #expect(hello.data.boot == "contract")
    let clock = try APIClient.decoder.decode(Clock.self, from: fixture("ws-clock"))
    #expect(clock.type == "clock")
    #expect(clock.data.t0 == 1000)
    #expect(clock.data.t1 == 1_790_262_000_000)
}

/// `free.json` lists no feeds, so `FreeFeed` itself was never built.
@Test func aFreeFeedDecodesTheHandlerShape() throws {
    let feed = try APIClient.decoder.decode(APIClient.FreeFeed.self, from: Data("""
    {"kind":"fastchannels","name":"FastChannels","addr":"http://127.0.0.1:5523","playlist":"http://127.0.0.1:5523/feeds/default/m3u","guide":"http://127.0.0.1:5523/feeds/default/epg.xml"}
    """.utf8))
    #expect(feed.kind == "fastchannels")
    #expect(feed.name == "FastChannels")
    #expect(feed.addr == "http://127.0.0.1:5523")
    #expect(feed.playlist.hasSuffix("/feeds/default/m3u"))
    #expect(feed.guide.hasSuffix("/feeds/default/epg.xml"))
}

/// `multiview.json` omits offers and stops. The handler sends both when the picker has rows.
@Test func multiviewOffersAndStopsDecodeTheHandlerShape() throws {
    let plan = try APIClient.decoder.decode(MultiviewPlan.self, from: Data("""
    {"playable":[{"channelId":1,"frequencyHz":0,"shared":false}],"blocked":[],"tunersNeeded":1,"tunersFree":1,"offers":[{"channelId":2,"cost":"same","label":"Same tune as 4.1"}],"stops":[{"channelId":1,"reason":"4.1 stops at 3:00 PM. Quiz Hour is recording.","at":"2026-09-25T15:00:00Z"}]}
    """.utf8))
    let offer = try #require(plan.offers?.first)
    #expect(offer.channelId == 2)
    #expect(offer.cost == "same")
    #expect(offer.label == "Same tune as 4.1")
    let stop = try #require(plan.stops?.first)
    #expect(stop.channelId == 1)
    #expect(stop.reason == "4.1 stops at 3:00 PM. Quiz Hour is recording.")
    #expect(stop.at == ISO8601DateFormatter.plain.date(from: "2026-09-25T15:00:00Z"))
}
