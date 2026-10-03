@testable import BroadwaveKit
import Testing

private func channel(
    _ id: Int64,
    _ number: String,
    hidden: Bool = false,
    protected: Bool = false,
    standard: String? = nil,
    twinId: Int64? = nil,
    playsAs: Int64? = nil
) -> Channel {
    Channel(
        id: id, deviceId: "flex", guideNumber: number, guideName: number,
        displayNumber: number, displayName: number,
        hd: true, favorite: false, enabled: true, hidden: hidden, present: true,
        protected: protected ? true : nil, standard: standard, twinId: twinId, playsAs: playsAs
    )
}

@Test func anEncryptedStationPlaysItsClearTwin() {
    let clear = channel(5, "5.1")
    let sealed = channel(115, "115.1", hidden: true, protected: true, standard: "atsc3", playsAs: 5)
    let choice = ClearBroadcast.play(id: 115, visible: [clear], lineup: [clear, sealed])
    #expect(choice?.channel.id == 5)
    #expect(choice?.note == ClearBroadcast.encryptedNote)
}

@Test func aVisibleChannelPlaysAsItself() {
    let next = channel(104, "104.1", standard: "atsc3")
    let choice = ClearBroadcast.play(id: 104, visible: [next], lineup: [next])
    #expect(choice?.channel.id == 104)
    #expect(choice?.note == nil)
}

@Test func aHiddenOnePointZeroPlaysItsVisibleTwin() {
    let older = channel(4, "4.1", hidden: true, twinId: 104)
    let next = channel(104, "104.1", standard: "atsc3", twinId: 4)
    let choice = ClearBroadcast.play(id: 4, visible: [next], lineup: [older, next])
    #expect(choice?.channel.id == 104)
    #expect(choice?.note == nil)
}

@Test func anEncryptedStationWithNoTwinDoesNotPlay() {
    let sealed = channel(115, "115.1", hidden: true, protected: true, standard: "atsc3")
    #expect(ClearBroadcast.play(id: 115, visible: [], lineup: [sealed]) == nil)
    #expect(ClearBroadcast.play(id: 9, visible: [], lineup: [sealed]) == nil)
}
