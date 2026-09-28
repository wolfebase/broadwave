@testable import BroadwaveKit
import Foundation
import Testing

private func rec(status: String = "done", position: Double? = nil, duration: Double? = nil, watched: Int? = nil, category: String? = nil) -> Recording {
    Recording(
        id: 1, channelId: 1, guideNumber: "4.1", title: "News", category: category,
        status: status, startedAt: Date(timeIntervalSince1970: 0),
        position: position, durationSec: duration, watched: watched
    )
}

@Test func aRecordingIsWatchedLikeTheWebLibrary() {
    #expect(!rec().isWatched)
    #expect(!rec(position: 600, duration: 3600).isWatched)
    #expect(rec(position: 3590, duration: 3600).isWatched)
    #expect(rec(position: 3240, duration: 3600).isWatched)
    #expect(!rec(position: 5, duration: 8).isWatched)
    #expect(rec(watched: 1).isWatched)
    #expect(!rec(position: 3590, duration: 3600, watched: 2).isWatched)
}

@Test func aRecordingSaysWhatHappenedOnlyWhenItIsNotPlainlyDone() {
    #expect(rec().statusLabel == nil)
    #expect(rec(status: "recording").statusLabel == "Recording")
    #expect(rec(status: "stopped").statusLabel == "Stopped early")
    #expect(rec(status: "failed").statusLabel == "Failed")
    #expect(rec(category: "Movie").isMovie)
    #expect(!rec(category: "News").isMovie)
}

@Test func aBreakIsFoundUntilItsLastMoment() {
    let markers = [Marker(id: 1, start: 10, end: 20), Marker(id: 2, start: 40, end: 70)]
    #expect(BreakSkip.marker(in: markers, at: 9.9) == nil)
    #expect(BreakSkip.marker(in: markers, at: 10)?.id == 1)
    #expect(BreakSkip.marker(in: markers, at: 19.9)?.id == 1)
    #expect(BreakSkip.marker(in: markers, at: 19.96) == nil)
    #expect(BreakSkip.marker(in: markers, at: 20) == nil)
    #expect(BreakSkip.marker(in: markers, at: 55)?.id == 2)
    #expect(BreakSkip.marker(in: [], at: 55) == nil)
}

@Test func playbackStartReadsMarkersAndOlderServers() throws {
    let now = try APIClient.decoder.decode(PlaybackStart.self, from: Data("""
    {"playlist":"/p.m3u8","position":12,"growing":false,"markers":[{"id":3,"start":60,"end":180}]}
    """.utf8))
    #expect(now.markers?.first?.end == 180)
    let older = try APIClient.decoder.decode(PlaybackStart.self, from: Data("""
    {"playlist":"/p.m3u8","position":0,"growing":true}
    """.utf8))
    #expect(older.markers == nil)
}

@Test func aFailedActionSaysTheServersSentenceOrAPlainOne() {
    #expect(PlaybackOutage.actionMessage(APIError(code: "not_found", message: "That recording is gone.", status: 404)) == "That recording is gone.")
    #expect(PlaybackOutage.actionMessage(URLError(.timedOut)) == PlaybackOutage.requestFailed)
    #expect(PlaybackOutage.actionMessage(APIError(code: "internal", message: "", status: 500)) == PlaybackOutage.requestFailed)
}

private func planned(skipped: Bool = false, reason: String? = nil, later: Suggestion? = nil) -> PlannedAiring {
    let start = Date(timeIntervalSince1970: 1_800_000_000)
    return PlannedAiring(
        passId: 7,
        airing: Airing(id: 1, channelId: 4, title: "News", start: start, end: start.addingTimeInterval(3600)),
        priority: 0, padBefore: 1, padAfter: 2, conflict: skipped, skipped: skipped, reason: reason, suggestion: later
    )
}

@Test func aPlannedAiringSaysWhenItRecordsOrWhyNot() {
    let item = planned()
    #expect(item.recordWindow.lowerBound == item.airing.start.addingTimeInterval(-60))
    #expect(item.recordWindow.upperBound == item.airing.end.addingTimeInterval(120))
    #expect(item.statusLine(tuners: 2).hasPrefix("Will record "))
    #expect(planned(skipped: true).statusLine(tuners: 2) == "Lower priority · 2 tuners")
    #expect(planned(skipped: true).statusLine(tuners: 1) == "Lower priority · 1 tuner")
    #expect(planned(skipped: true, reason: "Both tuners are busy.").statusLine(tuners: 2) == "Both tuners are busy.")
    #expect(item.key == planned().key)
}

@Test func aLaterAiringNamesItsChannelWhenKnown() {
    let start = Date(timeIntervalSince1970: 1_800_086_400)
    let later = Suggestion(channelId: 5, guideNumber: "5.1", title: "News", start: start, end: start.addingTimeInterval(3600))
    #expect(planned().laterLine(later).hasPrefix("Later on "))
    #expect(planned().laterLine(later).hasSuffix(" on 5.1."))
    var bare = later
    bare.guideNumber = nil
    #expect(!planned().laterLine(bare).contains(" on 5.1"))
}
