@testable import BroadwaveKit
import Foundation
import Testing

private let moment = Date(timeIntervalSince1970: 1_790_500_000)

private func at(_ day: Double) -> Date {
    Date(timeIntervalSince1970: 1_790_000_000 + day * 86400)
}

private func playlist(segments: Int, seconds: Double = 1, target: Int = 2, hold: String? = nil) -> String {
    var lines = ["#EXTM3U", "#EXT-X-TARGETDURATION:\(target)"]
    if let hold {
        lines.append("#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,HOLD-BACK=\(hold),PART-HOLD-BACK=1.500")
    }
    for index in 0 ..< segments {
        lines.append("#EXTINF:\(seconds),")
        lines.append("seg\(index).m4s")
    }
    return lines.joined(separator: "\n")
}

private func recording(
    _ id: Int64,
    title: String = "Harbor",
    subtitle: String? = nil,
    category: String? = nil,
    programId: String? = nil,
    gameId: String? = nil,
    status: String = "complete",
    on day: Double = 0,
    started: Date? = nil,
    ends: Date? = nil,
    bytes: Int64? = nil,
    position: Double? = nil,
    duration: Double? = nil,
    watched: Int? = nil,
    missing: Bool? = nil,
    season: Int? = nil,
    episode: Int? = nil,
    label: String? = nil,
    played: Date? = nil,
    channel: Int64 = 4
) -> Recording {
    Recording(
        id: id, channelId: channel, guideNumber: "\(channel).1", title: title, subtitle: subtitle, category: category,
        programId: programId, gameId: gameId, status: status, startedAt: started ?? at(day), endsAt: ends, bytes: bytes,
        position: position, durationSec: duration, watched: watched, missing: missing, season: season, episode: episode,
        episodeLabel: label, progressAt: played
    )
}

private func channel(_ id: Int64, name: String, favorite: Bool = false, enabled: Bool = true) -> Channel {
    Channel(
        id: id, deviceId: "home", guideNumber: "\(id).1", guideName: name, displayNumber: "\(id).1", displayName: name,
        hd: true, favorite: favorite, enabled: enabled, hidden: false, present: true
    )
}

private func airing(
    _ id: Int64, on channel: Int64, title: String, subtitle: String? = nil, guide: String? = nil, from: Double, to: Double
) -> Airing {
    Airing(
        id: id, channelId: channel, title: title, subtitle: subtitle, guideNumber: guide, start: moment.addingTimeInterval(from),
        end: moment.addingTimeInterval(to)
    )
}

private func planned(_ airing: Airing) -> PlannedAiring {
    PlannedAiring(passId: 1, airing: airing, priority: 1, padBefore: 0, padAfter: 0, conflict: false, skipped: false)
}

private func row(_ title: String, start: Date? = nil, end: Date? = nil) -> WidgetFeed.Row {
    WidgetFeed.Row(id: title, number: "4.1", title: title, start: start, end: end, link: URL(string: "broadwave://watch/4")!)
}

private func healthySnap() -> RecoverySnap {
    RecoverySnap(health: true, freeTuner: true, tunerAnswers: true, online: true, signalLost: false)
}

@Test func aZeroOrUnreadableHoldBackUsesThreeTargetDurations() {
    // Target 2. Three targets plus one is 8 s. A hold of 0, a negative hold, or a hold that
    // does not parse is that distance, not zero and not PART-HOLD-BACK (1.5 s, which would pass at 4 s).
    for hold in ["0", "-1", "nope"] {
        #expect(!LiveReadiness.ready(playlist(segments: 4, hold: hold)))
        #expect(LiveReadiness.ready(playlist(segments: 8, hold: hold)))
    }
}

@Test func paddedPlaylistLinesStillCount() {
    // Hold-back 2 and target 2: 4 s is enough. Three targets would still be short, and the part hold-back is 9 s.
    let lines = [
        "  #EXTM3U",
        "\t#EXT-X-TARGETDURATION:2  ",
        " #EXT-X-SERVER-CONTROL:HOLD-BACK=2,PART-HOLD-BACK=9",
        "  #EXTINF:2.0,",
        "a.m4s",
        "  #EXTINF:2.0,",
        "b.m4s",
    ]
    #expect(LiveReadiness.ready(lines.joined(separator: "\r\n")))
}

