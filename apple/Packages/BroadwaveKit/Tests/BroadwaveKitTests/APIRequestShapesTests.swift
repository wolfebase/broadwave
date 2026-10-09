@testable import BroadwaveKit
import Foundation
import Testing

/// Request method, path, and body for APIClient calls the other suites do not already lock.
/// One stub, so the suite runs one test at a time.
@Suite(.serialized)
struct APIRequestShapesTests {
    @Test func mediaAndBackupURLsStayOnTheClientBase() throws {
        let api = try APIClient(base: #require(URL(string: "http://shape.invalid")))
        // pathSegment percent-encodes a segment that is only "." or "..".
        // Left raw, ".." collapses to /api/v1/ and "." collapses to /api/v1/backups/.
        let parent = api.backupURL(name: "..")
        let here = api.backupURL(name: ".")
        let nightly = api.backupURL(name: "nightly.db")
        stays(api.posterURL(recordingID: 44), "http://shape.invalid/media/poster/44")
        stays(api.artURL(kind: "airing", id: 44, width: 320), "http://shape.invalid/media/art/airing/44?w=320")
        stays(api.supportURL(), "http://shape.invalid/api/v1/support")
        stays(api.frameURL(channelID: 12, width: 1600), "http://shape.invalid/api/v1/channels/12/frame?w=1280")
        stays(api.frameURL(channelID: 12, width: 640), "http://shape.invalid/api/v1/channels/12/frame?w=480")
        stays(api.catalogBackupURL(), "http://shape.invalid/api/v1/backup")
        stays(nightly, "http://shape.invalid/api/v1/backups/nightly.db")
        stays(parent, "http://shape.invalid/api/v1/backups/%2E%2E")
        stays(here, "http://shape.invalid/api/v1/backups/%2E")
        #expect(parent.absoluteString != "http://shape.invalid/api/v1/")
        #expect(here.absoluteString != "http://shape.invalid/api/v1/backups/")
        #expect(!nightly.absoluteString.contains("%2E"))
        #expect(api.frameURL(channelID: 12, width: 480, listed: []) == nil)
        let listed = try #require(api.frameURL(channelID: 12, width: 1600, listed: [12]))
        #expect(listed.absoluteString == "http://shape.invalid/api/v1/channels/12/frame?w=1280")
    }

    @Test func favoriteAndHiddenSendOnlyThatFlag() async throws {
        let api = try shapeAPI()
        ShapeStub.set("PATCH", "/api/v1/channels/12", body: Data(shapeChannel.utf8))

        let favored = try await api.setFavorite(labChannel(), true)
        #expect(favored.id == 12)
        var body = try expectJSON("PATCH", "/api/v1/channels/12")
        #expect(Set(body.keys) == ["favorite"])
        #expect(body["favorite"] as? Bool == true)

        let shown = try await api.setHidden(labChannel(), false)
        #expect(shown.hidden == false)
        body = try expectJSON("PATCH", "/api/v1/channels/12")
        #expect(Set(body.keys) == ["hidden"])
        #expect(body["hidden"] as? Bool == false)
    }

    @Test func addSourceSendsEveryKeyTheMethodHas() async throws {
        let api = try shapeAPI()
        ShapeStub.set("POST", "/api/v1/sources", body: Data(shapeSourceAdd.utf8))
        let added = try await api.addSource(kind: "m3u", url: "http://10.1.2.9/list.m3u", groups: "News, -Shopping", keep: "desk-12")
        #expect(added.added == 2)
        #expect(added.message == "Pick groups.")
        let call = try expectCall("POST", "/api/v1/sources")
        #expect(!call.url.contains("10.1.2.9"))
        let body = try jsonObject(call)
        #expect(Set(body.keys) == ["kind", "name", "url", "username", "password", "groups", "keep"])
        #expect(body["kind"] as? String == "m3u")
        #expect(body["name"] as? String == "")
        #expect(body["url"] as? String == "http://10.1.2.9/list.m3u")
        #expect(body["username"] as? String == "")
        #expect(body["password"] as? String == "")
        #expect(body["groups"] as? String == "News, -Shopping")
        #expect(body["keep"] as? String == "desk-12")
    }

    @Test func playlistUploadDropsAQuoteInTheFileName() async throws {
        let api = try shapeAPI()
        ShapeStub.set("POST", "/api/v1/sources", body: Data(shapeSourceAdd.utf8))
        let file = Data("EXTM3U-lab\n".utf8)
        let added = try await api.addPlaylistFile(name: "Lab list", fileName: "late\"night.m3u", data: file, groups: "News", keep: "desk-12")
        #expect(added.added == 2)
        let call = try expectCall("POST", "/api/v1/sources")
        #expect(call.timeout == 60)
        let header = try #require(call.contentType)
        #expect(header.hasPrefix("multipart/form-data; boundary="))
        let boundary = String(header.split(separator: "boundary=").last ?? "")
        let text = try #require(String(data: call.body, encoding: .utf8))
        #expect(text.contains("--\(boundary)"))
        #expect(text.contains("filename=\"latenight.m3u\""))
        #expect(!text.contains("late\"night"))
        #expect(!text.contains("%22"))
        #expect(text.contains("name=\"name\"\r\n\r\nLab list\r\n"))
        #expect(text.contains("name=\"groups\"\r\n\r\nNews\r\n"))
        #expect(text.contains("name=\"keep\"\r\n\r\ndesk-12\r\n"))
        #expect(call.body.range(of: file) != nil)
    }

    @Test func discoverPutsTheAddressInTheBody() async throws {
        let api = try shapeAPI()
        ShapeStub.set("POST", "/api/v1/sources/discover", body: Data(shapeDevices.utf8))
        let found = try await api.discover(ip: "203.0.113.44")
        #expect(found.map(\.deviceId) == ["lab-1"])
        let call = try expectCall("POST", "/api/v1/sources/discover")
        #expect(!call.url.contains("203.0.113"))
        let body = try jsonObject(call)
        #expect(Set(body.keys) == ["ip"])
        #expect(body["ip"] as? String == "203.0.113.44")
    }

    @Test func refreshGuidePostsAndReturnsTheAiringCount() async throws {
        let api = try shapeAPI()
        ShapeStub.set("POST", "/api/v1/guide/refresh", body: Data(#"{"airings":17}"#.utf8))
        let count = try await api.refreshGuide()
        #expect(count == 17)
        let body = try expectJSON("POST", "/api/v1/guide/refresh")
        #expect(body.isEmpty)
    }

    @Test func healthBackupsSignalsStarsAndSetupDecode() async throws {
        let api = try shapeAPI()
        ShapeStub.set("GET", "/api/v1/devices/health", body: Data("""
        {"devices":[{"deviceId":"lab-1","model":"Dual","firmwareVersion":"2026.1","tuners":[{"index":0,"locked":false},{"index":1,"locked":true}]}]}
        """.utf8))
        let health = try await api.deviceHealth()
        try expectGet("/api/v1/devices/health")
        #expect(health.count == 1)
        #expect(health.first?.deviceId == "lab-1")
        #expect(health.first?.model == "Dual")
        #expect(health.first?.firmwareVersion == "2026.1")
        #expect(health.first?.tuners.map(\.locked) == [false, true])

        ShapeStub.set("GET", "/api/v1/backups", body: Data("""
        {"backups":[{"name":"nightly.db","kind":"daily","takenAt":"2026-10-01T04:00:00Z","bytes":4096}]}
        """.utf8))
        let backups = try await api.backups()
        try expectGet("/api/v1/backups")
        let backup = try #require(backups.first)
        #expect(backup.name == "nightly.db")
        #expect(backup.kind == "daily")
        #expect(backup.bytes == 4096)
        let taken = try #require(ISO8601DateFormatter.plain.date(from: "2026-10-01T04:00:00Z"))
        #expect(backup.takenAt == taken)

        ShapeStub.set("POST", "/api/v1/signals/check", body: Data(#"{"running":true,"message":"Reading the antenna."}"#.utf8))
        let message = try await api.checkSignals()
        #expect(message == "Reading the antenna.")
        let check = try expectJSON("POST", "/api/v1/signals/check")
        #expect(check.isEmpty)

        ShapeStub.set("POST", "/api/v1/channels/star", body: Data("""
        {"starred":[{"id":12,"guideName":"North Desk","displayNumber":"12.1","network":"North"}]}
        """.utf8))
        let starred = try await api.starNetworks()
        let star = try expectJSON("POST", "/api/v1/channels/star")
        #expect(star.isEmpty)
        let row = try #require(starred.first)
        #expect(row.id == 12)
        #expect(row.guideName == "North Desk")
        #expect(row.displayNumber == "12.1")
        #expect(row.network == "North")

        ShapeStub.set("POST", "/api/v1/setup/finish", body: Data("""
        {"running":true,"ready":"Guide loaded.","channelId":12,"steps":[{"id":"guide","title":"Guide","state":"done","detail":"17 listings"}]}
        """.utf8))
        let started = try await api.startSetupFinish()
        let startBody = try expectJSON("POST", "/api/v1/setup/finish")
        #expect(startBody.isEmpty)
        #expect(started.running)
        #expect(started.ready == "Guide loaded.")
        #expect(started.channelId == 12)
        #expect(started.steps.map(\.id) == ["guide"])
        #expect(started.steps.first?.state == "done")

        ShapeStub.set("GET", "/api/v1/setup/finish", body: Data("""
        {"running":false,"ready":"Done.","steps":[{"id":"guide","title":"Guide","state":"done"}]}
        """.utf8))
        let finished = try await api.setupFinish()
        try expectGet("/api/v1/setup/finish")
        #expect(!finished.running)
        #expect(finished.ready == "Done.")
        #expect(finished.channelId == nil)
    }

    @Test func readsUseGetAndAnEmptyBody() async throws {
        let api = try shapeAPI()
        ShapeStub.set("GET", "/api/v1/health", status: 204, body: Data())
        ShapeStub.set("GET", "/live/12/index.m3u8", body: Data("#EXTM3U\n".utf8))
        ShapeStub.set("GET", "/api/v1/server", body: Data(#"{"id":"lab","name":"Lab","version":"1.0.0","apiVersion":1,"features":["dvr"]}"#.utf8))
        ShapeStub.set("GET", "/api/v1/clock", body: Data(#"{"serverTime":1700000000.5}"#.utf8))
        ShapeStub.set("GET", "/api/v1/channels", body: Data(shapeChannels.utf8))
        ShapeStub.set("GET", "/api/v1/airings", body: Data("""
        {"airings":[{"id":1,"channelId":12,"title":"Night Desk","start":"2026-10-01T04:00:00Z","end":"2026-10-01T05:00:00Z"}]}
        """.utf8))
        ShapeStub.set("GET", "/api/v1/diagnostics", body: Data("""
        {"doctor":[{"id":"disk","message":"The recordings disk is low."}],"guide":{"channels":2,"channelsWithListings":1,"airings":9}}
        """.utf8))
        ShapeStub.set("GET", "/api/v1/settings", body: Data(#"{"hideScores":"0"}"#.utf8))
        ShapeStub.set("GET", "/api/v1/tuners", body: Data(#"{"tuners":[{"index":0,"ours":true,"name":"Lab"}]}"#.utf8))
        ShapeStub.set("GET", "/api/v1/recordings", body: Data(shapeRecordings.utf8))
        ShapeStub.set("GET", "/api/v1/virtuals", body: Data(shapeVirtuals.utf8))
        ShapeStub.set("GET", "/api/v1/virtuals/schedule", body: Data("""
        {"virtuals":[{"id":2,"number":"900","name":"Desk","recordings":[8],"slots":[{"recordingId":8,"title":"Night Desk","start":"2026-10-01T04:00:00Z","end":"2026-10-01T05:00:00Z"}]}]}
        """.utf8))
        ShapeStub.set("GET", "/api/v1/schedule", body: Data(#"{"tunerCount":1,"items":[]}"#.utf8))
        ShapeStub.set("GET", "/api/v1/events", body: Data("""
        {"events":[{"id":1,"at":"2026-10-01T04:00:00Z","kind":"guide","message":"Guide loaded."}]}
        """.utf8))
        ShapeStub.set("GET", "/api/v1/sports/scoreboard", body: Data(#"{"games":[{"id":"g-1","state":"pre"}]}"#.utf8))
        ShapeStub.set("GET", "/api/v1/teams", body: Data(#"{"teams":[{"id":1,"name":"North"}]}"#.utf8))
        ShapeStub.set("GET", "/api/v1/passes", body: Data(shapePasses.utf8))
        ShapeStub.set("GET", "/api/v1/frames", body: Data(#"{"channels":[12,13]}"#.utf8))
        ShapeStub.set("GET", "/api/v1/devices", body: Data(shapeDevices.utf8))
        ShapeStub.set("GET", "/api/v1/storage", body: Data(#"{"path":"/data/recordings","freeBytes":1000,"totalBytes":2000,"watermarkGB":5}"#.utf8))
        ShapeStub.set("GET", "/api/v1/storage/shows", body: Data("""
        {"shows":[{"title":"Night Desk","count":2,"bytes":1000,"oldest":"2026-10-01T04:00:00Z","newest":"2026-10-02T04:00:00Z"}]}
        """.utf8))
        ShapeStub.set("GET", "/api/v1/signals", body: Data("""
        {"channels":[{"channelId":12,"number":"12.1","name":"North Desk","verdict":"strong"}],"running":false}
        """.utf8))

        let reach = await api.reach()
        #expect(reach == ServerReach(up: true, online: true))
        let health = try expectCall("GET", "/api/v1/health")
        #expect(health.timeout == 2)
        #expect(health.body.isEmpty)
        #expect(await api.reachable())

        #expect(await api.playlistFound("/live/12/index.m3u8") == true)
        let playlist = try expectCall("GET", "/live/12/index.m3u8")
        #expect(playlist.timeout == 2)
        #expect(!playlist.url.contains("/api/v1"))
        #expect(await api.playlistText("/live/12/index.m3u8") == "#EXTM3U\n")

        let server = try await api.server()
        try expectGet("/api/v1/server")
        #expect(server.name == "Lab")
        #expect(server.features == ["dvr"])

        #expect(try await api.clock() == 1_700_000_000.5)
        try expectGet("/api/v1/clock")

        let guideChannels = try await api.channels()
        try expectGet("/api/v1/channels", query: "guide=1")
        #expect(guideChannels.map(\.id) == [12])
        _ = try await api.allChannels()
        try expectGet("/api/v1/channels", noQuery: true)
        _ = try await api.lineup()
        try expectGet("/api/v1/channels", noQuery: true)

        let airings = try await api.airings(hours: 6)
        try expectGet("/api/v1/airings", query: "hours=6")
        #expect(airings.map(\.title) == ["Night Desk"])

        let notes = try await api.doctorNotes()
        try expectGet("/api/v1/diagnostics")
        #expect(notes.map(\.id) == ["disk"])
        #expect(notes.first?.message == "The recordings disk is low.")
        let depth = try #require(try await api.guideDepth())
        try expectGet("/api/v1/diagnostics")
        #expect(depth.channels == 2)
        #expect(depth.channelsWithListings == 1)
        #expect(depth.airings == 9)
        #expect(try await api.guideAirings() == 9)

        #expect(try await api.settings()["hideScores"] == "0")
        try expectGet("/api/v1/settings")

        #expect(try await api.tuners().first?.ours == true)
        try expectGet("/api/v1/tuners")
        #expect(try await api.recordings().map(\.title) == ["Night Desk"])
        try expectGet("/api/v1/recordings")
        #expect(try await api.virtuals().map(\.number) == ["900"])
        try expectGet("/api/v1/virtuals")
        #expect(try await api.virtualSchedule().first?.slots.count == 1)
        try expectGet("/api/v1/virtuals/schedule")
        #expect(try await api.schedule().tunerCount == 1)
        try expectGet("/api/v1/schedule")
        #expect(try await api.events().first?.message == "Guide loaded.")
        try expectGet("/api/v1/events")
        #expect(try await api.scoreboard().map(\.id) == ["g-1"])
        try expectGet("/api/v1/sports/scoreboard")
        #expect(try await api.teams().map(\.name) == ["North"])
        try expectGet("/api/v1/teams")
        #expect(try await api.passes().map(\.title) == ["Night Desk"])
        try expectGet("/api/v1/passes")
        #expect(try await api.frames().channels == [12, 13])
        try expectGet("/api/v1/frames")
        #expect(try await api.devices().map(\.friendlyName) == ["Lab Tuner"])
        try expectGet("/api/v1/devices")

        let storage = try await api.storage()
        try expectGet("/api/v1/storage")
        #expect(storage.path == "/data/recordings")
        #expect(storage.freeBytes == 1000)
        #expect(storage.watermarkGB == 5)
        #expect(try await api.storageShows().shows.map(\.title) == ["Night Desk"])
        try expectGet("/api/v1/storage/shows")

        let signals = try await api.signals()
        try expectGet("/api/v1/signals")
        #expect(!signals.running)
        #expect(signals.channels.map(\.name) == ["North Desk"])
    }

    @Test func writesUseTheMethodPathAndJSONBody() async throws {
        let api = try shapeAPI()
        let caps = Caps(platform: "test", video: ["h264"], audio: ["aac"], alternates: true)
        let prefs = Prefs(quality: .high)
        ShapeStub.set("PATCH", "/api/v1/channels/12", body: Data(shapeChannel.utf8))
        _ = try await api.patchChannel(12, ChannelPatch(enabled: false, customName: "Desk"))
        let patch = try expectJSON("PATCH", "/api/v1/channels/12")
        #expect(Set(patch.keys) == ["customName", "enabled"])
        #expect(patch["enabled"] as? Bool == false)
        #expect(patch["customName"] as? String == "Desk")

        ShapeStub.set("POST", "/api/v1/watch", body: Data("""
        {"channelId":12,"playlist":"/live/12/index.m3u8","rendition":"720.aac","stream":{"rendition":"720.aac","video":"720","audio":"aac","reason":"Original picture"},"encoder":"copy","shared":false,"viewers":1}
        """.utf8))
        _ = try await api.watch(channelID: 12, caps: caps, prefs: prefs, confirmLive: true)
        let watch = try expectJSON("POST", "/api/v1/watch")
        #expect(Set(watch.keys) == ["caps", "channelId", "confirmLive", "prefs"])
        #expect(watch["channelId"] as? Int == 12)
        #expect(watch["confirmLive"] as? Bool == true)
        #expect(watch["room"] == nil)
        #expect((watch["caps"] as? [String: Any])?["platform"] as? String == "test")
        #expect((watch["prefs"] as? [String: Any])?["quality"] as? String == "high")

        ShapeStub.set("POST", "/api/v1/watch/12/warm", body: Data(#"{"warm":true}"#.utf8))
        #expect(await api.warm(channelID: 12, caps: caps, prefs: prefs))
        let warm = try expectJSON("POST", "/api/v1/watch/12/warm")
        #expect(Set(warm.keys) == ["caps", "prefs"])
        #expect(ShapeStub.last()?.timeout == 5)

        await api.stopWatching(channelID: 12, rendition: "720.aac", boot: "boot-1")
        let stop = try expectJSON("POST", "/api/v1/watch/12/stop")
        #expect(Set(stop.keys) == ["boot", "rendition"])
        #expect(stop["rendition"] as? String == "720.aac")
        #expect(stop["boot"] as? String == "boot-1")

        ShapeStub.set("POST", "/api/v1/multiview/plan", body: Data("""
        {"playable":[{"channelId":12,"frequencyHz":500000000,"shared":true}],"blocked":[],"tunersNeeded":1,"tunersFree":1}
        """.utf8))
        let plan = try await api.planMultiview([12, 13])
        #expect(plan.tunersNeeded == 1)
        let planBody = try expectJSON("POST", "/api/v1/multiview/plan")
        #expect(Set(planBody.keys) == ["channelIds", "picker"])
        #expect(planBody["picker"] as? Bool == true)
        #expect(planBody["channelIds"] as? [Int] == [12, 13])

        ShapeStub.set("POST", "/api/v1/recordings", body: Data(shapeRecording.utf8))
        let recording = try await api.record(channelID: 12, title: "Night Desk")
        #expect(recording.id == 8)
        let recordBody = try expectJSON("POST", "/api/v1/recordings")
        #expect(Set(recordBody.keys) == ["channelId", "minutes", "title"])
        #expect(recordBody["channelId"] as? Int == 12)
        #expect(recordBody["minutes"] as? Int == 0)
        #expect(recordBody["title"] as? String == "Night Desk")

        try await api.stopRecording(8)
        #expect(try expectJSON("POST", "/api/v1/recordings/8/stop").isEmpty)

        ShapeStub.set("POST", "/api/v1/recordings/8/play", body: Data(#"{"playlist":"/play/8/index.m3u8","position":0,"growing":true}"#.utf8))
        #expect(try await api.play(recordingID: 8).playlist == "/play/8/index.m3u8")
        #expect(try expectJSON("POST", "/api/v1/recordings/8/play").isEmpty)

        try await api.deleteRecording(8)
        let deleted = try expectCall("DELETE", "/api/v1/recordings/8")
        #expect(deleted.body.isEmpty)

        try await api.setWatched(recordingID: 8, false)
        let watched = try expectJSON("PUT", "/api/v1/recordings/8/watched")
        #expect(Set(watched.keys) == ["watched"])
        #expect(watched["watched"] as? Bool == false)

        ShapeStub.set("POST", "/api/v1/recordings/8/detect", body: Data(#"{"markers":[{"id":3,"start":10,"end":70}]}"#.utf8))
        #expect(try await api.detectBreaks(recordingID: 8).map(\.id) == [3])
        let detect = try expectCall("POST", "/api/v1/recordings/8/detect")
        #expect(detect.timeout == 600)
        #expect(try jsonObject(detect).isEmpty)

        ShapeStub.set("POST", "/api/v1/recordings/8/markers", body: Data(#"{"id":3,"start":12.5,"end":40}"#.utf8))
        let marker = try await api.addMarker(recordingID: 8, start: 12.5, end: 40)
        #expect(marker.id == 3)
        let markerBody = try expectJSON("POST", "/api/v1/recordings/8/markers")
        #expect(Set(markerBody.keys) == ["end", "start"])
        #expect(markerBody["start"] as? Double == 12.5)
        #expect(markerBody["end"] as? Double == 40)

        try await api.deleteMarker(3)
        #expect(try expectCall("DELETE", "/api/v1/markers/3").body.isEmpty)

        await api.saveProgress(recordingID: 8, position: 15.5)
        let progress = try expectJSON("PUT", "/api/v1/recordings/8/progress")
        #expect(Set(progress.keys) == ["position"])
        #expect(progress["position"] as? Double == 15.5)

        ShapeStub.set("POST", "/api/v1/virtuals", body: Data(#"{"id":2,"number":"900","name":"Desk","recordings":[8]}"#.utf8))
        let created = try await api.createVirtual(number: "900", name: "Desk", recordings: [8])
        #expect(created.id == 2)
        let virtualBody = try expectJSON("POST", "/api/v1/virtuals")
        #expect(Set(virtualBody.keys) == ["name", "number", "recordings"])
        #expect(virtualBody["number"] as? String == "900")
        #expect(virtualBody["name"] as? String == "Desk")
        #expect(virtualBody["recordings"] as? [Int] == [8])

        ShapeStub.set("POST", "/api/v1/virtuals/2/play", body: Data("""
        {"usesTuner":false,"index":1,"count":1,"playlist":"/play/8/index.m3u8","number":"900","name":"Desk","recording":\(shapeRecording)}
        """.utf8))
        #expect(try await api.playVirtual(2, index: 1).index == 1)
        let playVirtual = try expectJSON("POST", "/api/v1/virtuals/2/play")
        #expect(Set(playVirtual.keys) == ["index"])
        #expect(playVirtual["index"] as? Int == 1)

        let start = Date(timeIntervalSince1970: 1_700_000_000)
        let item = PlannedAiring(
            passId: 4,
            airing: Airing(id: 1, channelId: 12, title: "Night Desk", start: start, end: start.addingTimeInterval(3600)),
            priority: 1, padBefore: 1, padAfter: 2, conflict: false, skipped: false
        )
        let later = Suggestion(channelId: 13, guideNumber: "13.1", title: "Night Desk", start: start.addingTimeInterval(7200), end: start.addingTimeInterval(10800))
        ShapeStub.set("POST", "/api/v1/schedule/fix", body: Data(#"{"tunerCount":1,"items":[]}"#.utf8))
        #expect(try await api.fixSchedule(item, later: later).tunerCount == 1)
        let fix = try expectJSON("POST", "/api/v1/schedule/fix")
        #expect(Set(fix.keys) == ["channelId", "passId", "start", "suggestionChannelId", "suggestionStart"])
        #expect(fix["passId"] as? Int == 4)
        #expect(fix["channelId"] as? Int == 12)
        #expect(fix["suggestionChannelId"] as? Int == 13)
        #expect(fix["start"] as? String == ISO8601DateFormatter.plain.string(from: start))
        #expect(fix["suggestionStart"] as? String == ISO8601DateFormatter.plain.string(from: later.start))

        ShapeStub.set("POST", "/api/v1/passes", body: Data(shapePasses.utf8))
        _ = try await api.addPass(title: "Night Desk", channelID: 12)
        var passBody = try expectJSON("POST", "/api/v1/passes")
        #expect(Set(passBody.keys) == ["channelId", "title"])
        #expect(passBody["title"] as? String == "Night Desk")
        #expect(passBody["channelId"] as? Int == 12)
        #expect(passBody["airingStart"] == nil)
        _ = try await api.addPass(title: "Night Desk", channelID: 12, airingStart: start)
        passBody = try expectJSON("POST", "/api/v1/passes")
        #expect(passBody["airingStart"] as? String == ISO8601DateFormatter.plain.string(from: start))

        ShapeStub.set("DELETE", "/api/v1/passes/4", body: Data(shapePasses.utf8))
        #expect(try await api.deletePass(4).map(\.id) == [4])
        #expect(try expectCall("DELETE", "/api/v1/passes/4").body.isEmpty)

        ShapeStub.set("POST", "/api/v1/sources/look", body: Data(#"{"found":[{"kind":"tuner","name":"Lab Tuner","addr":"10.1.2.3","id":"lab-1"}]}"#.utf8))
        let looked = try await api.lookHarder()
        #expect(looked.first?.deviceID == "lab-1")
        #expect(try expectJSON("POST", "/api/v1/sources/look").isEmpty)

        ShapeStub.set("GET", "/api/v1/sources/free", body: Data("""
        {"found":[{"kind":"fast","name":"Lab Fast","addr":"10.1.2.9","playlist":"http://10.1.2.9/list.m3u","guide":"http://10.1.2.9/guide.xml"}],"guide":"One feed answered."}
        """.utf8))
        let free = try await api.freeSources()
        try expectGet("/api/v1/sources/free")
        #expect(free.guide == "One feed answered.")
        #expect(free.found.first?.playlist == "http://10.1.2.9/list.m3u")

        ShapeStub.set("POST", "/api/v1/sources/free", body: Data(#"{"message":"Added the feed."}"#.utf8))
        #expect(try await api.addFree(kind: "fast", addr: "10.1.2.9", playlist: "http://10.1.2.9/list.m3u", guide: "http://10.1.2.9/guide.xml", name: "Lab Fast") == "Added the feed.")
        let freeBody = try expectJSON("POST", "/api/v1/sources/free")
        #expect(Set(freeBody.keys) == ["addr", "guide", "kind", "name", "playlist"])
        #expect(freeBody["kind"] as? String == "fast")
        #expect(freeBody["addr"] as? String == "10.1.2.9")
        #expect(freeBody["name"] as? String == "Lab Fast")

        ShapeStub.set("GET", "/api/v1/sources", body: Data(#"{"sources":[{"id":3,"kind":"m3u","name":"Lab list","enabled":true}]}"#.utf8))
        #expect(try await api.sources().map(\.name) == ["Lab list"])
        try expectGet("/api/v1/sources")

        ShapeStub.set("GET", "/api/v1/home", body: Data("""
        {"places":[{"id":"p1","group":"lan","kind":"tuner","name":"Lab Tuner","addr":"10.1.2.3","action":"add"}],"tunerAddress":"10.1.2.3","sharing":false}
        """.utf8))
        let home = try await api.home()
        try expectGet("/api/v1/home", noQuery: true)
        #expect(home.tunerAddress == "10.1.2.3")
        #expect(!home.sharing)
        #expect(home.places.first?.action == "add")
        _ = try await api.home(fresh: true)
        try expectGet("/api/v1/home", query: "fresh=1")

        ShapeStub.set("DELETE", "/api/v1/devices/..", body: Data(shapeDevices.utf8))
        ShapeStub.set("DELETE", "/api/v1/devices/%2E%2E", body: Data(shapeDevices.utf8))
        #expect(try await api.removeDevice("..").map(\.deviceId) == ["lab-1"])
        let removed = try #require(ShapeStub.last())
        #expect(removed.method == "DELETE")
        #expect(removed.url.contains("/api/v1/devices/%2E%2E"))
        #expect(removed.path == "/api/v1/devices/..")
        #expect(removed.body.isEmpty)

        try await api.restoreBackup(name: "nightly.db")
        let namedCall = try expectCall("POST", "/api/v1/backups/nightly.db/restore")
        #expect(try jsonObject(namedCall).isEmpty)
        #expect(!namedCall.url.contains("%2E"))

        try await api.restoreBackup(name: "..")
        let dotted = try #require(ShapeStub.last())
        #expect(dotted.method == "POST")
        #expect(dotted.url.contains("/api/v1/backups/%2E%2E/restore"))
        #expect(dotted.path == "/api/v1/backups/../restore")
        #expect(try jsonObject(dotted).isEmpty)

        let catalog = Data("catalog-bytes".utf8)
        try await api.restoreCatalog(catalog)
        let putBack = try expectCall("POST", "/api/v1/backup")
        #expect(putBack.contentType == "application/octet-stream")
        #expect(putBack.body == catalog)
        #expect(putBack.timeout == 60)
    }
}

private func stays(_ url: URL, _ absolute: String) {
    #expect(url.scheme == "http")
    #expect(url.host() == "shape.invalid")
    #expect(url.absoluteString == absolute)
}

private func labChannel() -> Channel {
    Channel(
        id: 12, deviceId: "lab-1", guideNumber: "12.1", guideName: "North Desk",
        displayNumber: "12.1", displayName: "North Desk", hd: true, favorite: false,
        enabled: true, hidden: false, present: true
    )
}

private let shapeChannel = """
{"id":12,"deviceId":"lab-1","guideNumber":"12.1","guideName":"North Desk","displayNumber":"12.1","displayName":"North Desk","hd":true,"favorite":true,"enabled":true,"hidden":false,"present":true}
"""

private let shapeChannels = "{\"channels\":[\(shapeChannel)]}"

private let shapeRecording = """
{"id":8,"channelId":12,"guideNumber":"12.1","title":"Night Desk","status":"done","startedAt":"2026-10-01T04:00:00Z"}
"""

private let shapeRecordings = "{\"recordings\":[\(shapeRecording)]}"

private let shapeDevices = """
{"devices":[{"deviceId":"lab-1","friendlyName":"Lab Tuner","baseUrl":"http://10.1.2.3:5004","tunerCount":2}]}
"""

private let shapeVirtuals = """
{"virtuals":[{"id":2,"number":"900","name":"Desk","recordings":[8]}]}
"""

private let shapePasses = #"{"passes":[{"id":4,"title":"Night Desk"}]}"#

private let shapeSourceAdd = #"{"added":2,"pick":true,"message":"Pick groups.","groups":["News"]}"#

private func shapeAPI() throws -> APIClient {
    ShapeStub.reset()
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [ShapeStub.self]
    return try APIClient(base: #require(URL(string: "http://shape.invalid")), session: URLSession(configuration: config))
}

@discardableResult
private func expectCall(_ method: String, _ path: String, query: String? = nil, noQuery: Bool = false) throws -> ShapeStub.Call {
    let call = try #require(ShapeStub.last())
    #expect(call.method == method)
    #expect(call.path == path)
    #expect(call.url.hasPrefix("http://shape.invalid/"))
    if noQuery {
        #expect(call.query == nil)
    } else if let query {
        #expect(call.query == query)
    }
    return call
}

private func expectGet(_ path: String, query: String? = nil, noQuery: Bool = false) throws {
    let call = try expectCall("GET", path, query: query, noQuery: noQuery || query == nil)
    #expect(call.body.isEmpty)
}

private func expectJSON(_ method: String, _ path: String) throws -> [String: Any] {
    try jsonObject(expectCall(method, path))
}

private func jsonObject(_ call: ShapeStub.Call) throws -> [String: Any] {
    #expect(call.contentType?.hasPrefix("application/json") == true)
    if call.body.isEmpty {
        return [:]
    }
    return try #require(JSONSerialization.jsonObject(with: call.body) as? [String: Any])
}

private final class ShapeStub: URLProtocol, @unchecked Sendable {
    struct Call: Sendable {
        var method: String
        var url: String
        var path: String
        var query: String?
        var body: Data
        var contentType: String?
        var timeout: TimeInterval
    }

    private struct Reply {
        var status: Int
        var body: Data
    }

    private static let lock = NSLock()
    private nonisolated(unsafe) static var calls: [Call] = []
    private nonisolated(unsafe) static var replies: [String: Reply] = [:]

    static func reset() {
        lock.withLock {
            calls = []
            replies = [:]
        }
    }

    static func set(_ method: String, _ path: String, status: Int = 200, body: Data = Data("{}".utf8)) {
        lock.withLock {
            replies["\(method) \(path)"] = Reply(status: status, body: body)
        }
    }

    static func last() -> Call? {
        lock.withLock { calls.last }
    }

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        let data = request.httpBody ?? request.httpBodyStream.map(Self.read) ?? Data()
        let method = request.httpMethod ?? "GET"
        let path = request.url?.path ?? ""
        let call = Call(
            method: method,
            url: request.url?.absoluteString ?? "",
            path: path,
            query: request.url?.query,
            body: data,
            contentType: request.value(forHTTPHeaderField: "Content-Type"),
            timeout: request.timeoutInterval
        )
        let reply = Self.lock.withLock {
            Self.calls.append(call)
            return Self.replies["\(method) \(path)"] ?? Reply(status: 200, body: Data("{}".utf8))
        }
        guard let url = request.url else {
            client?.urlProtocol(self, didFailWithError: URLError(.badURL))
            return
        }
        let response = HTTPURLResponse(url: url, statusCode: reply.status, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: reply.body)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}

    private static func read(_ stream: InputStream) -> Data {
        stream.open()
        defer { stream.close() }
        var out = Data()
        var buf = [UInt8](repeating: 0, count: 4096)
        while stream.hasBytesAvailable {
            let n = stream.read(&buf, maxLength: buf.count)
            if n <= 0 {
                break
            }
            out.append(buf, count: n)
        }
        return out
    }
}
