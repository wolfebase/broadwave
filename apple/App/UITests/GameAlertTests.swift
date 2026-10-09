import XCTest

/// A game alert shows while the app is open, and Watch plays that channel.
/// Opt-in: BROADWAVE_SERVER or TEST_RUNNER_BROADWAVE_SERVER, a local harness.
@MainActor
final class GameAlertTests: XCTestCase {
    func testWatchPlaysTheChannel() throws {
        executionTimeAllowance = 180
        let server = lane("BROADWAVE_SERVER")
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        try put(server, #"{"setupComplete":"1"}"#)

        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveFakeGameAlert", "YES",
            "-BroadwaveInfo", "YES",
        ]
        app.launch()

        let watch = app.descendants(matching: .any)["game-alert-watch"]
        XCTAssertTrue(watch.waitForExistence(timeout: 30), app.debugDescription)
        let text = app.descendants(matching: .any)["game-alert-text"]
        XCTAssertTrue(text.label.contains("Starting now: CHI at LV"), text.label)
        let number = watch.label.replacingOccurrences(of: "Watch ", with: "")
        XCTAssertFalse(number.isEmpty, watch.label)
        shot(app, "banner")

        openWatch(app, watch)
        #if os(tvOS)
            let chrome = app.descendants(matching: .any)["player-chrome"]
            XCTAssertTrue(chrome.waitForExistence(timeout: 20), app.debugDescription)
            XCTAssertTrue(chrome.label.contains(number), "player \(chrome.label) for \(number)")
        #else
            XCTAssertTrue(app.buttons["Minimize"].waitForExistence(timeout: 20), app.debugDescription)
            let title = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "\(number) ")).firstMatch
            XCTAssertTrue(title.waitForExistence(timeout: 8), "no \(number) in the player\n\(app.debugDescription)")
        #endif
        let banner = app.descendants(matching: .any)["game-alert"]
        XCTAssertFalse(banner.waitForExistence(timeout: 2), "the alert stayed up over the player")
        shot(app, "playing")
        print("l110 \(device) watch \(number)")
    }

    private func openWatch(_ app: XCUIApplication, _ watch: XCUIElement) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            // Leave the sidebar, then Up into the banner's focus section. The 12 second
            // timer is already running, so this stays short. tvOS has no tap().
            remote.press(.right)
            for step in 0 ..< 8 {
                let here = focused(app)
                print("l110 focus \(step) \(here)")
                if watch.hasFocus || here.contains("Watch ") {
                    remote.press(.select)
                    return
                }
                remote.press(step.isMultiple(of: 4) ? .right : .up)
            }
            XCTFail("Watch never took focus\n\(app.debugDescription)")
        #else
            if watch.isHittable {
                watch.tap()
            } else {
                watch.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
        #endif
    }

    private func focused(_ app: XCUIApplication) -> String {
        let focused = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        guard focused.exists else { return "" }
        return "\(focused.identifier) \(focused.label)"
    }

    private func put(_ server: String, _ body: String) throws {
        var request = URLRequest(url: URL(string: server + "/api/v1/settings")!)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = Data(body.utf8)
        let result = try exchange(request)
        XCTAssertTrue((200 ..< 300).contains(result.status), "put \(result.status) \(result.text)")
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

    private func shot(_ app: XCUIApplication, _ name: String) {
        let dir = lane("BROADWAVE_SHOT_DIR")
        guard !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let url = folder.appending(path: "\(device)-\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
        print("l110-shot \(url.path)")
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
