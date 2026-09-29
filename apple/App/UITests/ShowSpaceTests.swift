import XCTest

/// Space by show lists finished recordings under Settings and opens that show.
/// Opt-in: BROADWAVE_SERVER or TEST_RUNNER_BROADWAVE_SERVER, a local harness.
@MainActor
final class ShowSpaceTests: XCTestCase {
    func testAShowOpensItsRecordings() throws {
        executionTimeAllowance = 360
        let server = lane("BROADWAVE_SERVER")
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        try put(server, #"{"setupComplete":"1","watermarkGB":"0"}"#)

        let channels = try channels(server)
        let news = try XCTUnwrap(channels.first, "no channels")
        let other = channels.dropFirst().first ?? news
        var made: [(id: Int, title: String)] = []
        defer {
            for rec in made {
                self.delete(server, rec.id)
            }
        }

        try made.append(record(server, channel: news.id, title: "Evening News"))
        try made.append(record(server, channel: news.id, title: "Evening News"))
        try made.append(record(server, channel: other.id, title: "Night Talk"))

        let shows = try shows(server)
        let title = made[0].title
        let row = try XCTUnwrap(shows.first { same($0.title, title) }, "no row for \(title) in \(shows)")
        XCTAssertGreaterThanOrEqual(row.count, 2, "\(shows)")
        let elsewhere = shows.filter { !same($0.title, title) }
        print("l68 shows \(shows.map { "\($0.title) \($0.count) \($0.bytes)" }.joined(separator: "; "))")

        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveTab", "settings",
            "-BroadwaveSettingsAnchor", "storage",
        ]
        app.launch()

        let button = showButton(app, title)
        XCTAssertTrue(button.waitForExistence(timeout: 30), app.debugDescription)
        let count = row.count == 1 ? "1 recording" : "\(row.count) recordings"
        let size = ByteCountFormatter.string(fromByteCount: row.bytes, countStyle: .file)
        reveal(button, title: title, in: app)
        XCTAssertTrue(button.label.contains(title), button.label)
        XCTAssertTrue(button.label.contains(count), button.label)
        XCTAssertTrue(button.label.contains(size), "\(button.label) wanted \(size)")
        shot(app, "settings")

        open(button, title: title, in: app)
        let filtered = app.descendants(matching: .any)["recordings-filtered"]
        XCTAssertTrue(filtered.waitForExistence(timeout: 15), app.debugDescription)
        let listed = recordingRows(app)
        XCTAssertEqual(listed.count, row.count, app.debugDescription)
        for name in elsewhere.map(\.title) {
            let stray = listed.matching(NSPredicate(format: "label CONTAINS %@", name))
            XCTAssertEqual(stray.count, 0, "\(name) stayed in the filtered list")
        }
        shot(app, "filtered")

        let showAll = app.descendants(matching: .any)["show-all"]
        XCTAssertTrue(showAll.waitForExistence(timeout: 5), app.debugDescription)
        choose(showAll, label: "Show all", in: app)
        let cleared = until(5) { !filtered.exists }
        XCTAssertTrue(cleared, "the show filter stayed up")
        for name in elsewhere.map(\.title) {
            let back = recordingRows(app).matching(NSPredicate(format: "label CONTAINS %@", name))
            XCTAssertGreaterThan(back.count, 0, "\(name) did not come back")
        }
    }

