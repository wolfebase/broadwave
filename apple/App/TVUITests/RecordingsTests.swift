import XCTest

/// Manages recordings with the remote against a real server. Opt-in: run with
/// TEST_RUNNER_BROADWAVE_SERVER=http://host:port on a server that has two
/// finished recordings titled "Harness News" and "Harness Movie", the second
/// with a commercial marker from 5 to 20 s. Each test changes them, so the
/// server needs fresh ones for every run.
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
        // Delete is third in the menu. The dialog starts on Cancel, with Delete this file to its right.
        XCUIRemote.shared.press(.down)
        wait(0.5)
        XCUIRemote.shared.press(.down)
        wait(0.5)
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
