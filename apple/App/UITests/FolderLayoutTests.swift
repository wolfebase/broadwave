import XCTest

/// The recording-folders picker writes folderLayout and the server reads it back.
/// Opt-in: BROADWAVE_SERVER or TEST_RUNNER_BROADWAVE_SERVER, a local harness.
@MainActor
final class FolderLayoutTests: XCTestCase {
    private let hint = "By show puts new recordings in TV/Show/Season 01 and Movies/Title (Year). Point a TV library at TV and a movie library at Movies."

    func testPickerSavesAllInOneFolder() throws {
        executionTimeAllowance = 240
        let server = lane("BROADWAVE_SERVER")
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        try put(server, #"{"setupComplete":"1","folderLayout":"shows"}"#)
        let opened = try settings(server)
        XCTAssertEqual(opened["folderLayout"], "shows", "fresh layout \(opened["folderLayout"] ?? "missing")")
        defer { try? self.put(server, #"{"folderLayout":"shows"}"#) }

        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveTab", "settings",
            "-BroadwaveSettingsAnchor", "storage",
        ]
        app.launch()

        let picker = try findPicker(app)
        XCTAssertTrue(until(20) { picker.isEnabled }, "picker stayed disabled\n\(app.debugDescription)")
        reveal(picker, in: app)
        let shown = app.staticTexts.matching(NSPredicate(format: "label == %@", hint)).firstMatch
        XCTAssertTrue(shown.waitForExistence(timeout: 8), "missing hint\n\(app.debugDescription)")
        shot(app, "folders")

        choose(app, picker, "All in one folder")
        let saved = try waitLayout(server, "flat")
        XCTAssertEqual(saved, "flat")
        shot(app, "folders-flat")
        print("l109 \(device) folderLayout=\(saved)")
    }

    private func findPicker(_ app: XCUIApplication) throws -> XCUIElement {
        let byID = app.descendants(matching: .any)["recording-folders"]
        if byID.waitForExistence(timeout: 25) {
            return byID
        }
        let byLabel = app.descendants(matching: .any)["Recording folders"]
        XCTAssertTrue(byLabel.waitForExistence(timeout: 5), app.debugDescription)
        return byLabel
    }

    /// The tvOS focus sits on the row, not the inner button, so the button's hasFocus stays false.
    private func focusedText(_ app: XCUIApplication) -> String {
        let focused = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        guard focused.exists else { return "" }
        let value = focused.value as? String ?? ""
        return "\(focused.identifier) \(focused.label) \(value)"
    }

    private func onFolders(_ app: XCUIApplication) -> Bool {
        focusedText(app).contains("Recording folders")
    }

    private func reveal(_ picker: XCUIElement, in app: XCUIApplication) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            for step in 0 ..< 40 {
                let here = focusedText(app)
                print("l109 focus \(step) \(here)")
                if here.contains("Recording folders") {
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
        _ = app
    }

    private func choose(_ app: XCUIApplication, _ picker: XCUIElement, _ label: String) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            if !onFolders(app) {
                reveal(picker, in: app)
            }
            XCTAssertTrue(onFolders(app), "picker never took focus, at \(focusedText(app))")
            remote.press(.select)
            for step in 0 ..< 8 {
                let here = focusedText(app)
                print("l109 option \(step) \(here)")
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

    private func put(_ server: String, _ body: String) throws {
        var request = URLRequest(url: URL(string: server + "/api/v1/settings")!)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = Data(body.utf8)
        let result = try exchange(request)
        XCTAssertTrue((200 ..< 300).contains(result.status), "put \(result.status) \(result.text)")
    }

    private func settings(_ server: String) throws -> [String: String] {
        let request = URLRequest(url: URL(string: server + "/api/v1/settings")!)
        let result = try exchange(request)
        XCTAssertEqual(result.status, 200, result.text)
        let parsed = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: String]
        return try XCTUnwrap(parsed, result.text)
    }

    private func waitLayout(_ server: String, _ want: String) throws -> String {
        var last = ""
        let found = until(12) {
            guard let values = try? self.settings(server) else { return false }
            last = values["folderLayout"] ?? ""
            return last == want
        }
        XCTAssertTrue(found, "folderLayout stayed \(last)")
        return last
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
        print("l109-shot \(url.path)")
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
