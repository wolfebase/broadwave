#if os(iOS)
    import XCTest

    /// The iPhone and iPad pages a viewer browses: the guide's times, rows that play,
    /// pages that scroll under the tab bar, search while typing, games-only Sports, and
    /// Settings split by this device and the server. Opt-in: TEST_RUNNER_BROADWAVE_SERVER
    /// (a server with listings) and TEST_RUNNER_BROADWAVE_SHOT_DIR for screenshots.
    /// `-BroadwaveChrome YES` keeps a tap that plays from taking a tuner. Records nothing.
    @MainActor
    final class PhonePagesTests: XCTestCase {
        override func setUp() {
            continueAfterFailure = false
            XCUIDevice.shared.orientation = .portrait
        }

        // MARK: Guide

        /// Tonight and Tomorrow change what the portrait list shows; Back to now returns.
        func testGuideTimesChangeTheList() throws {
            try requirePhone()
            let app = try launch(["-BroadwaveTab", "guide"])
            let now = app.buttons["guide-now"]
            XCTAssertTrue(now.waitForExistence(timeout: 30), app.debugDescription)
            XCTAssertTrue(app.buttons["guide-row-info"].firstMatch.waitForExistence(timeout: 20), app.debugDescription)
            pause(2)
            shot("guide-now")
            let before = firstRowLabel(app)
            let tonight = app.buttons["guide-tonight"]
            if tonight.exists {
                tonight.tap()
                let time = app.staticTexts["guide-list-time"]
                XCTAssertTrue(time.waitForExistence(timeout: 5), app.debugDescription)
                XCTAssertTrue(time.label.contains("8:00"), time.label)
                XCTAssertTrue(tonight.isSelected, "Tonight is not marked")
                pause(1)
                shot("guide-tonight")
                XCTAssertNotEqual(firstRowLabel(app), before, "Tonight shows the same programs as now")
            }
            let tomorrow = app.buttons["Tomorrow"]
            if tomorrow.exists {
                tomorrow.tap()
                XCTAssertTrue(app.staticTexts["guide-list-time"].waitForExistence(timeout: 5), app.debugDescription)
                pause(1)
                shot("guide-tomorrow")
            }
            let back = app.buttons["guide-back-to-now"]
            if back.exists {
                back.tap()
                XCTAssertTrue(until(5) { !app.staticTexts["guide-list-time"].exists }, app.debugDescription)
                XCTAssertTrue(now.isSelected, "Now is not marked")
            }
        }

        /// A tap on a row plays the channel. The info button opens details with Watch first.
        func testGuideRowPlaysAndInfoOpensDetails() throws {
            try requirePhone()
            let app = try launch(["-BroadwaveTab", "guide", "-BroadwaveChrome", "YES"])
            let info = app.buttons["guide-row-info"].firstMatch
            XCTAssertTrue(info.waitForExistence(timeout: 30), app.debugDescription)
            pause(1)
            info.tap()
            let watch = app.buttons["Watch"].firstMatch
            XCTAssertTrue(watch.waitForExistence(timeout: 8), app.debugDescription)
            XCTAssertFalse(app.staticTexts["From the tuner guide."].exists, "the sheet names the guide source")
            pause(2)
            shot("guide-details")
            app.swipeDown(velocity: .fast)
            XCTAssertTrue(until(5) { !watch.exists || !watch.isHittable }, app.debugDescription)

            let row = app.cells.firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 5), app.debugDescription)
            row.coordinate(withNormalizedOffset: CGVector(dx: 0.3, dy: 0.5)).tap()
            XCTAssertTrue(app.buttons["Minimize"].waitForExistence(timeout: 10), "a row tap did not play\n\(app.debugDescription)")
            shot("guide-row-tap-plays")
        }

        /// The list runs on under the tab bar instead of stopping 88 points above it.
        func testGuideScrollsUnderTheTabBar() throws {
            try requirePhone()
            let app = try launch(["-BroadwaveTab", "guide"])
            XCTAssertTrue(app.buttons["guide-row-info"].firstMatch.waitForExistence(timeout: 30), app.debugDescription)
            pause(1)
            let window = app.windows.firstMatch.frame
            app.swipeUp(velocity: .slow)
            pause(1)
            shot("guide-scrolled")
            let lowest = app.cells.allElementsBoundByIndex.filter(\.exists).map(\.frame.maxY).max() ?? 0
            XCTAssertGreaterThan(lowest, window.maxY - 60, "rows stop short of the bottom edge")
        }

        /// Landscape: one row of chips, no navigation bar, the same five tabs, and room for rows.
        func testGuideLandscapeFitsRows() throws {
            try requirePhone()
            let app = try launch(["-BroadwaveTab", "guide"])
            XCTAssertTrue(app.buttons["guide-now"].waitForExistence(timeout: 30), app.debugDescription)
            XCUIDevice.shared.orientation = .landscapeLeft
            pause(3)
            shot("guide-landscape")
            XCTAssertFalse(app.tabBars.buttons["More"].exists, "More is in the tab bar")
            XCTAssertFalse(app.navigationBars["Guide"].exists && app.navigationBars["Guide"].frame.height > 30, "the navigation bar takes room")
            let rail = app.buttons.matching(NSPredicate(format: "label MATCHES %@", "^[0-9]+\\.[0-9]+ .*")).allElementsBoundByIndex
            let window = app.windows.firstMatch.frame
            let shown = rail.filter { $0.exists && $0.frame.minY >= 0 && $0.frame.maxY <= window.maxY - 50 }
            print("landscape rows \(shown.count)")
            XCTAssertGreaterThanOrEqual(shown.count, 3, "fewer than three channel rows show in landscape")
            XCUIDevice.shared.orientation = .portrait
        }

        // MARK: Helpers

        private func firstRowLabel(_ app: XCUIApplication) -> String {
            let row = app.cells.firstMatch
            guard row.exists else { return "" }
            return row.buttons.firstMatch.label
        }

        func launch(_ extra: [String]) throws -> XCUIApplication {
            guard let server = ProcessInfo.processInfo.environment["BROADWAVE_SERVER"], !server.isEmpty else {
                throw XCTSkip("Set TEST_RUNNER_BROADWAVE_SERVER")
            }
            let app = XCUIApplication()
            app.launchArguments = ["-BroadwaveServerURL", server, "-ApplePersistenceIgnoreState", "YES"] + extra
            app.launch()
            return app
        }

        func requirePhone() throws {
            if UIDevice.current.userInterfaceIdiom != .phone {
                throw XCTSkip("iPhone")
            }
        }

        func requirePad() throws {
            if UIDevice.current.userInterfaceIdiom != .pad {
                throw XCTSkip("iPad")
            }
        }

        func pause(_ seconds: TimeInterval) {
            RunLoop.current.run(until: Date().addingTimeInterval(seconds))
        }

        func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
            let end = Date().addingTimeInterval(seconds)
            while Date() < end {
                if done() {
                    return true
                }
                pause(0.3)
            }
            return done()
        }

        func shot(_ name: String) {
            guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
            let folder = URL(fileURLWithPath: dir, isDirectory: true)
            try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            let device = UIDevice.current.userInterfaceIdiom == .pad ? "pad" : "phone"
            let url = folder.appending(path: "\(device)-\(name).png")
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
            print("pages-shot \(url.path)")
        }
    }
#endif
