import XCTest

/// Settings can forget a tuner or playlist that is gone. Opt-in: BROADWAVE_SERVER, a local harness.
@MainActor
final class RemoveDeviceTests: XCTestCase {
    func testRemoveLinkSource() throws {
        executionTimeAllowance = 240
        let server = lane("BROADWAVE_SERVER")
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        try put(server, #"{"setupComplete":"1"}"#)
        for id in try deviceIDs(server, name: "Old Box") {
            deleteDevice(server, id)
        }

        _ = try post(server, #"{"kind":"link","name":"Old Box","url":"http://127.0.0.1:9/old.ts"}"#)
        let deviceID = try XCTUnwrap(deviceIDs(server, name: "Old Box").last, "Old Box was not stored")
        defer { self.deleteDevice(server, deviceID) }

        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveTab", "settings",
            "-BroadwaveSources", "YES",
        ]
        app.launch()

        let button = app.descendants(matching: .any)["remove-device-\(deviceID)"]
        XCTAssertTrue(button.waitForExistence(timeout: 30), app.debugDescription)
        let label = button.label
        XCTAssertTrue(label.contains("Old Box"), label)
        shot(app, "before")
        choose(button, label: "Old Box", in: app)

        let alert = app.alerts.firstMatch
        XCTAssertTrue(alert.waitForExistence(timeout: 8), app.debugDescription)
        XCTAssertTrue(alert.label.contains("Remove Old Box?"), alert.label)
        XCTAssertTrue(alert.label.contains("Recordings stay."), alert.label)
        let confirm = app.buttons.matching(identifier: "confirm-remove")
        XCTAssertTrue(confirm.firstMatch.waitForExistence(timeout: 5), app.debugDescription)
        shot(app, "confirm")
        confirmRemove(confirm.element(boundBy: 0), in: app)

        XCTAssertTrue(until(20) { self.gone(server, deviceID) }, "Old Box stayed in the lineup")
        XCTAssertTrue(until(15) { !button.exists }, app.debugDescription)
        shot(app, "after")
    }

    private func choose(_ row: XCUIElement, label: String, in app: XCUIApplication) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            for step in 0 ..< 40 {
                let here = focusedText(app)
                print("l84 focus \(step) \(here)")
                if here.contains(label), here.contains("Remove") {
                    remote.press(.select)
                    return
                }
                remote.press(.down)
            }
            XCTFail("\(label) never took focus, at \(focusedText(app))")
            _ = row
        #else
            if !row.isHittable {
                for _ in 0 ..< 8 {
                    app.swipeUp()
                    if row.isHittable {
                        break
                    }
                }
            }
            if row.isHittable {
                row.tap()
            } else {
                row.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
        #endif
    }

    /// The alert's Remove, not a row whose name also contains Remove.
    private func confirmRemove(_ button: XCUIElement, in app: XCUIApplication) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            for step in 0 ..< 8 {
                let here = focusedText(app)
                print("l84 alert \(step) \(here)")
                if here.contains("confirm-remove") || (here.contains(" Remove") && !here.contains("Old Box")) {
                    remote.press(.select)
                    return
                }
                remote.press(step.isMultiple(of: 2) ? .down : .right)
            }
            XCTFail("confirm never took focus, at \(focusedText(app))")
            _ = button
        #else
            button.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        #endif
    }

    private func focusedText(_ app: XCUIApplication) -> String {
        let focused = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        guard focused.exists else { return "" }
        let value = focused.value as? String ?? ""
        return "\(focused.identifier) \(focused.label) \(value)"
    }

    private func deviceIDs(_ server: String, name: String) throws -> [String] {
        let result = try get(server, "/api/v1/sources")
        guard let root = try JSONSerialization.jsonObject(with: Data(result.utf8)) as? [String: Any],
              let rows = root["sources"] as? [[String: Any]]
        else { return [] }
        return rows.compactMap { row in
            guard (row["name"] as? String) == name else { return nil }
            let id = row["deviceId"] as? String ?? ""
            return id.isEmpty ? nil : id
        }
    }

    private func gone(_ server: String, _ id: String) -> Bool {
        guard let text = try? get(server, "/api/v1/devices") else { return false }
        return !text.contains("\"deviceId\":\"\(id)\"")
    }

    private func post(_ server: String, _ body: String) throws -> String {
        var request = URLRequest(url: URL(string: server + "/api/v1/sources")!)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = Data(body.utf8)
        let result = try exchange(request)
        XCTAssertTrue((200 ..< 300).contains(result.status), "post \(result.status) \(result.text)")
        guard let root = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any] else {
            return ""
        }
        return root["deviceId"] as? String ?? ""
    }

    private func put(_ server: String, _ body: String) throws {
        var request = URLRequest(url: URL(string: server + "/api/v1/settings")!)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = Data(body.utf8)
        let result = try exchange(request)
        XCTAssertTrue((200 ..< 300).contains(result.status), "put \(result.status) \(result.text)")
    }

    private func get(_ server: String, _ path: String) throws -> String {
        var request = URLRequest(url: URL(string: server + path)!)
        request.httpMethod = "GET"
        let result = try exchange(request)
        XCTAssertTrue((200 ..< 300).contains(result.status), "get \(path) \(result.status) \(result.text)")
        return result.text
    }

    private func deleteDevice(_ server: String, _ id: String) {
        guard !id.isEmpty else { return }
        var request = URLRequest(url: URL(string: server + "/api/v1/devices/\(id)")!)
        request.httpMethod = "DELETE"
        _ = try? exchange(request)
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
        print("l84-shot \(url.path)")
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
