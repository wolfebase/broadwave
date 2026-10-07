#if os(iOS)
    import XCTest

    /// The recordings library on iPhone: Continue watching, a show's page by season,
    /// and marking several at once. Opt-in: TEST_RUNNER_BROADWAVE_SERVER on a server
    /// seeded with five "Mystery Hour" episodes over two seasons (S1 E1 watched, S1 E2
    /// half watched), "A Western" half watched, and one game.
    /// The select test changes them, so seed again before the next run.
    @MainActor
    final class LibraryTests: XCTestCase {
        func testContinueWatchingAndAShowPageBySeason() throws {
            try requirePhone()
            let server = try serverURL()
            let app = launch(server)
            XCTAssertTrue(app.staticTexts["Continue watching"].waitForExistence(timeout: 20), app.debugDescription)
            shot("iphone-library")
            let line = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "5 recordings · 4 unwatched")).firstMatch
            XCTAssertTrue(find(line, in: app), app.debugDescription)
            let all = app.buttons["show-page"]
            XCTAssertTrue(find(all, in: app), app.debugDescription)
            XCTAssertEqual(all.label, "All 5 of Mystery Hour")
            all.tap()
            XCTAssertTrue(app.navigationBars["Mystery Hour"].waitForExistence(timeout: 10), app.debugDescription)
            XCTAssertTrue(app.staticTexts["SEASON 1"].exists || app.staticTexts["Season 1"].exists, app.debugDescription)
            let rows = app.buttons.matching(identifier: "recording-row")
            XCTAssertTrue(rows.firstMatch.waitForExistence(timeout: 5))
            XCTAssertTrue(rows.firstMatch.label.contains("The Locked Room"), rows.firstMatch.label)
            XCTAssertTrue(rows.firstMatch.label.contains("S1 E1"), rows.firstMatch.label)
            shot("iphone-show-page")
        }

        func testSelectSeveralAndMarkThemWatched() throws {
            try requirePhone()
            let server = try serverURL()
            let app = launch(server)
            let select = app.buttons["library-select"]
            XCTAssertTrue(find(select, in: app), app.debugDescription)
            select.tap()
            let picks = app.buttons.matching(identifier: "recording-pick")
            XCTAssertTrue(find(picks.firstMatch, in: app), app.debugDescription)
            let lighthouse = picks.matching(NSPredicate(format: "label CONTAINS %@", "The Lighthouse")).firstMatch
            let town = picks.matching(NSPredicate(format: "label CONTAINS %@", "A New Town")).firstMatch
            XCTAssertTrue(find(lighthouse, in: app), app.debugDescription)
            lighthouse.tap()
            XCTAssertTrue(find(town, in: app), app.debugDescription)
            town.tap()
            XCTAssertTrue(lighthouse.isSelected && town.isSelected, "\(lighthouse.isSelected) \(town.isSelected)")
            let count = app.staticTexts["selected-count"]
            XCTAssertTrue(find(count, in: app, up: true), app.debugDescription)
            XCTAssertEqual(count.label.uppercased(), "2 SELECTED")
            shot("iphone-select")
            app.buttons["Mark watched"].tap()
            XCTAssertTrue(app.staticTexts["Marked 2 recordings watched."].waitForExistence(timeout: 10), app.debugDescription)
            let marked = try recordings(server).filter { ["The Lighthouse", "A New Town"].contains($0["subtitle"] as? String) }
            XCTAssertEqual(marked.compactMap { $0["watched"] as? Int }, [1, 1], "\(marked)")
            XCTAssertFalse(app.buttons["recording-pick"].exists)
        }

        private func recordings(_ server: String) throws -> [[String: Any]] {
            let data = try Data(contentsOf: URL(string: server + "/api/v1/recordings")!)
            let body = try JSONSerialization.jsonObject(with: data) as? [String: Any]
            return body?["recordings"] as? [[String: Any]] ?? []
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

        private func launch(_ server: String) -> XCUIApplication {
            let app = XCUIApplication()
            app.launchArguments = ["-BroadwaveServerURL", server, "-BroadwaveTab", "recordings", "-ApplePersistenceIgnoreState", "YES"]
            app.launch()
            return app
        }

        private func find(_ element: XCUIElement, in app: XCUIApplication, up: Bool = false) -> Bool {
            if element.waitForExistence(timeout: 5), element.isHittable {
                return true
            }
            for _ in 0 ..< 8 {
                if up {
                    app.swipeDown()
                } else {
                    app.swipeUp()
                }
                if element.waitForExistence(timeout: 1), element.isHittable {
                    return true
                }
            }
            return element.exists
        }

        private func shot(_ name: String) {
            guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
            try? FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: dir).appending(path: "\(name).png"))
        }
    }
#endif
