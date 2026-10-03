import XCTest

/// An encrypted 3.0 station stays off the guide and Home. Search plays its clear broadcast.
/// Opt-in: TEST_RUNNER_BROADWAVE_SERVER=http://127.0.0.1:port on the FLEX-4K harness
/// with a "Sealed Signal" listing on 115.1.
final class EncryptedTwinTests: XCTestCase {
    private let note = "The 3.0 version is encrypted. Showing the regular broadcast."

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    func testHiddenEncryptedStationPlaysItsClearTwin() throws {
        let env = ProcessInfo.processInfo.environment
        guard let server = env["BROADWAVE_SERVER"], !server.isEmpty else {
            throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER to run against the fake lineup")
        }
        let guide = launch(server, tab: "guide")
        let clear = labeled(guide, "104.1")
        XCTAssertTrue(clear.waitForExistence(timeout: 30), guide.debugDescription)
        XCTAssertTrue(clear.label.contains("ATSC 3.0"), clear.label)
        XCTAssertFalse(labeled(guide, "115.1").exists, guide.debugDescription)
        shot(guide, "tv-guide")

        let home = launch(server, tab: "home")
        let card = labeled(home, "104.1")
        XCTAssertTrue(card.waitForExistence(timeout: 30), home.debugDescription)
        XCTAssertTrue(card.label.contains("ATSC 3.0"), card.label)
        XCTAssertFalse(labeled(home, "115.1").exists, home.debugDescription)
        shot(home, "tv-home")

        let app = launch(server, tab: "search", info: true, search: "Sealed Signal")
        let found = app.staticTexts["Sealed Signal"]
        XCTAssertTrue(found.waitForExistence(timeout: 20), app.debugDescription)
        XCTAssertTrue(app.staticTexts["ATSC 3.0"].waitForExistence(timeout: 5), app.debugDescription)
        shot(app, "tv-search")
        select(app, "search-watch")
        assertPlayingClear(app)
        shot(app, "tv-player")

        let id = try encryptedID(server)
        let link = try XCTUnwrap(URL(string: "broadwave://watch/\(id)"))
        app.open(link)
        assertPlayingClear(app)
    }

    /// The channel page's 3.0 / 1.0 / Both choice changes which half the guide lists.
    func testShowChoiceChangesTheGuide() throws {
        let env = ProcessInfo.processInfo.environment
        guard let server = env["BROADWAVE_SERVER"], !server.isEmpty else {
            throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER to run against the fake lineup")
        }
        let pair = try channelID(server, number: "104.1")
        defer { setChoice(server, id: pair, choice: "atsc3") }

        let found = launch(server, tab: "search", search: "Clear Picture")
        XCTAssertTrue(found.staticTexts["Clear Picture"].waitForExistence(timeout: 20), found.debugDescription)
        let hit = labeled(found, "104.1")
        XCTAssertTrue(hit.waitForExistence(timeout: 5), found.debugDescription)
        XCTAssertTrue(hit.label.contains("ATSC 3.0"), hit.label)
        shot(found, "tv-search-104")

        let edit = launch(server, tab: "settings", channels: true, openFirst: true)
        let picker = edit.descendants(matching: .any)["twin-choice"]
        XCTAssertTrue(picker.waitForExistence(timeout: 20), edit.debugDescription)
        shot(edit, "tv-channel")
        choose(edit, "1.0 only")

        XCTAssertTrue(choice(server, id: pair, equals: "atsc1"), "the Show choice did not save 1.0 only")
        let guide = launch(server, tab: "guide")
        let one = guide.buttons["4.1 KBWV"]
        XCTAssertTrue(one.waitForExistence(timeout: 30), guide.debugDescription)
        XCTAssertFalse(guide.buttons.matching(NSPredicate(format: "label CONTAINS %@", "104.1")).firstMatch.exists)
        XCTAssertFalse(labeled(guide, "115.1").exists, guide.debugDescription)
        shot(guide, "tv-guide-1")

        setChoice(server, id: pair, choice: "atsc3")
        let back = launch(server, tab: "guide")
        let three = labeled(back, "104.1")
        XCTAssertTrue(three.waitForExistence(timeout: 30), back.debugDescription)
        XCTAssertTrue(three.label.contains("ATSC 3.0"), three.label)
        XCTAssertFalse(begins(back, "4.1").exists, back.debugDescription)
    }

    /// The Show row takes focus, which closes the sidebar. Select opens the list.
    /// The open list starts on the current value, so one Down reaches the next.
    private func choose(_ app: XCUIApplication, _ value: String) {
        let remote = XCUIRemote.shared
        let deadline = Date().addingTimeInterval(8)
        while Date() < deadline, !focusedLabel(app).contains("Show") {
            remote.press(.right)
            RunLoop.current.run(until: Date().addingTimeInterval(0.25))
        }
        XCTAssertTrue(focusedLabel(app).contains("Show"), focusedLabel(app))
        XCTAssertFalse(app.buttons["Settings"].firstMatch.isEnabled, "sidebar still covers the channel page")
        remote.press(.select)
        XCTAssertTrue(app.buttons[value].waitForExistence(timeout: 5), app.debugDescription)
        shot(app, "tv-choice")
        remote.press(.down)
        RunLoop.current.run(until: Date().addingTimeInterval(0.4))
        XCTAssertTrue(focusedLabel(app).contains(value), focusedLabel(app))
        remote.press(.select)
        RunLoop.current.run(until: Date().addingTimeInterval(0.6))
    }

