import XCTest

/// First run to side by side on a fresh server with the fake tuner: find the
/// server over Bonjour, finish setup, open the guide, play a channel in sync,
/// hold it while a browser plays the same channel, record, and play two
/// channels together. Opt-in. scripts/e2e-apple.sh sets
/// TEST_RUNNER_BROADWAVE_E2E_SERVER, _NAME, _ADDRESS, and _DIR.
final class EndToEndTests: XCTestCase {
    private var server = ""
    private var dir = ""
    private var trail: [String] = []

    func testFirstRunToMultiview() throws {
        executionTimeAllowance = 900
        continueAfterFailure = false
        server = value("BROADWAVE_E2E_SERVER")
        dir = value("BROADWAVE_E2E_DIR")
        let name = value("BROADWAVE_E2E_NAME")
        guard !server.isEmpty, !dir.isEmpty, !name.isEmpty else {
            throw XCTSkip("set TEST_RUNNER_BROADWAVE_E2E_SERVER, _NAME, and _DIR")
        }
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveDiscover", "YES", "-BroadwaveSyncProbe", "YES", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()

        try findServer(app, name: name)
        passed(1, "found \(name)")

        try finishSetup(app)
        passed(2, "setup finished")

        try openGuide(app)
        passed(3, "guide shows KBWV, KBWV2, WTST")
        #if os(tvOS)
            try reachRail(app)
        #endif

        let channel = 3
        try watch(app, "WTST")
        passed(4, "channel \(channel) locked \(probe(app))")

        try holdForBrowser(app, channel: channel)
        passed(7, "held channel \(channel) while the browser sampled")

        try closePlayer(app)
        try record(app, "WTST", channel: channel)
        passed(6, "recording on channel \(channel)")

        try multiview(app, [1, channel])
        passed(5, "tiles 1 and \(channel) locked")
    }

    // MARK: Steps

