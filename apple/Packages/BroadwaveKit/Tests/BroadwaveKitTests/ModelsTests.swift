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

@Test func decodesServerResponses() throws {
    struct Channels: Decodable { var channels: [Channel] }
    struct Airings: Decodable { var airings: [Airing] }
    struct Recordings: Decodable { var recordings: [Recording] }

    let channels = try APIClient.decoder.decode(Channels.self, from: fixture("channels")).channels
    #expect(!channels.isEmpty)
    #expect(channels.allSatisfy { !$0.displayNumber.isEmpty })
    #expect(channels.first { $0.guideName == "WDAF" }?.network == "FOX")
    #expect(channels.first { $0.guideName == "WDAF2" }?.network == nil)

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

    struct FoundDevices: Decodable { var devices: [Device]; var found: Int }
    struct Look: Decodable {
        struct Item: Decodable { var kind: String; var name: String; var addr: String; var id: String? }
        var found: [Item]
    }
    struct Home: Decodable {
        struct Place: Decodable { var id: String; var group: String; var kind: String; var name: String; var action: String }
        var places: [Place]
        var tunerAddress: String
        var sharing: Bool
    }
    struct Calls: Decodable { var calls: [String: String] }
    struct Starred: Decodable {
        struct Item: Decodable { var id: Int64; var network: String }
        var starred: [Item]
    }
    struct FreeList: Decodable {
        struct Feed: Decodable { var kind: String; var name: String; var addr: String; var playlist: String; var guide: String }
        var found: [Feed]
        var guide: String
    }
    struct FreeAdd: Decodable { var id: Int64; var kind: String; var name: String }
    struct Scan: Decodable { var scanning: Bool; var found: Int? }
    struct SignalCheck: Decodable { var running: Bool; var message: String? }
    struct Activity: Decodable { var type: String; var data: Event }
    struct SourcesFound: Decodable {
        struct Body: Decodable { var found: Int }
        var type: String
        var data: Body
    }
    struct LiveChanged: Decodable { var type: String }
    let discovered = try APIClient.decoder.decode(FoundDevices.self, from: fixture("discover"))
    #expect(discovered.found == 1)
    let looked = try APIClient.decoder.decode(Look.self, from: fixture("look"))
    #expect(looked.found.first?.id == "FAKEHDHR")
    let home = try APIClient.decoder.decode(Home.self, from: fixture("home"))
    #expect(home.places.first?.action == "added")
    #expect(home.tunerAddress.hasSuffix(":8478"))
    #expect(!home.sharing)
    let calls = try APIClient.decoder.decode(Calls.self, from: fixture("affiliations"))
    #expect(calls.calls["KSHB"] == "NBC")
    let starred = try APIClient.decoder.decode(Starred.self, from: fixture("star"))
    #expect(starred.starred.contains { $0.network == "FOX" })
    _ = try APIClient.decoder.decode(FreeList.self, from: fixture("free"))
    let added = try APIClient.decoder.decode(FreeAdd.self, from: fixture("free-add"))
    #expect(added.kind == "free")
    let xtream = try APIClient.decoder.decode(Source.self, from: fixture("xtream"))
    #expect(xtream.kind == "xtream")
    let watch = try APIClient.decoder.decode(APIErrorBody.self, from: fixture("watch"))
    #expect(watch.code == "internal")
    struct Stopped: Decodable { var ok: Bool }
    let stopped = try APIClient.decoder.decode(Stopped.self, from: fixture("watch-stop"))
    #expect(stopped.ok)
    let scan = try APIClient.decoder.decode(Scan.self, from: fixture("scan"))
    #expect(scan.scanning)
    let scanStatus = try APIClient.decoder.decode(Scan.self, from: fixture("scan-status"))
    #expect(scanStatus.scanning)
    #expect(scanStatus.found == 0)
    let checking = try APIClient.decoder.decode(SignalCheck.self, from: fixture("signals-check"))
    #expect(checking.running)
    let idle = try APIClient.decoder.decode(SetupFinish.self, from: fixture("setup-finish"))
    #expect(!idle.running)
    let started = try APIClient.decoder.decode(SetupFinish.self, from: fixture("setup-finish-post"))
    #expect(started.running)
    let done = try APIClient.decoder.decode(SetupFinish.self, from: fixture("setup-finish-done"))
    #expect(done.channelId == 1)
    #expect(done.ready?.hasPrefix("Ready:") == true)
    let sources = try APIClient.decoder.decode(SourcesFound.self, from: fixture("ws-sources"))
    #expect(sources.type == "sources.found")
    #expect(sources.data.found == 1)
    let live = try APIClient.decoder.decode(LiveChanged.self, from: fixture("ws-live"))
    #expect(live.type == "live.changed")
    let activity = try APIClient.decoder.decode(Activity.self, from: fixture("ws-activity"))
    #expect(activity.type == "activity")
    #expect(activity.data.kind == "source")
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
