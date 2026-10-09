import XCTest

/// The two clean-up pickers save, and Keep forever marks the recording.
/// Opt-in: BROADWAVE_SERVER. The shell plants one finished recording named
/// "The Keepsake" before the run; this process cannot open sqlite.
@MainActor
final class KeepSettingsTests: XCTestCase {
    private let roomHint = "Delete makes room before a new recording is skipped, oldest watched first. Unwatched and kept recordings stay."
    private let daysHint = "Counts from when a recording was played to its end or marked watched. Recordings you keep forever stay."
    private let subtitle = "The Keepsake"

    func testCleanUpSettingsAndKeepForever() throws {
        executionTimeAllowance = 360
        let server = lane("BROADWAVE_SERVER")
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")

        try put(server, #"{"setupComplete":"1","makeRoom":"0","deleteWatchedDays":"0"}"#)
        let id = try plantedID(server)
        try putKeep(server, id, false)
        defer {
            try? self.put(server, #"{"makeRoom":"0","deleteWatchedDays":"0"}"#)
            self.deleteRecording(server, id)
        }

        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveTab", "settings",
            "-BroadwaveSettingsAnchor", "storage",
        ]
        app.launch()

        let room = try find(app, id: "make-room", label: "When space runs low")
        XCTAssertTrue(until(20) { room.isEnabled }, "picker stayed disabled\n\(app.debugDescription)")
        reveal(room, in: app, marker: "When space runs low")
        XCTAssertTrue(app.staticTexts[roomHint].waitForExistence(timeout: 5), "missing room hint\n\(app.debugDescription)")
        XCTAssertTrue(app.staticTexts[daysHint].waitForExistence(timeout: 5), "missing days hint\n\(app.debugDescription)")
        shot(app, "settings")
        choose(app, room, "Delete the oldest watched", marker: "When space runs low")
        let days = try find(app, id: "delete-watched", label: "Delete watched recordings")
        reveal(days, in: app, marker: "Delete watched recordings")
        choose(app, days, "After 14 days", marker: "Delete watched recordings")
        let saved = try waitSettings(server, room: "1", days: "14")
        XCTAssertEqual(saved["makeRoom"], "1")
        XCTAssertEqual(saved["deleteWatchedDays"], "14")
        shot(app, "settings-saved")
        app.terminate()

        let library = XCUIApplication()
        library.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveTab", "recordings",
        ]
        library.launch()
        let row = row(library, subtitle)
        XCTAssertTrue(row.waitForExistence(timeout: 25), library.debugDescription)
        XCTAssertFalse(row.label.contains("Kept forever"), row.label)
        XCTAssertTrue(openMenu(library, on: row), "no Keep forever\n\(library.debugDescription)")
        shot(library, "menu")
        chooseKeep(library)
        XCTAssertTrue(until(12) { self.row(library, self.subtitle).label.contains("Kept forever") }, self.row(library, subtitle).label)
        let listed = try recording(server, id)
        XCTAssertEqual(listed, true, "keep flag")
        shot(library, "kept")
        print("l108 \(device) makeRoom=1 deleteWatchedDays=14 keep=\(listed)")
    }

    private func find(_ app: XCUIApplication, id: String, label: String) throws -> XCUIElement {
        let byID = app.descendants(matching: .any)[id]
        if byID.waitForExistence(timeout: 25) {
            return byID
        }
        let byLabel = app.descendants(matching: .any)[label]
        XCTAssertTrue(byLabel.waitForExistence(timeout: 5), app.debugDescription)
        return byLabel
    }

    private func focusedText(_ app: XCUIApplication) -> String {
        let focused = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        guard focused.exists else { return "" }
        let value = focused.value as? String ?? ""
        return "\(focused.identifier) \(focused.label) \(value)"
    }

