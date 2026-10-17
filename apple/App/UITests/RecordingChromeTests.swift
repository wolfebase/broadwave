#if os(iOS)
    import XCTest

    /// A recording plays over the whole screen with one way out: no Back stacked on a
    /// Back on an iPhone on its side, no second X on iPad. Opt-in: TEST_RUNNER_BROADWAVE_SERVER
    /// on a server with a finished recording (the local harness after a short recording)
    /// and TEST_RUNNER_BROADWAVE_SHOT_DIR for screenshots.
    @MainActor
    final class RecordingChromeTests: XCTestCase {
        func testARecordingHasOneWayOut() throws {
            let server = ProcessInfo.processInfo.environment["BROADWAVE_SERVER"] ?? ""
            try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
            let pad = UIDevice.current.userInterfaceIdiom == .pad
            XCUIDevice.shared.orientation = pad ? .landscapeLeft : .portrait
            let app = XCUIApplication()
            app.launchArguments = ["-BroadwaveServerURL", server, "-BroadwaveTab", "recordings", "-ApplePersistenceIgnoreState", "YES"]
            app.launch()
            let row = app.buttons["recording-row"].firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 30), app.debugDescription)
            row.tap()
            XCTAssertTrue(app.buttons["recording-start-over"].waitForExistence(timeout: 20), app.debugDescription)
            XCUIDevice.shared.orientation = .landscapeLeft
            RunLoop.current.run(until: Date().addingTimeInterval(3))
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            RunLoop.current.run(until: Date().addingTimeInterval(1))
            shot(pad ? "ipad-recording" : "iphone-recording-landscape")

            let ways = app.buttons.allElementsBoundByIndex.filter { button in
                guard button.exists, button.isHittable else { return false }
                let name = button.label
                return button.identifier == "BackButton" || name == "Close" || name == "Back" || name == "Done" || name == "Recordings"
            }
            XCTAssertEqual(ways.count, 1, "ways out: \(ways.map { "\($0.identifier) \($0.label)" })")
            ways.first?.tap()
            // On its side, the lazy list has no rows above the fold.
            XCUIDevice.shared.orientation = pad ? .landscapeLeft : .portrait
            let back = app.buttons["recording-row"].firstMatch.waitForExistence(timeout: 10)
            shot(pad ? "ipad-recording-closed" : "iphone-recording-closed")
            XCTAssertTrue(back, "the way out did not go back to the list\n\(app.debugDescription)")
            // The list knows the player closed: the next recording opens.
            app.buttons["recording-row"].firstMatch.tap()
            XCTAssertTrue(app.buttons["recording-start-over"].waitForExistence(timeout: 20), "a second recording did not open")
            XCUIDevice.shared.orientation = .portrait
        }

        private func shot(_ name: String) {
            guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
            let folder = URL(fileURLWithPath: dir, isDirectory: true)
            try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: folder.appending(path: "\(name).png"))
        }
    }
#endif
