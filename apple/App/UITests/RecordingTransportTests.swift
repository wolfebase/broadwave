#if os(iOS)
    import XCTest

    /// The recording player shows the system transport on a tap, so a recording can be
    /// paused and sought; the live player keeps its own chrome. Opt-in: run with
    /// TEST_RUNNER_BROADWAVE_SERVER=http://host:port on a server with a finished recording
    /// (the local harness with BROADWAVE_E2E=1).
    @MainActor
    final class RecordingTransportTests: XCTestCase {
        private var server: String {
            get throws {
                try XCTUnwrap(ProcessInfo.processInfo.environment["BROADWAVE_SERVER"].flatMap { $0.isEmpty ? nil : $0 }, "set TEST_RUNNER_BROADWAVE_SERVER")
            }
        }

        func testATapShowsTheTransport() throws {
            let app = XCUIApplication()
            app.launchArguments = try ["-BroadwaveServerURL", server, "-BroadwaveTab", "recordings", "-ApplePersistenceIgnoreState", "YES"]
            app.launch()
            let row = app.buttons["recording-row"].firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 20), app.debugDescription)
            row.tap()
            XCTAssertTrue(app.buttons["recording-start-over"].waitForExistence(timeout: 15), app.debugDescription)
            sleep(3)

            app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            let play = app.buttons["Play/Pause"]
            XCTAssertTrue(play.waitForExistence(timeout: 5), "no transport after a tap: \(app.debugDescription)")
            XCTAssertEqual(play.label, "Pause")
            play.tap()
            XCTAssertTrue(until(5) { play.label == "Play" }, "did not pause: \(play.label)")

            let position = app.sliders.matching(NSPredicate(format: "identifier == %@", "Current position")).firstMatch
            XCTAssertTrue(position.exists, app.debugDescription)
            let elapsed = app.staticTexts["Elapsed Time"]
            let before = elapsed.label
            position.adjust(toNormalizedSliderPosition: 0.8)
            XCTAssertTrue(until(5) { elapsed.exists && elapsed.label != before }, "did not seek: \(before) → \(elapsed.label)")
        }

        /// AVKit's transport stays out of the live player, whose own chrome has the controls.
        func testTheLivePlayerKeepsItsOwnChrome() throws {
            let app = XCUIApplication()
            app.launchArguments = try ["-BroadwaveServerURL", server, "-BroadwaveWatch", "1", "-ApplePersistenceIgnoreState", "YES"]
            app.launch()
            XCTAssertTrue(app.otherElements["player-gesture"].waitForExistence(timeout: 20), app.debugDescription)
            sleep(3)
            for point in [CGVector(dx: 0.5, dy: 0.5), CGVector(dx: 0.02, dy: 0.5), CGVector(dx: 0.95, dy: 0.6)] {
                app.coordinate(withNormalizedOffset: point).tap()
                sleep(1)
                XCTAssertFalse(app.buttons["Play/Pause"].exists || app.sliders.matching(NSPredicate(format: "identifier == %@", "Current position")).firstMatch.exists, "AVKit transport after a tap at \(point): \(app.debugDescription)")
            }
        }

        private func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
            let end = Date().addingTimeInterval(seconds)
            while Date() < end {
                if done() {
                    return true
                }
                RunLoop.current.run(until: Date().addingTimeInterval(0.25))
            }
            return done()
        }
    }
#endif