    private func reveal(_ picker: XCUIElement, in app: XCUIApplication, marker: String) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            for step in 0 ..< 40 {
                let here = focusedText(app)
                print("l108 focus \(step) \(here)")
                if here.contains(marker) {
                    return
                }
                remote.press(.down)
            }
            _ = picker
        #else
            if picker.isHittable {
                return
            }
            for _ in 0 ..< 8 {
                app.swipeUp()
                if picker.isHittable {
                    return
                }
            }
        #endif
    }

    private func choose(_ app: XCUIApplication, _ picker: XCUIElement, _ label: String, marker: String) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            if !focusedText(app).contains(marker) {
                reveal(picker, in: app, marker: marker)
            }
            XCTAssertTrue(focusedText(app).contains(marker), "never focused \(marker), at \(focusedText(app))")
            remote.press(.select)
            for step in 0 ..< 12 {
                let here = focusedText(app)
                print("l108 option \(step) \(here)")
                if here.contains(label) {
                    remote.press(.select)
                    return
                }
                remote.press(.down)
            }
            XCTFail("could not focus \(label), at \(focusedText(app))")
        #else
            if picker.isHittable {
                picker.tap()
            } else {
                picker.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
            let option = app.buttons[label].firstMatch
            if option.waitForExistence(timeout: 4) {
                option.tap()
                return
            }
            let any = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", label)).firstMatch
            XCTAssertTrue(any.waitForExistence(timeout: 4), "no \(label)\n\(app.debugDescription)")
            any.tap()
        #endif
    }

    private func row(_ app: XCUIApplication, _ name: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "identifier == 'recording-row' AND label CONTAINS %@", name)).firstMatch
    }

    private func openMenu(_ app: XCUIApplication, on row: XCUIElement) -> Bool {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            for _ in 0 ..< 16 where !focusedText(app).contains(subtitle) {
                remote.press(.down)
                RunLoop.current.run(until: Date().addingTimeInterval(0.3))
            }
            for _ in 0 ..< 2 {
                remote.press(.select, forDuration: 1.5)
                if entry(app, "Keep forever").waitForExistence(timeout: 4) {
                    return true
                }
            }
            return false
        #else
            if !row.isHittable {
                for _ in 0 ..< 6 {
                    app.swipeUp()
                    if row.isHittable {
                        break
                    }
                }
            }
            if row.isHittable {
                row.press(forDuration: 1.2)
            } else {
                row.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).press(forDuration: 1.2)
            }
            return entry(app, "Keep forever").waitForExistence(timeout: 4)
        #endif
    }

    private func chooseKeep(_ app: XCUIApplication) {
        #if os(tvOS)
            // The menu opens on Mark watched. Keep forever is the next row, and those rows do not report focus.
            XCUIRemote.shared.press(.down)
            RunLoop.current.run(until: Date().addingTimeInterval(0.4))
            XCUIRemote.shared.press(.select)
        #else
            let item = entry(app, "Keep forever")
            if item.isHittable {
                item.tap()
            } else {
                item.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
        #endif
    }

    private func entry(_ app: XCUIApplication, _ label: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", label)).firstMatch
    }

    /// The id of the finished recording the shell planted before this run.
    private func plantedID(_ server: String) throws -> Int {
        let request = URLRequest(url: URL(string: server + "/api/v1/recordings")!)
        let result = try exchange(request)
        XCTAssertEqual(result.status, 200, result.text)
        let body = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any]
        let rows = body?["recordings"] as? [[String: Any]] ?? []
        let row = rows.first { ($0["subtitle"] as? String) == subtitle }
        let id = (row?["id"] as? NSNumber)?.intValue
        return try XCTUnwrap(id, "plant \(subtitle) on the harness before the test\n\(result.text)")
    }

    private func putKeep(_ server: String, _ id: Int, _ keep: Bool) throws {
        var request = URLRequest(url: URL(string: server + "/api/v1/recordings/\(id)/keep")!)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = Data("{\"keep\":\(keep)}".utf8)
        let result = try exchange(request)
        XCTAssertTrue((200 ..< 300).contains(result.status), "keep \(result.status) \(result.text)")
    }

    private func put(_ server: String, _ body: String) throws {
        var request = URLRequest(url: URL(string: server + "/api/v1/settings")!)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = Data(body.utf8)
        let result = try exchange(request)
        XCTAssertTrue((200 ..< 300).contains(result.status), "put \(result.status) \(result.text)")
    }

    private func deleteRecording(_ server: String, _ id: Int) {
        var request = URLRequest(url: URL(string: server + "/api/v1/recordings/\(id)")!)
        request.httpMethod = "DELETE"
        _ = try? exchange(request)
    }

    private func recording(_ server: String, _ id: Int) throws -> Bool {
        let request = URLRequest(url: URL(string: server + "/api/v1/recordings")!)
        let result = try exchange(request)
        XCTAssertEqual(result.status, 200, result.text)
        let body = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any]
        let rows = body?["recordings"] as? [[String: Any]] ?? []
        let row = rows.first { ($0["id"] as? Int) == id || ($0["id"] as? Int64) == Int64(id) }
        return (row?["keep"] as? Bool) == true
    }

    private func waitSettings(_ server: String, room: String, days: String) throws -> [String: String] {
        var last: [String: String] = [:]
        let found = until(12) {
            guard let values = try? self.settings(server) else { return false }
            last = values
            return values["makeRoom"] == room && values["deleteWatchedDays"] == days
        }
        XCTAssertTrue(found, "settings stayed makeRoom=\(last["makeRoom"] ?? "") days=\(last["deleteWatchedDays"] ?? "")")
        return last
    }

    private func settings(_ server: String) throws -> [String: String] {
        let request = URLRequest(url: URL(string: server + "/api/v1/settings")!)
        let result = try exchange(request)
        XCTAssertEqual(result.status, 200, result.text)
        let parsed = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: String]
        return try XCTUnwrap(parsed, result.text)
    }

    private func exchange(_ request: URLRequest) throws -> (status: Int, text: String) {
        var output: (Int, String)?
        var failed: Error?
        let done = expectation(description: request.url?.absoluteString ?? "request")
        URLSession.shared.dataTask(with: request) { data, response, error in
            if let error {
                failed = error
            } else {
                let status = (response as? HTTPURLResponse)?.statusCode ?? 0
                output = (status, String(data: data ?? Data(), encoding: .utf8) ?? "")
            }
            done.fulfill()
        }.resume()
        wait(for: [done], timeout: 10)
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
        print("l108-shot \(url.path)")
        _ = app
    }

    private var device: String {
        #if os(tvOS)
            "tv"
        #else
            UIDevice.current.userInterfaceIdiom == .pad ? "ipad" : "iphone"
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
