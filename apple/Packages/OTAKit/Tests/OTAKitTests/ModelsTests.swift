import Foundation
@testable import OTAKit
import Testing

private func fixture(_ name: String) throws -> Data {
    var url = URL(fileURLWithPath: #filePath)
    for _ in 0 ..< 6 {
        url.deleteLastPathComponent()
    }
    return try Data(contentsOf: url.appendingPathComponent("api/fixtures/\(name).json"))
}

@Test func decodesServerResponses() throws {
    struct Channels: Decodable { var channels: [Channel] }
    struct Airings: Decodable { var airings: [Airing] }
    struct Recordings: Decodable { var recordings: [Recording] }

    let channels = try APIClient.decoder.decode(Channels.self, from: fixture("channels")).channels
    #expect(!channels.isEmpty)
    #expect(channels.allSatisfy { !$0.displayNumber.isEmpty })

    let airings = try APIClient.decoder.decode(Airings.self, from: fixture("airings")).airings
    #expect(!airings.isEmpty)
    #expect(airings.allSatisfy { $0.end > $0.start })

    _ = try APIClient.decoder.decode(Recordings.self, from: fixture("recordings"))
    let server = try APIClient.decoder.decode(ServerInfo.self, from: fixture("server"))
    #expect(server.apiVersion == 1)
    #expect(server.features.contains("wholeHomeSync"))

    struct Devices: Decodable { var devices: [Device] }
    struct Passes: Decodable { var passes: [Pass] }
    struct Teams: Decodable { var teams: [TeamFollow] }
    struct Events: Decodable { var events: [Event] }
    struct Markers: Decodable { var markers: [Marker] }
    struct Signals: Decodable { var channels: [ChannelSignal] }
    struct Tuners: Decodable { var tuners: [Tuner] }
    struct SyncFrame: Decodable { var data: RoomState }
    _ = try APIClient.decoder.decode(Devices.self, from: fixture("devices"))
    _ = try APIClient.decoder.decode(Passes.self, from: fixture("passes"))
    _ = try APIClient.decoder.decode(Teams.self, from: fixture("teams"))
    _ = try APIClient.decoder.decode(Events.self, from: fixture("events"))
    _ = try APIClient.decoder.decode(Markers.self, from: fixture("markers"))
    _ = try APIClient.decoder.decode(Signals.self, from: fixture("signals"))
    _ = try APIClient.decoder.decode(Tuners.self, from: fixture("tuners"))
    _ = try APIClient.decoder.decode(Settings.self, from: fixture("settings"))
    _ = try APIClient.decoder.decode(MultiviewPlan.self, from: fixture("multiview"))
    struct Hello: Decodable { var type: String }
    let hello = try APIClient.decoder.decode(Hello.self, from: fixture("ws-hello"))
    #expect(hello.type == "hello")
    let sync = try APIClient.decoder.decode(SyncFrame.self, from: fixture("ws-sync"))
    #expect(sync.data.room == "channel:1")
}

@Test func airingProgress() {
    let start = Date(timeIntervalSince1970: 1000)
    let a = Airing(id: 1, channelId: 1, title: "News", start: start, end: start.addingTimeInterval(1800))
    #expect(a.progress(at: start.addingTimeInterval(900)) == 0.5)
    #expect(a.progress(at: start.addingTimeInterval(-10)) == 0)
    #expect(a.isOn(at: start.addingTimeInterval(1)))
    #expect(!a.isOn(at: start.addingTimeInterval(1800)))
}

@Test func multiviewPlanNamesTheBusyChannel() throws {
    let json = Data("""
    {"playable":[{"channelId":1,"frequencyHz":593000000,"shared":false}],"blocked":[{"channelId":2,"reason":"The tuner is busy. 4.1 is on.","holders":["4.1"]}],"tunersNeeded":1,"tunersFree":0}
    """.utf8)
    let plan = try APIClient.decoder.decode(MultiviewPlan.self, from: json)
    #expect(plan.playable.count == 1)
    #expect(plan.blocked.first?.reason == "The tuner is busy. 4.1 is on.")
}

@Test @MainActor func secondTileKeepsTheMultiviewRoom() throws {
    let socket = try EventSocket(base: #require(URL(string: "http://127.0.0.1:18477")))
    socket.join(room: "multiview:abc", channelID: 1)
    socket.join(room: "multiview:abc", channelID: 3)
    #expect(socket.membership(of: "multiview:abc") == 2)
    socket.leave(room: "multiview:abc")
    #expect(socket.membership(of: "multiview:abc") == 1)
    socket.leave(room: "multiview:abc")
    #expect(socket.membership(of: "multiview:abc") == 0)
}

@Test func roomTargetAdvancesOnlyWhilePlaying() {
    var room = RoomState(room: "channel:1", channelId: 1, mode: "follow", anchorServer: 10000, anchorMedia: 5000, rate: 1, latency: "balanced", version: 1, members: 2)
    #expect(room.target(atServer: 12000) == 7000)
    room.rate = 0
    #expect(room.target(atServer: 12000) == 5000)
}

@Test func appleDevicesAskForTheOriginalBroadcast() {
    let caps = Capabilities.current()
    #expect(caps.video.contains("h264"))
    #expect(caps.audio.contains("ac3"))
}

extension Airing {
    init(id: Int64, channelId: Int64, title: String, start: Date, end: Date) {
        self.init(id: id, channelId: channelId, title: title, subtitle: nil, description: nil, category: nil, programId: nil, new: nil, start: start, end: end)
    }
}
