@testable import BroadwaveKit
import Foundation
import Testing

private let now = Date(timeIntervalSince1970: 1_790_500_000)

private func channel(_ id: Int64, favorite: Bool = false, hidden: Bool = false) -> Channel {
    Channel(
        id: id, deviceId: "d", guideNumber: "\(id).1", guideName: "K\(id)", displayNumber: "\(id).1", displayName: "K\(id)",
        hd: true, favorite: favorite, enabled: true, hidden: hidden, present: true
    )
}

private func airing(
    _ id: Int64, on channel: Int64, title: String, category: String? = nil, game: String? = nil, from: Double = -600, to: Double = 1200
) -> Airing {
    Airing(
        id: id, channelId: channel, title: title, category: category, gameId: game, guideNumber: "\(channel).1",
        start: now.addingTimeInterval(from), end: now.addingTimeInterval(to)
    )
}

private func planned(_ airing: Airing, skipped: Bool = false, conflict: Bool = false) -> PlannedAiring {
    PlannedAiring(passId: 1, airing: airing, priority: 1, padBefore: 0, padAfter: 0, conflict: conflict, skipped: skipped)
}

@Test func onNowListsFavoritesAndOffersRecordOnlyWhenNothingIsSet() {
    let news = airing(1, on: 4, title: "Evening News")
    let quiz = airing(2, on: 5, title: "Quiz Night")
    let snap = TopShelf.Snapshot(
        channels: [channel(4, favorite: true), channel(5, favorite: true), channel(7, favorite: true), channel(9), channel(12, favorite: true, hidden: true)],
        airings: [news, quiz, airing(3, on: 9, title: "Movie"), airing(4, on: 7, title: "Later", from: 600)],
        recordings: [Recording(id: 1, channelId: 4, guideNumber: "4.1", title: "Evening News", status: "recording", startedAt: now)],
        plan: [planned(quiz)]
    )
    let rows = WidgetFeed.onNow(snap, now: now)
    #expect(rows.map(\.title) == ["Evening News", "Quiz Night", "K7"])
    #expect(rows[0].detail == "Recording")
    #expect(rows[0].record == nil)
    #expect(rows[1].detail == "Set to record")
    #expect(rows[1].record == nil)
    // Nothing listed now: the channel's name, no repeat of it, and nothing to record.
    #expect(rows[2].detail == "")
    #expect(rows[2].record == nil)
    #expect(rows[0].link == URL(string: "broadwave://watch/4"))
}

@Test func onNowWithoutFavoritesShowsWhatIsOn() {
    let snap = TopShelf.Snapshot(channels: [channel(4), channel(5)], airings: [airing(1, on: 5, title: "Quiz Night")])
    let rows = WidgetFeed.onNow(snap, now: now)
    #expect(rows.map(\.title) == ["Quiz Night"])
    #expect(rows[0].record == WidgetFeed.RecordAsk(channelID: 5, title: "Quiz Night", start: now.addingTimeInterval(-600)))
}

@Test func teamsShowsFollowedGamesLiveFirstWithTheScoreAndNoRecordWhenRecording() {
    let snap = TopShelf.Snapshot(
        channels: [channel(4), channel(5), channel(9)],
        airings: [
            airing(1, on: 5, title: "Chiefs at Broncos", category: "Sports event", game: "g1", from: 3600, to: 14400),
            airing(2, on: 4, title: "Royals at Twins", category: "Sports event", game: "g2"),
            airing(3, on: 9, title: "Jets at Bills", category: "Sports event", game: "g3"),
            airing(4, on: 4, title: "Chiefs Kingdom Report", category: "News"),
        ],
        recordings: [Recording(id: 1, channelId: 4, guideNumber: "4.1", title: "Royals at Twins", status: "recording", startedAt: now.addingTimeInterval(-600))]
    )
    let follows = [TeamFollow(name: "Kansas City Chiefs", short: "Chiefs"), TeamFollow(name: "Kansas City Royals", short: "Royals")]
    let scores = [ScoreGame(id: "g2", state: "in", teams: [
        ScoreTeam(name: "Royals", abbr: "KC", score: "3", home: false), ScoreTeam(name: "Twins", abbr: "MIN", score: "1", home: true),
    ])]
    let rows = WidgetFeed.teams(snap, follows: follows, scores: scores, now: now)
    #expect(rows.map(\.title) == ["Royals at Twins", "Chiefs at Broncos"])
    #expect(rows[0].detail == "KC 3 · MIN 1 · Recording")
    #expect(rows[0].record == nil)
    #expect(rows[1].detail == "")
    #expect(rows[1].record == WidgetFeed.RecordAsk(channelID: 5, title: "Chiefs at Broncos", start: now.addingTimeInterval(3600)))
}

@Test func upNextKeepsConflictsAndNamesTheChannel() {
    let items = [
        planned(airing(1, on: 4, title: "Later", from: 7200, to: 9000)),
        planned(airing(2, on: 4, title: "Skipped", from: 600, to: 1200), skipped: true),
        planned(airing(3, on: 5, title: "Soon", from: 300, to: 2100), skipped: true, conflict: true),
    ]
    let rows = WidgetFeed.upNext(items, channels: [channel(4), channel(5)], now: now)
    #expect(rows.map(\.title) == ["Soon", "Later"])
    #expect(rows.map(\.number) == ["5.1", "4.1"])
    #expect(rows[0].detail == "Conflict")
    #expect(WidgetFeed.refresh(after: rows, now: now) == now.addingTimeInterval(300))
    #expect(WidgetFeed.refresh(after: [], now: now) == now.addingTimeInterval(900))
}
