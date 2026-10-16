@testable import BroadwaveKit
import Foundation
import Testing

private let now = Date(timeIntervalSince1970: 1_790_500_000)
private let api = APIClient(base: URL(string: "http://tv.local:8477")!)

private func channel(_ id: Int64, favorite: Bool = false, hidden: Bool = false, hd: Bool = true) -> Channel {
    Channel(
        id: id, deviceId: "d", guideNumber: "\(id).1", guideName: "K\(id)", displayNumber: "\(id).1", displayName: "K\(id)",
        hd: hd, favorite: favorite, enabled: true, hidden: hidden, present: true
    )
}

private func airing(_ id: Int64, on channel: Int64, title: String, category: String? = nil, to: Double = 1500) -> Airing {
    Airing(
        id: id, channelId: channel, title: title, description: "About \(title).", category: category, episodeLabel: id == 1 ? "S2 E4" : nil,
        start: now.addingTimeInterval(-600), end: now.addingTimeInterval(to)
    )
}

@Test func carouselLeadsWithRecentThenFavoritesThenGamesThenStills() {
    let snap = TopShelf.Snapshot(
        channels: [channel(2), channel(4, favorite: true), channel(5), channel(7), channel(9), channel(11, favorite: true, hidden: true)],
        airings: [
            airing(1, on: 4, title: "Evening News"),
            airing(2, on: 5, title: "Chiefs at Broncos", category: "Sports event"),
            airing(3, on: 7, title: "Quiz Night"),
            airing(4, on: 9, title: "Old Movie"),
        ],
        framed: [2, 4, 7, 11]
    )
    let slides = TopShelf.carousel(snap, api: api, now: now, recent: [7, 99])
    // 7 watched last; 4 a favorite; 5 a game. The rest need both a still and a listing:
    // 2 has no listing and 9 no still. 11 is hidden.
    #expect(slides.map(\.id) == ["channel-7", "channel-4", "channel-5"])
}

@Test func liveSlideCarriesStillTimeLeftAndLinksAndNeverAPreview() {
    let snap = TopShelf.Snapshot(channels: [channel(4, favorite: true)], airings: [airing(1, on: 4, title: "Evening News")], framed: [4])
    let slide = TopShelf.carousel(snap, api: api, now: now)[0]
    #expect(slide.title == "Evening News")
    #expect(slide.context == "Live on 4.1 K4")
    #expect(slide.summary == "S2 E4 · About Evening News.")
    #expect(slide.genre == "News")
    #expect(slide.play == URL(string: "broadwave://watch/4"))
    #expect(slide.info == URL(string: "broadwave://guide?channel=4"))
    #expect(slide.preview == nil)
    #expect(slide.progress == nil)
    #expect(slide.duration == 0)
    #expect(slide.facts.first?.name == "Time left")
    #expect(slide.facts.first?.value == "25 min")
    let image = try? #require(slide.image)
    #expect(image?.path == "/api/v1/channels/4/frame")
    #expect(image?.query?.contains("w=1280") == true)
}

@Test func carouselUsesListedCategoryForGenreAndProgramArtWithoutAStill() {
    var listed = airing(3, on: 7, title: "Quiz Night", category: "Game show, Comedy")
    listed.imageUrl = "https://example.com/a.jpg"
    let snap = TopShelf.Snapshot(channels: [channel(7, favorite: true)], airings: [listed])
    let slide = TopShelf.carousel(snap, api: api, now: now)[0]
    #expect(slide.genre == "Game show")
    #expect(slide.image?.path == "/media/art/airing/3")
}

@Test func recordingsFollowLiveWithPreviewAndProgress() {
    let rec = Recording(
        id: 31, channelId: 4, guideNumber: "4.1", title: "Late Show", subtitle: "Guests", status: "completed",
        startedAt: now.addingTimeInterval(-86400), position: 900, durationSec: 3600
    )
    let snap = TopShelf.Snapshot(
        channels: [channel(4, favorite: true)], airings: [airing(1, on: 4, title: "Evening News")], recordings: [rec], framed: [4]
    )
    let slides = TopShelf.carousel(snap, api: api, now: now)
    #expect(slides.map(\.id) == ["channel-4", "recording-31"])
    let shelf = slides[1]
    #expect(shelf.progress == 0.25)
    #expect(shelf.duration == 3600)
    #expect(shelf.preview?.path == "/media/file/31/index.m3u8")
    #expect(shelf.play == URL(string: "broadwave://recording/31"))
    #expect(shelf.facts.first?.value == "45 min")
}

@Test func carouselRespectsTheLimit() {
    let lineup = (1 ... 20).map { channel(Int64($0), favorite: true) }
    let snap = TopShelf.Snapshot(channels: lineup, framed: Set(lineup.map(\.id)))
    #expect(TopShelf.carousel(snap, api: api, now: now, limit: 8).count == 8)
}

@Test func recentsKeepNewestFirstWithoutRepeats() {
    #expect(SharedRecents.adding(4, to: [7, 4, 9]) == [4, 7, 9])
    #expect(SharedRecents.adding(1, to: [2, 3, 4, 5, 6, 7]) == [1, 2, 3, 4, 5, 6])
}
