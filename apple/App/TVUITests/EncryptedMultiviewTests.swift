import XCTest

/// A multiview link that names a hidden half, or an encrypted 3.0 station, plays
/// the channel on the guide. Opt-in: TEST_RUNNER_BROADWAVE_SERVER on the FLEX-4K harness.
final class EncryptedMultiviewTests: XCTestCase {
    private let note = "The 3.0 version is encrypted. Showing the regular broadcast."

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    func testALinkPlaysTheHalfOnTheGuide() throws {
        let env = ProcessInfo.processInfo.environment
        guard let server = env["BROADWAVE_SERVER"], !server.isEmpty else {
            throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER to run against the fake lineup")
        }
        let hidden = try channelID(server, number: "4.1")
        let wide = try channelID(server, number: "104.1")
        let encrypted = try channelID(server, number: "115.1")
        let clear = try channelID(server, number: "5.1")
        let before = try choice(server, id: wide)
        setChoice(server, id: wide, choice: "atsc3")
        defer { setChoice(server, id: wide, choice: before) }

        let app = launch(server, tab: "guide")
        let shown = labeled(app, "104.1")
        XCTAssertTrue(shown.waitForExistence(timeout: 30), app.debugDescription)
        XCTAssertFalse(app.buttons["tile-\(hidden)"].exists)

        let link = try XCTUnwrap(URL(string: "broadwave://multiview?ch=\(hidden),\(encrypted)&layout=2up"))
        app.open(link)

        let wideTiles = tiles(app, wide)
        let clearTiles = tiles(app, clear)
        XCTAssertTrue(wideTiles.firstMatch.waitForExistence(timeout: 30), app.debugDescription)
        XCTAssertTrue(clearTiles.firstMatch.waitForExistence(timeout: 15), app.debugDescription)
        XCTAssertEqual(tiles(app, hidden).count, 0, app.debugDescription)
        XCTAssertEqual(tiles(app, encrypted).count, 0, app.debugDescription)
        let clearText = joined(clearTiles)
        let wideText = joined(wideTiles)
        XCTAssertTrue(clearText.contains("5.1"), clearText)
        XCTAssertFalse(clearText.contains("115.1"), clearText)
        XCTAssertTrue(wideText.contains("104.1"), wideText)
        // The tile is one accessibility element, so the sentence is its value.
        XCTAssertTrue(clearText.contains(note), clearText)
        XCTAssertFalse(wideText.contains(note), wideText)
        shot(app, "tv-multiview")
    }

    private func tiles(_ app: XCUIApplication, _ id: Int64) -> XCUIElementQuery {
        app.descendants(matching: .any).matching(NSPredicate(format: "identifier == %@", "tile-\(id)"))
    }

    /// Label and value of every element that carries the tile's identifier.
    private func joined(_ query: XCUIElementQuery) -> String {
        query.allElementsBoundByIndex.map { element in
            let value = element.value as? String ?? ""
            return "\(element.label) \(value)"
        }.joined(separator: " | ")
    }

    private func launch(_ server: String, tab: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveServerURL", server, "-BroadwaveTab", tab, "-ApplePersistenceIgnoreState", "YES"]
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

    private func choice(_ server: String, id: Int64) throws -> String {
        let url = try XCTUnwrap(URL(string: server + "/api/v1/channels"))
        let data = try Data(contentsOf: url)
        let body = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        let channels = body?["channels"] as? [[String: Any]] ?? []
        let row = channels.first { ($0["id"] as? NSNumber)?.int64Value == id }
        return row?["twinChoice"] as? String ?? "atsc3"
    }

    private func setChoice(_ server: String, id: Int64, choice: String) {
        guard let url = URL(string: server + "/api/v1/channels/\(id)") else { return }
        var request = URLRequest(url: url)
        request.httpMethod = "PATCH"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = Data("{\"twinChoice\":\"\(choice)\"}".utf8)
        let done = expectation(description: "choice")
        URLSession.shared.dataTask(with: request) { _, _, _ in done.fulfill() }.resume()
        wait(for: [done], timeout: 5)
    }
}