    private func focusedLabel(_ app: XCUIApplication) -> String {
        let el = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        guard el.exists else { return "" }
        let value = (el.value as? String) ?? ""
        return "\(el.identifier) \(el.label) \(value)"
    }

    private func assertPlayingClear(_ app: XCUIApplication) {
        let chrome = app.descendants(matching: .any)["player-chrome"]
        XCTAssertTrue(chrome.waitForExistence(timeout: 20), app.debugDescription)
        XCTAssertTrue(chrome.label.contains("5.1"), chrome.label)
        XCTAssertFalse(chrome.label.contains("115.1"), chrome.label)
        let line = app.descendants(matching: .any)["playback-note"]
        XCTAssertTrue(line.waitForExistence(timeout: 5), app.debugDescription)
        XCTAssertEqual(line.label, note)
    }

    /// Down lands on the row, then Left reaches Watch. Record sits to its right.
    private func select(_ app: XCUIApplication, _ identifier: String) {
        let remote = XCUIRemote.shared
        let button = app.buttons[identifier]
        XCTAssertTrue(button.waitForExistence(timeout: 10), app.debugDescription)
        for _ in 0 ..< 8 where !button.hasFocus {
            remote.press(.down)
            RunLoop.current.run(until: Date().addingTimeInterval(0.3))
        }
        for _ in 0 ..< 4 where !button.hasFocus {
            remote.press(.left)
            RunLoop.current.run(until: Date().addingTimeInterval(0.3))
        }
        XCTAssertTrue(button.hasFocus, app.debugDescription)
        remote.press(.select)
    }

    private func launch(
        _ server: String,
        tab: String,
        info: Bool = false,
        search: String? = nil,
        channels: Bool = false,
        openFirst: Bool = false
    ) -> XCUIApplication {
        let app = XCUIApplication()
        var args = ["-BroadwaveServerURL", server, "-BroadwaveTab", tab, "-ApplePersistenceIgnoreState", "YES"]
        if info {
            args += ["-BroadwaveInfo", "YES"]
        }
        if let search {
            args += ["-BroadwaveSearch", search]
        }
        if channels {
            args += ["-BroadwaveChannels", "YES"]
        }
        if openFirst {
            args += ["-BroadwaveOpenFirst", "YES"]
        }
        app.launchArguments = args
        app.launch()
        return app
    }

    private func shot(_: XCUIApplication, _ name: String) {
        guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let url = folder.appendingPathComponent("\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
    }

    private func labeled(_ app: XCUIApplication, _ number: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", number)).firstMatch
    }

    private func encryptedID(_ server: String) throws -> Int64 {
        try channelID(server, number: "115.1")
    }

    private func channelID(_ server: String, number: String) throws -> Int64 {
        let url = try XCTUnwrap(URL(string: server + "/api/v1/channels"))
        let data = try Data(contentsOf: url)
        let body = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        let channels = body?["channels"] as? [[String: Any]] ?? []
        let row = channels.first { $0["guideNumber"] as? String == number }
        let id = (row?["id"] as? NSNumber)?.int64Value ?? 0
        XCTAssertGreaterThan(id, 0, "\(number) missing from \(server)")
        return id
    }

    private func begins(_ app: XCUIApplication, _ number: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label BEGINSWITH %@", number + " ")).firstMatch
    }

    private func choice(_ server: String, id: Int64, equals want: String) -> Bool {
        let deadline = Date().addingTimeInterval(5)
        while Date() < deadline {
            if twinChoice(server, id: id) == want {
                return true
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        }
        return false
    }

    private func twinChoice(_ server: String, id: Int64) -> String? {
        guard let url = URL(string: server + "/api/v1/channels") else { return nil }
        guard let data = try? Data(contentsOf: url) else { return nil }
        guard let body = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return nil }
        let channels = body["channels"] as? [[String: Any]] ?? []
        let row = channels.first { ($0["id"] as? NSNumber)?.int64Value == id }
        return row?["twinChoice"] as? String
    }

    private func setChoice(_ server: String, id: Int64, choice: String) {
        guard let url = URL(string: server + "/api/v1/channels/\(id)") else { return }
        var req = URLRequest(url: url)
        req.httpMethod = "PATCH"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = Data(#"{"twinChoice":"\#(choice)"}"#.utf8)
        let done = expectation(description: "twin choice")
        URLSession.shared.dataTask(with: req) { _, _, _ in done.fulfill() }.resume()
        wait(for: [done], timeout: 10)
    }
}
