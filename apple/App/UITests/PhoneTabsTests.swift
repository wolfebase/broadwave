#if os(iOS)
    import XCTest

    /// A compact iPhone has five tabs, so More never stacks a second navigation bar.
    /// Settings is the gear on Home. Coming up, a recording, and Diagnostics each
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
            shot(app, "iphone-diagnostics")
        }

        func testRecordingsHaveOneNavigationBar() throws {
            try requirePhone()
            let server = try serverURL()
            try finishSetup(server)
            let app = launch(server, tab: "recordings")
            let coming = app.buttons["Coming up"]
            XCTAssertTrue(coming.waitForExistence(timeout: 20), app.debugDescription)
            coming.tap()
            XCTAssertTrue(app.navigationBars["Coming up"].waitForExistence(timeout: 10), app.debugDescription)
            assertOneBar(app, "coming-up")
            shot(app, "iphone-coming-up")

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
            print("l42-bars \(name) count=\(bars.count) \(lines.joined(separator: " ;; "))")
            XCTAssertEqual(bars.count, 1, lines.joined(separator: "\n"))
            XCTAssertFalse(lines.contains { $0.hasPrefix("More") }, lines.joined(separator: "\n"))
        }

        private func shot(_: XCUIApplication, _ name: String) {
            guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
            let folder = URL(fileURLWithPath: dir, isDirectory: true)
            try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            let url = folder.appending(path: "\(name).png")
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
            print("l42-shot \(url.path)")
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
