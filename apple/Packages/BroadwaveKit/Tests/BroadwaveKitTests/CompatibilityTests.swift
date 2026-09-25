@testable import BroadwaveKit
import Foundation
import Testing

@Test func olderServerFixtureStillDecodes() throws {
    let older = Data("""
    {"id":"s","name":"Home","version":"0.9.0","apiVersion":1,"features":["live","dvr","passes"]}
    """.utf8)
    let server = try APIClient.decoder.decode(ServerInfo.self, from: older)
    #expect(server.apiVersion == 1)
    #expect(server.minAppVersion == nil)
    #expect(!server.features.contains("wholeHomeSync"))
    #expect(Compatibility.gateFeature(server, "wholeHomeSync") == "Update your Broadwave server to use this")
    #expect(Compatibility.gateApp(server) == nil)
}

@Test func appOlderThanTheServerAsksFor() throws {
    let server = try APIClient.decoder.decode(ServerInfo.self, from: Data("""
    {"id":"s","name":"Home","version":"2.0.0","apiVersion":1,"features":["live","wholeHomeSync"],"minAppVersion":"1.1"}
    """.utf8))
    #expect(Compatibility.gateApp(server, app: "1.0") == "Update Broadwave to use this server")
    #expect(Compatibility.gateApp(server, app: "1.1") == nil)
    #expect(Compatibility.gateApp(server, app: "1.1.0") == nil)
    #expect(Compatibility.gateFeature(server, "wholeHomeSync") == nil)
    #expect(Compatibility.compare("1.9", "1.10") < 0)
    #expect(Compatibility.gateApp(nil) == nil)
}