@Test func aPlaylistWithoutAPositiveTargetDurationIsNotReady() {
    let media = "#EXTINF:30,\nseg0.m4s\n"
    #expect(!LiveReadiness.ready("#EXTM3U\n\(media)"))
    #expect(!LiveReadiness.ready("#EXTM3U\n#EXT-X-TARGETDURATION:soon\n\(media)"))
    #expect(!LiveReadiness.ready("#EXTM3U\n#EXT-X-TARGETDURATION:0\n\(media)"))
}

@Test func anUnreadableSegmentDurationAddsNothing() {
    let good = """
    #EXTM3U
    #EXT-X-TARGETDURATION:2
    #EXT-X-SERVER-CONTROL:HOLD-BACK=2
    #EXTINF:nope,
    bad.m4s
    #EXTINF:4.0,
    good.m4s
    """
    let onlyBad = """
    #EXTM3U
    #EXT-X-TARGETDURATION:2
    #EXT-X-SERVER-CONTROL:HOLD-BACK=2
    #EXTINF:nope,
    bad.m4s
    """
    #expect(LiveReadiness.ready(good))
    #expect(!LiveReadiness.ready(onlyBad))
}

@Test func aNonFiniteOrNegativeDurationHasNoCardTime() {
    // Infinity is greater than zero, so only the finite check keeps it from falling through.
    #expect(UpNext.cardTime(duration: .infinity, creditsStart: 10) == nil)
    #expect(UpNext.cardTime(duration: -20, creditsStart: nil) == nil)
}

@Test func theUpNextClockIgnoresANonFinitePlayheadAndCapsAfterASeekBack() {
    var next = UpNext(autoplay: true)
    #expect(next.observe(3590, duration: 3600, creditsStart: nil, hasNext: true) == .card(left: 10))
    #expect(next.observe(.infinity, duration: 3600, creditsStart: nil, hasNext: true) == .none)
    #expect(next.observe(3592, duration: 3600, creditsStart: nil, hasNext: true) == .card(left: 10))

    var seek = UpNext(autoplay: true)
    #expect(seek.observe(3596, duration: 3600, creditsStart: nil, hasNext: true) == .card(left: 10))
    #expect(seek.observe(3591, duration: 3600, creditsStart: nil, hasNext: true) == .card(left: 10))
    // Still counted from 3596. Starting the clock over at 3591 would leave 8 s here, and no cap would have said 15.
    #expect(seek.observe(3593, duration: 3600, creditsStart: nil, hasNext: true) == .card(left: 10))
}

@Test func aShortTeamNameDoesNotMatchAndAnEmptyOneUsesTheFullName() {
    let harbor = airing(1, on: 4, title: "Harbor", from: 0, to: 3600)
    #expect(!WidgetFeed.followed(harbor, [TeamFollow(name: "Harbor", short: "Har")]))
    #expect(WidgetFeed.followed(harbor, [TeamFollow(name: "Harbor", short: "")]))
    #expect(WidgetFeed.followed(airing(2, on: 4, title: "Harbor at Valley", from: 0, to: 3600), [TeamFollow(name: "Valley")]))
    #expect(WidgetFeed.followed(airing(3, on: 4, title: "Evening News", from: 0, to: 3600), [TeamFollow(name: "Evening News", short: "News")]))
    let subtitle = airing(4, on: 4, title: "Evening News", subtitle: "harbor", from: 0, to: 3600)
    #expect(WidgetFeed.followed(subtitle, [TeamFollow(name: "Evening News", short: "Harbor")]))
}

@Test func refreshIgnoresAPastEdgeAndWaitsAtMostFifteenMinutes() {
    let past = row("Harbor", start: moment.addingTimeInterval(-60), end: moment)
    let ending = row("Valley", start: moment.addingTimeInterval(-60), end: moment.addingTimeInterval(120))
    let later = row("Evening News", start: moment.addingTimeInterval(20 * 60), end: moment.addingTimeInterval(40 * 60))
    #expect(WidgetFeed.refresh(after: [past], now: moment) == moment.addingTimeInterval(900))
    #expect(WidgetFeed.refresh(after: [ending], now: moment) == moment.addingTimeInterval(120))
    #expect(WidgetFeed.refresh(after: [later], now: moment) == moment.addingTimeInterval(900))
}

