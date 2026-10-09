@testable import BroadwaveKit
import Foundation
import Testing

private let now = Date(timeIntervalSince1970: 1_790_500_000)

private func channel(_ id: Int64, hidden: Bool = false) -> Channel {
    Channel(
        id: id, deviceId: "d", guideNumber: "\(id).1", guideName: "K\(id)", displayNumber: "\(id).1", displayName: "K\(id)",
        hd: true, favorite: false, enabled: true, hidden: hidden, present: true
    )
}

private func game(_ id: Int64, on channel: Int64, title: String, game: String? = nil, from: Double = -600, to: Double = 1200) -> Airing {
    Airing(
        id: id, channelId: channel, title: title, category: "Sports event", gameId: game, guideNumber: "\(channel).1",
        start: now.addingTimeInterval(from), end: now.addingTimeInterval(to)
    )
}

private let chiefs = [TeamFollow(name: "Kansas City Chiefs", short: "Chiefs")]

private func score(_ id: String, _ state: String, away: String = "3", home: String = "7") -> ScoreGame {
    ScoreGame(id: id, state: state, teams: [
        ScoreTeam(name: "Chiefs", abbr: "KC", score: away, home: false), ScoreTeam(name: "Broncos", abbr: "DEN", score: home, home: true),
    ])
}

@Test func aRecordingInProgressGetsAnActivityThatEndsWhenItStops() {
    let rec = Recording(
        id: 9, channelId: 4, guideNumber: "4.1", title: "Evening News", subtitle: "Storm coverage", status: "recording",
        startedAt: now.addingTimeInterval(-60), endsAt: now.addingTimeInterval(1800)
    )
    let items = LiveActivityFeed.wanted(TopShelf.Snapshot(channels: [channel(4)], airings: [], recordings: [rec]), follows: [], scores: [], now: now)
    #expect(items.map(\.id) == ["rec-9"])
    #expect(items[0].content == LiveActivityFeed.Content(
        title: "Evening News", number: "4.1", detail: "Storm coverage", start: now.addingTimeInterval(-60), end: now.addingTimeInterval(1800)
    ))
    #expect(items[0].link == URL(string: "broadwave://watch/4"))

    let started = LiveActivityFeed.plan(running: [:], wanted: items)
    #expect(started.start == items)

    var done = rec
    done.status = "complete"
    let snap = TopShelf.Snapshot(channels: [channel(4)], recordings: [done])
    let after = LiveActivityFeed.wanted(snap, follows: [], scores: [], now: now)
    let finals = LiveActivityFeed.finals(snap, follows: [], scores: [])
    let plan = LiveActivityFeed.plan(running: ["rec-9": items[0].content], wanted: after, finals: finals)
    #expect(plan.start.isEmpty && plan.update.isEmpty && plan.dismiss.isEmpty)
    #expect(plan.end["rec-9"]?.live == false)
    #expect(plan.end["rec-9"]?.title == "Evening News")
}

@Test func aFailedRecordingOrATurnedOffSettingGoesAtOnceWithoutSayingRecorded() {
    let running = Recording(id: 9, channelId: 4, guideNumber: "4.1", title: "News", status: "recording", startedAt: now)
    let item = LiveActivityFeed.wanted(TopShelf.Snapshot(channels: [channel(4)], recordings: [running]), follows: [], scores: [], now: now)[0]
    var failed = running
    failed.status = "failed"
    let snap = TopShelf.Snapshot(channels: [channel(4)], recordings: [failed])
    let plan = LiveActivityFeed.plan(
        running: [item.id: item.content], wanted: LiveActivityFeed.wanted(snap, follows: [], scores: [], now: now),
        finals: LiveActivityFeed.finals(snap, follows: [], scores: [])
    )
    #expect(plan.dismiss == ["rec-9"])
    #expect(plan.end.isEmpty)

    let off = LiveActivityFeed.Options(recordings: false, games: true)
    let still = TopShelf.Snapshot(channels: [channel(4)], recordings: [running])
    let offPlan = LiveActivityFeed.plan(
        running: [item.id: item.content], wanted: LiveActivityFeed.wanted(still, follows: [], scores: [], now: now, options: off),
        finals: LiveActivityFeed.finals(still, follows: [], scores: [], options: off)
    )
    #expect(offPlan.dismiss == ["rec-9"])
}

