import XCTest

/// The display ask for a 59.94 source and a 29.97 source, and Picture in Picture
/// from the transport bar. Opt-in: TEST_RUNNER_BROADWAVE_SERVER and SHOTS.
@MainActor
final class DisplayMatchTests: XCTestCase {
    private var server: String {
        ProcessInfo.processInfo.environment["BROADWAVE_SERVER"] ?? ""
    }

    private var shots: String {
        ProcessInfo.processInfo.environment["BROADWAVE_SHOTS"] ?? ""
    }

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    func testEachSourceAsksForItsBroadcastRate() throws {
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        let channels = try channelIDs()
        let fast = try XCTUnwrap(channels["4.1"], "no 4.1")
        let slow = try XCTUnwrap(channels["5.1"], "no 5.1")
        let app = launch(watching: fast)
        let probe = app.staticTexts["displayProbe"]
        XCTAssertTrue(probe.waitForExistence(timeout: 20), app.debugDescription)
        XCTAssertTrue(until(45) { probe.label.contains("59.94") }, "4.1 asked \(probe.label)")
        shot(app, "rate-5994")

        app.terminate()
        let next = launch(watching: slow)
        let again = next.staticTexts["displayProbe"]
        XCTAssertTrue(again.waitForExistence(timeout: 20), next.debugDescription)
        XCTAssertTrue(until(45) { again.label.contains("29.97") }, "5.1 asked \(again.label)")
        shot(next, "rate-2997")
    }

    func testPictureInPictureFromTheTransportBar() throws {
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        let channels = try channelIDs()
        let fast = try XCTUnwrap(channels["4.1"], "no 4.1")
        let app = launch(watching: fast)
        let now = app.staticTexts["channel-now"]
        XCTAssertTrue(now.waitForExistence(timeout: 20), app.debugDescription)
        let channel = now.label
        XCTAssertFalse(channel.isEmpty, "no channel")
        XCTAssertTrue(until(40) { app.staticTexts["displayProbe"].label.contains("59.94") }, "picture was not ready")

        if app.staticTexts["transportProbe"].label != "shown" {
            XCUIRemote.shared.press(.select)
        }
        XCTAssertTrue(until(8) { app.staticTexts["transportProbe"].label == "shown" }, "transport stayed hidden")
        let pip = pictureButton(app)
        shot(app, "transport")
        // The Apple TV simulator reports Picture in Picture unsupported, so
        // AVKit leaves the button off the transport bar. A TV that supports
        // it must show the button.
        let supported = app.staticTexts["pipProbe"].label == "yes"
        guard supported else {
            XCTAssertFalse(pip.exists, "Picture in Picture showed on a TV that does not support it")
            return
        }
        guard pip.exists else {
            XCTFail("Picture in Picture is not in the transport bar\n\(buttonLabels(app))\n\(app.debugDescription)")
            return
        }
        focus(app, pip)
        XCUIRemote.shared.press(.select)
        shot(app, "pip")
        let started = Date()
        let deadline = started.addingTimeInterval(120)
        while Date() < deadline {
            XCTAssertFalse(app.staticTexts["playback-outage"].exists, "the picture stopped during Picture in Picture")
            XCTAssertEqual(now.label, channel, "left \(channel)")
            Thread.sleep(forTimeInterval: 2)
        }
        shot(app, "pip-held")
        XCUIRemote.shared.press(.menu)
        XCTAssertTrue(until(8) { now.label == channel }, "Menu did not return to \(channel), now \(now.label)")
        shot(app, "pip-back")
    }

    private func launch(watching channel: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveWatch", channel,
            "-BroadwaveChannelNow", "YES",
            "-BroadwaveSyncLog", "1",
        ]
        app.launch()
        return app
    }

    private func channelIDs() throws -> [String: String] {
        let url = try XCTUnwrap(URL(string: server + "/api/v1/channels"))
        let data = try Data(contentsOf: url)
        let page = try JSONDecoder().decode(ChannelPage.self, from: data)
        var out: [String: String] = [:]
        for channel in page.channels {
            out[channel.displayNumber] = String(channel.id)
        }
        return out
    }

    private func pictureButton(_ app: XCUIApplication) -> XCUIElement {
        let named = app.buttons["Picture in Picture"]
        if named.exists {
            return named
        }
        return app.descendants(matching: .any).matching(
            NSPredicate(format: "label CONTAINS[c] 'picture in picture'")
        ).firstMatch
    }

    private func buttonLabels(_ app: XCUIApplication) -> String {
        app.buttons.allElementsBoundByIndex.compactMap { button -> String? in
            let label = button.label
            return label.isEmpty ? nil : label
        }.joined(separator: " | ")
    }

    private func focus(_ app: XCUIApplication, _ target: XCUIElement) {
        for _ in 0 ..< 12 {
            let focused = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
            if focused.exists, focused.identifier == target.identifier || focused.label == target.label {
                return
            }
            XCUIRemote.shared.press(.down)
            RunLoop.current.run(until: Date().addingTimeInterval(0.3))
        }
    }

    private func shot(_ app: XCUIApplication, _ name: String) {
        guard !shots.isEmpty else { return }
        let folder = URL(fileURLWithPath: shots, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let url = folder.appending(path: "\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
        _ = app
    }

    private func until(_ seconds: TimeInterval, _ check: () -> Bool) -> Bool {
        let deadline = Date().addingTimeInterval(seconds)
        while Date() < deadline {
            if check() {
                return true
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        }
        return check()
    }
}

private struct ChannelPage: Decodable {
    struct Item: Decodable {
        let id: Int64
        let displayNumber: String
    }

    let channels: [Item]
}
