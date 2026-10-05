#if os(iOS)
    import XCTest

    /// A compact iPhone has five tabs, so More never stacks a second navigation bar.
    /// Settings is the gear on Home. Upcoming, a recording, and Diagnostics each
    /// keep one bar. Opt-in: TEST_RUNNER_BROADWAVE_SERVER, with a finished recording
    /// titled "Evening News".
    @MainActor
    final class PhoneTabsTests: XCTestCase {
        func testFiveTabsAndOneBackButton() throws {
            try requirePhone()
            let server = try serverURL()
            try finishSetup(server)
            let app = launch(server, tab: nil)
            let gear = app.buttons["home-settings"]
            XCTAssertTrue(gear.waitForExistence(timeout: 25), app.debugDescription)
            XCTAssertEqual(gear.label, "Settings")
            let tabs = ["Home", "Guide", "Search", "Sports", "Recordings"]
            for name in tabs {
                XCTAssertTrue(tabButton(app, name).exists, "missing \(name)\n\(app.debugDescription)")
            }
            XCTAssertFalse(app.tabBars.buttons["More"].exists, app.debugDescription)
            XCTAssertFalse(app.tabBars.buttons["Settings"].exists, app.debugDescription)
            shot(app, "iphone-tabs")

            gear.tap()
            let diagnostics = app.buttons["Diagnostics"]
            XCTAssertTrue(find(diagnostics, in: app), "no Diagnostics\n\(app.debugDescription)")
            diagnostics.tap()
            XCTAssertTrue(app.navigationBars["Diagnostics"].waitForExistence(timeout: 10), app.debugDescription)
            assertOneBar(app, "diagnostics")
            assertOneBackButton(app)
            shot(app, "iphone-diagnostics")
        }

        /// Upcoming lists will-record and a conflict, with both actions, under one navigation bar.
        func testUpcomingShowsTheConflict() throws {
            try requirePhone()
            let server = try serverURL()
            try finishSetup(server)
            let app = launch(server, tab: "recordings")
            let coming = app.buttons["Upcoming"]
            XCTAssertTrue(coming.waitForExistence(timeout: 20), app.debugDescription)
            coming.tap()
            XCTAssertTrue(app.navigationBars["Upcoming"].waitForExistence(timeout: 10), app.debugDescription)
            assertOneBar(app, "upcoming")
            XCTAssertTrue(app.buttons["Record the later airing"].waitForExistence(timeout: 10), app.debugDescription)
            XCTAssertTrue(app.buttons["Watch anyway"].waitForExistence(timeout: 5), app.debugDescription)
            XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "Will record")).firstMatch.waitForExistence(timeout: 5) || app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", "Will record")).firstMatch.exists, app.debugDescription)
            XCTAssertTrue(app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", "Lower priority")).firstMatch.exists, app.debugDescription)
            shot(app, "iphone-upcoming-before")
        }

        /// After Record the later airing, the early showing reads Skipped once and the actions are gone.
        func testUpcomingAfterTheFix() throws {
            try requirePhone()
            let server = try serverURL()
            try finishSetup(server)
            let app = launch(server, tab: "recordings")
            let coming = app.buttons["Upcoming"]
            XCTAssertTrue(coming.waitForExistence(timeout: 20), app.debugDescription)
            coming.tap()
            XCTAssertTrue(app.navigationBars["Upcoming"].waitForExistence(timeout: 10), app.debugDescription)
            assertOneBar(app, "upcoming-after")
            let skipped = app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", "Skipped once")).firstMatch
            XCTAssertTrue(skipped.waitForExistence(timeout: 10), app.debugDescription)
            XCTAssertFalse(app.buttons["Record the later airing"].exists)
            XCTAssertFalse(app.buttons["Watch anyway"].exists)
            shot(app, "iphone-upcoming-after")
        }

        func testRecordingsHaveOneNavigationBar() throws {
            try requirePhone()
            let server = try serverURL()
            try finishSetup(server)
            let app = launch(server, tab: "recordings")
            let coming = app.buttons["Upcoming"]
            XCTAssertTrue(coming.waitForExistence(timeout: 20), app.debugDescription)
            coming.tap()
            XCTAssertTrue(app.navigationBars["Upcoming"].waitForExistence(timeout: 10), app.debugDescription)
            assertOneBar(app, "upcoming")
            shot(app, "iphone-upcoming")

            app.navigationBars.buttons.element(boundBy: 0).tap()
            let row = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Evening News")).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 15), app.debugDescription)
            row.tap()
            XCTAssertTrue(until(10) { app.navigationBars.count >= 1 })
            assertOneBar(app, "recording")
            shot(app, "iphone-recording")
        }

        /// -BroadwaveTab settings used to select the Settings tab. On iPhone it pushes Settings.
        func testSettingsLinkPushesOnTheOpenTab() throws {
            try requirePhone()
            let server = try serverURL()
            try finishSetup(server)
            let app = launch(server, tab: "settings")
            XCTAssertTrue(app.descendants(matching: .any)["home-scan"].waitForExistence(timeout: 25), app.debugDescription)
            XCTAssertTrue(app.navigationBars["Settings"].waitForExistence(timeout: 10), app.debugDescription)
            assertOneBar(app, "settings-link")
            XCTAssertFalse(tabButton(app, "More").exists)
            shot(app, "iphone-settings")
        }

        func testIPadKeepsSixSections() throws {
            if UIDevice.current.userInterfaceIdiom != .pad {
                throw XCTSkip("iPad")
            }
            let server = try serverURL()
            try finishSetup(server)
            let app = launch(server, tab: nil)
            XCTAssertTrue(app.navigationBars["Home"].waitForExistence(timeout: 25), app.debugDescription)
            XCTAssertFalse(app.buttons["home-settings"].exists)
            let names = ["Home", "Guide", "Search", "Sports", "Recordings", "Settings"]
            // The floating bar fits five names and keeps Settings under Next Page.
            // The sidebar lists all six.
            let sidebar = app.buttons["Toggle sidebar"]
            XCTAssertTrue(sidebar.waitForExistence(timeout: 8), app.debugDescription)
            sidebar.tap()
            for name in names {
                XCTAssertTrue(section(app, name), "missing \(name)\n\(app.debugDescription)")
            }
            shot(app, "ipad-sections")
        }

        /// The guide's filter chips draw with the mini player up, in portrait and landscape,
        /// and landscape has room for a channel row. The chips stayed in the accessibility
        /// tree, hittable, while the screen showed black.
        func testGuideFiltersDrawWithTheMiniPlayer() throws {
            try requirePhone()
            let server = try serverURL()
            try finishSetup(server)
            let app = XCUIApplication()
            app.launchArguments = ["-BroadwaveServerURL", server, "-ApplePersistenceIgnoreState", "YES", "-BroadwaveWatch", "1"]
            app.launch()
            let minimize = app.buttons["Minimize"].firstMatch
            XCTAssertTrue(minimize.waitForExistence(timeout: 30), app.debugDescription)
            minimize.tap()
            let guide = tabButton(app, "Guide")
            XCTAssertTrue(guide.waitForExistence(timeout: 10), app.debugDescription)
            guide.tap()
            let all = app.buttons["guide-filter-all"]
            let favorites = app.buttons["guide-filter-favorites"]
            XCTAssertTrue(all.waitForExistence(timeout: 10), app.debugDescription)
            RunLoop.current.run(until: Date().addingTimeInterval(1.5))
            shot(app, "guide-mini-portrait")
            XCTAssertGreaterThan(drawn(all), 0.05, "All is not drawn in portrait")
            XCTAssertGreaterThan(drawn(favorites), 0.02, "Favorites is not drawn in portrait")
            XCUIDevice.shared.orientation = .landscapeLeft
            RunLoop.current.run(until: Date().addingTimeInterval(2))
            shot(app, "guide-mini-landscape")
            let filtersDrawn = drawn(all)
            // The first channel row fits above the mini player.
            let row = app.buttons.matching(NSPredicate(format: "label BEGINSWITH '4.1 '")).firstMatch
            let mini = app.buttons["miniPlayer"]
            let rowFits = row.exists && mini.exists && row.frame.minY + 44 <= mini.frame.minY && drawn(row) > 0.005
            print("landscape row \(row.exists ? "\(row.frame)" : "-") mini \(mini.exists ? "\(mini.frame)" : "-")")
            XCUIDevice.shared.orientation = .portrait
            XCTAssertGreaterThan(filtersDrawn, 0.05, "All is not drawn in landscape")
            XCTAssertTrue(rowFits, "no channel row above the mini player in landscape")
        }

        /// Share of the element's pixels that are not near black.
        private func drawn(_ element: XCUIElement) -> Double {
            guard element.exists, let image = element.screenshot().image.cgImage else { return 0 }
            let width = image.width
            let height = image.height
            var pixels = [UInt8](repeating: 0, count: width * height * 4)
            let drawnPixels = pixels.withUnsafeMutableBytes { buffer -> Int in
                guard let context = CGContext(
                    data: buffer.baseAddress, width: width, height: height, bitsPerComponent: 8, bytesPerRow: width * 4,
                    space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
                ) else { return 0 }
                context.draw(image, in: CGRect(x: 0, y: 0, width: width, height: height))
                var count = 0
                for index in stride(from: 0, to: buffer.count, by: 4) where max(buffer[index], buffer[index + 1], buffer[index + 2]) > 90 {
                    count += 1
                }
                return count
            }
            let share = Double(drawnPixels) / Double(max(width * height, 1))
            print("drawn \(element.identifier) \(String(format: "%.3f", share))")
            return share
        }

        private func requirePhone() throws {
            if UIDevice.current.userInterfaceIdiom != .phone {
                throw XCTSkip("iPhone")
            }
        }

        private func serverURL() throws -> String {
            guard let server = ProcessInfo.processInfo.environment["BROADWAVE_SERVER"], !server.isEmpty else {
                throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER")
            }
            return server
        }

        private func finishSetup(_ server: String) throws {
            var request = URLRequest(url: URL(string: server + "/api/v1/settings")!)
            request.httpMethod = "PUT"
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = Data(#"{"setupComplete":"1"}"#.utf8)
            let done = expectation(description: "setup")
            URLSession.shared.dataTask(with: request) { _, _, _ in done.fulfill() }.resume()
            wait(for: [done], timeout: 10)
        }

        private func launch(_ server: String, tab: String?) -> XCUIApplication {
            let app = XCUIApplication()
            var args = ["-BroadwaveServerURL", server, "-ApplePersistenceIgnoreState", "YES"]
            if let tab {
                args += ["-BroadwaveTab", tab]
            }
            app.launchArguments = args
            app.launch()
            return app
        }

        private func tabButton(_ app: XCUIApplication, _ name: String) -> XCUIElement {
            let bar = app.tabBars.buttons[name]
            if bar.exists {
                return bar
            }
            return app.buttons[name]
        }

        private func section(_ app: XCUIApplication, _ name: String) -> Bool {
            app.buttons[name].exists || app.tabBars.buttons[name].exists || app.staticTexts[name].exists
        }

        private func find(_ element: XCUIElement, in app: XCUIApplication) -> Bool {
            if element.waitForExistence(timeout: 3) {
                return true
            }
            for _ in 0 ..< 6 {
                app.swipeUp()
                if element.waitForExistence(timeout: 1) {
                    return true
                }
            }
            return false
        }

        private func assertOneBar(_ app: XCUIApplication, _ name: String) {
            let bars = app.navigationBars.allElementsBoundByIndex.filter(\.exists)
            let lines = bars.map { bar in
                let buttons = (0 ..< bar.buttons.count).map { bar.buttons.element(boundBy: $0).label }
                return "\(bar.label) [\(buttons.joined(separator: " | "))]"
            }
            print("l48-bars \(name) count=\(bars.count) \(lines.joined(separator: " ;; "))")
            XCTAssertEqual(bars.count, 1, lines.joined(separator: "\n"))
            XCTAssertFalse(lines.contains { $0.hasPrefix("More") }, lines.joined(separator: "\n"))
        }

        /// More used to keep its own bar above Settings, so Diagnostics showed two Back buttons.
        private func assertOneBackButton(_ app: XCUIApplication) {
            let labels = navigationButtons(app)
            print("l48-back \(labels.joined(separator: " | "))")
            XCTAssertEqual(labels.count, 1, "expected one back button, got \(labels.joined(separator: ", "))")
        }

        private func navigationButtons(_ app: XCUIApplication) -> [String] {
            app.navigationBars.allElementsBoundByIndex.filter(\.exists).flatMap { bar in
                (0 ..< bar.buttons.count).compactMap { index -> String? in
                    let button = bar.buttons.element(boundBy: index)
                    return button.exists ? button.label : nil
                }
            }
        }

        private func shot(_: XCUIApplication, _ name: String) {
            guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
            let folder = URL(fileURLWithPath: dir, isDirectory: true)
            try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            let url = folder.appending(path: "\(name).png")
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
            print("l48-shot \(url.path)")
        }

        private func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
            let end = Date().addingTimeInterval(seconds)
            while Date() < end {
                if done() {
                    return true
                }
                RunLoop.current.run(until: Date().addingTimeInterval(0.3))
            }
            return done()
        }
    }
