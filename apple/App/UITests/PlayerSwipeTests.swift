import XCTest

#if os(iOS)
    /// A vertical swipe changes channel. A pinch fills the picture, and a pinch back fits it.
    /// Opt-in: TEST_RUNNER_BROADWAVE_SERVER and TEST_RUNNER_BROADWAVE_SHOTS.
    final class PlayerSwipeTests: XCTestCase {
        private var server: String {
            ProcessInfo.processInfo.environment["BROADWAVE_SERVER"] ?? ""
        }

        private var shots: String {
            ProcessInfo.processInfo.environment["BROADWAVE_SHOTS"] ?? ""
        }

        override func setUp() {
            super.setUp()
            continueAfterFailure = false
            XCUIDevice.shared.orientation = .portrait
        }

        func testSwipeChangesChannelAndPinchFills() throws {
            try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
            let app = try launch()
            let pad = app.descendants(matching: .any)["player-gesture"]
            XCTAssertTrue(pad.waitForExistence(timeout: 20), app.debugDescription)
            XCTAssertEqual(pad.label, "Picture")
            let now = app.staticTexts["channel-now"]
            XCTAssertTrue(until(15) { !now.label.isEmpty }, "no channel")
            let before = now.label
            shot(app, "playing")

            let start = pad.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.58))
            let end = pad.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.22))
            start.press(forDuration: 0.05, thenDragTo: end)
            let moved = until(8) { !now.label.isEmpty && now.label != before }
            shot(app, "swipe")
            XCTAssertTrue(moved, "stayed on \(now.label), hittable \(pad.isHittable)")

            pad.pinch(withScale: 1.8, velocity: 1.5)
            let filled = until(5) { (pad.value as? String) == "Fill" }
            shot(app, "fill")
            XCTAssertTrue(filled, "value \(String(describing: pad.value))")

            pad.pinch(withScale: 0.4, velocity: -1.5)
            let fitted = until(5) { (pad.value as? String) == "Fit" }
            shot(app, "fit")
            XCTAssertTrue(fitted, "value \(String(describing: pad.value))")
        }

        private func launch() throws -> XCUIApplication {
            let channel = try firstChannelID()
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

        private func firstChannelID() throws -> String {
            let url = try XCTUnwrap(URL(string: server + "/api/v1/channels"))
            let data = try Data(contentsOf: url)
            let page = try JSONDecoder().decode(ChannelPage.self, from: data)
            return try String(XCTUnwrap(page.channels.first?.id, "no channels"))
        }

        private func shot(_: XCUIApplication, _ name: String) {
            guard !shots.isEmpty else { return }
            let folder = URL(fileURLWithPath: shots, isDirectory: true)
            try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            let url = folder.appending(path: "iphone-\(name).png")
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
            print("l52-shot \(url.path)")
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
        }

        let channels: [Item]
    }
#endif
