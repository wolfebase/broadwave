import XCTest

/// The pages over the guide, used the way a viewer does: remote presses only.
/// Search needs a server with listings (TEST_RUNNER_BROADWAVE_SERVER, read-only: nothing is
/// recorded). The empty Recordings page and Join run offline.
final class TVPagesTests: XCTestCase {
    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    /// Results show while typing, under the keyboard, with no press of the search key.
    func testSearchShowsResultsWhileTyping() throws {
        let server = try serverURL()
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveServerURL", server, "-BroadwaveTab", "search", "-lastChannel", "0", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()
        let page = app.descendants(matching: .any)["search-page"]
        XCTAssertTrue(page.waitForExistence(timeout: 30), app.debugDescription)
        let keyboard = app.keyboards.firstMatch
        XCTAssertTrue(keyboard.waitForExistence(timeout: 10), "no keyboard on the search page: \(focused(app))")
        shot("search-0-open")
        app.typeText("news")
        let result = app.buttons.matching(NSPredicate(format: "identifier == 'search-watch' OR identifier == 'search-later' OR identifier == 'search-recording'")).firstMatch
        XCTAssertTrue(result.waitForExistence(timeout: 15), "no results while typing\n\(app.debugDescription)")
        XCTAssertTrue(keyboard.exists, "the keyboard closed")
        XCTAssertTrue(result.isHittable, "results sit behind the keyboard")
        shot("search-1-typed")
        // Down from the keyboard reaches a result.
        for _ in 0 ..< 6 where !focusedID(app).hasPrefix("search-") {
            XCUIRemote.shared.press(.down)
            pause(0.6)
        }
        XCTAssertTrue(focusedID(app).hasPrefix("search-"), "Down did not reach a result: \(focused(app))")
        shot("search-2-result")
    }

    /// Settings holds what a viewer changes from the couch. Whole-house setup is on the web
    /// app; the page shows its address and a code instead of forms.
    func testSettingsIsAListOfCouchSettings() {
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveFocus", "settings", "-lastChannel", "0", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()
        XCTAssertTrue(app.descendants(matching: .any)["settings-playback"].waitForExistence(timeout: 20), app.debugDescription)
        XCTAssertTrue(until(6) { focusedLabel(app).hasPrefix("Playback and Picture") }, "Settings did not open on its first row: \(focused(app))")
        for word in ["Backups", "Schedules Direct", "HDHomeRun", "Scan again", "Run setup again"] {
            XCTAssertFalse(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", word)).firstMatch.exists, "\(word) is still on the TV")
        }
        shot("settings-0")
        for _ in 0 ..< 6 where !focusedLabel(app).hasPrefix("Whole-Home Sync") {
            XCUIRemote.shared.press(.down)
            pause(0.5)
        }
        XCUIRemote.shared.press(.select)
        let row = app.descendants(matching: .any)["settings-playback"]
        XCTAssertTrue(until(6) { !row.exists || !row.isHittable }, "Whole-Home Sync did not open: \(focused(app))")
        pause(1)
        shot("settings-1-sync")
        XCTAssertFalse(focused(app) == "nothing", "nothing focused on Whole-Home Sync")
        XCUIRemote.shared.press(.menu)
        XCTAssertTrue(until(6) { focusedLabel(app).hasPrefix("Whole-Home Sync") }, "Back did not return to the row: \(focused(app))")
    }

    private func focusedLabel(_ app: XCUIApplication) -> String {
        let element = focusedElement(app)
        return element.exists ? element.label : ""
    }

    // MARK: Helpers

    private func serverURL() throws -> String {
        guard let server = ProcessInfo.processInfo.environment["BROADWAVE_SERVER"], !server.isEmpty else {
            throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER to a server with listings")
        }
        return server
    }

    private func focusedElement(_ app: XCUIApplication) -> XCUIElement {
        app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
    }

    private func focusedID(_ app: XCUIApplication) -> String {
        let element = focusedElement(app)
        return element.exists ? element.identifier : ""
    }

    private func focused(_ app: XCUIApplication) -> String {
        let element = focusedElement(app)
        guard element.exists else { return "nothing" }
        return "\(element.identifier) '\(element.label)'"
    }

    private func shot(_ name: String) {
        guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: folder.appending(path: "\(name).png"))
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
            pause(0.2)
        }
        return done()
    }
}
