import Foundation
@testable import OTAKit
import Testing

@Test func catalogCacheRoundTrip() throws {
    let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let start = Date(timeIntervalSince1970: 1_700_000_000)
    let channel = Channel(
        id: 1, deviceId: "duo", guideNumber: "4.1", guideName: "WDAF",
        displayNumber: "4.1", displayName: "WDAF", hd: true, favorite: true,
        enabled: true, hidden: false, present: true
    )
    let airing = Airing(id: 9, channelId: 1, title: "News", start: start, end: start.addingTimeInterval(1800))
    let recording = Recording(
        id: 3, channelId: 1, guideNumber: "4.1", title: "News", status: "finished",
        startedAt: start, endsAt: start.addingTimeInterval(1800)
    )
    let snap = CatalogSnapshot(channels: [channel], airings: [airing], recordings: [recording])
    CatalogCache.save(snap, serverID: "abc-123", directory: dir)
    let loaded = try #require(CatalogCache.load(serverID: "abc-123", directory: dir))
    #expect(loaded.channels.first?.displayNumber == "4.1")
    #expect(loaded.airings.first?.title == "News")
    #expect(loaded.recordings.first?.title == "News")
    #expect(CatalogCache.load(serverID: "other", directory: dir) == nil)
}

@Test func artLayoutMatchesPictureSize() {
    #expect(ArtLayout.choose(width: 1920, height: 1080, slot: 1400) == "bleed")
    #expect(ArtLayout.choose(width: 1280, height: 720, slot: 1920) == "composed")
    #expect(ArtLayout.choose(width: 0, height: 0, slot: 800) == "composed")
    #expect(ArtLayout.displayEdge(native: 800, slot: 2000) == 1000)
}