    private func showButton(_ app: XCUIApplication, _ title: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "identifier == 'show-space-row' AND label CONTAINS %@", title)).firstMatch
    }

    private func recordingRows(_ app: XCUIApplication) -> XCUIElementQuery {
        app.descendants(matching: .any).matching(NSPredicate(format: "identifier == 'recording-row'"))
    }

    private func reveal(_ row: XCUIElement, title: String, in app: XCUIApplication) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            for step in 0 ..< 45 {
                let here = focusedText(app)
                print("l68 focus \(step) \(here)")
                if here.contains(title) {
                    return
                }
                remote.press(.down)
            }
            _ = row
        #else
            if row.isHittable {
                return
            }
            for _ in 0 ..< 8 {
                app.swipeUp()
                if row.isHittable {
                    return
                }
            }
        #endif
    }

    /// Select a control that is already on screen. On Apple TV, move the remote until its name is focused.
    private func choose(_ row: XCUIElement, label: String, in app: XCUIApplication) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            // A recording row takes focus when the tab opens. The control may sit above it.
            for _ in 0 ..< 6 {
                if focusedText(app).contains(label) {
                    break
                }
                remote.press(.up)
            }
            for _ in 0 ..< 16 {
                if focusedText(app).contains(label) {
                    break
                }
                remote.press(.down)
            }
            XCTAssertTrue(focusedText(app).contains(label), "\(label) never took focus, at \(focusedText(app))")
            remote.press(.select)
        #else
            if row.isHittable {
                row.tap()
            } else {
                row.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
        #endif
    }

    private func open(_ row: XCUIElement, title: String, in app: XCUIApplication) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            if !focusedText(app).contains(title) {
                reveal(row, title: title, in: app)
            }
            XCTAssertTrue(focusedText(app).contains(title), "show row never took focus, at \(focusedText(app))")
            remote.press(.select)
        #else
            if row.isHittable {
                row.tap()
            } else {
                row.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
        #endif
    }

    private func focusedText(_ app: XCUIApplication) -> String {
        let focused = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        guard focused.exists else { return "" }
        let value = focused.value as? String ?? ""
        return "\(focused.identifier) \(focused.label) \(value)"
    }

    private struct Channel {
        var id: Int
        var name: String
    }

    private struct Show {
        var title: String
        var count: Int
        var bytes: Int64
    }

    private func channels(_ server: String) throws -> [Channel] {
        let result = try exchange(URLRequest(url: URL(string: server + "/api/v1/channels")!))
        XCTAssertEqual(result.status, 200, result.text)
        let root = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any]
        let rows = root?["channels"] as? [[String: Any]] ?? []
        return rows.compactMap { row in
            guard let id = (row["id"] as? NSNumber)?.intValue else { return nil }
            let name = row["displayName"] as? String ?? ""
            return Channel(id: id, name: name)
        }
    }

    private func shows(_ server: String) throws -> [Show] {
        let result = try exchange(URLRequest(url: URL(string: server + "/api/v1/storage/shows")!))
        XCTAssertEqual(result.status, 200, result.text)
        let root = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any]
        let rows = root?["shows"] as? [[String: Any]] ?? []
        return rows.compactMap { row in
            guard let title = row["title"] as? String, let count = (row["count"] as? NSNumber)?.intValue else { return nil }
            let bytes = (row["bytes"] as? NSNumber)?.int64Value ?? 0
            return Show(title: title, count: count, bytes: bytes)
        }
    }

    private func record(_ server: String, channel: Int, title: String) throws -> (id: Int, title: String) {
        let body = try JSONSerialization.data(withJSONObject: ["channelId": channel, "minutes": 2, "title": title])
        var created: (Int, String)?
        var last = ""
        for _ in 0 ..< 6 {
            var request = URLRequest(url: URL(string: server + "/api/v1/recordings")!)
            request.httpMethod = "POST"
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = body
            let result = try exchange(request)
            last = "\(result.status) \(result.text)"
            guard (200 ..< 300).contains(result.status) else {
                Thread.sleep(forTimeInterval: 1)
                continue
            }
            let parsed = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any]
            guard let id = (parsed?["id"] as? NSNumber)?.intValue else {
                Thread.sleep(forTimeInterval: 1)
                continue
            }
            created = (id, parsed?["title"] as? String ?? title)
            break
        }
        let made = try XCTUnwrap(created, "record failed \(last)")
        let grew = until(25) {
            guard let row = try? self.recording(server, made.0) else { return false }
            return row.bytes > 1000
        }
        XCTAssertTrue(grew, "recording \(made.0) stayed empty")
        var stop = URLRequest(url: URL(string: server + "/api/v1/recordings/\(made.0)/stop")!)
        stop.httpMethod = "POST"
        stop.setValue("application/json", forHTTPHeaderField: "Content-Type")
        stop.httpBody = Data("{}".utf8)
        let stopped = try exchange(stop)
        XCTAssertTrue((200 ..< 300).contains(stopped.status), stopped.text)
        let done = until(25) {
            (try? self.recording(server, made.0))?.status == "complete"
        }
        XCTAssertTrue(done, "recording \(made.0) did not finish")
        return (made.0, made.1)
    }

    private func recording(_ server: String, _ id: Int) throws -> (status: String, bytes: Int64)? {
        let result = try exchange(URLRequest(url: URL(string: server + "/api/v1/recordings")!))
        guard result.status == 200,
              let root = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any],
              let rows = root["recordings"] as? [[String: Any]]
        else { return nil }
        guard let row = rows.first(where: { ($0["id"] as? NSNumber)?.intValue == id }) else { return nil }
        return (row["status"] as? String ?? "", (row["bytes"] as? NSNumber)?.int64Value ?? 0)
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
        XCTAssertTrue((200 ..< 300).contains(result.status), "put \(result.status) \(result.text)")
    }

    private func same(_ left: String, _ right: String) -> Bool {
        left.trimmingCharacters(in: .whitespacesAndNewlines).localizedCaseInsensitiveCompare(right.trimmingCharacters(in: .whitespacesAndNewlines)) == .orderedSame
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
        print("l68-shot \(url.path)")
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
        let direct = env[name] ?? ""
        if !direct.isEmpty {
            return direct
        }
        return env["TEST_RUNNER_\(name)"] ?? ""
    }
}
