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

    /// Live Delay says what it does: every channel starts about 5 s behind whichever is picked.
    func testLiveDelaySaysWhatItDoes() {
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveFocus", "settings", "-lastChannel", "0", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()
        XCTAssertTrue(app.descendants(matching: .any)["settings-playback"].waitForExistence(timeout: 20), app.debugDescription)
        for _ in 0 ..< 8 where !focusedLabel(app).hasPrefix("Live Delay") {
            XCUIRemote.shared.press(.down)
            pause(0.5)
        }
        XCTAssertTrue(focusedLabel(app).hasPrefix("Live Delay"), "no Live Delay row: \(focused(app))")
        XCUIRemote.shared.press(.select)
        let note = app.staticTexts.containing(NSPredicate(format: "label CONTAINS 'about 5 seconds'")).firstMatch
        XCTAssertTrue(note.waitForExistence(timeout: 6), "no explanation: \(focused(app))")
        pause(1)
        shot("settings-2-delay")
    }

    /// An empty library still has something to press: Open the guide.
    func testEmptyRecordingsHasAFocusedWayForward() {
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveFocus", "recordings", "-lastChannel", "0", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()
        let open = app.buttons["recordings-open-guide"]
        XCTAssertTrue(open.waitForExistence(timeout: 20), app.debugDescription)
        XCTAssertTrue(until(8) { open.hasFocus }, "nothing focused on the empty page: \(focused(app))")
        XCTAssertTrue(app.buttons["recordings-how"].exists)
        shot("recordings-empty")
        XCUIRemote.shared.press(.select)
        XCTAssertTrue(until(8) { app.buttons["4.1 One"].firstMatch.isHittable }, "Open the guide did not open it: \(focused(app))")
    }

    /// A library with recordings is shelves of posters. Read-only: nothing is played or changed.
    func testRecordingsAreShelves() throws {
        let server = try serverURL()
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveServerURL", server, "-BroadwaveTab", "recordings", "-lastChannel", "0", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()
        let card = app.buttons["recording-card"].firstMatch
        guard card.waitForExistence(timeout: 30) else {
            throw XCTSkip("no recordings on \(server)")
        }
        XCTAssertTrue(until(8) { focusedID(app) == "recording-card" }, "no recording focused: \(focused(app))")
        XCTAssertTrue(app.staticTexts["Recently recorded"].exists || app.staticTexts["Continue watching"].exists, app.debugDescription)
        XCTAssertFalse(app.staticTexts.containing(NSPredicate(format: "label MATCHES %@", ".*[0-9.]+ (KB|MB|GB).*")).firstMatch.exists, "a file size is on screen")
        shot("recordings-0")
        for _ in 0 ..< 6 where focusedID(app) != "show-page" {
            XCUIRemote.shared.press(.down)
            pause(0.6)
        }
        XCTAssertEqual(focusedID(app), "show-page", "Down did not reach the Shows shelf: \(focused(app))")
        let name = focusedLabel(app).components(separatedBy: ",")[0]
        shot("recordings-1-shows")
        XCUIRemote.shared.press(.select)
        let title = app.descendants(matching: .any)["page-title"].firstMatch
        XCTAssertTrue(until(8) { title.exists && title.label.hasPrefix(name) && focusedLabel(app).hasPrefix(name) }, "\(name) did not open on an episode: \(focused(app))")
        shot("recordings-2-show")
        XCUIRemote.shared.press(.menu)
        XCTAssertTrue(until(8) { focusedID(app) == "show-page" }, "Back did not return to the show: \(focused(app))")
    }

    /// Join a home server opens on the address field, on screen, not on a saved server.
    /// Two saved servers (unreachable test addresses) push the buttons down the column,
    /// the layout a fixed press count never reached.
    func testJoinFocusesTheAddressField() {
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveDiscover", "YES", "-ApplePersistenceIgnoreState", "YES", "-servers", savedServers(2)]
        app.launch()
        let open = app.buttons["join-open"]
        XCTAssertTrue(open.waitForExistence(timeout: 30), app.debugDescription)
        shot("connect-0")
        XCTAssertTrue(walkToJoin(app, open), "could not reach Join: \(focused(app))")
        shot("connect-1-on-join")
        XCUIRemote.shared.press(.select)
        let address = app.textFields["join-address"]
        XCTAssertTrue(address.waitForExistence(timeout: 10), app.debugDescription)
        XCTAssertTrue(until(6) { address.hasFocus }, "the address field did not take focus: \(focused(app))")
        XCTAssertTrue(address.isHittable, "the address field is off screen")
        pause(2)
        XCTAssertTrue(address.hasFocus, "focus moved off the address field: \(focused(app))")
        shot("connect-2-join")
    }

    /// Sports lists games, not shows about sports, each once, with something focused.
    /// Read-only: nothing is played or recorded.
    func testSportsListsGamesOnly() throws {
        let server = try serverURL()
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveServerURL", server, "-BroadwaveTab", "sports", "-lastChannel", "0", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()
        XCTAssertTrue(app.descendants(matching: .any)["sports-page"].waitForExistence(timeout: 30), app.debugDescription)
        let settled = until(15) { focusedID(app).hasPrefix("sports-") }
        shot("sports-0")
        XCTAssertTrue(settled, "nothing focused on Sports: \(focused(app))\n\(app.debugDescription)")
        for show in ["SportsPulse", "Stay Fierce", "El pelotazo", "In Depth With Graham Bensinger"] {
            XCTAssertFalse(app.buttons.containing(NSPredicate(format: "label BEGINSWITH %@", show)).firstMatch.exists, "\(show) is not a game")
        }
        var labels: [String] = []
        for card in app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'sports-l'")).allElementsBoundByIndex {
            labels.append(card.label)
        }
        XCTAssertEqual(labels.count, Set(labels).count, "a game is listed twice: \(labels)")
        XCUIRemote.shared.press(.down)
        pause(1)
        shot("sports-1-down")
    }

    /// Home opens on the hero's Watch button, under a title that scrolls with the page.
    func testHomeOpensOnTheHero() throws {
        let server = try serverURL()
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveServerURL", server, "-BroadwaveTab", "home", "-lastChannel", "0", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()
        let title = app.descendants(matching: .any)["page-title"].firstMatch
        XCTAssertTrue(title.waitForExistence(timeout: 30), app.debugDescription)
        XCTAssertTrue(until(10) { focusedLabel(app) == "Watch" }, "Home did not open on Watch: \(focused(app))")
        pause(2)
        shot("home-0")
        XCUIRemote.shared.press(.down)
        pause(1)
        shot("home-1-down")
    }

    /// A server that stops answering: the note sits along the bottom, off the guide's chips,
    /// and Down reaches Try again. Nothing is changed on any server.
    func testOfflineNoteOffersAWayOut() {
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveServerURL", "http://127.0.0.1:9", "-lastChannel", "0", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()
        let note = app.staticTexts["offline-banner"]
        XCTAssertTrue(note.waitForExistence(timeout: 30), app.debugDescription)
        XCTAssertGreaterThan(note.frame.minY, 800, "the note is not along the bottom: \(note.frame)")
        shot("offline-0")
        let retry = app.buttons["offline-retry"]
        XCTAssertTrue(retry.exists)
        XCTAssertTrue(app.buttons["offline-choose"].exists)
        for _ in 0 ..< 12 where !retry.hasFocus && !app.buttons["offline-choose"].hasFocus {
            XCUIRemote.shared.press(.down)
            pause(0.4)
        }
        XCTAssertTrue(retry.hasFocus || app.buttons["offline-choose"].hasFocus, "Down did not reach the note's buttons: \(focused(app))")
        shot("offline-1-buttons")
    }

    /// The saved-servers list as the argument domain takes it: JSON data in plist form.
    private func savedServers(_ count: Int) -> String {
        let servers = (1 ... count).map { ["id": "test-\($0)", "name": "Test server \($0)", "url": "http://192.0.2.\($0):8477"] }
        let data = (try? JSONSerialization.data(withJSONObject: servers)) ?? Data()
        return "<" + data.map { String(format: "%02x", $0) }.joined() + ">"
    }

    /// Walks the remote to Join by where focus is, not by a fixed count: saved and found
    /// servers come and go above the buttons while the page looks, so the column is never
    /// the same height twice. Right leaves the brand column; Down goes past the server
    /// cards to the row of buttons; Right along it reaches Join.
    private func walkToJoin(_ app: XCUIApplication, _ open: XCUIElement) -> Bool {
        var stuck = 0
        var last = ""
        for _ in 0 ..< 30 {
            if open.hasFocus {
                return true
            }
            let id = focusedID(app)
            let direction: XCUIRemote.Button = switch id {
            case "connect-server": .down
            case "connect-address": .right
            case "": .down
            default: .right
            }
            stuck = id == last ? stuck + 1 : 0
            last = id
            // A press that moved nothing: try the way the walk has not tried yet.
            XCUIRemote.shared.press(stuck > 1 ? (direction == .down ? .right : .down) : direction)
            pause(0.45)
        }
        return open.hasFocus
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
