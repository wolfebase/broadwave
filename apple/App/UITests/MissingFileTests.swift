import XCTest

/// A recording whose file is gone stays off Home, says so on the row, and can only be removed.
/// Opt-in: TEST_RUNNER_BROADWAVE_SERVER and _SHOT_DIR. The harness inserts "Moved Away"
/// (a path that is not a file) and points 4.1's art at http://127.0.0.1:18792/logo.jpg.
@MainActor
final class MissingFileTests: XCTestCase {
    private let title = "Moved Away"
    private let spoken = "Moved Away, 4.1, The file is gone. It was moved or deleted outside Broadwave."
    private let visible = "4.1 · The file is gone. It was moved or deleted outside Broadwave."

    func testAMissingFileLeavesHomeAndCanOnlyBeRemoved() throws {
        executionTimeAllowance = 360
        let server = lane("BROADWAVE_SERVER")
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        try put(server, #"{"setupComplete":"1"}"#)

        let channel = try XCTUnwrap(try channels(server).first { $0.number == "4.1" }, "no 4.1")
        let id = try XCTUnwrap(try planted(server), "plant Moved Away before the test")
        defer { self.delete(server, id) }

        let listed = try recording(server, id)
        XCTAssertEqual(listed?.missing, true)
        XCTAssertEqual(listed?.duration, 0)

        let home = XCUIApplication()
        home.launchArguments = ["-ApplePersistenceIgnoreState", "YES", "-BroadwaveServerURL", server, "-BroadwaveTab", "home"]
        home.launch()
        let settled = until(25) { home.staticTexts[channel.name].exists || home.staticTexts["No channels yet"].exists }
        XCTAssertTrue(settled, home.debugDescription)
        // Give a late recordings refresh a chance to put the row on Home if the filter is wrong.
        RunLoop.current.run(until: Date().addingTimeInterval(2))
        XCTAssertFalse(home.staticTexts[title].exists, "Home listed \(title)\n\(home.debugDescription)")
        XCTAssertFalse(home.staticTexts["Recently recorded"].exists && home.staticTexts[title].exists)
        shot(home, "home")
        home.terminate()

        let app = XCUIApplication()
        app.launchArguments = ["-ApplePersistenceIgnoreState", "YES", "-BroadwaveServerURL", server, "-BroadwaveTab", "recordings"]
        app.launch()
        let row = app.descendants(matching: .any).matching(NSPredicate(format: "identifier == 'recording-row' AND label CONTAINS %@", title)).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 25), app.debugDescription)
        XCTAssertEqual(row.label, spoken, "VoiceOver label")
        XCTAssertTrue(app.staticTexts[visible].exists || row.label.contains(visible.replacingOccurrences(of: " · ", with: ", ")))
        let hits = until(8) { (try? self.artHits()) ?? 0 >= 1 }
        XCTAssertTrue(hits, "the row did not ask for the channel picture")
        let posters = lane("BROADWAVE_POSTERS")
        if !posters.isEmpty {
            XCTAssertFalse(FileManager.default.fileExists(atPath: posters), "the row asked for a poster of the missing file")
        }
        shot(app, "row")

        XCTAssertTrue(openMenu(app, on: row), "no context menu\n\(app.debugDescription)")
        let forbidden = ["Mark watched", "Mark unwatched", "Make a channel", "Find commercials", "Play", "Stop recording"]
        let shown = forbidden.filter { app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", $0)).firstMatch.exists }
        XCTAssertTrue(menuHasDelete(app), "menu has no Delete\n\(app.debugDescription)")
        XCTAssertTrue(shown.isEmpty, "menu also offered \(shown)")
        shot(app, "menu")
        dismissMenu(app)

        openRow(app, row)
        let remove = app.buttons["Remove from the list"]
        XCTAssertTrue(remove.waitForExistence(timeout: 6), "no remove\n\(app.debugDescription)")
        shot(app, "remove")
        choose(remove, in: app)
        let gone = until(12) { !row.exists }
        XCTAssertTrue(gone, "row stayed\n\(app.debugDescription)")
        let after = try recording(server, id)
        XCTAssertNil(after, "still in /recordings")
        shot(app, "gone")
    }

