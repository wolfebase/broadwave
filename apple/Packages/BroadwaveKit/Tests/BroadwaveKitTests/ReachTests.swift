@testable import BroadwaveKit
import Foundation
import Testing

@Test func reconnectWaitStaysInsideTheCap() {
    #expect(EventSocket.reconnectWait(0, random: 0) == 0.25)
    #expect(EventSocket.reconnectWait(0, random: 1) == 0.5)
    #expect(EventSocket.reconnectWait(16, random: 0) == 2.5)
    #expect(EventSocket.reconnectWait(16, random: 1) == 5)
    #expect(EventSocket.reconnectWait(100, random: 0) == 2.5)
    #expect(EventSocket.reconnectWait(100, random: 1) == 5)
    // A negative retry makes the exponent negative. The wait stays finite.
    let early = EventSocket.reconnectWait(-1, random: 0)
    #expect(early.isFinite && early >= 0)
    let earlyFull = EventSocket.reconnectWait(-1, random: 1)
    #expect(earlyFull.isFinite && earlyFull >= 0)
}

@Test func serverURLKeepsAUniqueLocalStripsTheZoneAndDropsLinkLocal() {
    #expect(Discovery.serverURL(host: "fd12::5%en1", port: 9)?.absoluteString == "http://[fd12::5]:9")
    #expect(Discovery.serverURL(host: "fe80::5%en1", port: 9) == nil)
    #expect(Discovery.serverURL(host: "10.1.2.3", port: 8477)?.absoluteString == "http://10.1.2.3:8477")
}

@Test func localHostsAreLoopbackOrPrivate() {
    #expect(FinderPacket.isLocalHost("10.1.2.3"))
    #expect(FinderPacket.isLocalHost("127.0.0.1"))
    #expect(FinderPacket.isLocalHost("192.168.50.2"))
    #expect(FinderPacket.isLocalHost("172.16.0.1"))
    #expect(FinderPacket.isLocalHost("172.31.255.255"))
    #expect(!FinderPacket.isLocalHost("172.15.0.1"))
    #expect(!FinderPacket.isLocalHost("172.32.0.1"))
    #expect(!FinderPacket.isLocalHost("192.169.1.1"))
    #expect(!FinderPacket.isLocalHost("11.0.0.1"))
    #expect(!FinderPacket.isLocalHost("169.254.1.1"))
    #expect(!FinderPacket.isLocalHost("::1"))
    #expect(!FinderPacket.isLocalHost("example.test"))
    #expect(!FinderPacket.isLocalHost("10.0.0.256"))
}

@Test func aProbeURLMustBeLocalHTTPWithoutCredentials() throws {
    let local = try #require(URL(string: "http://10.1.2.3:8477"))
    #expect(FinderPacket.isLocal(local))
    let creds = try #require(URL(string: "http://user:secret@10.1.2.3:8477"))
    #expect(!FinderPacket.isLocal(creds))
    let ftp = try #require(URL(string: "ftp://10.1.2.3:8477"))
    #expect(!FinderPacket.isLocal(ftp))
    let pub = try #require(URL(string: "http://203.0.113.9:9"))
    #expect(!FinderPacket.isLocal(pub))
}

@Test func theSamePlaceIgnoresDefaultPortHostCaseAndPath() throws {
    let bare = try #require(URL(string: "http://10.1.2.3"))
    let port80 = try #require(URL(string: "http://10.1.2.3:80"))
    #expect(ServerFollow.samePlace(bare, port80))

    let tls = try #require(URL(string: "https://10.1.2.3"))
    let port443 = try #require(URL(string: "https://10.1.2.3:443"))
    #expect(ServerFollow.samePlace(tls, port443))

    let http = try #require(URL(string: "http://10.1.2.3:8477"))
    let https = try #require(URL(string: "https://10.1.2.3:8477"))
    #expect(!ServerFollow.samePlace(http, https))

    let upper = try #require(URL(string: "http://Example.Test:8477"))
    let lower = try #require(URL(string: "http://example.test:8477"))
    #expect(ServerFollow.samePlace(upper, lower))

    let other = try #require(URL(string: "http://10.1.2.3:8478"))
    #expect(!ServerFollow.samePlace(http, other))

    let guide = try #require(URL(string: "http://10.1.2.3:8477/guide"))
    let player = try #require(URL(string: "http://10.1.2.3:8477/player"))
    #expect(ServerFollow.samePlace(guide, player))

    let v6 = try #require(URL(string: "http://[fd12::5]:8477"))
    let v6Upper = try #require(URL(string: "http://[FD12::5]:8477/guide"))
    #expect(ServerFollow.samePlace(v6, v6Upper))
}
