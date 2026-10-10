@testable import BroadwaveKit
import Foundation
import Testing

private func server(_ id: String, _ name: String) -> FoundServer {
    FoundServer(id: id, name: name, url: URL(filePath: "/\(id)"))
}

private let home = server("srv-home", "Living room")
private let cabin = server("srv-cabin", "Cabin")

private func channel(_ id: Int64, _ number: String, _ name: String) -> Channel {
    Channel(
        id: id, deviceId: "flex", guideNumber: number, guideName: name,
        displayNumber: number, displayName: name,
        hd: true, favorite: false, enabled: true, hidden: false, present: true
    )
}

@Test func aLiveChannelMakesAnInviteThatSurvivesTheTrip() throws {
    let invite = try #require(WatchTogether.invite(server: home, channel: channel(7, "4.1", "KBWV")))
    #expect(invite == WatchTogether(serverID: "srv-home", serverName: "Living room", channelID: 7, channelName: "4.1 KBWV"))
    let back = try JSONDecoder().decode(WatchTogether.self, from: JSONEncoder().encode(invite))
    #expect(back == invite)
}

@Test func theDemoAndAServerStillConnectingShareNothing() {
    let demo = server("demo", "Demo")
    let pending = server("pending", "Server")
    #expect(WatchTogether.invite(server: demo, channel: channel(1, "1", "A")) == nil)
    #expect(WatchTogether.invite(server: pending, channel: channel(1, "1", "A")) == nil)
    #expect(WatchTogether.invite(server: home, channel: nil) == nil)
    #expect(WatchTogether.invite(server: nil, channel: channel(1, "1", "A")) == nil)
}

@Test func aDeviceOnTheSameServerJustWatches() {
    let invite = WatchTogether(serverID: "srv-home", serverName: "Living room", channelID: 7, channelName: "4.1 KBWV")
    #expect(TogetherJoin.decide(invite, server: home, remembered: [home, cabin]) == .watch(7))
}

@Test func aDeviceThatKnowsTheServerSwitchesToIt() {
    let invite = WatchTogether(serverID: "srv-home", serverName: "Living room", channelID: 7, channelName: "4.1 KBWV")
    #expect(TogetherJoin.decide(invite, server: cabin, remembered: [cabin, home]) == .connect(home, 7))
    #expect(TogetherJoin.decide(invite, server: nil, remembered: [home]) == .connect(home, 7))
}

@Test func aDeviceThatWasNeverSetUpWithTheServerSaysSo() {
    let invite = WatchTogether(serverID: "srv-home", serverName: "Living room", channelID: 7, channelName: "4.1 KBWV")
    #expect(TogetherJoin.decide(invite, server: cabin, remembered: [cabin]) ==
        .cannot("4.1 KBWV is on Living room, which this device isn't set up with."))
    let unnamed = WatchTogether(serverID: "srv-x", serverName: " ", channelID: 7, channelName: "4.1 KBWV")
    #expect(TogetherJoin.decide(unnamed, server: nil, remembered: []) ==
        .cannot("4.1 KBWV is on another Broadwave server, which this device isn't set up with."))
}

@Test func aBrokenOrDemoInviteOpensNothing() {
    let demo = WatchTogether(serverID: "demo", serverName: "Demo", channelID: 1, channelName: "1 A")
    let noChannel = WatchTogether(serverID: "srv-home", serverName: "Living room", channelID: 0, channelName: "")
    let demoServer = server("demo", "Demo")
    #expect(TogetherJoin.decide(demo, server: demoServer, remembered: []) == .cannot("This SharePlay can't be opened here."))
    #expect(TogetherJoin.decide(noChannel, server: home, remembered: []) == .cannot("This SharePlay can't be opened here."))
}
