import XCTest

/// Down from Now must reach the first channel, including after a relaunch.
/// The day row used to sit in the middle of the screen, beside the channel column.
/// Opt-in: TEST_RUNNER_BROADWAVE_SERVER.
@MainActor
final class GuideFocusTests: XCTestCase {
    private var server: String {
        let env = ProcessInfo.processInfo.environment
        if let direct = env["BROADWAVE_SERVER"], !direct.isEmpty {
            return direct
        }
        return env["TEST_RUNNER_BROADWAVE_SERVER"] ?? ""
    }

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    func testDownFromNowReachesTheFirstChannel() throws {
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        try finishSetup()
        let label = try firstChannelLabel()
        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveTab", "guide",
        ]
        app.launch()

        let now = app.buttons["guide-now"]
        XCTAssertTrue(now.waitForExistence(timeout: 25), app.debugDescription)
        XCTAssertTrue(until(10) { now.hasFocus }, "Now did not take focus: \(focused(app))\n\(app.debugDescription)")

        let channel = app.buttons[label].firstMatch
        XCTAssertTrue(channel.waitForExistence(timeout: 15), "no \(label)\n\(app.debugDescription)")
        var trail: [String] = []
        for _ in 0 ..< 3 {
            if channel.hasFocus {
                break
            }
            XCUIRemote.shared.press(.down)
            pause(0.7)
            trail.append(focused(app))
        }
        XCTAssertTrue(channel.hasFocus, "Down from Now did not reach \(label): \(trail)")
        shot(app, "guide-channel")

        XCUIRemote.shared.press(.up)
        pause(0.7)
        let tonight = app.buttons["Tonight"].firstMatch
        // The channel row is wider than Now, so Up can land on Tonight. Left brings it back.
        if !now.hasFocus {
            XCTAssertTrue(tonight.hasFocus, "Up from the channel left the day row: \(focused(app))")
            XCUIRemote.shared.press(.left)
            pause(0.7)
        }
        XCTAssertTrue(now.hasFocus, "the day row did not return to Now: \(focused(app))")
        XCUIRemote.shared.press(.right)
        pause(0.7)
        XCTAssertTrue(app.buttons["Tonight"].firstMatch.hasFocus, "Right from Now did not reach Tonight: \(focused(app))")
        XCUIRemote.shared.press(.left)
        pause(0.7)
        XCTAssertTrue(now.hasFocus, "Left from Tonight did not return to Now: \(focused(app))")

        XCUIRemote.shared.press(.down)
        pause(0.7)
        XCTAssertTrue(channel.hasFocus, "Down from Now did not return to \(label): \(focused(app))")
        XCUIRemote.shared.press(.down)
        pause(0.7)
        XCTAssertFalse(channel.hasFocus, "Down stayed on \(label): \(focused(app))")
        shot(app, "guide-next")
    }

    /// The same move after the viewer opens Guide from the sidebar. Launching straight onto
    /// the tab is not enough: the collapsed sidebar still sits in the focus search.
    func testDownFromNowAfterChoosingGuide() throws {
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        try finishSetup()
        let label = try firstChannelLabel()
        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
        ]
        app.launch()

        let guide = app.buttons["Guide"].firstMatch
        XCTAssertTrue(guide.waitForExistence(timeout: 20), app.debugDescription)
        if !guide.isEnabled {
            XCUIRemote.shared.press(.menu)
            XCTAssertTrue(until(5) { guide.isEnabled }, "the sidebar did not open: \(focused(app))")
        }
        var trail: [String] = []
        let moves: [XCUIRemote.Button] = Array(repeating: .down, count: 6) + Array(repeating: .up, count: 7)
        for move in moves {
            if guide.hasFocus {
                break
            }
            XCUIRemote.shared.press(move)
            pause(0.4)
            trail.append(focused(app))
        }
        XCTAssertTrue(guide.hasFocus, "no focus on Guide: \(trail)")
        XCUIRemote.shared.press(.select)
        pause(1)

        let now = app.buttons["guide-now"]
        XCTAssertTrue(now.waitForExistence(timeout: 15), app.debugDescription)
        XCTAssertTrue(until(8) { now.hasFocus }, "Now did not take focus: \(focused(app))")
        let channel = app.buttons[label].firstMatch
        XCTAssertTrue(channel.waitForExistence(timeout: 15), "no \(label)")
        trail = []
        for _ in 0 ..< 3 {
            if channel.hasFocus {
                break
            }
            XCUIRemote.shared.press(.down)
            pause(0.7)
            trail.append(focused(app))
        }
        XCTAssertTrue(channel.hasFocus, "Down from Now did not reach \(label) after opening Guide: \(trail)")
        shot(app, "guide-from-home")
    }

    private func finishSetup() throws {
        var request = try URLRequest(url: XCTUnwrap(URL(string: server + "/api/v1/settings")))
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "content-type")
        request.httpBody = Data(#"{"setupComplete":"1"}"#.utf8)
        let done = expectation(description: "setup")
        URLSession.shared.dataTask(with: request) { _, _, _ in done.fulfill() }.resume()
        wait(for: [done], timeout: 10)
    }

    private func firstChannelLabel() throws -> String {
        let url = try XCTUnwrap(URL(string: server + "/api/v1/channels"))
        let data = try Data(contentsOf: url)
        let page = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        let channels = page?["channels"] as? [[String: Any]] ?? []
        let first = try XCTUnwrap(channels.first, "no channels")
        let number = first["displayNumber"] as? String ?? ""
        let name = first["displayName"] as? String ?? ""
        let label = "\(number) \(name)"
        XCTAssertFalse(number.isEmpty || name.isEmpty, "channel had no name: \(first)")
        return label
    }

    private func focused(_ app: XCUIApplication) -> String {
        let element = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        guard element.exists else { return "nothing" }
        return "\(element.identifier) \(element.label)"
    }

    private func shot(_ app: XCUIApplication, _ name: String) {
        guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOTS"]
            ?? ProcessInfo.processInfo.environment["TEST_RUNNER_BROADWAVE_SHOTS"], !dir.isEmpty
        else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let url = folder.appending(path: "\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
        _ = app
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
