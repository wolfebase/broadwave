import XCTest

/// Manages recordings with the remote against a real server. Opt-in: run with
/// TEST_RUNNER_BROADWAVE_SERVER=http://host:port on a server that has finished
/// recordings titled "Harness News", "Harness Movie" (with a commercial marker
/// from 5 to 20 s), and "Evening News", and series passes where "Movie Hour" is
/// skipped for lack of a tuner with a later airing that fits. Each test changes
/// them, so the server needs fresh ones for every run.
final class RecordingsTests: XCTestCase {
    func testMarkWatchedThenDelete() throws {
        let server = try serverURL()
        let app = launch(server)
        let row = try focusRow(app, "Harness News")
        XCTAssertFalse(row.label.contains("Watched"), row.label)

        XCTAssertTrue(openMenu(app, showing: "Mark watched"), "no menu, focus on \(focused(app))")
        XCUIRemote.shared.press(.select)
        XCTAssertTrue(until(10) { self.row(app, "Harness News").label.contains("Watched") }, self.row(app, "Harness News").label)
        XCTAssertTrue(until(5) { !self.entry(app, "Mark watched").exists }, "menu stayed open")

        XCTAssertTrue(openMenu(app, showing: "Mark unwatched"), "no menu, focus on \(focused(app))")
        // Menu entries do not report focus, so the remote moves by position:
        // Delete is fourth in the menu. The dialog starts on Cancel, with Delete this file to its right.
        for _ in 0 ..< 3 {
            XCUIRemote.shared.press(.down)
            wait(0.5)
        }
        XCUIRemote.shared.press(.select)
        XCTAssertTrue(entry(app, "Delete this file").waitForExistence(timeout: 5), "no confirmation")
        XCUIRemote.shared.press(.right)
        XCTAssertTrue(until(3) { self.entry(app, "Delete this file").hasFocus }, "focus on \(focused(app))")
        if let shot = ProcessInfo.processInfo.environment["BROADWAVE_SHOT"] {
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: shot))
        }
        XCUIRemote.shared.press(.select)
        XCTAssertTrue(until(10) { !self.row(app, "Harness News").exists }, "row still listed")
        XCTAssertFalse(try titles(server).contains("Harness News"))
    }

    /// With Skip them, playing from the start moves past the 5-20 s break.
    func testPlayingSkipsABreak() throws {
        let server = try serverURL()
        let app = launch(server)
        _ = try focusRow(app, "Harness Movie")
        XCUIRemote.shared.press(.select)
        wait(14)
        let position = try leavePlayer(server) { $0["title"] as? String == "Harness Movie" }
        let passed = position > 20
        XCTAssertTrue(passed, "saved position \(position): the break played")
    }

    /// With Show a Skip button, the player offers Skip break inside a break, and it skips.
    /// Uses "Evening News" (a minute long): its resume point is cleared and a 5-40 s break added.
    func testSkipButtonInABreak() throws {
        let server = try serverURL()
        let id = try XCTUnwrap(try recordings(server).first { $0["title"] as? String == "Evening News" }?["id"] as? Int)
        try call(server, "PUT", "/api/v1/recordings/\(id)/progress", ["position": 0])
        try call(server, "POST", "/api/v1/recordings/\(id)/markers", ["start": 5, "end": 40])
        let app = XCUIApplication()
        app.launchArguments = [
            "-BroadwaveServerURL", server, "-BroadwaveTab", "recordings", "-breakSkip", "button", "-ApplePersistenceIgnoreState", "YES",
        ]
        app.launch()
        _ = try focusRow(app, "Evening News")
        XCUIRemote.shared.press(.select)
        // AVKit keeps its contextual actions out of the accessibility tree, so the test
        // presses Select inside the break: Skip break has focus then, and without it
        // Select pauses the player about 12 s in.
        wait(12)
        if let shot = ProcessInfo.processInfo.environment["BROADWAVE_SHOT"] {
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: shot))
        }
        XCUIRemote.shared.press(.select)
        wait(2)
        let position = try leavePlayer(server) { $0["id"] as? Int == id }
        let passed = position >= 39.5
        XCTAssertTrue(passed, "saved position \(position): Skip break did not skip")
    }

    /// Make a channel turns "Harness Movie" into a library channel, which plays from the list
    /// without a tuner. In its player the transport bar marks a break by hand and removes it.
    func testMakeAChannelThenMarkABreak() throws {
        let server = try serverURL()
        let app = launch(server)
        _ = try focusRow(app, "Harness Movie")
        XCTAssertTrue(openMenu(app, showing: "Make a channel"), "no menu, focus on \(focused(app))")
        XCUIRemote.shared.press(.down)
        wait(0.5)
        XCUIRemote.shared.press(.select)
        let library = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", "Harness Movie channel")).firstMatch
        XCTAssertTrue(library.waitForExistence(timeout: 10), "no library channel listed")
        let made = try get(server, "/api/v1/virtuals")["virtuals"] as? [[String: Any]] ?? []
        XCTAssertEqual(made.first?["name"] as? String, "Harness Movie channel")
        for _ in 0 ..< 8 where !focused(app).contains("Harness Movie channel") {
            XCUIRemote.shared.press(.up)
            wait(0.5)
        }
        XCTAssertTrue(focused(app).contains("Harness Movie channel"), "focus on \(focused(app))")
        XCUIRemote.shared.press(.select)
        wait(8)
        shot("BROADWAVE_SHOT")
        let tuners = try get(server, "/api/v1/tuners")["tuners"] as? [[String: Any]] ?? []
        XCTAssertFalse(tuners.contains { $0["ours"] as? Bool == true }, "a library channel took a tuner")

        // AVKit keeps the transport bar's own entries out of the accessibility tree, so the
        // remote moves by position: Up from the scrubber lands on the first entry, and the
        // player's note says what happened.
        let recording = try XCTUnwrap(made.first?["recordings"] as? [Int]).first ?? 0
        pressEntry(0)
        XCTAssertTrue(note(app, "Break starts at").waitForExistence(timeout: 5), "no start mark: \(app.debugDescription)")
        XCUIRemote.shared.press(.playPause)
        wait(4)
        pressEntry(0)
        XCTAssertTrue(note(app, "Marked a break").waitForExistence(timeout: 5), "no break marked")
        let marks = try get(server, "/api/v1/recordings/\(recording)/markers")["markers"] as? [[String: Any]] ?? []
        XCTAssertEqual(marks.count, 1)
        let length = (marks.first?["end"] as? Double ?? 0) - (marks.first?["start"] as? Double ?? 0)
        XCTAssertTrue(length > 2 && length < 8, "marked \(length) s")
        shot("BROADWAVE_SHOT_BAR")

        // Remove a break is the second entry and opens a list of the breaks.
        pressEntry(1)
        wait(1)
        shot("BROADWAVE_SHOT_BAR2")
        XCUIRemote.shared.press(.select)
        XCTAssertTrue(note(app, "Removed the break").waitForExistence(timeout: 5), "no break removed")
        let left = try get(server, "/api/v1/recordings/\(recording)/markers")["markers"] as? [[String: Any]] ?? []
        XCTAssertTrue(left.isEmpty, "\(left)")
    }

    /// Pauses to bring up the transport bar, then selects its entry at `index`. The
    /// entries sit above the scrubber on the right.
    private func pressEntry(_ index: Int) {
        XCUIRemote.shared.press(.playPause)
        wait(1.5)
        XCUIRemote.shared.press(.up)
        wait(0.8)
        for _ in 0 ..< index {
            XCUIRemote.shared.press(.right)
            wait(0.5)
        }
        XCUIRemote.shared.press(.select)
    }

    private func note(_ app: XCUIApplication, _ start: String) -> XCUIElement {
        app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", start)).firstMatch
    }

    private func get(_ server: String, _ path: String) throws -> [String: Any] {
        let data = try Data(contentsOf: URL(string: server + path)!)
        return try JSONSerialization.jsonObject(with: data) as? [String: Any] ?? [:]
    }

    private func shot(_ key: String) {
        if let path = ProcessInfo.processInfo.environment[key] {
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: path))
        }
    }

    /// Coming up shows the skipped airing and records the later one instead.
    func testComingUpRecordsTheLaterAiring() throws {
        let server = try serverURL()
        let app = launch(server)
        XCTAssertTrue(until(20) { self.row(app, "Coming up").exists }, "no Coming up")
        for _ in 0 ..< 6 where !focused(app).contains("Coming up") {
            XCUIRemote.shared.press(.up)
            wait(0.5)
        }
        XCTAssertTrue(focused(app).contains("Coming up"), "focus on \(focused(app))")
        XCUIRemote.shared.press(.select)
        let later = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", "Record the later airing")).firstMatch
        XCTAssertTrue(later.waitForExistence(timeout: 10), "no later airing offered")
        for _ in 0 ..< 8 where !focused(app).contains("Record the later airing") {
            XCUIRemote.shared.press(.down)
            wait(0.5)
        }
        XCTAssertTrue(focused(app).contains("Record the later airing"), "focus on \(focused(app))")
        if let shot = ProcessInfo.processInfo.environment["BROADWAVE_SHOT"] {
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: shot))
        }
        XCUIRemote.shared.press(.select)
        XCTAssertTrue(until(10) { !later.exists }, "the later airing is still offered")
        let data = try Data(contentsOf: URL(string: server + "/api/v1/schedule")!)
        let items = try (JSONSerialization.jsonObject(with: data) as? [String: Any])?["items"] as? [[String: Any]] ?? []
        XCTAssertFalse(items.contains { $0["suggestion"] != nil }, "the server still suggests a later airing")
    }

    /// Presses Menu until the player closes and saves its position, and returns it. The
    /// first press can only hide the transport bar, and the list stays in the tree under
    /// the player, so the saved position is what tells the player closed.
    private func leavePlayer(_ server: String, _ match: @escaping ([String: Any]) -> Bool) throws -> Double {
        var position = 0.0
        for _ in 0 ..< 3 {
            XCUIRemote.shared.press(.menu)
            let saved = until(4) {
                position = (try? self.recordings(server).first(where: match)?["position"] as? Double) ?? 0
                return position > 0
            }
            if saved {
                break
            }
        }
        return position
    }

    private func call(_ server: String, _ method: String, _ path: String, _ body: [String: Any]) throws {
        var req = URLRequest(url: URL(string: server + path)!)
        req.httpMethod = method
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONSerialization.data(withJSONObject: body)
        let done = expectation(description: path)
        URLSession.shared.dataTask(with: req) { _, _, _ in done.fulfill() }.resume()
        wait(for: [done], timeout: 10)
    }

    private func serverURL() throws -> String {
        guard let server = ProcessInfo.processInfo.environment["BROADWAVE_SERVER"], !server.isEmpty else {
            throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER to run against a server")
        }
        return server
    }

    private func launch(_ server: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = [
            "-BroadwaveServerURL", server, "-BroadwaveTab", "recordings", "-breakSkip", "auto", "-ApplePersistenceIgnoreState", "YES",
        ]
        app.launch()
        return app
    }

    /// Menu and dialog entries are not buttons on tvOS, so they are found by label.
    private func entry(_ app: XCUIApplication, _ label: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", label)).firstMatch
    }

    private func row(_ app: XCUIApplication, _ title: String) -> XCUIElement {
        app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", title)).firstMatch
    }

    private func focusRow(_ app: XCUIApplication, _ title: String) throws -> XCUIElement {
        XCTAssertTrue(until(20) { self.row(app, title).exists }, "no row \(title)")
        for _ in 0 ..< 6 where !focused(app).contains(title) {
            XCUIRemote.shared.press(.down)
            wait(0.5)
        }
        XCTAssertTrue(focused(app).contains(title), "focus on \(focused(app))")
        return row(app, title)
    }

    private func recordings(_ server: String) throws -> [[String: Any]] {
        let data = try Data(contentsOf: URL(string: server + "/api/v1/recordings")!)
        let body = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        return body?["recordings"] as? [[String: Any]] ?? []
    }

    private func titles(_ server: String) throws -> [String] {
        try recordings(server).compactMap { $0["title"] as? String }
    }

    /// Holds Select for the row's menu. A hold right after a menu closes can be
    /// lost while the menu animates away, so it waits and tries once more.
    private func openMenu(_ app: XCUIApplication, showing item: String) -> Bool {
        for _ in 0 ..< 2 {
            wait(1)
            XCUIRemote.shared.press(.select, forDuration: 1.5)
            if entry(app, item).waitForExistence(timeout: 4) {
                return true
            }
        }
        return false
    }

    private func focused(_ app: XCUIApplication) -> String {
        let element = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        return element.exists ? element.label : "nothing"
    }

    private func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while Date() < end {
            if done() {
                return true
            }
            wait(0.5)
        }
        return done()
    }

    private func wait(_ seconds: TimeInterval) {
        RunLoop.current.run(until: Date().addingTimeInterval(seconds))
    }
}