@Test func aFollowedGameShowsTheScoreAndUpdatesOnlyWhenItChanges() {
    let airings = [game(1, on: 5, title: "Chiefs at Broncos", game: "g1"), game(2, on: 6, title: "Jets at Bills", game: "g2")]
    let first = LiveActivityFeed.wanted(
        TopShelf.Snapshot(channels: [channel(5), channel(6)], airings: airings, recordings: []), follows: chiefs, scores: [score("g1", "in")], now: now
    )
    #expect(first.map(\.id) == ["game-g1"])
    #expect(first[0].kind == .game)
    #expect(first[0].content.detail == "KC 3 · DEN 7")

    let same = LiveActivityFeed.plan(running: ["game-g1": first[0].content], wanted: first)
    #expect(same == LiveActivityFeed.Plan())

    let next = LiveActivityFeed.wanted(
        TopShelf.Snapshot(channels: [channel(5), channel(6)], airings: airings, recordings: []), follows: chiefs, scores: [score("g1", "in", away: "10")], now: now
    )
    let plan = LiveActivityFeed.plan(running: ["game-g1": first[0].content], wanted: next)
    #expect(plan.update.map(\.content.detail) == ["KC 10 · DEN 7"])
}

@Test func aGameFollowsTheScoreboardPastItsListingAndEndsWhenFinal() {
    // Listed until 10 minutes ago: overtime keeps it while the scoreboard says it is on.
    let late = [game(1, on: 5, title: "Chiefs at Broncos", game: "g1", from: -12000, to: -600)]
    let overtime = LiveActivityFeed.wanted(
        TopShelf.Snapshot(channels: [channel(5)], airings: late, recordings: []), follows: chiefs, scores: [score("g1", "in")], now: now
    )
    #expect(overtime.map(\.id) == ["game-g1"])
    let final = LiveActivityFeed.wanted(
        TopShelf.Snapshot(channels: [channel(5)], airings: late, recordings: []), follows: chiefs, scores: [score("g1", "post")], now: now
    )
    #expect(final.isEmpty)
    // The scoreboard does not know the game yet: the listing decides.
    let pre = LiveActivityFeed.wanted(
        TopShelf.Snapshot(channels: [channel(5)], airings: [game(2, on: 5, title: "Chiefs at Broncos", from: 600, to: 9000)], recordings: []),
        follows: chiefs, scores: [], now: now
    )
    #expect(pre.isEmpty)
}

@Test func aGameTheHouseRecordsShowsAsTheRecordingWithoutTheScore() {
    let rec = Recording(id: 3, channelId: 5, guideNumber: "5.1", title: "Chiefs at Broncos", status: "recording", startedAt: now.addingTimeInterval(-600))
    let items = LiveActivityFeed.wanted(
        TopShelf.Snapshot(channels: [channel(5)], airings: [game(1, on: 5, title: "Chiefs at Broncos", game: "g1")], recordings: [rec]),
        follows: chiefs, scores: [score("g1", "in")], now: now
    )
    #expect(items.map(\.id) == ["rec-3"])
    #expect(!items[0].content.detail.contains("KC"))
}

@Test func hiddenChannelsAndTurnedOffKindsGetNoActivity() {
    let rec = Recording(id: 3, channelId: 4, guideNumber: "4.1", title: "News", status: "recording", startedAt: now)
    let airings = [game(1, on: 5, title: "Chiefs at Broncos", game: "g1"), game(2, on: 7, title: "Chiefs Rewind")]
    let channels = [channel(4), channel(5, hidden: true), channel(7)]
    let all = LiveActivityFeed.wanted(TopShelf.Snapshot(channels: channels, airings: airings, recordings: [rec]), follows: chiefs, scores: [score("g1", "in")], now: now)
    // A Chiefs title on a hidden channel and a non-sports listing do not count; Chiefs Rewind is sports here and on now.
    #expect(all.map(\.id) == ["rec-3", "game-2"])
    let gamesOnly = LiveActivityFeed.wanted(
        TopShelf.Snapshot(channels: channels, airings: airings, recordings: [rec]), follows: chiefs, scores: [], now: now,
        options: .init(recordings: false, games: true)
    )
    #expect(gamesOnly.map(\.id) == ["game-2"])
    let recordingsOnly = LiveActivityFeed.wanted(
        TopShelf.Snapshot(channels: channels, airings: airings, recordings: [rec]), follows: chiefs, scores: [], now: now,
        options: .init(recordings: true, games: false)
    )
    #expect(recordingsOnly.map(\.id) == ["rec-3"])
}

