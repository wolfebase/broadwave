@testable import BroadwaveKit
import Foundation
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
    #expect(ArtLayout.choose(width: 1280, height: 720, slot: 1600) == "bleed")
    #expect(ArtLayout.choose(width: 1280, height: 720, slot: 1601) == "composed")
    #expect(ArtLayout.showsCellArt(slot: 221))
    #expect(!ArtLayout.showsCellArt(slot: 220))
    #expect(ArtLayout.cappedPoints(native: 480, slotPoints: 250, scale: 3) == 200)
    #expect(ArtLayout.cappedPoints(native: 1920, slotPoints: 400, scale: 2) == 400)
    #expect(ArtLayout.cappedPoints(native: 0, slotPoints: 250, scale: 2) == 0)
}

@Test func recordingFallsBackToProgramArt() {
    let start = Date(timeIntervalSince1970: 1_700_000_000)
    let early = Airing(
        id: 1, channelId: 1, title: "News", programId: "sh-1",
        imageUrl: "https://img.example/early.jpg", imageWidth: 1280, imageHeight: 720,
        start: start, end: start.addingTimeInterval(1800)
    )
    let later = Airing(
        id: 2, channelId: 1, title: "News", programId: "sh-1",
        imageUrl: "https://img.example/later.jpg", imageWidth: 1920, imageHeight: 1080,
        start: start.addingTimeInterval(3600), end: start.addingTimeInterval(5400)
    )
    let movie = Airing(
        id: 3, channelId: 1, title: "Movie",
        imageUrl: "https://img.example/movie.jpg", imageWidth: 600, imageHeight: 900,
        start: start, end: start.addingTimeInterval(7200)
    )
    let bare = Airing(id: 4, channelId: 2, title: "News", start: start, end: start.addingTimeInterval(1800))
    let index = GuideIndex([early, later, movie, bare])
    let byProgram = Recording(
        id: 1, channelId: 1, guideNumber: "4.1", title: "Evening", programId: "sh-1",
        status: "finished", startedAt: start.addingTimeInterval(3600)
    )
    #expect(index.artAiring(for: byProgram)?.id == 2)
    let byTitle = Recording(
        id: 2, channelId: 1, guideNumber: "4.1", title: "Movie",
        status: "finished", startedAt: start
    )
    #expect(index.artAiring(for: byTitle)?.id == 3)
    let noArt = Recording(
        id: 3, channelId: 2, guideNumber: "5.1", title: "News",
        status: "finished", startedAt: start
    )
    #expect(index.artAiring(for: noArt) == nil)
}
