import XCTest

/// Watching on Apple TV with the remote, against a real server: the live player,
/// the guide, and recordings. Opt-in: TEST_RUNNER_BROADWAVE_SERVER=http://host:port.
/// TEST_RUNNER_BROADWAVE_SHOTS=<dir> keeps a screenshot of every step.
@MainActor
class WatchingCase: XCTestCase {
    let app = XCUIApplication()

    var server: String {
        get throws {
            guard let server = ProcessInfo.processInfo.environment["BROADWAVE_SERVER"], !server.isEmpty else {
                throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER to run against a server")
            }
            return server
        }
    }

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    func launch(_ extra: [String] = []) throws {
        app.launchArguments = try ["-ApplePersistenceIgnoreState", "YES", "-BroadwaveServerURL", server, "-lastChannel", "0"] + extra
        app.launch()
    }

    func shot(_ name: String) {
        let env = ProcessInfo.processInfo.environment
        guard let dir = env["BROADWAVE_SHOTS"] ?? env["TEST_RUNNER_BROADWAVE_SHOTS"], !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: folder.appending(path: "\(name).png"))
    }

    func press(_ button: XCUIRemote.Button, _ times: Int = 1, wait seconds: TimeInterval = 0.5) {
        for _ in 0 ..< times {
            XCUIRemote.shared.press(button)
            wait(seconds)
        }
    }

    func focused() -> XCUIElement {
        app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
    }

    func focusedLabel() -> String {
        let element = focused()
        return element.exists ? element.label : "nothing"
    }

    func element(_ id: String) -> XCUIElement {
        app.descendants(matching: .any)[id]
    }

    func text(_ label: String) -> XCUIElement {
        app.staticTexts.matching(NSPredicate(format: "label == %@", label)).firstMatch
    }

    @discardableResult
    func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while Date() < end {
            if done() {
                return true
            }
            wait(0.1)
        }
        return done()
    }

    func wait(_ seconds: TimeInterval) {
        RunLoop.current.run(until: Date().addingTimeInterval(seconds))
    }

    // MARK: Server

    @discardableResult
    func call(_ method: String, _ path: String, _ body: [String: Any]? = nil) throws -> Any? {
        var request = try URLRequest(url: URL(string: server + path)!, timeoutInterval: 60)
        request.httpMethod = method
        if let body {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONSerialization.data(withJSONObject: body)
        }
        let done = expectation(description: path)
        nonisolated(unsafe) var result: (Data?, Int) = (nil, 0)
        URLSession.shared.dataTask(with: request) { data, response, _ in
            result = (data, (response as? HTTPURLResponse)?.statusCode ?? 0)
            done.fulfill()
        }.resume()
        wait(for: [done], timeout: 70)
        guard (200 ..< 300).contains(result.1) else {
            throw NSError(domain: "server", code: result.1, userInfo: [NSLocalizedDescriptionKey: "\(method) \(path): \(result.1)"])
        }
        return result.0.flatMap { try? JSONSerialization.jsonObject(with: $0) }
    }

    func recordings() throws -> [[String: Any]] {
        try (call("GET", "/api/v1/recordings") as? [String: Any])?["recordings"] as? [[String: Any]] ?? []
    }
}