#endif

#if os(tvOS)
    import XCTest

    /// Apple TV keeps all six sections in the sidebar.
    @MainActor
    final class TVTabsTests: XCTestCase {
        func testSidebarListsSixSections() {
            let app = XCUIApplication()
            app.launchArguments = ["-ApplePersistenceIgnoreState", "YES", "-BroadwaveFocus", "guide"]
            app.launch()
            // Guide takes focus and closes the sidebar. Menu opens it again.
            XCTAssertTrue(app.descendants(matching: .any)["guide-now"].waitForExistence(timeout: 20), app.debugDescription)
            XCUIRemote.shared.press(.menu)
            let names = ["Home", "Guide", "Search", "Sports", "Recordings", "Settings"]
            let ready = Date().addingTimeInterval(8)
            while Date() < ready, !names.allSatisfy({ app.buttons[$0].exists }) {
                RunLoop.current.run(until: Date().addingTimeInterval(0.3))
            }
            for name in names {
                XCTAssertTrue(app.buttons[name].exists, "missing \(name)\n\(app.debugDescription)")
            }
            // Guide is focused, so Home sits just above the open sidebar.
            XCUIRemote.shared.press(.up)
            RunLoop.current.run(until: Date().addingTimeInterval(0.8))
            if let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty {
                let folder = URL(fileURLWithPath: dir, isDirectory: true)
                try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
                let url = folder.appending(path: "tv-sections.png")
                try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
                print("l42-shot \(url.path)")
            }
        }
    }
#endif
