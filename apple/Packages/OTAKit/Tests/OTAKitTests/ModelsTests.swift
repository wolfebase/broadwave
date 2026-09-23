import Foundation
import Testing
@testable import OTAKit

private func fixture(_ name: String) throws -> Data {
    let url = try #require(Bundle.module.url(forResource: name, withExtension: "json", subdirectory: "Fixtures"))
    return try Data(contentsOf: url)
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
}

@Test func airingProgress() {
    let start = Date(timeIntervalSince1970: 1_000)
    let a = Airing(id: 1, channelId: 1, title: "News", start: start, end: start.addingTimeInterval(1_800))
    #expect(a.progress(at: start.addingTimeInterval(900)) == 0.5)
    #expect(a.progress(at: start.addingTimeInterval(-10)) == 0)
    #expect(a.isOn(at: start.addingTimeInterval(1)))
    #expect(!a.isOn(at: start.addingTimeInterval(1_800)))
}

@Test func roomTargetAdvancesOnlyWhilePlaying() {
    var room = RoomState(room: "channel:1", channelId: 1, mode: "follow", anchorServer: 10_000, anchorMedia: 5_000, rate: 1, latency: "balanced", version: 1, members: 2)
    #expect(room.target(atServer: 12_000) == 7_000)
    room.rate = 0
    #expect(room.target(atServer: 12_000) == 5_000)
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