/// Recordings play as on-demand video, and one Back leaves them.
final class RecordingPlaybackTests: WatchingCase {
    /// A finished recording: 0:00 to its length, time left, no LIVE badge or air-time clock.
    func testAFinishedRecordingPlaysOnDemand() throws {
        let rec = try XCTUnwrap(try recordings().first { $0["status"] as? String == "complete" && ($0["durationSec"] as? Double ?? 0) > 120 })
        let title = try XCTUnwrap(rec["title"] as? String)
        try launch(["-BroadwaveTab", "recordings", "-BroadwaveChannelNow", "YES"])
        try open(title)
        XCTAssertTrue(until(20) { self.playing() }, "the recording never started")
        wait(3)
        press(.select, wait: 1.5)
        shot("rec-finished-bar")
        let elapsed = element("AXElapsedTime")
        XCTAssertTrue(until(5) { elapsed.exists }, "no transport bar")
        XCTAssertFalse(elapsed.label.contains("AM") || elapsed.label.contains("PM"), "air-time clock: \(elapsed.label)")
        XCTAssertFalse(text("LIVE").exists && text("LIVE").frame.width > 1, "LIVE badge on a recording")
        let remaining = element("AXRemainingTime")
        if remaining.exists {
            XCTAssertTrue(remaining.label.hasPrefix("-"), "time left: \(remaining.label)")
        }
        press(.menu, wait: 2)
        shot("rec-finished-back")
        XCTAssertTrue(until(5) { !elapsed.exists }, "one Back left the player up")
        XCTAssertTrue(focusedLabel().contains(title), "focus on \(focusedLabel())")
    }

    /// A recording still on the air starts within a few seconds, plays on demand, and one Back leaves.
    func testARecordingStillOnTheAirStarts() throws {
        let title = "Watching Test \(Int(Date().timeIntervalSince1970) % 10000)"
        let channels = try (call("GET", "/api/v1/channels") as? [String: Any])?["channels"] as? [[String: Any]] ?? []
        let channel = try XCTUnwrap(channels.first { $0["guideNumber"] as? String == "5.1" } ?? channels.first)
        let made = try XCTUnwrap(try call("POST", "/api/v1/recordings", ["channelId": channel["id"] as Any, "minutes": 4, "title": title]) as? [String: Any])
        let id = try XCTUnwrap(made["id"] as? Int)
        addTeardownBlock { [self] in
            _ = try? call("POST", "/api/v1/recordings/\(id)/stop")
            _ = try? call("DELETE", "/api/v1/recordings/\(id)")
        }
        // A few segments on disk, as a viewer who opens it right after it starts.
        wait(25)
        try launch(["-BroadwaveTab", "recordings", "-BroadwaveChannelNow", "YES"])
        try open(title)
        let start = Date()
        XCTAssertTrue(until(20) { self.playing() }, "the recording never started")
        print("TIMING in-progress recording select->playing \(String(format: "%.2f", Date().timeIntervalSince(start)))s")
        wait(4)
        press(.select, wait: 1.5)
        shot("rec-growing-bar")
        let elapsed = element("AXElapsedTime")
        XCTAssertTrue(until(5) { elapsed.exists }, "no transport bar")
        XCTAssertFalse(elapsed.label.contains("AM") || elapsed.label.contains("PM"), "air-time clock: \(elapsed.label)")
        press(.menu, wait: 2)
        shot("rec-growing-back")
        XCTAssertTrue(until(5) { !elapsed.exists }, "one Back left the player up")
    }

    private func open(_ title: String) throws {
        let row = app.buttons.matching(NSPredicate(format: "identifier == 'recording-row' AND label CONTAINS %@", title)).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 20), "no row \(title)")
        for _ in 0 ..< 12 where !focusedLabel().contains(title) {
            press(.down, wait: 0.4)
        }
        XCTAssertTrue(focusedLabel().contains(title), "focus on \(focusedLabel())")
        shot("rec-row")
        press(.select, wait: 0.2)
    }

    /// The player's clock moves.
    private func playing() -> Bool {
        let probe = element("recording-time")
        guard probe.exists, let first = Double(probe.label), first > 0 else { return false }
        wait(1)
        return (Double(probe.label) ?? first) > first
    }
}

