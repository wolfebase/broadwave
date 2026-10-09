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

@Test func aCleanRecordingSaysNothingAboutTheSignal() {
    #expect(rec().signalLine == nil)
    var once = rec()
    once.health = RecordingHealth(continuityErrors: 1, transportErrors: 0, syncLosses: 0, packets: 8)
    #expect(once.signalLine == "Signal broke up once")
    var many = rec()
    many.health = RecordingHealth(continuityErrors: 12, transportErrors: 1, syncLosses: 1, packets: 40)
    #expect(many.signalLine == "Signal broke up 14 times")
    var clean = rec()
    clean.health = RecordingHealth(continuityErrors: 0, transportErrors: 0, syncLosses: 0, packets: 40)
    #expect(clean.signalLine == nil)
}

@Test func lostSecondsOutrankBreakupsInTheSignalLine() {
    var dropped = rec()
    dropped.health = RecordingHealth(continuityErrors: 30, transportErrors: 0, syncLosses: 2, packets: 40, gaps: 3, lostSeconds: 11.6, damaged: true)
    #expect(dropped.signalLine == "Signal dropped for 12 s")
    var brief = rec()
    brief.health = RecordingHealth(continuityErrors: 2, transportErrors: 0, syncLosses: 0, packets: 40, gaps: 1, lostSeconds: 0.4, damaged: false)
    #expect(brief.signalLine == "Signal broke up 2 times")
    var older = rec()
    older.health = RecordingHealth(continuityErrors: 1, transportErrors: 0, syncLosses: 0, packets: 8)
    #expect(older.signalLine == "Signal broke up once")
}

@Test func onlyADamagedFinishedRecordingOffersRecordItAgain() {
    var damaged = rec()
    damaged.health = RecordingHealth(continuityErrors: 0, transportErrors: 0, syncLosses: 0, packets: 40, lostSeconds: 6, damaged: true)
    #expect(damaged.isDamaged)
    // Nothing names the episode, so there is no airing to find.
    #expect(!RecordAgain.offered(damaged))
    damaged.subtitle = "The Lighthouse"
    #expect(RecordAgain.offered(damaged))
    var running = damaged
    running.status = "recording"
    #expect(!RecordAgain.offered(running))
    var gone = damaged
    gone.missing = true
    #expect(!RecordAgain.offered(gone))
    var fine = rec()
    fine.health = RecordingHealth(continuityErrors: 3, transportErrors: 0, syncLosses: 0, packets: 40, damaged: false)
    #expect(!fine.isDamaged)
    #expect(!RecordAgain.offered(fine))
    #expect(!rec().isDamaged)
}