@Test func atMostThreeRunOldestFirst() {
    let recs = (1 ... 5).map { i in
        Recording(id: Int64(i), channelId: 4, guideNumber: "4.1", title: "Show \(i)", status: "recording", startedAt: now.addingTimeInterval(Double(-i) * 60))
    }
    let items = LiveActivityFeed.wanted(TopShelf.Snapshot(channels: [channel(4)], recordings: recs), follows: [], scores: [], now: now)
    #expect(items.map(\.id) == ["rec-5", "rec-4", "rec-3"])
}

@Test func aFinalGameEndsOnTheFinalScoreAndAGamePushedOutGoesAtOnce() {
    let late = [game(1, on: 5, title: "Chiefs at Broncos", game: "g1", from: -12000, to: -600)]
    let snap = TopShelf.Snapshot(channels: [channel(5)], airings: late)
    let running = ["game-g1": LiveActivityFeed.Content(title: "Chiefs at Broncos", number: "5.1", detail: "KC 3 · DEN 7", start: now)]
    let finalScores = [score("g1", "post", away: "24", home: "21")]
    let plan = LiveActivityFeed.plan(
        running: running, wanted: LiveActivityFeed.wanted(snap, follows: chiefs, scores: finalScores, now: now),
        finals: LiveActivityFeed.finals(snap, follows: chiefs, scores: finalScores)
    )
    #expect(plan.end["game-g1"]?.detail == "KC 24 · DEN 21")
    #expect(plan.end["game-g1"]?.live == false)

    // Three recordings started: the game is past the limit, not over.
    let recs = (1 ... 3).map { i in
        Recording(id: Int64(i), channelId: 4, guideNumber: "4.1", title: "Show \(i)", status: "recording", startedAt: now.addingTimeInterval(Double(-i) * 60))
    }
    let busy = TopShelf.Snapshot(channels: [channel(4), channel(5)], airings: [game(1, on: 5, title: "Chiefs at Broncos", game: "g1")], recordings: recs)
    let inScores = [score("g1", "in")]
    let pushed = LiveActivityFeed.plan(
        running: running, wanted: LiveActivityFeed.wanted(busy, follows: chiefs, scores: inScores, now: now),
        finals: LiveActivityFeed.finals(busy, follows: chiefs, scores: inScores)
    )
    #expect(pushed.dismiss == ["game-g1"])
    #expect(pushed.end.isEmpty)
}

@Test func aGameRecordedOnAnotherChannelOrOnTwoChannelsIsNoSpoilerAndOneActivity() {
    let airings = [game(1, on: 5, title: "Chiefs at Broncos", game: "g1"), game(2, on: 6, title: "Chiefs at Broncos", game: "g1")]
    let channels = [channel(5), channel(6)]
    let both = LiveActivityFeed.wanted(
        TopShelf.Snapshot(channels: channels, airings: airings), follows: chiefs, scores: [score("g1", "in")], now: now
    )
    #expect(both.map(\.id) == ["game-g1"])
    // Recorded on the twin, and the recording already stopped at its listed end: still no score.
    let twin = Recording(id: 7, channelId: 99, guideNumber: "105.1", title: "Chiefs at Broncos", gameId: "g1", status: "complete", startedAt: now.addingTimeInterval(-9000))
    let recorded = LiveActivityFeed.wanted(
        TopShelf.Snapshot(channels: channels, airings: airings, recordings: [twin]), follows: chiefs, scores: [score("g1", "in")], now: now
    )
    #expect(recorded.isEmpty)
}