/// Down in the live player has one meaning: the channel strip.
final class ChannelStripTests: WatchingCase {
    func testDownOpensTheChannelStripAndNothingElse() throws {
        try launch(["-BroadwaveChannelNow", "YES"])
        try startWatching()
        let before = channelNow
        shot("strip-0-playing")

        press(.down, wait: 1.2)
        shot("strip-1-down")
        let strip = element("channel-strip")
        XCTAssertTrue(until(3) { strip.exists }, "Down opened no strip, focus on \(focusedLabel())")
        XCTAssertEqual(channelNow, before, "Down changed channel")
        XCTAssertTrue(element("panelProbe").exists, "no panel probe")
        XCTAssertFalse(infoPanelUp, "Down opened AVKit's Info panel as well")
        XCTAssertTrue(until(3) { self.focused().identifier.hasPrefix("strip-") }, "focus on \(focused().identifier) \(focusedLabel())")
        let first = focused().identifier

        press(.right, wait: 0.8)
        shot("strip-2-right")
        let picked = focused().identifier
        XCTAssertNotEqual(picked, first, "Right did not move along the strip")
        let pickedLabel = focusedLabel()
        press(.select, wait: 1.5)
        shot("strip-3-select")
        XCTAssertTrue(until(3) { !strip.exists }, "Select left the strip up")
        XCTAssertNotEqual(channelNow, before, "Select did not change channel")
        XCTAssertTrue(pickedLabel.hasPrefix(channelNow + " "), "playing \(channelNow), picked \(pickedLabel)")

        // Back closes the strip and nothing else.
        waitForPictureOnly()
        let playing = channelNow
        press(.down, wait: 1.2)
        XCTAssertTrue(until(3) { strip.exists }, "second Down opened no strip")
        press(.menu, wait: 1.2)
        shot("strip-4-back")
        XCTAssertFalse(strip.exists, "Back left the strip up")
        XCTAssertTrue(element("player-channel").exists, "Back left the player")
        XCTAssertEqual(channelNow, playing)

        // Up with nothing on screen still changes channel.
        waitForPictureOnly()
        press(.up, wait: 1.5)
        shot("strip-5-up")
        XCTAssertNotEqual(channelNow, playing, "Up did not change channel")
        XCTAssertFalse(strip.exists, "Up opened the strip")

        // With the bar up, Down is AVKit's again: its Info panel.
        waitForPictureOnly()
        press(.select, wait: 1.5)
        press(.down, wait: 1.5)
        shot("strip-6-bar-down")
        XCTAssertTrue(until(3) { self.infoPanelUp }, "Down with the bar up opened no Info panel")
        XCTAssertFalse(strip.exists, "Down with the bar up opened the strip")
    }

    private var channelNow: String {
        let probe = app.staticTexts["channel-now"]
        return probe.exists ? probe.label : "-"
    }

    private var infoPanelUp: Bool {
        let probe = element("panelProbe")
        return probe.exists && probe.label == "panel"
    }

    /// Select on the first channel in the guide, then wait for the picture and for the bar to hide.
    private func startWatching() throws {
        XCTAssertTrue(until(30) {
            let focus = self.focused()
            return focus.exists && focus.elementType == .button && focus.frame.minX < 120
        }, "the guide never focused a channel: \(focusedLabel())")
        press(.select, wait: 0.5)
        let tuning = element("tuning")
        XCTAssertTrue(until(30) { self.element("player-channel").exists && !tuning.exists }, "the picture never started")
        waitForPictureOnly()
    }

    private func waitForPictureOnly() {
        let probe = element("transportProbe")
        until(12) { probe.exists && probe.label == "hidden" }
        wait(0.5)
    }
}