@Test func upNextUsesTheGuideNumberWhenTheChannelIsMissing() {
    let soon = airing(1, on: 8, title: "Harbor", subtitle: "Valley", guide: "8.1", from: 300, to: 900)
    let past = airing(2, on: 9, title: "Valley", subtitle: "Harbor", guide: "9.1", from: -10, to: 900)
    let bare = airing(3, on: 10, title: "Evening News", from: 600, to: 1200)
    let rows = WidgetFeed.upNext([planned(past), planned(bare), planned(soon)], channels: [], now: moment)
    #expect(rows.map(\.title) == ["Harbor", "Evening News"])
    #expect(rows.map(\.number) == ["8.1", ""])
    #expect(rows.map(\.detail) == ["Valley", ""])
    #expect(rows[0].link == URL(string: "broadwave://recordings"))
}

@Test func onNowSkipsADisabledFavoriteAndHonorsTheLimit() {
    let snap = TopShelf.Snapshot(
        channels: [
            channel(4, name: "Harbor", favorite: true, enabled: false),
            channel(5, name: "Valley", favorite: true),
            channel(6, name: "Evening News", favorite: true),
        ],
        airings: [
            airing(1, on: 5, title: "Valley", from: -60, to: 600),
            airing(2, on: 6, title: "Evening News", from: -60, to: 600),
        ]
    )
    #expect(WidgetFeed.onNow(snap, now: moment, limit: 1).map(\.title) == ["Valley"])
}

@Test func recordAgainSaysWhatTheGuideHas() {
    #expect(RecordAgain.label == "Record it again")
    #expect(RecordAgain.noAiring == "The guide has no other airing yet.")
    #expect(RecordAgain.failed == "Could not schedule it. Try again.")
}

@Test func recordAgainIsOfferedWhenTheProgramIsNamed() {
    var episode = recording(1, title: "Harbor", programId: "SH-HARBOR")
    episode.health = RecordingHealth(
        continuityErrors: 0, transportErrors: 0, syncLosses: 0, packets: 20, lostSeconds: 6, damaged: true
    )
    #expect(RecordAgain.offered(episode))
}

@Test func recordAgainNamesTheDayOnlyPastSixDays() {
    // Thursday 2026-10-08 19:00 UTC. Six days out is still just the weekday.
    let start = Date(timeIntervalSince1970: 1_791_486_000)
    let edge = RecordAgain.scheduled(start, now: start.addingTimeInterval(-6 * 86400), locale: Locale(identifier: "en_US"), timeZone: .gmt)
    let past = RecordAgain.scheduled(start, now: start.addingTimeInterval(-6 * 86400 - 1), locale: Locale(identifier: "en_US"), timeZone: .gmt)
    #expect(edge.replacingOccurrences(of: "\u{202F}", with: " ") == "Records again Thu 7:00 PM.")
    #expect(past.replacingOccurrences(of: "\u{202F}", with: " ").contains("Oct 8"))
}

@Test func continueWatchingSkipsAShortOrMissingPlayAndFallsBackToWhenItAired() {
    let rows = [
        recording(9, position: 30, duration: 3600, played: at(20)),
        recording(8, position: nil, duration: 3600, played: at(20)),
        recording(7, position: 100, duration: 3600, missing: true, played: at(20)),
        recording(4, on: 0, position: 100, duration: 3600, played: at(4)),
        recording(1, on: 3, position: 31, duration: 3600),
        recording(3, on: 9, position: 100, duration: 3600, played: at(2)),
        recording(2, on: 1, position: 100, duration: 3600),
    ]
    #expect(Library.continueWatching(rows, limit: 3).map(\.id) == [4, 1, 3])
    #expect(Library.continueWatching(rows).map(\.id) == [4, 1, 3, 2])
}

@Test func showsSortByTheirLatestRecording() {
    let all = [
        recording(1, title: "Harbor", on: 0, season: 1, episode: 1),
        recording(2, title: "Harbor", on: 5, season: 2, episode: 1),
        recording(4, title: "Evening News", on: 3, season: 4, episode: 2),
        recording(3, title: "Valley", on: 1, season: 1, episode: 1),
    ]
    let newest = Library.build(all, sort: .newest).shows
    #expect(newest.map(\.title) == ["Harbor", "Evening News", "Valley"])
    #expect(newest[1].seasons.map(\.title) == ["Season 4"])
    #expect(Library.build(all, sort: .oldest).shows.map(\.title) == ["Valley", "Evening News", "Harbor"])
}

