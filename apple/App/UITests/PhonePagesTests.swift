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

        // MARK: Search

        /// Results come in while typing, one card per show; a show's page asks before recording.
        func testSearchWhileTyping() throws {
            let app = try launch(["-BroadwaveTab", "search"])
            let field = app.searchFields.firstMatch
            XCTAssertTrue(field.waitForExistence(timeout: 30), app.debugDescription)
            pause(2)
            shot("search-start")
            field.tap()
            field.typeText("news")
            pause(0.4)
            shot("search-typing")
            let card = app.buttons.matching(identifier: "search-show-live").firstMatch
            let any = app.descendants(matching: .any).matching(NSPredicate(format: "identifier BEGINSWITH 'search-show'")).firstMatch
            XCTAssertTrue(any.waitForExistence(timeout: 10), "no results without pressing Search\n\(app.debugDescription)")
            pause(2)
            shot("search-results")
            let titles = app.descendants(matching: .any).matching(NSPredicate(format: "identifier BEGINSWITH 'search-show'")).allElementsBoundByIndex.map(\.label)
            print("search shows \(titles.count)")
            (card.exists ? card : any).firstMatch.tap()
            let series = app.buttons["show-record-series"]
            XCTAssertTrue(series.waitForExistence(timeout: 8), app.debugDescription)
            pause(1)
            shot("search-show")
            if series.isEnabled {
                series.tap()
                let confirm = app.buttons["show-record-series-confirm"]
                XCTAssertTrue(confirm.waitForExistence(timeout: 5), "Record series did not ask first")
                shot("search-confirm")
                // A popover on iPhone has no Cancel; a tap outside closes it without recording.
                if app.buttons["Cancel"].exists {
                    app.buttons["Cancel"].firstMatch.tap()
                } else {
                    app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.8)).tap()
                }
                XCTAssertTrue(until(5) { !confirm.exists }, app.debugDescription)
            }
        }

        // MARK: Sports and Home

        /// Games only: no fishing, hunting, or sitcom about football.
        func testSportsListsGamesOnly() throws {
            let app = try launch(["-BroadwaveTab", "sports"])
            XCTAssertTrue(app.descendants(matching: .any)["sports-page"].waitForExistence(timeout: 30), app.debugDescription)
            pause(4)
            shot("sports")
            let cards = app.buttons.matching(NSPredicate(format: "identifier IN %@", ["sports-live", "sports-later"])).allElementsBoundByIndex
            let labels = cards.map(\.label)
            print("sports cards \(labels.count): \(labels.prefix(6))")
            for word in ["Outdoors", "DeerGear", "Charlie Moore", "The Game,"] {
                XCTAssertFalse(labels.contains { $0.contains(word) }, "\(word) is on the Sports page")
            }
            app.swipeUp(velocity: .slow)
            pause(1)
            shot("sports-scrolled")
        }

        /// The hero is something worth watching, and its card does not take the 3.0 tag's identifier.
        func testHomeLeadsWithAShowWorthWatching() throws {
            let app = try launch(["-BroadwaveTab", "home"])
            let watch = app.buttons["home-watch"]
            XCTAssertTrue(watch.waitForExistence(timeout: 30), app.debugDescription)
            pause(3)
            shot("home")
            XCTAssertFalse(app.descendants(matching: .any)["atsc3-tag"].exists, "a card answers to atsc3-tag")
            app.swipeUp(velocity: .slow)
            pause(1)
            shot("home-scrolled")
            app.swipeUp(velocity: .slow)
            pause(1)
            shot("home-scrolled2")
        }

        // MARK: Settings

        /// iPhone: a list that separates this device from the server, then a page per topic.
        func testPhoneSettingsListThenPage() throws {
            try requirePhone()
            let app = try launch(["-BroadwaveTab", "settings"])
            let playback = app.buttons["settings-playback"]
            XCTAssertTrue(playback.waitForExistence(timeout: 30), app.debugDescription)
            XCTAssertTrue(app.staticTexts["THIS IPHONE"].exists || app.staticTexts["This iPhone"].exists, app.debugDescription)
            pause(1)
            shot("settings")
            app.swipeUp(velocity: .slow)
            pause(1)
            shot("settings-scrolled")
            let recording = app.descendants(matching: .any)["settings-recording"].firstMatch
            XCTAssertTrue(recording.waitForExistence(timeout: 5), app.debugDescription)
            recording.tap()
            let autoplay = app.switches.matching(NSPredicate(format: "label CONTAINS 'next episode'")).firstMatch
            XCTAssertTrue(autoplay.waitForExistence(timeout: 8), "Recording and Storage did not open\n\(app.navigationBars.allElementsBoundByIndex.map(\.identifier))")
            let keep = app.staticTexts["Keep this much free"]
            for _ in 0 ..< 4 where !keep.isHittable {
                app.swipeUp(velocity: .slow)
            }
            XCTAssertTrue(app.textFields["keep-free"].exists, app.debugDescription)
            XCTAssertTrue(app.buttons.matching(NSPredicate(format: "label CONTAINS 'When space runs low'")).firstMatch.isEnabled, "the server's settings are greyed out")
            shot("settings-recording")
        }

        /// iPad: the list on the left, the page on the right, both on screen.
        func testPadSettingsSplit() throws {
            try requirePad()
            let app = try launch(["-BroadwaveTab", "settings"])
            let playback = app.buttons["settings-playback"]
            XCTAssertTrue(playback.waitForExistence(timeout: 30), app.debugDescription)
            XCTAssertTrue(app.staticTexts["Quality"].waitForExistence(timeout: 8) || app.buttons["Quality"].exists, "no page beside the list")
            pause(2)
            shot("settings")
            app.descendants(matching: .any)["settings-server"].firstMatch.tap()
            pause(2)
            shot("settings-server")
        }

        // MARK: First run and Recordings

        /// The first screen says what Broadwave is and offers both ways in.
        func testFirstRunWelcome() {
            let app = XCUIApplication()
            app.launchArguments = ["-ApplePersistenceIgnoreState", "YES", "-BroadwaveExplain", "YES"]
            app.launch()
            let look = app.buttons["connect-look"]
            XCTAssertTrue(look.waitForExistence(timeout: 20), app.debugDescription)
            XCTAssertTrue(app.buttons["connect-demo"].exists, app.debugDescription)
            pause(1)
            shot("first-run")
        }

        /// A server that answered or was saved is the first, prominent choice, listed once, above the demo.
        func testFirstRunServerLeads() throws {
            _ = try launch([])
            let app = XCUIApplication()
            app.terminate()
            app.launchArguments = ["-ApplePersistenceIgnoreState", "YES", "-BroadwaveDiscover", "YES"]
            app.launch()
            let server = app.buttons["connect-server"].firstMatch
            XCTAssertTrue(server.waitForExistence(timeout: 20), app.debugDescription)
            pause(3)
            shot("first-run-finder")
            let demo = app.buttons["connect-demo"]
            XCTAssertTrue(demo.exists, app.debugDescription)
            XCTAssertLessThan(server.frame.minY, demo.frame.minY, "the demo is above the server")
            let names = app.buttons.matching(identifier: "connect-server").allElementsBoundByIndex.map(\.label)
            XCTAssertEqual(names.count, Set(names).count, "a server is listed twice: \(names)")
        }

        /// No recordings: the page says so and offers the guide.
        func testRecordingsEmptyOffersTheGuide() throws {
            let app = try launch(["-BroadwaveTab", "recordings"])
            let open = app.buttons["recordings-open-guide"]
            guard open.waitForExistence(timeout: 20) else {
                throw XCTSkip("this server has recordings")
            }
            shot("recordings-empty")
            open.tap()
            XCTAssertTrue(app.buttons["guide-now"].waitForExistence(timeout: 10), "Open the guide did not open it")
        }

        /// With recordings: filters in one menu, no header that repeats its only row, and plain counts.
        func testRecordingsPage() throws {
            let app = try launch(["-BroadwaveTab", "recordings"])
            let row = app.buttons["recording-row"].firstMatch
            guard row.waitForExistence(timeout: 20) else {
                throw XCTSkip("this server has no recordings")
            }
            XCTAssertTrue(app.buttons["library-filter"].exists, app.debugDescription)
            XCTAssertFalse(app.staticTexts["Plays 0 recordings in turn. Uses no tuner."].exists)
            XCTAssertTrue(row.label.contains(","), row.label)
            pause(2)
            shot("recordings")
            app.swipeUp(velocity: .slow)
            pause(1)
            shot("recordings-scrolled")
            app.buttons["library-filter"].tap()
            pause(1)
            shot("recordings-filter")
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