/// The day row scrolls the grid, the header says which day it shows, and Back comes back to now.
final class GuideDayTests: WatchingCase {
    func testDayButtonsScrollTheGridAndBackReturnsToNow() throws {
        try launch()
        XCTAssertTrue(until(30) { self.onChannel() }, "the guide never focused a channel: \(focusedLabel())")
        wait(1.5)
        let day = element("guide-day")
        XCTAssertTrue(until(5) { day.exists }, "no day in the time header")
        XCTAssertEqual(day.label, "Today")
        shot("day-0-guide")

        try pick("Tomorrow")
        shot("day-1-tomorrow")
        XCTAssertTrue(until(3) { day.label == "Tomorrow" }, "Tomorrow shows \(day.label)")
        let eight = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH '8:00' AND label ENDSWITH 'PM'")).firstMatch
        XCTAssertTrue(eight.exists, "8 pm is not in the header")

        // Down lands on the first channel, and Right on its program tomorrow evening.
        press(.down, wait: 1)
        XCTAssertTrue(onChannel(), "Down from the days: \(focusedLabel())")
        press(.right, wait: 1)
        shot("day-2-tomorrow-cell")
        XCTAssertFalse(onChannel(), "Right stayed on the channel column: \(focusedLabel())")
        XCTAssertEqual(day.label, "Tomorrow")

        // Back from tomorrow comes back to now, on the channel column.
        press(.menu, wait: 1.2)
        shot("day-3-back")
        XCTAssertTrue(until(3) { day.label == "Today" }, "Back left the grid on \(day.label)")
        XCTAssertTrue(onChannel(), "focus on \(focusedLabel())")
        XCTAssertFalse(element("player-channel").exists, "Back started a channel")

        // The day after tomorrow, by its name, when the listings reach it.
        let third = try XCTUnwrap(Calendar.current.date(byAdding: .day, value: 2, to: Date())?.formatted(.dateTime.weekday(.abbreviated)))
        guard app.buttons[third].exists else { return }
        try pick(third)
        shot("day-4-third")
        XCTAssertTrue(until(3) { day.label.hasPrefix(third) }, "\(third) shows \(day.label)")
    }

    private func onChannel() -> Bool {
        let focus = focused()
        // The rail is 300 wide at the left edge. A program that began earlier also starts left of it.
        let frame = focus.frame
        return focus.exists && focus.elementType == .button && frame.minX >= 0 && frame.minX < 120 && frame.maxX <= 420 && frame.minY > 150
    }

    /// Up to the day row, then along it to the button.
    private func pick(_ label: String) throws {
        let third = Calendar.current.date(byAdding: .day, value: 2, to: Date())!.formatted(.dateTime.weekday(.abbreviated))
        let days = ["Now", "Tonight", "Tomorrow", third]
        for _ in 0 ..< 4 where !days.contains(focusedLabel()) && focused().frame.minY > 140 {
            press(.up, wait: 0.5)
        }
        // Past it, on the filters.
        if !days.contains(focusedLabel()) {
            press(.down, wait: 0.5)
        }
        XCTAssertTrue(days.contains(focusedLabel()), "not on the day row: \(focusedLabel())")
        for _ in 0 ..< 6 where focusedLabel() != label {
            press(.right, wait: 0.4)
        }
        XCTAssertEqual(focusedLabel(), label)
        press(.select, wait: 1.5)
    }
}

/// How long a channel takes from Select to a moving picture, cold and back to the one before.
final class ChannelStartTests: WatchingCase {
    func testChannelStartTimes() throws {
        try launch(["-BroadwaveChannelNow", "YES"])
        XCTAssertTrue(until(30) { self.focused().exists && self.focused().frame.minX < 120 }, "no guide focus")
        let tuning = element("tuning")
        let probe = app.staticTexts["channel-now"]
        var times: [String] = []
        func timed(_ name: String, _ action: () -> Void) {
            let start = Date()
            action()
            let shown = until(3) { tuning.exists }
            if shown {
                wait(0.3)
                shot("start-\(name)-card")
            }
            XCTAssertTrue(until(30) { !tuning.exists && probe.exists }, "\(name) never started")
            let took = Date().timeIntervalSince(start)
            times.append("\(name)=\(String(format: "%.2f", took))s")
            print("TIMING \(name) select->picture \(String(format: "%.2f", took))s card=\(shown)")
        }
        press(.down, 2, wait: 0.5)
        timed("cold-1") { XCUIRemote.shared.press(.select) }
        wait(6)
        // Up goes to the channel above, which is warm by now.
        timed("up-warm") { XCUIRemote.shared.press(.up) }
        wait(6)
        // Back to the one before, the other warm guess.
        timed("down-strip-previous") {
            XCUIRemote.shared.press(.down)
            wait(1)
            XCUIRemote.shared.press(.right)
            wait(0.5)
            XCUIRemote.shared.press(.select)
        }
        print("TIMING all \(times.joined(separator: " "))")
    }
}

