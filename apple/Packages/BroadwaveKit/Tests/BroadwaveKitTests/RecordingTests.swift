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