    private func findServer(_ app: XCUIApplication, name: String) throws {
        let row = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", name)).firstMatch
        if value("BROADWAVE_E2E_TYPED") == "1" {
            try typeAddress(app)
        } else if row.waitForExistence(timeout: 45) {
            note("discovery bonjour \(row.label)")
            shot("1-finder")
            activate(row)
            if !until(15, { !row.exists }) {
                // The row's address did not answer. Say so, then type it.
                let problem = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "No Broadwave server answered")).firstMatch
                note("defect the Bonjour row \(row.label) did not connect: \(problem.exists ? problem.label : "no message")")
                shot("1-finder-failed")
                try typeAddress(app)
            }
        } else {
            note("defect no Bonjour row for \(name)\n\(app.debugDescription)")
            shot("1-finder-empty")
            try typeAddress(app)
        }
        XCTAssertTrue(until(30) { !app.buttons["Try the demo"].exists }, "the finder stayed up\n\(app.debugDescription)")
    }

    private func typeAddress(_ app: XCUIApplication) throws {
        let address = value("BROADWAVE_E2E_ADDRESS")
        guard !address.isEmpty else {
            XCTFail("no address to type\n\(app.debugDescription)")
            return
        }
        #if os(tvOS)
            let enter = app.buttons["Enter an address"]
            if enter.waitForExistence(timeout: 10) {
                activate(enter)
            }
        #endif
        let field = app.textFields.firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: 10), app.debugDescription)
        #if os(tvOS)
            activate(field)
            pause(1.5)
            app.typeText(address)
            pause(0.5)
            XCUIRemote.shared.press(.menu)
            pause(1.5)
        #else
            field.tap()
            field.typeText(address)
        #endif
        note("discovery typed \(address)")
        let connect = app.buttons["Connect"]
        #if os(tvOS)
            // Connect sits to the right of the field.
            XCTAssertTrue(focus(connect, [.right, .right, .down, .right, .up, .right]), "no focus on Connect: \(trail) field=\(field.value ?? "")")
            XCUIRemote.shared.press(.select)
        #else
            connect.tap()
        #endif
    }

    private func finishSetup(_ app: XCUIApplication) throws {
        let title = app.staticTexts["Let's set up your TV"]
        XCTAssertTrue(title.waitForExistence(timeout: 30), "no setup wizard\n\(app.debugDescription)")
        let watch = app.buttons["Watch"]
        var sawTuner = false
        let ready = until(120) {
            if !sawTuner, app.staticTexts["Found"].exists || app.staticTexts["Your tuner"].exists {
                sawTuner = true
                self.shot("2-tuner")
            }
            return watch.exists && watch.isEnabled
        }
        XCTAssertTrue(ready, "setup did not finish\n\(app.debugDescription)")
        XCTAssertTrue(try devices() > 0, "the server has no tuner")
        note("setup tuner shown=\(sawTuner)")
        shot("2-ready")
        activate(watch)
        XCTAssertTrue(until(20) { (try? self.settings()["needsSetup"] as? String) == "0" }, "the server still needs setup")
        // Watch plays the first channel. Close it to reach the tabs.
        XCTAssertTrue(app.staticTexts["syncState"].waitForExistence(timeout: 30), "Watch did not open the player\n\(app.debugDescription)")
        try closePlayer(app)
    }

    private func openGuide(_ app: XCUIApplication) throws {
        #if os(tvOS)
            try tvTab(app, "Guide")
        #else
            let tab = app.tabBars.buttons["Guide"].exists ? app.tabBars.buttons["Guide"] : app.buttons["Guide"]
            XCTAssertTrue(tab.waitForExistence(timeout: 15), app.debugDescription)
            tab.tap()
        #endif
        for pattern in [".*KBWV(?!2).*", ".*KBWV2.*", ".*WTST.*"] {
            let row = app.buttons.matching(NSPredicate(format: "label MATCHES %@", pattern)).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 20), "no guide row \(pattern)\n\(app.debugDescription)")
        }
        shot("3-guide")
    }

    private func watch(_ app: XCUIApplication, _ channel: String) throws {
        try openProgram(app, channel)
        let watch = app.buttons["Watch"]
        XCTAssertTrue(watch.waitForExistence(timeout: 10), app.debugDescription)
        activate(watch)
        let state = app.staticTexts["syncState"]
        XCTAssertTrue(state.waitForExistence(timeout: 30), "no player\n\(app.debugDescription)")
        XCTAssertTrue(until(90) { self.locked(app) }, "the player did not lock: \(probe(app))")
        shot("4-playing")
    }

    /// The script opens the same channel in Chrome once `watching` exists and
    /// writes `sampled` when it has enough paired samples.
    private func holdForBrowser(_ app: XCUIApplication, channel: Int) throws {
        let watching = URL(fileURLWithPath: dir).appendingPathComponent("watching")
        let sampled = URL(fileURLWithPath: dir).appendingPathComponent("sampled")
        try? FileManager.default.removeItem(at: sampled)
        try "\(channel)\n".write(to: watching, atomically: true, encoding: .utf8)
        var lines: [String] = []
        let done = until(Double(value("BROADWAVE_E2E_SYNC_WAIT")) ?? 240) {
            let text = self.probe(app)
            let ms = Int(Date().timeIntervalSince1970 * 1000)
            lines.append(#"{"t":\#(ms),"probe":"\#(text)"}"#)
            return FileManager.default.fileExists(atPath: sampled.path)
        }
        try? (lines.joined(separator: "\n") + "\n").write(
            to: URL(fileURLWithPath: dir).appendingPathComponent("apple-probe.jsonl"), atomically: true, encoding: .utf8
        )
        try? FileManager.default.removeItem(at: watching)
        XCTAssertTrue(done, "the browser did not finish sampling")
        XCTAssertTrue(locked(app), "the player left sync while the browser played: \(probe(app))")
        shot("7-synced")
    }

    private func record(_ app: XCUIApplication, _ channel: String, channel id: Int) throws {
        let before = try recordings().count
        try openProgram(app, channel)
        let record = app.buttons["Record"].firstMatch
        XCTAssertTrue(record.waitForExistence(timeout: 10), app.debugDescription)
        activate(record)
        var found: [String: Any]?
        XCTAssertTrue(until(30) {
            let list = (try? self.recordings()) ?? []
            found = list.first { ($0["channelId"] as? Int) == id }
            return list.count > before && found != nil
        }, "no recording on the server: \((try? recordings()) ?? [])")
        note("recording \(found ?? [:])")
        XCTAssertTrue(app.buttons["Stop recording"].waitForExistence(timeout: 10), "the sheet did not show the recording\n\(app.debugDescription)")
        shot("6-recording")
        dismissSheet(app)
    }

    private func multiview(_ app: XCUIApplication, _ channels: [Int]) throws {
        app.terminate()
        app.launchArguments = [
            "-BroadwaveMultiview", channels.map(String.init).joined(separator: ","),
            "-BroadwaveMultiviewLayout", "2up",
            "-BroadwaveTileStats", "YES",
            "-BroadwaveSyncLog", "1",
            "-ApplePersistenceIgnoreState", "YES",
        ]
        app.launch()
        let tiles = channels.map { app.buttons.matching(identifier: "tile-\($0)").firstMatch }
        for tile in tiles {
            XCTAssertTrue(tile.waitForExistence(timeout: 40), app.debugDescription)
        }
        XCTAssertTrue(until(90) { tiles.allSatisfy { self.settled($0) } }, "tiles did not lock: \(tiles.map { $0.value as? String ?? "" })")
        note("tiles \(tiles.map { $0.value as? String ?? "" })")
        shot("5-multiview")
    }

    // MARK: Navigation

    private func openProgram(_ app: XCUIApplication, _ channel: String) throws {
        let row = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", channel)).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 20), app.debugDescription)
        activate(row)
    }

    private func closePlayer(_ app: XCUIApplication) throws {
        let state = app.staticTexts["syncState"]
        #if os(tvOS)
            XCTAssertTrue(until(20) {
                if !state.exists {
                    return true
                }
                XCUIRemote.shared.press(.menu)
                self.pause(1.5)
                return !state.exists
            }, "the player stayed open\n\(app.debugDescription)")
        #else
            let minimize = app.buttons["Minimize"].firstMatch
            if minimize.waitForExistence(timeout: 5) {
                minimize.tap()
            } else {
                app.swipeDown()
            }
            XCTAssertTrue(until(10) { !state.exists }, "the player stayed open\n\(app.debugDescription)")
            let close = app.buttons["Close"].firstMatch
            if close.waitForExistence(timeout: 5) {
                close.tap()
            }
        #endif
    }

    private func dismissSheet(_ app: XCUIApplication) {
        #if os(tvOS)
            XCUIRemote.shared.press(.menu)
        #else
            app.swipeDown(velocity: .fast)
        #endif
        pause(1)
    }

    #if os(tvOS)
        /// Menu opens the collapsed sidebar on the current tab. A second Menu would leave the app.
        private func tvTab(_ app: XCUIApplication, _ name: String) throws {
            let tab = app.buttons[name].firstMatch
            XCTAssertTrue(tab.waitForExistence(timeout: 15), app.debugDescription)
            if !tab.isEnabled {
                XCUIRemote.shared.press(.menu)
                XCTAssertTrue(until(5) { tab.isEnabled }, "the sidebar did not open: \(focused(app))\n\(app.debugDescription)")
            }
            let moves: [XCUIRemote.Button] = Array(repeating: .down, count: 6) + Array(repeating: .up, count: 7)
            XCTAssertTrue(focus(tab, moves), "no focus on \(name): \(focused(app))\n\(app.debugDescription)")
            XCUIRemote.shared.press(.select)
            pause(1)
        }

        /// Right after setup the guide's channel column once refused focus: Down
        /// from Now went nowhere. If that comes back, the run fails with a note
        /// and the hierarchy, and a relaunch lets the other steps still run.
        private func reachRail(_ app: XCUIApplication) throws {
            let first = app.buttons["4.1 KBWV"].firstMatch
            if focus(first, [.down, .down, .down]) {
                return
            }
            note("defect guide channels unreachable after setup: \(trail)")
            shot("3-guide-stuck")
            try? app.debugDescription.write(toFile: "\(dir)/3-guide-stuck.txt", atomically: true, encoding: .utf8)
            app.terminate()
            app.launchArguments = ["-BroadwaveSyncProbe", "YES", "-ApplePersistenceIgnoreState", "YES"]
            app.launch()
            try tvTab(app, "Guide")
            XCTAssertTrue(focus(first, [.down, .down, .down]), "no focus on the guide's channels after a relaunch: \(trail)")
        }

        /// Presses through the moves until the element has focus.
        private func focus(_ element: XCUIElement, _ moves: [XCUIRemote.Button]) -> Bool {
            trail = []
            for move in moves {
                if element.hasFocus {
                    return true
                }
                XCUIRemote.shared.press(move)
                pause(0.7)
                trail.append("\(move.rawValue) \(focused(XCUIApplication()))")
            }
            return element.hasFocus
        }

        private func focused(_ app: XCUIApplication) -> String {
            let element = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
            return element.exists ? "\(element.identifier) \(element.label)" : "nothing"
        }
    #endif

    /// Tap on iPhone. On Apple TV, move focus onto it and press Select.
    private func activate(_ element: XCUIElement) {
        #if os(tvOS)
            let moves: [XCUIRemote.Button] = Array(repeating: .down, count: 6) + Array(repeating: .up, count: 8)
            if !focus(element, moves) {
                shot("focus-miss")
                XCTFail("no focus on \(element.label): \(trail)\n\(XCUIApplication().debugDescription)")
            }
            XCUIRemote.shared.press(.select)
        #else
            element.tap()
        #endif
    }

    // MARK: Checks

    private func probe(_ app: XCUIApplication) -> String {
        let state = app.staticTexts["syncState"]
        return state.exists ? state.label : "none"
    }

    private func locked(_ app: XCUIApplication) -> Bool {
        let text = probe(app)
        guard text.hasPrefix("locked drift="), let drift = Int(text.dropFirst("locked drift=".count)) else { return false }
        return abs(drift) <= 80
    }

    private func settled(_ tile: XCUIElement) -> Bool {
        let text = tile.value as? String ?? ""
        guard let range = text.range(of: #"drift (-?\d+), rate ([0-9.]+), locked"#, options: .regularExpression) else { return false }
        let parts = text[range].split(separator: " ")
        let drift = Int(parts[1].dropLast()) ?? 999
        let rate = Double(parts[3].dropLast()) ?? 0
        return abs(drift) <= 80 && rate >= 0.5
    }

    // MARK: Server

    private func settings() throws -> [String: Any] {
        try get("/api/v1/settings")
    }

    private func devices() throws -> Int {
        try (get("/api/v1/devices")["devices"] as? [Any])?.count ?? 0
    }

    private func recordings() throws -> [[String: Any]] {
        try get("/api/v1/recordings")["recordings"] as? [[String: Any]] ?? []
    }

    private func get(_ path: String) throws -> [String: Any] {
        let url = try XCTUnwrap(URL(string: server + path))
        var body: Data?
        let done = expectation(description: path)
        URLSession.shared.dataTask(with: url) { data, _, _ in
            body = data
            done.fulfill()
        }.resume()
        wait(for: [done], timeout: 10)
        let data = try XCTUnwrap(body, "no answer from \(path)")
        return try JSONSerialization.jsonObject(with: data) as? [String: Any] ?? [:]
    }

    // MARK: Helpers

    private func value(_ name: String) -> String {
        let env = ProcessInfo.processInfo.environment
        if let direct = env[name], !direct.isEmpty {
            return direct
        }
        return env["TEST_RUNNER_\(name)"] ?? ""
    }

    private func passed(_ step: Int, _ detail: String) {
        print("broadwave-e2e pass \(step) \(detail)")
    }

    private func note(_ text: String) {
        print("broadwave-e2e note \(text)")
    }

    private func shot(_ name: String) {
        let url = URL(fileURLWithPath: dir).appendingPathComponent("\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
    }

    /// A screenshot is evidence, not a check. On a busy CI runner one timed
    /// out after the player had locked, and XCTest failed the run for it.
    override func record(_ issue: XCTIssue) {
        if issue.compactDescription.contains("Failed to get screenshot") {
            note("screenshot skipped: \(issue.compactDescription)")
            return
        }
        super.record(issue)
    }

    private func pause(_ seconds: TimeInterval) {
        RunLoop.current.run(until: Date().addingTimeInterval(seconds))
    }

    private func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while Date() < end {
            if done() {
                return true
            }
            pause(0.5)
        }
        return done()
    }
}