/// Back from the player keeps the channel playing beside the guide; Back again is instant.
final class GuideLivePaneTests: WatchingCase {
    func testBackKeepsTheChannelInTheGuideAndBackAgainResumes() throws {
        try launch(["-BroadwaveChannelNow", "YES"])
        XCTAssertTrue(until(30) { self.focused().exists && self.focused().frame.minX < 120 }, "no guide focus")
        press(.down, wait: 0.5)
        press(.select, wait: 0.5)
        let tuning = element("tuning")
        let player = element("player-channel")
        XCTAssertTrue(until(30) { player.exists && !tuning.exists }, "the picture never started")
        let playing = player.value as? String ?? ""
        wait(3)
        shot("pane-0-playing")

        press(.menu, wait: 1.5)
        shot("pane-1-back")
        let pane = element("guide-live-pane")
        XCTAssertTrue(until(4) { pane.exists }, "Back left no live pane in the guide")
        XCTAssertFalse(player.exists, "the player is still full screen")
        XCTAssertTrue(pane.label.hasPrefix("Playing \(playing) "), "pane shows \(pane.label), played \(playing)")
        XCTAssertTrue(focused().frame.minX < 120, "focus on \(focusedLabel())")

        wait(2)
        let start = Date()
        press(.menu, wait: 0)
        XCTAssertTrue(until(5) { player.exists }, "Back on the guide did not resume")
        let shown = Date().timeIntervalSince(start)
        wait(0.4)
        shot("pane-2-resumed")
        XCTAssertFalse(tuning.exists, "resuming tuned again")
        print("TIMING resume from pane \(String(format: "%.2f", shown))s")
        XCTAssertEqual(player.value as? String, playing)

        // One Back still leaves the resumed player.
        press(.menu, wait: 1.5)
        XCTAssertFalse(player.exists, "one Back did not leave the resumed player")
    }
}

/// The guide opens on whole programs, keeps a row of lookahead, and shows where focus is.
final class GuideLookTests: WatchingCase {
    func testTheGridOpensCleanAndLooksAhead() throws {
        try launch()
        XCTAssertTrue(until(30) { self.focused().exists && self.focused().frame.minX < 120 && self.focused().frame.maxX <= 420 }, "no guide focus")
        wait(2)
        shot("look-0-guide")
        let edge = focused().frame.maxX
        // No program shows only its last few minutes at the left edge of the grid.
        let cells = try buttons().filter { $0.frame.minX < edge - 2 && $0.frame.maxX > edge && $0.frame.minY > 150 }
        let slivers = cells.filter { $0.frame.maxX - edge < 90 }.map(\.label)
        XCTAssertTrue(slivers.isEmpty, "slivers at the left edge: \(slivers)")

        // Walking down the column keeps the next channel on screen.
        for step in 1 ... 7 {
            press(.down, wait: 0.6)
            let row = focused().frame
            let below = try buttons().first { abs($0.frame.minX - row.minX) < 4 && abs($0.frame.minY - row.maxY) < 30 }
            XCTAssertNotNil(below, "step \(step): no row under \(focusedLabel())")
            if let below {
                XCTAssertLessThanOrEqual(below.frame.maxY, 1080 + 1, "step \(step): the next row is off screen")
            }
        }
        shot("look-1-down7")
        press(.right, wait: 0.8)
        shot("look-2-cell")
        XCTAssertGreaterThan(focused().frame.minX, edge - 40, "Right did not go into the grid: \(focusedLabel())")
    }

    /// Every button's frame and label in one pass; element queries one by one take minutes.
    private func buttons() throws -> [(frame: CGRect, label: String)] {
        var out: [(frame: CGRect, label: String)] = []
        var stack = try [app.snapshot()]
        while let next = stack.popLast() {
            if next.elementType == .button {
                out.append((next.frame, next.label))
            }
            stack.append(contentsOf: next.children)
        }
        return out
    }
}
