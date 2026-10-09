@testable import BroadwaveKit
import CryptoKit
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

@Test func sharedAddressSpaceCountsAsALocalHost() {
    // The server answers on 100.64.0.0/10. Go's IsPrivate includes that range.
    #expect(FinderPacket.isLocalHost("100.64.0.1"))
    #expect(FinderPacket.isLocalHost("100.127.255.255"))
    #expect(!FinderPacket.isLocalHost("100.63.255.255"))
    #expect(!FinderPacket.isLocalHost("100.128.0.1"))
    let raw = Data(#"BWDP!{"id":"abc","name":"Home","url":"http://100.64.1.5:8477"}"#.utf8)
    #expect(FinderPacket.parse(raw)?.url.host() == "100.64.1.5")
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

@Test func aProbeSignatureMustMatchTheStoredKey() throws {
    let key = Curve25519.Signing.PrivateKey()
    let pub = key.publicKey.rawRepresentation.base64EncodedString()
    let nonce = Data(repeating: 7, count: 16)
    let url = "http://10.1.2.3:8477"
    let id = "lab"
    let message = FinderPacket.message(nonce: nonce, url: url, id: id)
    let sig = try key.signature(for: message).base64EncodedString()
    #expect(FinderPacket.accepts(sig, nonce: nonce, url: url, id: id, key: pub))
    #expect(!FinderPacket.accepts(sig, nonce: nonce, url: url, id: "other", key: pub))
    #expect(!FinderPacket.accepts(sig, nonce: nonce, url: "http://10.1.2.9:8477", id: id, key: pub))
    #expect(!FinderPacket.accepts(sig, nonce: Data(repeating: 7, count: 15), url: url, id: id, key: pub))
    #expect(!FinderPacket.accepts(sig, nonce: nonce, url: url, id: id, key: "not-a-key"))
    #expect(!FinderPacket.accepts("%%%", nonce: nonce, url: url, id: id, key: pub))
    let other = Curve25519.Signing.PrivateKey().publicKey.rawRepresentation.base64EncodedString()
    #expect(!FinderPacket.accepts(sig, nonce: nonce, url: url, id: id, key: other))
}

@Test @MainActor func aStaleClockSampleAfterAWakeIsIgnored() throws {
    let base = try #require(URL(string: "http://10.1.2.3:18940"))
    let socket = EventSocket(base: base)
    let burst = EventSocket.nowMS()
    socket.beginClockBurst(at: burst)
    let stale = #"{"type":"clock","data":{"t0":\#(burst - 5000),"t1":\#(burst - 5000)}}"#
    socket.applyFrame(Data(stale.utf8))
    #expect(socket.offset == 0)

    let t0 = EventSocket.nowMS()
    let fresh = #"{"type":"clock","data":{"t0":\#(t0),"t1":\#(t0 + 40)}}"#
    socket.applyFrame(Data(fresh.utf8))
    #expect(abs(socket.offset - 40) < 30)
}

@Test @MainActor func aRestartedServerDropsTheRoomSnapshot() throws {
    let base = try #require(URL(string: "http://10.1.2.3:18940"))
    let socket = EventSocket(base: base)
    socket.applyFrame(Data(#"{"type":"sync.state","data":{"room":"den","rate":1}}"#.utf8))
    #expect(socket.roomState("den") != nil)
    socket.applyFrame(Data(#"{"type":"hello","data":{"boot":"a"}}"#.utf8))
    #expect(socket.roomState("den") != nil)
    socket.applyFrame(Data(#"{"type":"hello","data":{"boot":"a"}}"#.utf8))
    #expect(socket.roomState("den") != nil)
    socket.applyFrame(Data(#"{"type":"hello","data":{"boot":"b"}}"#.utf8))
    #expect(socket.roomState("den") == nil)
}

@Test @MainActor func aCommandWhileTheSocketIsDownWaitsAFewSeconds() throws {
    let base = try #require(URL(string: "http://10.1.2.3:18940"))
    let socket = EventSocket(base: base)
    socket.command(room: "group:den", action: "pause")
    let now = EventSocket.nowMS()
    let waiting = socket.pendingCommandTexts(at: now)
    #expect(waiting.count == 1)
    #expect(waiting[0].contains("\"pause\""))
    #expect(socket.pendingCommandTexts(at: now + 3000).isEmpty)
    #expect(EventSocket.freshCommands([(text: "{\"type\":\"sync.command\"}", at: now - 3000)], at: now).isEmpty)
    #expect(EventSocket.freshCommands([(text: "keep", at: now - 2999)], at: now) == ["keep"])
}