@Test func moviesFollowTheSameSortAndAnEmptyLibraryIsEmpty() {
    let movies = [
        recording(1, title: "Evening News", category: "Movie", on: 1, bytes: 40),
        recording(3, title: "Harbor", category: "Movie", on: 5, bytes: 40),
        recording(2, title: "Valley", category: "Movie", on: 2, bytes: 10),
    ]
    #expect(Library.build(movies, sort: .newest).movies.map(\.id) == [3, 2, 1])
    #expect(Library.build(movies, sort: .oldest).movies.map(\.id) == [1, 2, 3])
    #expect(Library.build(movies, sort: .title).movies.map(\.id) == [1, 3, 2])
    #expect(Library.build(movies, sort: .largest).movies.map(\.id) == [3, 1, 2])
    #expect(Library.build(movies, sort: .newest).shows.isEmpty)
    let empty = Library.build([], sort: .oldest)
    #expect(empty.shows.isEmpty)
    #expect(empty.movies.isEmpty)
}

@Test func equalNamesAndAiringsBreakById() {
    let tied = [
        recording(2, title: "Harbor", started: at(0), season: 1, episode: 1),
        recording(3, title: "Harbor", started: at(0), season: 1, episode: 1),
        recording(1, title: "Harbor", started: at(0), season: 1, episode: 1),
    ]
    #expect(Library.build(tied, sort: .newest).shows[0].items.map(\.id) == [3, 2, 1])
    #expect(Library.build(tied, sort: .oldest).shows[0].items.map(\.id) == [1, 2, 3])
    #expect(Library.nextEpisode(after: tied[2], in: tied)?.id == 2)
    #expect(Library.nextEpisode(after: tied[0], in: tied)?.id == 3)
    #expect(Library.nextEpisode(after: tied[1], in: tied) == nil)

    let named = [
        recording(1, title: "Harbor", subtitle: "Valley", bytes: 8, season: 1, episode: 1),
        recording(2, title: "Harbor", subtitle: "Valley", bytes: 8, season: 1, episode: 2),
        recording(4, title: "Harbor", subtitle: "Valley", bytes: 3, season: 1, episode: 3),
    ]
    #expect(Library.build(named, sort: .title).shows[0].items.map(\.id) == [4, 2, 1])
    #expect(Library.build(named, sort: .largest).shows[0].items.map(\.id) == [2, 1, 4])
    let fallback = [
        recording(1, title: "Harbor", season: 1, episode: 1),
        recording(2, title: "Harbor", subtitle: "Evening News", season: 1, episode: 2),
    ]
    #expect(Library.build(fallback, sort: .title).shows[0].items.map(\.id) == [2, 1])
}

@Test func theLibraryKeepsEveryKindAndAMovieWinsOverAGame() {
    let list = [
        recording(1, title: "Harbor"),
        recording(2, title: "Valley", category: "Movie"),
        recording(3, title: "Evening News", category: "Sports"),
        recording(4, title: "Harbor", watched: 1),
        recording(5, title: "Valley", category: "Movie", gameId: "game-1"),
        recording(6, title: "Evening News", gameId: ""),
    ]
    #expect(Library.filter(list, kind: .all, unwatchedOnly: false).map(\.id) == [1, 2, 3, 4, 5, 6])
    #expect(Library.filter(list, kind: .all, unwatchedOnly: true).map(\.id) == [1, 2, 3, 5, 6])
    #expect(Library.filter([list[4]], kind: .movies, unwatchedOnly: false).map(\.id) == [5])
    #expect(Library.filter([list[4]], kind: .sports, unwatchedOnly: false).isEmpty)
    #expect(Library.filter([list[5]], kind: .sports, unwatchedOnly: false).isEmpty)
    #expect(Library.filter([list[5]], kind: .shows, unwatchedOnly: false).map(\.id) == [6])
}

@Test func oneRecordingAndAFinishedShowReadInTheSingular() {
    let single = Library.build([recording(1, title: " Harbor ", bytes: nil)], sort: .newest).shows[0]
    #expect(single.id == "harbor")
    #expect(single.bytes == 0)
    #expect(single.line.hasPrefix("1 recording · 1 unwatched · "))
    let finished = Library.build([
        recording(1, title: "Harbor", bytes: 10, watched: 1),
        recording(2, title: " harbor", bytes: nil, watched: 1),
    ], sort: .title).shows
    #expect(finished.count == 1)
    #expect(finished[0].bytes == 10)
    #expect(finished[0].id == "harbor")
    #expect(finished[0].line.hasPrefix("2 recordings · all watched · "))
}

