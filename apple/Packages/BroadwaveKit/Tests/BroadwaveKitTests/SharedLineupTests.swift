@testable import BroadwaveKit
import Foundation
import Testing

private func channel(_ id: Int64, _ number: String, _ name: String, favorite: Bool = false, enabled: Bool = true, network: String? = nil) -> Channel {
    Channel(
        id: id, deviceId: "flex", guideNumber: number, guideName: name, displayNumber: number, displayName: name,
        hd: true, favorite: favorite, enabled: enabled, hidden: false, present: true, artUrl: "/art/\(id).png", network: network
    )
}

@Test func theKeptLineupNamesEnabledChannelsFavoritesFirst() throws {
    let lineup = [
        channel(1, "4.1", "KBWV", network: "FOX"),
        channel(2, "5.1", "WTST"),
        channel(3, "9.1", "Off", enabled: false),
        channel(4, "41.1", "KBWV2", favorite: true),
    ]
    let kept = SharedLineup.entries(lineup)
    #expect(kept.map(\.id) == [4, 1, 2])
    let back = try JSONDecoder().decode([SharedLineup.Entry].self, from: JSONEncoder().encode(kept))
    #expect(back == kept)
    // Built back, it is enough to say which channel a saved Shortcut meant.
    let channels = SharedLineup.channels(back)
    #expect(channels.map(\.displayNumber) == ["41.1", "4.1", "5.1"])
    #expect(channels[1].network == "FOX")
    #expect(Voice.channel("channel 4", in: channels)?.id == 1)
    #expect(Voice.channel("WTST", in: channels)?.id == 2)
}

@Test func aHugeLineupIsCut() {
    let many = (1 ... 900).map { channel(Int64($0), "\($0).1", "C\($0)") }
    #expect(SharedLineup.entries(many).count == 600)
}

@Test func withoutAKeychainGroupNothingIsKept() {
    // Tests run outside an app bundle: no group, so save and load do nothing.
    SharedLineup.save([channel(1, "4.1", "KBWV")])
    #expect(SharedLineup.load().isEmpty)
}
