@testable import BroadwaveKit
import Foundation
import Testing

@Test func supportURLIsOnTheServerOrigin() throws {
    let base = try #require(URL(string: "http://127.0.0.1:9/api/v1"))
    let support = APIClient(base: base).supportURL()
    #expect(support.path == "/api/v1/support")
    #expect(support.host() == "127.0.0.1")
}

@Test func frameURLUsesTheTwoStoredWidths() throws {
    let base = try #require(URL(string: "http://127.0.0.1:9/api/v1"))
    let api = APIClient(base: base)
    let hero = api.frameURL(channelID: 4, width: 1600)
    let card = api.frameURL(channelID: 4, width: 640)
    #expect(hero.path == "/api/v1/channels/4/frame")
    #expect(hero.query() == "w=1280")
    #expect(card.query() == "w=480")
    #expect(hero.host() == "127.0.0.1")
}