@Test func episodeTagsSkipAZeroAndAnEmptyLabel() {
    #expect(recording(1, season: 2, label: "Part 1").episodeTag == "Part 1")
    #expect(recording(1, season: 2, episode: 0, label: "Part 1").episodeTag == "Part 1")
    #expect(recording(1, season: 0, episode: 4).episodeTag == "E4")
    #expect(recording(1, season: 2).episodeTag == nil)
    #expect(recording(1, label: "").episodeTag == nil)
}

@Test func theLibrarySummaryCountsAMissingSizeAsZero() {
    let one = recording(1, title: "Evening News", bytes: nil)
    let two = recording(2, title: "Valley", bytes: 1500)
    #expect(Library.totalBytes([]) == 0)
    #expect(Library.totalBytes([one, two]) == 1500)
    #expect(Library.summary([]).hasPrefix("0 recordings · "))
    #expect(Library.summary([one]).hasPrefix("1 recording · "))
    #expect(Library.summary([one, two]).hasPrefix("2 recordings · "))
}

@Test func librarySortAndKindLabels() {
    #expect(Library.Sort.newest.label == "Newest first")
    #expect(Library.Sort.oldest.label == "Oldest first")
    #expect(Library.Sort.title.label == "By name")
    #expect(Library.Sort.largest.label == "Largest first")
    #expect(Library.Kind.all.label == "Everything")
    #expect(Library.Kind.shows.label == "Shows")
    #expect(Library.Kind.movies.label == "Movies")
    #expect(Library.Kind.sports.label == "Sports")
}

@Test func aFatalStallAsksAtOnceAndAQuietOneWaits() {
    let clock = ServerOutage()
    let now = Date(timeIntervalSince1970: 4000)
    #expect(clock.shouldProbe(at: now, fatal: true))
    #expect(!clock.shouldProbe(at: now, fatal: false))
}

@Test func anotherStallSampleDoesNotMoveTheClock() {
    var clock = ServerOutage()
    let start = Date(timeIntervalSince1970: 5000)
    clock.noteWaiting(at: start)
    clock.noteWaiting(at: start.addingTimeInterval(7))
    #expect(!clock.shouldProbe(at: start.addingTimeInterval(7.9), fatal: false))
    #expect(clock.shouldProbe(at: start.addingTimeInterval(8), fatal: false))
}

@Test func aNamedOutageStaysUpWhenThePictureMoves() {
    var clock = ServerOutage()
    let start = Date(timeIntervalSince1970: 6000)
    #expect(clock.surface(PlaybackOutage.pictureStopped) == PlaybackOutage.pictureStopped)
    clock.notePlaying()
    clock.noteWaiting(at: start)
    #expect(clock.resolve(at: start.addingTimeInterval(30), snap: healthySnap(), fatal: true) == nil)
}

@Test func aFineStallWithoutAPictureBeforeItIsNotNamed() {
    var clock = ServerOutage()
    let start = Date(timeIntervalSince1970: 7000)
    clock.noteWaiting(at: start)
    #expect(clock.resolve(at: start.addingTimeInterval(8), snap: healthySnap(), fatal: false) == nil)
    #expect(clock.resolve(at: start.addingTimeInterval(16), snap: healthySnap(), fatal: false) == nil)
    #expect(clock.shouldProbe(at: start.addingTimeInterval(24), fatal: false))
}

