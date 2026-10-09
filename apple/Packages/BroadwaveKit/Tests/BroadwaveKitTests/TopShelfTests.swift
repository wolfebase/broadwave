@testable import BroadwaveKit
import Foundation
import Testing

private let now = Date(timeIntervalSince1970: 1_790_500_000)
private let api = APIClient(base: URL(string: "http://tv.local:8477")!)

private func channel(_ id: Int64, favorite: Bool = false, hidden: Bool = false, art: String? = nil) -> Channel {
    Channel(
        id: id, deviceId: "d", guideNumber: "\(id).1", guideName: "K\(id)", displayNumber: "\(id).1", displayName: "K\(id)",
        hd: true, favorite: favorite, enabled: true, hidden: hidden, present: true, artUrl: art
    )
}

private func airing(_ id: Int64, on channel: Int64, title: String, category: String? = nil, image: String? = nil, from: Double = -600, to: Double = 1200) -> Airing {
    Airing(id: id, channelId: channel, title: title, category: category, imageUrl: image, start: now.addingTimeInterval(from), end: now.addingTimeInterval(to))
}

private func rec(_ id: Int64, title: String, subtitle: String? = nil, position: Double, duration: Double) -> Recording {
    Recording(
        id: id, channelId: 1, guideNumber: "4.1", title: title, subtitle: subtitle, status: "complete",
        startedAt: now.addingTimeInterval(-86400), position: position, durationSec: duration, progressAt: now
    )
}

@Test func topShelfShowsGamesThenFavoritesThenRecordingsToFinish() {
    let channels = [channel(4, favorite: true), channel(5, favorite: true), channel(9), channel(12, favorite: true, hidden: true)]
    let airings = [
        airing(1, on: 4, title: "Chiefs at Broncos", category: "Sports event"),
        airing(2, on: 5, title: "Evening News"),
        airing(3, on: 9, title: "Royals at Twins", category: "Sports event", image: "https://art/x.jpg"),
        airing(4, on: 12, title: "Hidden Game", category: "Sports event"),
        airing(5, on: 5, title: "Later Show", from: 1200, to: 3000),
    ]
    let recordings = [rec(7, title: "Mystery Hour", subtitle: "The Key", position: 900, duration: 3600), rec(8, title: "Done", position: 3600, duration: 3600)]
    let sections = TopShelf.sections(.init(channels: channels, airings: airings, recordings: recordings, framed: [4]), api: api, now: now)

    #expect(sections.map(\.title) == ["Games on now", "Favorites", "Continue watching"])
    // A favorite with a game on is listed once, under games. Hidden channels never show.
    #expect(sections[0].items.map(\.title) == ["4.1 · Chiefs at Broncos", "9.1 · Royals at Twins"])
    #expect(sections[1].items.map(\.title) == ["5.1 · Evening News"])
    #expect(sections[0].items[0].link == URL(string: "broadwave://watch/4"))
    // A live frame first, then the listing's art, then nothing.
    #expect(sections[0].items[0].image == URL(string: "http://tv.local:8477/api/v1/channels/4/frame?w=1280&t=\(Int(now.timeIntervalSince1970) / 60)"))
    #expect(sections[0].items[1].image == URL(string: "http://tv.local:8477/media/art/airing/3?w=1280"))
    #expect(sections[1].items[0].image == nil)
    let resume = sections[2].items
    #expect(resume.map(\.title) == ["Mystery Hour: The Key"])
    #expect(resume[0].link == URL(string: "broadwave://recording/7"))
    #expect(resume[0].progress == 0.25)
    #expect(resume[0].image == URL(string: "http://tv.local:8477/media/poster/7"))
}

@Test func topShelfLeavesOutEmptySectionsAndNamesAChannelWithNoListing() {
    let sections = TopShelf.sections(.init(channels: [channel(4, favorite: true, art: "logo.png")]), api: api, now: now)
    #expect(sections.map(\.title) == ["Favorites"])
    #expect(sections[0].items[0].title == "4.1 K4")
    #expect(sections[0].items[0].image == URL(string: "http://tv.local:8477/media/art/channel/4?w=320"))
    #expect(TopShelf.sections(.init(channels: [channel(4)]), api: api, now: now).isEmpty)
}

@Test func anOverlapShowsTheListingThatStartedFirst() {
    let early = airing(1, on: 4, title: "Early Show", from: -600, to: 600)
    let late = airing(2, on: 4, title: "Late Show", from: -60, to: 1200)
    let sections = TopShelf.sections(
        .init(channels: [channel(4, favorite: true)], airings: [early, late]), api: api, now: now
    )
    #expect(sections.map(\.title) == ["Favorites"])
    #expect(sections[0].items.map(\.title) == ["4.1 · Early Show"])
}
