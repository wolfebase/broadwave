@testable import BroadwaveKit
import CryptoKit
import Foundation
import Testing

private func signingKey() -> (privateKey: Curve25519.Signing.PrivateKey, publicKey: String) {
    let privateKey = Curve25519.Signing.PrivateKey()
    return (privateKey, privateKey.publicKey.rawRepresentation.base64EncodedString())
}

private func signedServer(id: String, url: String, privateKey: Curve25519.Signing.PrivateKey, nonce: Data) throws -> FoundServer {
    let message = FinderPacket.message(nonce: nonce, url: url, id: id)
    let signature = try privateKey.signature(for: message).base64EncodedString()
    return try FoundServer(
        id: id, name: "Living Room", url: #require(URL(string: url)),
        signature: signature, nonce: nonce, signedURL: url
    )
}

@Test func probeReplyParsesTheServerPacket() throws {
    let raw = Data(#"BWDP!{"id":"abc","name":"Living Room","url":"http://127.0.0.1:18477"}"#.utf8)
    let reply = try #require(FinderPacket.parse(raw))
    #expect(reply.id == "abc")
    #expect(reply.name == "Living Room")
    #expect(reply.url.port == 18477)
    #expect(reply.url.host() == "127.0.0.1")
}

@Test func probeIgnoresGarbageAndSecrets() {
    #expect(FinderPacket.parse(Data("hello".utf8)) == nil)
    #expect(FinderPacket.parse(Data("BWDP?".utf8)) == nil)
    #expect(FinderPacket.parse(Data("BWDP!{}".utf8)) == nil)
    let creds = Data(#"BWDP!{"id":"a","name":"n","url":"http://user:pass@127.0.0.1:9"}"#.utf8)
    #expect(FinderPacket.parse(creds) == nil)
    let ftp = Data(#"BWDP!{"id":"a","name":"n","url":"ftp://127.0.0.1/x"}"#.utf8)
    #expect(FinderPacket.parse(ftp) == nil)
    let pub = Data(#"BWDP!{"id":"a","name":"n","url":"http://203.0.113.9:8477"}"#.utf8)
    #expect(FinderPacket.parse(pub) == nil)
    let name = Data(#"BWDP!{"id":"a","name":"n","url":"http://example.com:8477"}"#.utf8)
    #expect(FinderPacket.parse(name) == nil)
    let empty = Data(#"BWDP!{"id":"","name":"n","url":"http://127.0.0.1:9"}"#.utf8)
    #expect(FinderPacket.parse(empty) == nil)
}

@Test func anAddressChangeWithoutASignatureIsNotFollowed() throws {
    let pair = signingKey()
    let saved = try FoundServer(id: "abc", name: "Living Room", url: #require(URL(string: "http://10.0.0.5:8477")), key: pair.publicKey)
    let moved = try FoundServer(id: "abc", name: "Living Room", url: #require(URL(string: "http://10.0.0.9:8477")))
    #expect(ServerFollow.updated(saved, found: [moved]) == nil)
}

@Test func aSignedMoveFollowsTheSameServer() throws {
    let pair = signingKey()
    let nonce = Data(repeating: 9, count: 16)
    let saved = try FoundServer(id: "abc", name: "Living Room", url: #require(URL(string: "http://10.0.0.5:8477")), key: pair.publicKey)
    let other = try FoundServer(id: "zzz", name: "Other", url: #require(URL(string: "http://10.0.0.8:8477")))
    let moved = try signedServer(id: "abc", url: "http://10.0.0.9:8477", privateKey: pair.privateKey, nonce: nonce)
    let next = ServerFollow.updated(saved, found: [other, moved])
    #expect(next?.url.host() == "10.0.0.9")
    #expect(next?.id == "abc")
    #expect(next?.key == pair.publicKey)
    #expect(next?.signature == nil)
}

@Test func aSpoofedSignatureIsNotFollowed() throws {
    let savedKey = signingKey()
    let attacker = signingKey()
    let nonce = Data(repeating: 4, count: 16)
    let saved = try FoundServer(id: "abc", name: "Living Room", url: #require(URL(string: "http://10.0.0.5:8477")), key: savedKey.publicKey)
    let spoof = try signedServer(id: "abc", url: "http://10.0.0.9:8477", privateKey: attacker.privateKey, nonce: nonce)
    #expect(ServerFollow.updated(saved, found: [spoof]) == nil)
    let real = try signedServer(id: "abc", url: "http://10.0.0.8:8477", privateKey: savedKey.privateKey, nonce: nonce)
    var lied = real
    lied.url = try #require(URL(string: "http://10.0.0.9:8477"))
    #expect(ServerFollow.updated(saved, found: [lied]) == nil)
    let unsigned = try FoundServer(id: "abc", name: "Living Room", url: #require(URL(string: "http://10.0.0.9:8477")), signature: spoof.signature, nonce: nonce, signedURL: "http://10.0.0.9:8477")
    let noKey = FoundServer(id: saved.id, name: saved.name, url: saved.url)
    #expect(ServerFollow.updated(noKey, found: [unsigned]) == nil)
}

@Test func theSameAddressIsNotAMove() throws {
    let saved = try FoundServer(id: "abc", name: "Living Room", url: #require(URL(string: "http://10.0.0.5:8477")))
    let again = try FoundServer(id: "abc", name: "Living Room", url: #require(URL(string: "http://10.0.0.5:8477")))
    #expect(ServerFollow.updated(saved, found: [again]) == nil)
}

@Test func aDemoServerIsNotFollowed() throws {
    let saved = try FoundServer(id: "demo", name: "Demo", url: #require(URL(string: "http://127.0.0.1:18649")))
    let moved = try FoundServer(id: "demo", name: "Demo", url: #require(URL(string: "http://127.0.0.1:18650")))
    #expect(ServerFollow.updated(saved, found: [moved]) == nil)
}

@Test func aPortChangeIsAMove() throws {
    let pair = signingKey()
    let nonce = Data(repeating: 3, count: 16)
    let saved = try FoundServer(id: "abc", name: "Living Room", url: #require(URL(string: "http://127.0.0.1:18477")), key: pair.publicKey)
    let moved = try signedServer(id: "abc", url: "http://127.0.0.1:18478", privateKey: pair.privateKey, nonce: nonce)
    #expect(ServerFollow.updated(saved, found: [moved])?.url.port == 18478)
}

@Test func aConnectLinkReadsTheServerURL() throws {
    let url = try #require(URL(string: "broadwave://connect?url=http://127.0.0.1:18477"))
    let server = try #require(ConnectLink.serverURL(from: url))
    #expect(server.host() == "127.0.0.1")
    #expect(server.port == 18477)
    #expect(try ConnectLink.serverURL(from: #require(URL(string: "broadwave://guide"))) == nil)
    #expect(try ConnectLink.serverURL(from: #require(URL(string: "https://example.com"))) == nil)
    let creds = try #require(URL(string: "broadwave://connect?url=http://user:pass@127.0.0.1:8477"))
    #expect(ConnectLink.serverURL(from: creds) == nil)
}

@Test func aPublicAddressIsNotFollowed() throws {
    let pair = signingKey()
    let nonce = Data(repeating: 2, count: 16)
    let saved = try FoundServer(id: "abc", name: "Living Room", url: #require(URL(string: "http://10.0.0.5:8477")), key: pair.publicKey)
    let away = try signedServer(id: "abc", url: "https://203.0.113.9/tv", privateKey: pair.privateKey, nonce: nonce)
    #expect(ServerFollow.updated(saved, found: [away]) == nil)
}

@Test func aRememberedServerKeepsTheKeyAndDropsTheSignature() throws {
    let nonce = Data([1, 2, 3, 4])
    let server = try FoundServer(
        id: "abc", name: "Living Room", url: #require(URL(string: "http://10.0.0.5:8477")),
        key: "abc", signature: "sig", nonce: nonce, signedURL: "http://10.0.0.5:8477"
    )
    let again = try JSONDecoder().decode(FoundServer.self, from: JSONEncoder().encode(server))
    #expect(again.key == "abc")
    #expect(again.signature == nil)
    #expect(again.nonce == nil)
    #expect(again.signedURL == nil)
}

@Test func rememberedServersKeepTheNewestAddress() throws {
    let first = try FoundServer(id: "abc", name: "Living Room", url: #require(URL(string: "http://10.0.0.5:8477")))
    let other = try FoundServer(id: "zzz", name: "Den", url: #require(URL(string: "http://10.0.0.8:8477")))
    let moved = try FoundServer(id: "abc", name: "Living Room", url: #require(URL(string: "http://10.0.0.9:8477")))
    let demo = try FoundServer(id: "demo", name: "Demo", url: #require(URL(string: "http://127.0.0.1:9")))
    var list = RememberedServers.upsert([], first)
    list = RememberedServers.upsert(list, other)
    list = RememberedServers.upsert(list, moved)
    list = RememberedServers.upsert(list, demo)
    #expect(list.count == 2)
    #expect(list[0].id == "abc")
    #expect(list[0].url.host() == "10.0.0.9")
    #expect(list[1].id == "zzz")
}
