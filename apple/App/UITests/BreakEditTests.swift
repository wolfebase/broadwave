#if os(iOS)
    import XCTest

    /// Marks a break by hand in the recording player and removes it. Opt-in: run with
    /// TEST_RUNNER_BROADWAVE_SERVER=http://host:port on a server with a finished
    /// recording titled "Evening News" at least 20 s long. Its markers are cleared first.
    final class BreakEditTests: XCTestCase {
        func testMarkThenRemoveABreak() throws {
            let server = try XCTUnwrap(ProcessInfo.processInfo.environment["BROADWAVE_SERVER"].flatMap { $0.isEmpty ? nil : $0 }, "set TEST_RUNNER_BROADWAVE_SERVER")
            let recordings = try get(server, "/api/v1/recordings")["recordings"] as? [[String: Any]] ?? []
            let id = try XCTUnwrap(recordings.first { $0["title"] as? String == "Evening News" }?["id"] as? Int)
            for marker in try markers(server, id) {
                try call(server, "DELETE", "/api/v1/markers/\(marker["id"] as? Int ?? 0)")
            }

            let app = XCUIApplication()
            app.launchArguments = ["-BroadwaveServerURL", server, "-BroadwaveTab", "recordings", "-breakSkip", "manual", "-ApplePersistenceIgnoreState", "YES"]
            app.launch()
            let row = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Evening News")).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 20), app.debugDescription)
            row.tap()

            let breaks = app.buttons["recording-menu"]
            XCTAssertTrue(breaks.waitForExistence(timeout: 15), app.debugDescription)
            wait(3)
            print("ios-player \(app.debugDescription)")
            if let shot = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_PLAYER"] {
                try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: shot))
            }
            breaks.tap()
            app.buttons["Break starts here"].tap()
            wait(4)
            breaks.tap()
            XCTAssertTrue(app.buttons["Break ends here"].waitForExistence(timeout: 5), app.debugDescription)
            app.buttons["Break ends here"].tap()
            XCTAssertTrue(until(10) { (try? self.markers(server, id).count) == 1 }, "no marker saved")
            let saved = try XCTUnwrap(try markers(server, id).first)
            let length = (saved["end"] as? Double ?? 0) - (saved["start"] as? Double ?? 0)
            XCTAssertTrue(length > 2 && length < 20, "marked \(length) s")
            // A menu tapped while the last one is still closing does not open.
            wait(1.5)
            breaks.tap()
            let remove = app.buttons["Remove a break"]
            XCTAssertTrue(remove.waitForExistence(timeout: 5), app.debugDescription)
            remove.tap()
            let span = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Remove the break from")).firstMatch
            XCTAssertTrue(span.waitForExistence(timeout: 5), app.debugDescription)
            if let shot = ProcessInfo.processInfo.environment["BROADWAVE_SHOT"] {
                try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: shot))
            }
            span.tap()
            XCTAssertTrue(until(10) { (try? self.markers(server, id).isEmpty) == true }, "marker not removed")
        }

        private func markers(_ server: String, _ id: Int) throws -> [[String: Any]] {
            try get(server, "/api/v1/recordings/\(id)/markers")["markers"] as? [[String: Any]] ?? []
        }

        private func get(_ server: String, _ path: String) throws -> [String: Any] {
            let data = try Data(contentsOf: URL(string: server + path)!)
            return try JSONSerialization.jsonObject(with: data) as? [String: Any] ?? [:]
        }

        private func call(_ server: String, _ method: String, _ path: String) throws {
            var req = URLRequest(url: URL(string: server + path)!)
            req.httpMethod = method
            let done = expectation(description: path)
            URLSession.shared.dataTask(with: req) { _, _, _ in done.fulfill() }.resume()
            wait(for: [done], timeout: 10)
        }

        private func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
            let end = Date().addingTimeInterval(seconds)
            while Date() < end {
                if done() {
                    return true
                }
                wait(0.5)
            }
            return done()
        }

        private func wait(_ seconds: TimeInterval) {
            RunLoop.current.run(until: Date().addingTimeInterval(seconds))
        }
    }
#endif
