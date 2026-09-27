@testable import BroadwaveKit
import Foundation
import Testing

@Test func backupURLsStayOnTheServerOrigin() throws {
    let base = try #require(URL(string: "http://127.0.0.1:9/api/v1"))
    let api = APIClient(base: base)
    let named = api.backupURL(name: "broadwave-20260926-daily.db")
    #expect(named.path == "/api/v1/backups/broadwave-20260926-daily.db")
    #expect(named.host() == "127.0.0.1")
    let catalog = api.catalogBackupURL()
    #expect(catalog.path == "/api/v1/backup")
    #expect(catalog.host() == "127.0.0.1")
    let escaped = api.backupURL(name: "a/../b.db")
    #expect(escaped.absoluteString.hasSuffix("/api/v1/backups/a%2F..%2Fb.db"))
    #expect(!escaped.absoluteString.contains("/../"))
}

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

@Test func frameURLIsOmittedUntilTheChannelIsListed() throws {
    let base = try #require(URL(string: "http://127.0.0.1:9/api/v1"))
    let api = APIClient(base: base)
    #expect(api.frameURL(channelID: 4, width: 480, listed: []) == nil)
    #expect(api.frameURL(channelID: 5, width: 1280, listed: [4]) == nil)
    let card = try #require(api.frameURL(channelID: 4, width: 640, listed: [4, 9]))
    #expect(card.query() == "w=480")
    let hero = try #require(api.frameURL(channelID: 4, width: 1600, listed: [4]))
    #expect(hero.query() == "w=1280")
}
