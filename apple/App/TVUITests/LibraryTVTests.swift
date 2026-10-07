import XCTest

/// The recordings library with the remote: Continue watching, then Select, one
/// recording, and Mark watched. Opt-in: TEST_RUNNER_BROADWAVE_SERVER on a server
/// seeded as `LibraryTests` describes; it marks one recording watched.
final class LibraryTVTests: XCTestCase {
    func testSelectOneWithTheRemoteAndMarkItWatched() throws {
        guard let server = ProcessInfo.processInfo.environment["BROADWAVE_SERVER"], !server.isEmpty else {
            throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER")
        }
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveServerURL", server, "-BroadwaveTab", "recordings", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()
        XCTAssertTrue(app.staticTexts["Continue watching"].waitForExistence(timeout: 20), app.debugDescription)
        XCTAssertTrue(until(10) { self.focused("recording-row", in: app) }, "focus on \(focus(app).label)")
        shot("BROADWAVE_SHOT")

        XCTAssertTrue(press(.up, until: { self.focused("library-select", in: app) }), "focus on \(focus(app).label)")
        XCUIRemote.shared.press(.select)
        XCTAssertTrue(press(.down, until: { self.focused("recording-pick", in: app) }), "focus on \(focus(app).label)")
        let name = focus(app).label
        XCUIRemote.shared.press(.select)
        let count = app.staticTexts["selected-count"]
        XCTAssertTrue(until(5) { count.label.uppercased() == "1 SELECTED" }, count.label)
        shot("BROADWAVE_SHOT_SELECT")

        XCTAssertTrue(press(.up, until: { self.focus(app).label == "Mark watched" }), "focus on \(focus(app).label)")
        XCUIRemote.shared.press(.select)
        XCTAssertTrue(app.staticTexts["Marked 1 recording watched."].waitForExistence(timeout: 10), app.debugDescription)
        let data = try Data(contentsOf: URL(string: server + "/api/v1/recordings")!)
        let list = try (JSONSerialization.jsonObject(with: data) as? [String: Any])?["recordings"] as? [[String: Any]] ?? []
        let marked = list.filter { $0["watched"] as? Int == 1 }.compactMap { ($0["subtitle"] as? String) ?? ($0["title"] as? String) }
        XCTAssertTrue(marked.contains { name.contains($0) }, "picked \(name), watched \(marked)")
    }

    /// In a list the focused element is the cell; the identifier is on the button inside it.
    private func focused(_ identifier: String, in app: XCUIApplication) -> Bool {
        let cell = focus(app)
        return cell.identifier == identifier || cell.descendants(matching: .any)[identifier].exists
    }

    private func focus(_ app: XCUIApplication) -> XCUIElement {
        app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
    }

    private func press(_ button: XCUIRemote.Button, until done: () -> Bool) -> Bool {
        for _ in 0 ..< 14 {
            if done() {
                return true
            }
            XCUIRemote.shared.press(button)
            RunLoop.current.run(until: Date().addingTimeInterval(0.5))
        }
        return done()
    }

    private func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while Date() < end {
            if done() {
                return true
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.5))
        }
        return done()
    }

    private func shot(_ key: String) {
        if let path = ProcessInfo.processInfo.environment[key] {
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: path))
        }
    }
}
