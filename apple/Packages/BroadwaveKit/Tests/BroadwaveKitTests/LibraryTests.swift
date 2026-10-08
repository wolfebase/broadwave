@testable import BroadwaveKit
import Foundation
import Testing

private func rec(
    _ id: Int64, title: String = "Mystery Hour", subtitle: String? = nil, category: String? = nil, gameId: String? = nil,
    status: String = "complete", day: Double = 0, bytes: Int64? = nil, position: Double? = nil, duration: Double? = nil,
    watched: Int? = nil, season: Int? = nil, episode: Int? = nil, label: String? = nil, played: Date? = nil
) -> Recording {
    Recording(
        id: id, channelId: 1, guideNumber: "4.1", title: title, subtitle: subtitle, category: category, gameId: gameId,
        status: status, startedAt: Date(timeIntervalSince1970: 1_790_000_000 + day * 86400), bytes: bytes,
        position: position, durationSec: duration, watched: watched, season: season, episode: episode,
        episodeLabel: label, progressAt: played
    )
}

@Test func continueWatchingIsStartedAndNotFinishedLastPlayedFirst() {
    let now = Date(timeIntervalSince1970: 1_790_500_000)
    let list = [
        rec(1, position: 600, duration: 3600, played: now.addingTimeInterval(-86400)),
        rec(2, position: 900, duration: 3600, played: now),
        rec(3, position: 3590, duration: 3600),
        rec(4, position: 10, duration: 3600),
        rec(5, position: 600, duration: 3600, watched: 1),
        rec(6, position: 600, duration: 3600, watched: 2, played: now.addingTimeInterval(-2 * 86400)),
        rec(7, status: "recording", position: 600, duration: 3600),
    ]
    #expect(Library.continueWatching(list).map(\.id) == [2, 1, 6])
}

@Test func aShowGroupsBySeasonNewestSeasonFirstAndEpisodesInOrder() {
    let list = [
        rec(1, subtitle: "B", season: 1, episode: 2),
        rec(2, subtitle: "C", season: 2, episode: 1),
        rec(3, subtitle: "A", season: 1, episode: 1),
        rec(4, subtitle: "Special"),
        rec(5, title: "A Western", category: "Movie", bytes: 9),
    ]
    let built = Library.build(list, sort: .newest)
    #expect(built.shows.count == 1)
    #expect(built.shows[0].seasons.map(\.season) == [2, 1, 0])
    #expect(built.shows[0].seasons[1].items.map(\.subtitle) == ["B", "A"])
    #expect(built.shows[0].seasons[2].title == "Other episodes")
    #expect(built.movies.map(\.id) == [5])
    let oldest = Library.build(list, sort: .oldest).shows[0]
    #expect(oldest.seasons.map(\.season) == [1, 2, 0])
    #expect(oldest.seasons[0].items.map(\.subtitle) == ["A", "B"])
}

@Test func aShowCountsUnwatchedAndSpace() {
    let list = [rec(1, bytes: 1000, watched: 1), rec(2, bytes: 500), rec(3, title: "mystery hour ", bytes: 250)]
    let shows = Library.build(list, sort: .newest).shows
    #expect(shows.count == 1)
    #expect(shows[0].unwatched == 2)
    #expect(shows[0].bytes == 1750)
    #expect(shows[0].line.hasPrefix("3 recordings · 2 unwatched · "))
}

@Test func showsSortBySizeAndName() {
    let list = [rec(1, title: "Zed", bytes: 10), rec(2, title: "Alpha", bytes: 5), rec(3, title: "Mid", bytes: 50)]
    #expect(Library.build(list, sort: .largest).shows.map(\.title) == ["Mid", "Zed", "Alpha"])
    #expect(Library.build(list, sort: .title).shows.map(\.title) == ["Alpha", "Mid", "Zed"])
}

@Test func theLibraryFiltersByKindAndUnwatched() {
    let list = [rec(1), rec(2, category: "Movie"), rec(3, category: "Sports"), rec(4, gameId: "g1"), rec(5, watched: 1)]
    #expect(Library.filter(list, kind: .movies, unwatchedOnly: false).map(\.id) == [2])
    #expect(Library.filter(list, kind: .sports, unwatchedOnly: false).map(\.id) == [3, 4])
    #expect(Library.filter(list, kind: .shows, unwatchedOnly: true).map(\.id) == [1])
}

@Test func episodeTagsReadLikeTheWeb() {
    #expect(rec(1, season: 2, episode: 5).episodeTag == "S2 E5")
    #expect(rec(1, episode: 5).episodeTag == "E5")
    #expect(rec(1, label: "Part 2").episodeTag == "Part 2")
    #expect(rec(1).episodeTag == nil)
}

@Test func aRecordingDecodesItsEpisodeAndWhenItWasPlayed() throws {
    let json = Data(#"{"id":1,"channelId":1,"guideNumber":"4.1","title":"Mystery Hour","status":"complete","startedAt":"2026-10-05T19:00:00Z","season":2,"episode":5,"episodeLabel":"S2E5","progressAt":"2026-10-05T20:00:00Z"}"#.utf8)
    let decoder = JSONDecoder()
    decoder.dateDecodingStrategy = .iso8601
    let got = try decoder.decode(Recording.self, from: json)
    #expect(got.episodeTag == "S2 E5")
    #expect(got.progressAt == ISO8601DateFormatter().date(from: "2026-10-05T20:00:00Z"))
}

@MainActor
@Test func aBulkActionWithNoServerFailsEveryRecording() async {
    let store = AppStore()
    let result = await store.apply(.delete, to: [rec(1), rec(2)])
    #expect(result.failed == 2)
    #expect((result.error as? APIError)?.code == "offline")
}

@Test func aResumeWaitsForTheGrowingPlaylistToReachIt() {
    #expect(!ResumeReach.reached(seekableEnd: nil, position: 40))
    #expect(!ResumeReach.reached(seekableEnd: 2.002, position: 40))
    #expect(!ResumeReach.reached(seekableEnd: .nan, position: 40))
    #expect(ResumeReach.reached(seekableEnd: 39.8, position: 40))
    #expect(ResumeReach.reached(seekableEnd: 65.3, position: 40))
}