@Test func systemPhrasesAndABlankLineDoNotReachTheViewer() {
    #expect(PlaybackOutage.viewerMessage("  \n\t ") == PlaybackOutage.channelDidNotStart)
    #expect(PlaybackOutage.viewerMessage("Bad Gateway") == PlaybackOutage.channelDidNotStart)
    #expect(PlaybackOutage.viewerMessage("Service Unavailable") == PlaybackOutage.channelDidNotStart)
    #expect(PlaybackOutage.viewerMessage("Gateway Timeout") == PlaybackOutage.channelDidNotStart)
    #expect(PlaybackOutage.viewerMessage("The body is not valid JSON.") == PlaybackOutage.channelDidNotStart)
    #expect(PlaybackOutage.viewerMessage("The data couldn't be read because it is missing.") == PlaybackOutage.channelDidNotStart)
    #expect(PlaybackOutage.viewerMessage("The operation couldn't be completed.") == PlaybackOutage.channelDidNotStart)
    #expect(PlaybackOutage.viewerMessage("The operation couldn’t be completed.") == PlaybackOutage.channelDidNotStart)
    #expect(PlaybackOutage.viewerMessage("NSURLErrorDomain") == PlaybackOutage.channelDidNotStart)
    #expect(PlaybackOutage.viewerMessage("  Harbor is on.  ") == "Harbor is on.")
    let gateway = APIError(code: "http_502", message: "Internal Server Error", status: 502)
    #expect(PlaybackOutage.actionMessage(gateway) == PlaybackOutage.requestFailed)
}

@Test func aClosedConnectionIsTheNetworkEvenWhenTheSentenceIsFine() {
    #expect(PlaybackOutage.viewerFailure(code: "", status: 0, message: "Harbor is on.", online: true).message == PlaybackOutage.serverStopped)
    #expect(PlaybackOutage.viewerFailure(code: "", status: 0, message: "Harbor is on.", online: false).message == PlaybackOutage.connectionDropped)
    #expect(PlaybackOutage.viewerFailure(code: "http", status: 200, message: "NetworkError", online: true).recovery == .server)
    let fetched = PlaybackOutage.viewerFailure(code: "http", status: 502, message: "Failed to fetch", online: false)
    #expect(fetched.message == PlaybackOutage.connectionDropped)
    #expect(PlaybackOutage.viewerFailure(code: "http", status: 404, message: "Load failed", online: true).message == PlaybackOutage.serverStopped)
}

@Test func aNamedStartFailureWinsOverANetworkStatus() {
    let busy = PlaybackOutage.viewerFailure(code: "tuners_busy", status: 0, message: "Harbor is on.", online: false)
    #expect(busy == OutageDecision(message: "Harbor is on.", recovery: .busy))
    let dark = PlaybackOutage.viewerFailure(code: "no_signal", status: 0, message: "Load failed", online: false)
    #expect(dark == OutageDecision(message: PlaybackOutage.noSignal, recovery: nil))
    let stream = PlaybackOutage.viewerFailure(code: "stream_down", status: 0, message: "Failed to fetch", online: false)
    #expect(stream == OutageDecision(message: PlaybackOutage.pictureStopped, recovery: nil))
}

@Test func aSignalOrAFullPictureBudgetIsNotAskedLikeATuner() {
    #expect(PlaybackOutage.startAttempts(code: "no_signal", message: "the tuner did not lock") == 1)
    #expect(PlaybackOutage.startAttempts(code: "tuners_busy", message: "tuner") == 1)
    #expect(PlaybackOutage.startAttempts(code: "pictures_full", message: "tuner") == 10)
}

@Test func aTunerWithOnlyAGuideOrOnlyATargetIsBusy() {
    #expect(!PlaybackOutage.aTunerIsFree([Tuner(index: 0, guide: "4.1", ours: true)]))
    #expect(!PlaybackOutage.aTunerIsFree([Tuner(index: 1, target: "live", ours: false)]))
}

@Test func aDownServerWinsOverALostSignal() {
    let down = RecoverySnap(health: false, freeTuner: true, tunerAnswers: true, online: true, signalLost: true, watchGone: true)
    #expect(PlaybackOutage.classify(down) == OutageDecision(message: PlaybackOutage.serverStopped, recovery: .server))
    let offline = RecoverySnap(health: false, freeTuner: true, tunerAnswers: true, online: false, signalLost: true, watchGone: true)
    #expect(PlaybackOutage.classify(offline) == OutageDecision(message: PlaybackOutage.connectionDropped, recovery: .server))
    let quiet = RecoverySnap(health: false, freeTuner: true, tunerAnswers: true, online: true, signalLost: false)
    #expect(!PlaybackOutage.recoveryReady(.signal, quiet))
    let phone = RecoverySnap(health: true, freeTuner: true, tunerAnswers: true, online: false, signalLost: false)
    #expect(!PlaybackOutage.recoveryReady(.restart, phone))
}