@Test func recordAgainReadsTheNextAiringOrNone() throws {
    var url = URL(fileURLWithPath: #filePath)
    for _ in 0 ..< 6 {
        url.deleteLastPathComponent()
    }
    let none = try APIClient.decoder.decode(RecordAgain.Answer.self, from: Data(contentsOf: url.appendingPathComponent("api/fixtures/again.json")))
    #expect(none.airing == nil)
    let next = try APIClient.decoder.decode(RecordAgain.Answer.self, from: Data("""
    {"airing":{"id":9,"channelId":5,"title":"News","start":"2026-10-08T23:00:00Z","end":"2026-10-09T00:00:00Z"}}
    """.utf8))
    #expect(next.airing?.channelId == 5)
    #expect(next.airing?.start == Date(timeIntervalSince1970: 1_791_500_400))
}

@Test func recordAgainAsksForTheRecordingsNextAiring() async throws {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [AgainStub.self]
    let api = try APIClient(base: #require(URL(string: "http://stub.invalid")), session: URLSession(configuration: config))
    let airing = try await api.recordAgain(recordingID: 42)
    #expect(AgainStub.lastRequest == "GET /api/v1/recordings/42/again")
    #expect(airing?.channelId == 5)
    #expect(airing?.title == "News")
}

private final class AgainStub: URLProtocol, @unchecked Sendable {
    nonisolated(unsafe) static var lastRequest: String?

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        Self.lastRequest = "\(request.httpMethod ?? "") \(request.url?.path ?? "")"
        let json = #"{"airing":{"id":9,"channelId":5,"title":"News","start":"2026-10-08T23:00:00Z","end":"2026-10-09T00:00:00Z"}}"#
        let res = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: res, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(json.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

@Test func recordAgainSaysWhenItRecords() {
    // Thursday 2026-10-08 19:00 UTC.
    let start = Date(timeIntervalSince1970: 1_791_486_000)
    let said = RecordAgain.scheduled(start, now: start.addingTimeInterval(-86400), locale: Locale(identifier: "en_US"), timeZone: .gmt)
    #expect(said.replacingOccurrences(of: "\u{202F}", with: " ") == "Records again Thu 7:00 PM.")
    // Past six days out, the weekday alone could be either week.
    let far = RecordAgain.scheduled(start, now: start.addingTimeInterval(-8 * 86400), locale: Locale(identifier: "en_US"), timeZone: .gmt)
    #expect(far.replacingOccurrences(of: "\u{202F}", with: " ").contains("Oct 8"))
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

@Test func onlyASureBreakIsSkippedWithoutAsking() throws {
    #expect(Marker(id: 1, start: 0, end: 1, confidence: 0.95).isSure)
    #expect(Marker(id: 1, start: 0, end: 1, confidence: 0.7).isSure)
    #expect(!Marker(id: 1, start: 0, end: 1, confidence: 0.6).isSure)
    // Set by hand, or from a server before break scores.
    #expect(Marker(id: 1, start: 0, end: 1).isSure)
    let scored = try APIClient.decoder.decode(Marker.self, from: Data(#"{"id":3,"start":60,"end":180,"confidence":0.6}"#.utf8))
    #expect(scored.confidence == 0.6)
}

@Test func twoQuickJumpsForwardInABreakSkipIt() {
    let markers = [Marker(id: 7, start: 100, end: 220)]
    let start = Date()
    var jumps = ForwardJumps()
    #expect(jumps.observe(110, at: start, markers: markers) == nil)
    #expect(jumps.observe(110.25, at: start + 0.25, markers: markers) == nil)
    #expect(jumps.observe(125.25, at: start + 0.5, markers: markers) == nil) // first jump
    #expect(jumps.observe(140.25, at: start + 1.25, markers: markers)?.id == 7) // second
}

@Test func slowOrOutsideJumpsAreJustJumps() {
    let markers = [Marker(id: 7, start: 100, end: 220)]
    let start = Date()
    var slow = ForwardJumps()
    _ = slow.observe(110, at: start, markers: markers)
    _ = slow.observe(125, at: start + 0.25, markers: markers)
    #expect(slow.observe(140, at: start + 3, markers: markers) == nil)
    var outside = ForwardJumps()
    _ = outside.observe(50, at: start, markers: markers)
    _ = outside.observe(65, at: start + 0.25, markers: markers)
    #expect(outside.observe(80, at: start + 0.75, markers: markers) == nil)
    // Playing along is not a jump.
    var playing = ForwardJumps()
    for step in 0 ..< 20 {
        #expect(playing.observe(110 + Double(step) * 0.25, at: start + Double(step) * 0.25, markers: markers) == nil)
    }
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

@Test func scheduleStatesDecodeFromTheServer() throws {
    let plan = try APIClient.decoder.decode(SchedulePlan.self, from: Data("""
    {"tunerCount":1,"items":[
      {"passId":1,"airing":{"id":1,"channelId":4,"title":"Evening News","start":"2026-09-28T23:00:00Z","end":"2026-09-29T00:00:00Z"},"priority":5,"padBefore":1,"padAfter":2,"conflict":false,"skipped":false},
      {"passId":2,"airing":{"id":2,"channelId":5,"title":"Night Talk","start":"2026-09-28T23:00:00Z","end":"2026-09-29T00:00:00Z"},"priority":0,"padBefore":1,"padAfter":2,"conflict":false,"skipped":true,"reason":"Skipped once"},
      {"passId":3,"airing":{"id":3,"channelId":6,"title":"The Afternoon Game","start":"2026-09-28T23:00:00Z","end":"2026-09-29T00:00:00Z"},"priority":0,"padBefore":0,"padAfter":0,"conflict":true,"skipped":true,"suggestion":{"channelId":6,"guideNumber":"5.1","title":"The Afternoon Game","start":"2026-09-29T02:00:00Z","end":"2026-09-29T03:00:00Z"}}
    ]}
    """.utf8))
    #expect(plan.tunerCount == 1)
    #expect(plan.items.map(\.scheduleState) == [.willRecord, .skippedOnce, .conflict])
    #expect(plan.items[0].summary(tuners: 1).contains("Will record"))
    #expect(plan.items[1].summary(tuners: 1).contains("Skipped once"))
    #expect(plan.items[2].summary(tuners: 1).contains("Lower priority · 1 tuner"))
    #expect(plan.items[2].summary(tuners: 1).contains("5.1"))
    let kept = PlannedAiring(
        passId: 4,
        airing: Airing(id: 4, channelId: 4, title: "Evening News", start: Date(timeIntervalSince1970: 1_800_000_000), end: Date(timeIntervalSince1970: 1_800_003_600)),
        priority: 0, padBefore: 1, padAfter: 2, conflict: false, skipped: true, reason: "Already recorded"
    )
    #expect(kept.scheduleState == nil)
}

@Test func aScheduleFixSendsOnlyTheFieldsTheServerAccepts() throws {
    let item = planned()
    let later = Suggestion(channelId: 5, guideNumber: "5.1", title: "News", start: item.airing.start.addingTimeInterval(7200), end: item.airing.end.addingTimeInterval(7200))
    let body = try #require(JSONSerialization.jsonObject(with: JSONEncoder().encode(ScheduleFixRequest(item: item, later: later))) as? [String: Any])
    #expect(Set(body.keys) == ["passId", "channelId", "start", "suggestionChannelId", "suggestionStart"])
    #expect(body["passId"] as? Int == 7)
    #expect(body["channelId"] as? Int == 4)
    #expect(body["suggestionChannelId"] as? Int == 5)
    #expect(body["start"] as? String == ISO8601DateFormatter.plainString(from: item.airing.start))
    #expect(body["suggestionStart"] as? String == ISO8601DateFormatter.plainString(from: later.start))
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

@Test func aBreakReadsAsAClockSpan() {
    #expect(Marker(id: 1, start: 90, end: 120.4).span == "1:30–2:00")
    #expect(Marker(id: 2, start: 3599.6, end: 3725).span == "1:00:00–1:02:05")
    #expect(Marker(id: 3, start: -1, end: 5).span == "0:00–0:05")
}

@Test func aNewLibraryChannelTakesTheFirstFreeNumberFrom900() {
    let taken = [
        VirtualChannel(id: 1, number: "900", name: "A", recordings: []),
        VirtualChannel(id: 2, number: "902", name: "B", recordings: []),
    ]
    #expect(VirtualChannel.nextNumber(after: []) == "900")
    #expect(VirtualChannel.nextNumber(after: taken) == "901")
    #expect(VirtualChannel.nextNumber(after: taken + [VirtualChannel(id: 3, number: "901", name: "C", recordings: [])]) == "903")
}

@Test func libraryChannelPlaybackDecodesTheServerAnswer() throws {
    let play = try APIClient.decoder.decode(VirtualPlayback.self, from: Data("""
    {"usesTuner":false,"index":1,"count":2,"playlist":"/play/7/index.m3u8","number":"900","name":"Jeopardy! channel",
     "recording":{"channelId":1,"guideNumber":"4.1","id":7,"startedAt":"2026-09-24T15:00:00Z","status":"complete","title":"Jeopardy!"},
     "markers":[{"id":2,"recordingId":7,"start":90,"end":120}]}
    """.utf8))
    #expect(play.usesTuner == false)
    #expect(play.recording.id == 7)
    #expect(play.count == 2)
    #expect(play.markers?.first?.recordingId == 7)
}

@Test func aRecordingWhoseFileIsGoneDecodesAsMissing() throws {
    let decoder = JSONDecoder()
    decoder.dateDecodingStrategy = .iso8601
    let gone = try decoder.decode(Recording.self, from: Data(#"{"id":4,"channelId":1,"guideNumber":"4.1","title":"Show","status":"complete","startedAt":"2026-09-23T19:58:31Z","missing":true}"#.utf8))
    #expect(gone.isMissing)
    #expect(gone.durationSec == nil)
    let kept = try decoder.decode(Recording.self, from: Data(#"{"id":5,"channelId":1,"guideNumber":"4.1","title":"Show","status":"complete","startedAt":"2026-09-23T19:58:31Z","bytes":188}"#.utf8))
    #expect(!kept.isMissing)
}