    private func openMenu(_ app: XCUIApplication, on row: XCUIElement) -> Bool {
        #if os(tvOS)
            focus(app, title)
            for _ in 0 ..< 2 {
                XCUIRemote.shared.press(.select, forDuration: 1.5)
                if menuHasDelete(app) {
                    return true
                }
            }
            return false
        #else
            if !row.isHittable {
                row.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).press(forDuration: 1.2)
            } else {
                row.press(forDuration: 1.2)
            }
            return menuHasDelete(app)
        #endif
    }

    private func menuHasDelete(_ app: XCUIApplication) -> Bool {
        app.descendants(matching: .any).matching(NSPredicate(format: "label == 'Delete'")).firstMatch.waitForExistence(timeout: 2)
    }

    private func dismissMenu(_ app: XCUIApplication) {
        #if os(tvOS)
            XCUIRemote.shared.press(.menu)
        #else
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.08)).tap()
        #endif
        _ = until(3) { !self.menuHasDelete(app) }
    }

    private func openRow(_ app: XCUIApplication, _ row: XCUIElement) {
        #if os(tvOS)
            focus(app, title)
            XCUIRemote.shared.press(.select)
        #else
            if row.isHittable {
                row.tap()
            } else {
                row.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
        #endif
    }

    private func choose(_ button: XCUIElement, in app: XCUIApplication) {
        #if os(tvOS)
            // The sheet draws the action twice in the accessibility tree, so focus is read from the
            // focused element instead of the button query.
            let remote = XCUIRemote.shared
            // The two actions stack. The sheet opens on Cancel, under Remove.
            for _ in 0 ..< 4 where !focused(app).contains("Remove from the list") {
                remote.press(.up)
                RunLoop.current.run(until: Date().addingTimeInterval(0.3))
            }
            XCTAssertTrue(focused(app).contains("Remove from the list"), "focus never reached Remove, at \(focused(app))")
            remote.press(.select)
            _ = button
        #else
            button.tap()
            _ = app
        #endif
    }

    private func focus(_ app: XCUIApplication, _ name: String) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            for _ in 0 ..< 12 where !focused(app).contains(name) {
                remote.press(.down)
                RunLoop.current.run(until: Date().addingTimeInterval(0.35))
            }
        #else
            _ = app
            _ = name
        #endif
    }

    private func focused(_ app: XCUIApplication) -> String {
        let element = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        return element.exists ? element.label : ""
    }

    private struct Channel {
        var id: Int
        var number: String
        var name: String
    }

    private func channels(_ server: String) throws -> [Channel] {
        let result = try exchange(URLRequest(url: URL(string: server + "/api/v1/channels")!))
        XCTAssertEqual(result.status, 200, result.text)
        let root = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any]
        let rows = root?["channels"] as? [[String: Any]] ?? []
        return rows.compactMap { row in
            guard let id = (row["id"] as? NSNumber)?.intValue, let number = row["guideNumber"] as? String else { return nil }
            return Channel(id: id, number: number, name: row["displayName"] as? String ?? number)
        }
    }

    private func recording(_ server: String, _ id: Int) throws -> (missing: Bool, duration: Double)? {
        let result = try exchange(URLRequest(url: URL(string: server + "/api/v1/recordings")!))
        guard result.status == 200,
              let root = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any],
              let rows = root["recordings"] as? [[String: Any]],
              let row = rows.first(where: { ($0["id"] as? NSNumber)?.intValue == id })
        else { return nil }
        return (row["missing"] as? Bool ?? false, (row["durationSec"] as? NSNumber)?.doubleValue ?? 0)
    }

    /// The row the harness planted. Its file is not on disk.
    private func planted(_ server: String) throws -> Int? {
        let result = try exchange(URLRequest(url: URL(string: server + "/api/v1/recordings")!))
        guard result.status == 200,
              let root = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any],
              let rows = root["recordings"] as? [[String: Any]]
        else { return nil }
        return rows.compactMap { row -> Int? in
            guard row["title"] as? String == title, row["missing"] as? Bool == true else { return nil }
            return (row["id"] as? NSNumber)?.intValue
        }.last
    }

    private func artHits() throws -> Int {
        let result = try exchange(URLRequest(url: URL(string: "http://127.0.0.1:18792/hits")!))
        return Int(result.text.trimmingCharacters(in: .whitespacesAndNewlines)) ?? 0
    }

    private func delete(_ server: String, _ id: Int) {
        var request = URLRequest(url: URL(string: server + "/api/v1/recordings/\(id)")!)
        request.httpMethod = "DELETE"
        _ = try? exchange(request)
    }

    private func put(_ server: String, _ body: String) throws {
        var request = URLRequest(url: URL(string: server + "/api/v1/settings")!)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = Data(body.utf8)
        let result = try exchange(request)
        XCTAssertTrue((200 ..< 300).contains(result.status), result.text)
    }

    private func exchange(_ request: URLRequest) throws -> (status: Int, text: String) {
        var output: (Int, String)?
        var failed: Error?
        let done = expectation(description: request.url?.absoluteString ?? "request")
        URLSession.shared.dataTask(with: request) { data, response, error in
            if let error {
                failed = error
            } else {
                output = ((response as? HTTPURLResponse)?.statusCode ?? 0, String(data: data ?? Data(), encoding: .utf8) ?? "")
            }
            done.fulfill()
        }.resume()
        wait(for: [done], timeout: 20)
        if let failed {
            throw failed
        }
        return try XCTUnwrap(output)
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

    private func shot(_ app: XCUIApplication, _ name: String) {
        let dir = lane("BROADWAVE_SHOT_DIR")
        guard !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let url = folder.appending(path: "\(device)-\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
        _ = app
    }

    private var device: String {
        #if os(tvOS)
            "tv"
        #else
            "iphone"
        #endif
    }

    private func lane(_ name: String) -> String {
        let env = ProcessInfo.processInfo.environment
        if let direct = env[name], !direct.isEmpty {
            return direct
        }
        return env["TEST_RUNNER_\(name)"] ?? ""
    }
}
