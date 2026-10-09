@testable import BroadwaveKit
import Foundation
import Testing

private func fixture(_ name: String) throws -> Data {
    var url = URL(fileURLWithPath: #filePath)
    for _ in 0 ..< 6 {
        url.deleteLastPathComponent()
    }
    return try Data(contentsOf: url.appendingPathComponent("api/fixtures/\(name).json"))
}

/// Every contract fixture decodes as the type the client uses for that route.
/// A loose stand-in would still pass after a required field was renamed.
@Test func contractFixturesDecodeAsTheClientTypes() throws {
    struct Channels: Decodable { var channels: [Channel] }
    struct Airings: Decodable { var airings: [Airing] }
    struct Recordings: Decodable { var recordings: [Recording] }
    struct Devices: Decodable { var devices: [Device] }
    struct Events: Decodable { var events: [Event] }
    struct Markers: Decodable { var markers: [Marker] }
    struct Signals: Decodable { var channels: [ChannelSignal]; var running: Bool }
    struct Tuners: Decodable { var tuners: [Tuner] }
    struct Virtuals: Decodable { var virtuals: [VirtualChannel] }
    struct Scheduled: Decodable { var virtuals: [ScheduledVirtual] }
    struct Sources: Decodable { var sources: [Source] }
    struct Games: Decodable { var games: [Game] }
    struct Scores: Decodable { var games: [ScoreGame] }
    struct Groups: Decodable { var groups: [RoomState] }
    struct Frame: Decodable { var type: String; var data: RoomState }
    struct Clock: Decodable { var serverTime: Double }
    struct Look: Decodable { var found: [APIClient.FoundHit] }
    struct Free: Decodable { var found: [APIClient.FreeFeed]; var guide: String }
    struct Starred: Decodable { var starred: [APIClient.StarredChannel] }
    struct Played: Decodable {
        var playlist: String
        var position: Double
        var growing: Bool
        var markers: [Marker]
        var recording: Recording
    }
    struct Diagnostics: Decodable {
        var guide: APIClient.GuideDepth
        var doctor: [APIClient.DoctorNote]
        var devices: [Device]
        var recentActivity: [Event]
    }

    let channels = try APIClient.decoder.decode(Channels.self, from: fixture("channels")).channels
    #expect(channels.count == 3)
    #expect(channels.first { $0.guideName == "KBWV" }?.network == "FOX")
    #expect(channels.allSatisfy { !$0.displayNumber.isEmpty })

    let patched = try APIClient.decoder.decode(Channel.self, from: fixture("channel"))
    #expect(patched.favorite)
    #expect(patched.displayName == "Fox 4")
    #expect(patched.guideName == "KBWV")

    let airings = try APIClient.decoder.decode(Airings.self, from: fixture("airings")).airings
    #expect(!airings.isEmpty)
    #expect(airings.allSatisfy { $0.end > $0.start })

    let recordings = try APIClient.decoder.decode(Recordings.self, from: fixture("recordings")).recordings
    #expect(recordings.first?.title == "Jeopardy!")
    #expect(recordings.first?.status == "complete")

    let server = try APIClient.decoder.decode(ServerInfo.self, from: fixture("server"))
    #expect(server.id == "contract-server")
    #expect(server.features.contains("wholeHomeSync"))
    let renamed = try APIClient.decoder.decode(ServerInfo.self, from: fixture("server-rename"))
    #expect(renamed.name == "Living Room")

    let devices = try APIClient.decoder.decode(Devices.self, from: fixture("devices")).devices
    #expect(devices.first?.deviceId == "FAKEHDHR")
    #expect(devices.first?.offline == false)
    let removed = try APIClient.decoder.decode(Devices.self, from: fixture("device-remove")).devices
    #expect(removed.map(\.deviceId) == ["FAKEHDHR", "src-1"])

    let passes = try APIClient.decoder.decode(PassList.self, from: fixture("passes")).passes
    #expect(passes.contains { $0.title == "Jeopardy!" })
    #expect(try APIClient.decoder.decode(PassList.self, from: fixture("pass-create")).passes.contains { $0.title == "Wheel of Fortune" })
    let deleted = try APIClient.decoder.decode(PassList.self, from: fixture("pass-delete")).passes
    #expect(!deleted.contains { $0.title == "Wheel of Fortune" })

    let teams = try APIClient.decoder.decode(TeamList.self, from: fixture("teams")).teams
    #expect(!teams.isEmpty)
    #expect(try APIClient.decoder.decode(TeamList.self, from: fixture("team-unfollow")).teams.isEmpty)

    let events = try APIClient.decoder.decode(Events.self, from: fixture("events")).events
    #expect(events.first?.kind == "guide")
    #expect(events.first?.message == "Listings are in.")

    let markers = try APIClient.decoder.decode(Markers.self, from: fixture("markers")).markers
    #expect(markers.allSatisfy { $0.end >= $0.start })
    let made = try APIClient.decoder.decode(Marker.self, from: fixture("marker-create"))
    #expect(made.recordingId == 1)
    #expect(try APIClient.decoder.decode(Markers.self, from: fixture("recording-detect")).markers.isEmpty)

    let signals = try APIClient.decoder.decode(Signals.self, from: fixture("signals"))
    #expect(!signals.running)
    #expect(signals.channels.map(\.number) == ["4.1", "4.2", "5.1"])

    let tuners = try APIClient.decoder.decode(Tuners.self, from: fixture("tuners")).tuners
    #expect(tuners.count == 2)
    #expect(tuners.allSatisfy { $0.ours == false })

    let settings = try APIClient.decoder.decode(Settings.self, from: fixture("settings"))
    #expect(settings.pictureMode == "broadcast")
    #expect(settings.bufferMinutes == "60")
    #expect(settings.lastGuidePull == nil)
    let saved = try APIClient.decoder.decode(Settings.self, from: fixture("settings-save"))
    #expect(saved.hideScores == "1")

    let plan = try APIClient.decoder.decode(MultiviewPlan.self, from: fixture("multiview"))
    #expect(plan.playable.count == 2)
    #expect(plan.tunersNeeded == 2)
    #expect(plan.blocked.isEmpty)

    let sync = try APIClient.decoder.decode(Frame.self, from: fixture("ws-sync"))
    #expect(sync.type == "sync.state")
    #expect(sync.data.room == "channel:1")
    #expect(sync.data.target(atServer: sync.data.anchorServer) == sync.data.anchorMedia)
    let group = try APIClient.decoder.decode(Frame.self, from: fixture("ws-group"))
    #expect(group.data.mode == "group")
    #expect(group.data.people?.first?.name == "Den TV")
    let groups = try APIClient.decoder.decode(Groups.self, from: fixture("groups")).groups
    #expect(groups.first?.room == "group:den")
    #expect(groups.first?.members == 1)

    let schedule = try APIClient.decoder.decode(SchedulePlan.self, from: fixture("schedule"))
    #expect(schedule.tunerCount == 2)
    #expect(schedule.items.isEmpty)

    let search = try APIClient.decoder.decode(SearchResult.self, from: fixture("search"))
    #expect(search.query == "Jeopardy")
    #expect(search.airings.first?.title == "Jeopardy!")
    #expect(search.recordings.first?.guideNumber == "4.1")

    let games = try APIClient.decoder.decode(Games.self, from: fixture("scoreboard")).games
    #expect(games.first?.id == "nfl-1")
    #expect(games.first?.league == "nfl")
    #expect(games.first?.name == "Bears at Bills")
    #expect(games.first?.state == "pre")
    #expect(games.first?.teams?.first?.abbr == "CHI")
    let scores = try APIClient.decoder.decode(Scores.self, from: fixture("scoreboard")).games
    #expect(scores.first?.id == "nfl-1")
    #expect(scores.first?.line == nil)

    let storage = try APIClient.decoder.decode(APIClient.StorageInfo.self, from: fixture("storage"))
    #expect(storage.watermarkGB == 10)
    #expect(storage.path == "/config/work/recordings")
    let shows = try APIClient.decoder.decode(StorageShows.self, from: fixture("storage-shows"))
    #expect(shows.shows.first?.title == "Evening News")
    #expect(shows.shows.first?.pass?.keep == 3)
    #expect(shows.shows.first?.newest ?? .distantPast > shows.shows.first?.oldest ?? .distantFuture)

    let frames = try APIClient.decoder.decode(FrameList.self, from: fixture("frames"))
    #expect(frames.channels.isEmpty)
    let clock = try APIClient.decoder.decode(Clock.self, from: fixture("clock"))
    #expect(clock.serverTime == 1_790_262_000_000)

    #expect(try APIClient.decoder.decode(Sources.self, from: fixture("sources")).sources.isEmpty)
    let xtream = try APIClient.decoder.decode(Source.self, from: fixture("xtream"))
    #expect(xtream.kind == "xtream")
    #expect(xtream.enabled)
    #expect(xtream.url?.contains("%E2%80%A2") == true)

    let home = try APIClient.decoder.decode(APIClient.HomeScan.self, from: fixture("home"))
    #expect(home.places.first?.action == "added")
    #expect(home.places.first?.kind == "hdhomerun")
    #expect(home.tunerAddress.hasSuffix(":8478"))
    #expect(!home.sharing)

    let looked = try APIClient.decoder.decode(Look.self, from: fixture("look")).found
    #expect(looked.first?.deviceID == "FAKEHDHR")
    #expect(looked.first?.kind == "hdhomerun")

    let free = try APIClient.decoder.decode(Free.self, from: fixture("free"))
    #expect(free.found.isEmpty)
    #expect(free.guide.contains("FastChannels"))

    let starred = try APIClient.decoder.decode(Starred.self, from: fixture("star")).starred
    #expect(starred.contains { $0.network == "FOX" })

    let scan = try APIClient.decoder.decode(APIClient.ScanProgress.self, from: fixture("scan-status"))
    #expect(scan.scanning)
    #expect(scan.found == 0)
    #expect(scan.line == "0 channels found.")

    let idle = try APIClient.decoder.decode(SetupFinish.self, from: fixture("setup-finish"))
    #expect(!idle.running)
    #expect(idle.steps.map(\.id) == ["scan", "guide", "folder", "favorites", "encoder", "signal"])
    let started = try APIClient.decoder.decode(SetupFinish.self, from: fixture("setup-finish-post"))
    #expect(started.running)
    let done = try APIClient.decoder.decode(SetupFinish.self, from: fixture("setup-finish-done"))
    #expect(done.channelId == 1)
    #expect(done.ready?.hasPrefix("Ready:") == true)

    let virtuals = try APIClient.decoder.decode(Virtuals.self, from: fixture("virtuals")).virtuals
    #expect(virtuals.first?.number == "9000")
    #expect(try APIClient.decoder.decode(VirtualChannel.self, from: fixture("virtual-create")).number == "9001")
    let scheduled = try APIClient.decoder.decode(Scheduled.self, from: fixture("virtual-schedule")).virtuals
    let slots = try #require(scheduled.first?.slots)
    #expect(slots.count > 20)
    #expect(slots.allSatisfy { $0.end > $0.start && $0.recordingId == 1 && $0.title == "Jeopardy!" })
    #expect(slots.first?.start.timeIntervalSince1970 == ISO8601DateFormatter.plain.date(from: "2026-09-24T15:00:00Z")?.timeIntervalSince1970)

    let played = try APIClient.decoder.decode(Played.self, from: fixture("recording-play"))
    #expect(played.playlist == "/media/file/1/index.m3u8")
    #expect(played.position == 12.5)
    #expect(!played.growing)
    #expect(played.markers.count == 1)
    #expect(played.recording.title == "Jeopardy!")
    let startOnly = try APIClient.decoder.decode(PlaybackStart.self, from: fixture("recording-play"))
    #expect(startOnly.playlist == played.playlist)
    #expect(startOnly.markers?.count == 1)

    let again = try APIClient.decoder.decode(RecordAgain.Answer.self, from: fixture("again"))
    #expect(again.airing == nil)

    let diagnostics = try APIClient.decoder.decode(Diagnostics.self, from: fixture("diagnostics"))
    #expect(diagnostics.guide.channels == 3)
    #expect(diagnostics.guide.channelsWithListings == 1)
    #expect(diagnostics.guide.airings == 1)
    #expect(diagnostics.doctor.isEmpty)
    #expect(diagnostics.devices.first?.friendlyName == "Fake HDHomeRun")
    #expect(diagnostics.recentActivity.first?.message == "Listings are in.")

    let discovered = try APIClient.decoder.decode(Devices.self, from: fixture("discover")).devices
    #expect(discovered.first?.tunerCount == 2)
}

/// Shapes the handlers write when the contract suite has no fixture file.
@Test func handlerShapesDecodeWhenTheContractSuiteSkipsThem() throws {
    struct Health: Decodable { var devices: [DeviceHealth] }
    let health = try APIClient.decoder.decode(Health.self, from: Data("""
    {"devices":[{"deviceId":"FAKEHDHR","model":"HDHR4-2US","firmwareVersion":"20260101","tuners":[{"index":0,"locked":true},{"index":1,"locked":false}],"error":"This tuner did not answer. Check that it is on."}]}
    """.utf8))
    #expect(health.devices.first?.tuners.map(\.locked) == [true, false])
    #expect(health.devices.first?.error == "This tuner did not answer. Check that it is on.")

    struct Backups: Decodable { var backups: [CatalogBackup] }
    let backups = try APIClient.decoder.decode(Backups.self, from: Data("""
    {"backups":[{"name":"broadwave-20260926-daily.db","kind":"daily","takenAt":"2026-09-26T08:00:00Z","bytes":4096}]}
    """.utf8))
    #expect(backups.backups.first?.kind == "daily")
    #expect(backups.backups.first?.bytes == 4096)

    let mosaic = try APIClient.decoder.decode(MosaicSession.self, from: Data("""
    {"key":"1-2","playlist":"/live/mosaic/1-2/index.m3u8","channelIds":[1,2],"encoder":"libx264","viewers":1}
    """.utf8))
    #expect(mosaic.channelIds == [1, 2])
    #expect(mosaic.viewers == 1)

    let alert = try APIClient.decoder.decode(GameAlert.self, from: Data("""
    {"id":"nfl-1-start","kind":"start","gameId":"nfl-1","channelId":1,"channel":"4.1","text":"Starting now: CHI at BUF","detail":""}
    """.utf8))
    #expect(alert.kind == "start")
    #expect(alert.channelId == 1)

    let preview = try APIClient.decoder.decode(PassPreview.self, from: Data("""
    {"tunerCount":2,"timeZone":"UTC","utcOffset":0,"items":[{"passId":1,"priority":0,"padBefore":0,"padAfter":1,"conflict":false,"skipped":true,"reason":"The tuner is busy.","airing":{"id":1,"channelId":1,"title":"News","start":"2026-09-24T15:00:00Z","end":"2026-09-24T16:00:00Z"}}],"bumps":[]}
    """.utf8))
    #expect(preview.tunerCount == 2)
    #expect(preview.timeZone == "UTC")
    #expect(preview.utcOffset == 0)
    #expect(preview.items.first?.skipped == true)
    #expect(preview.items.first?.airing.title == "News")
    #expect(preview.bumps.isEmpty)

    let damaged = try APIClient.decoder.decode(RecordingHealth.self, from: Data("""
    {"continuityErrors":2,"transportErrors":0,"syncLosses":1,"packets":1000,"gaps":1,"lostSeconds":0.4}
    """.utf8))
    #expect(damaged.continuityErrors == 2)
    #expect(damaged.lostSeconds == 0.4)
}

@Test func serverDatesKeepSubsecondsAndOffsets() throws {
    struct Dated: Decodable { var start: Date; var end: Date; var pulled: Date? }
    let body = Data("""
    {"start":"2026-09-24T15:04:05.123456789Z","end":"2026-09-24T10:00:00-05:00","pulled":"2026-09-24T15:04:05Z"}
    """.utf8)
    let dated = try APIClient.decoder.decode(Dated.self, from: body)
    let start = try #require(ISO8601DateFormatter.fractional.date(from: "2026-09-24T15:04:05.123Z"))
    #expect(abs(dated.start.timeIntervalSince(start)) < 0.001)
    let end = try #require(ISO8601DateFormatter.plain.date(from: "2026-09-24T15:00:00Z"))
    #expect(dated.end == end)
    #expect(dated.pulled == ISO8601DateFormatter.plain.date(from: "2026-09-24T15:04:05Z"))

    let settings = try APIClient.decoder.decode(Settings.self, from: Data("""
    {"pictureMode":"film","hideScores":"1","lastGuidePull":"2026-09-24T15:04:05Z","nextGuidePull":"2026-09-25T15:04:05.5Z","lastManualGuidePull":"2026-09-24T10:00:00-05:00"}
    """.utf8))
    #expect(settings.pictureMode == "film")
    #expect(settings.lastGuidePull == dated.pulled)
    #expect(settings.lastManualGuidePull == end)
    #expect(settings.nextGuidePull != nil)
}

@Test func decodesDoNotShareOneDecoder() async throws {
    let raw = Data(#"{"id":1,"channelId":1,"title":"News","start":"2026-09-24T15:00:00.123Z","end":"2026-09-24T16:00:00Z"}"#.utf8)
    let start = try #require(ISO8601DateFormatter.fractional.date(from: "2026-09-24T15:00:00.123Z"))
    let end = try #require(ISO8601DateFormatter.plain.date(from: "2026-09-24T16:00:00Z"))
    try await withThrowingTaskGroup(of: Airing.self) { group in
        for _ in 0 ..< 40 {
            group.addTask {
                try APIClient.decoder.decode(Airing.self, from: raw)
            }
        }
        for try await airing in group {
            #expect(airing.title == "News")
            #expect(airing.start == start)
            #expect(airing.end == end)
        }
    }
}
