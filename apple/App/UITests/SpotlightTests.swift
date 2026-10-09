import XCTest

/// A channel the app indexed shows up in Spotlight, and tapping it opens that channel live.
/// Opt-in: BROADWAVE_SERVER, a harness whose lineup has a channel named BROADWAVE_SPOTLIGHT_NAME
/// (default "WTST", 5.1 on the e2e fake tuner).
@MainActor
final class SpotlightTests: XCTestCase {
    func testChannelFromSpotlightOpensThePlayer() throws {
        #if os(tvOS)
            throw XCTSkip("Spotlight is iPhone and iPad only")
        #else
            executionTimeAllowance = 240
            let server = lane("BROADWAVE_SERVER")
            try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
            let name = lane("BROADWAVE_SPOTLIGHT_NAME").isEmpty ? "WTST" : lane("BROADWAVE_SPOTLIGHT_NAME")
            try put(server, #"{"setupComplete":"1"}"#)
            let number = try channelNumber(server, name)

            let app = XCUIApplication()
            app.launchArguments = ["-ApplePersistenceIgnoreState", "YES", "-BroadwaveServerURL", server]
            app.launch()
            XCTAssertTrue(app.buttons["Home"].waitForExistence(timeout: 25), app.debugDescription)
            // The index is written once the lineup and the recordings have loaded.
            RunLoop.current.run(until: Date().addingTimeInterval(6))

            XCUIDevice.shared.press(.home)
            let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
            XCTAssertTrue(springboard.wait(for: .runningForeground, timeout: 10))
            // Spotlight's results run in their own process on iOS 26, Springboard's before.
            let spotlight = XCUIApplication(bundleIdentifier: "com.apple.Spotlight")
            let pill = springboard.otherElements["spotlight-pill"].firstMatch
            if pill.waitForExistence(timeout: 3) {
                pill.tap()
            } else {
                springboard.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.62))
                    .press(forDuration: 0.05, thenDragTo: springboard.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.9)))
            }
            let host = [spotlight, springboard].first { field(in: $0).waitForExistence(timeout: 5) }
            let search = try XCTUnwrap(host, "no Spotlight field\n\(spotlight.debugDescription)\n\(springboard.debugDescription)")
            let input = field(in: search)
            let title = "\(number) \(name)"
            let result = search.descendants(matching: .any)
                .matching(NSPredicate(format: "label CONTAINS %@", title)).firstMatch
            // Spotlight keeps the last query, and the index can land a few seconds after the app writes it.
            for _ in 0 ..< 3 where !result.exists {
                input.tap()
                let typed = (input.value as? String) ?? ""
                if !typed.isEmpty, typed != input.placeholderValue {
                    input.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: typed.count))
                }
                input.typeText(name)
                _ = result.waitForExistence(timeout: 10)
            }
            XCTAssertTrue(result.exists, "no result \(title)\n\(search.debugDescription)")
            // App results sit below the suggestions, under the keyboard. A scroll puts the keyboard away.
            if !result.isHittable || result.frame.maxY > search.keyboards.firstMatch.frame.minY {
                search.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.45))
                    .press(forDuration: 0.05, thenDragTo: search.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.15)))
            }
            RunLoop.current.run(until: Date().addingTimeInterval(1.5))
            print("i4 result \(result.label) at \(result.frame)")
            shot("spotlight-results")
            result.tap()

            XCTAssertTrue(app.wait(for: .runningForeground, timeout: 10), "Broadwave did not open")
            let player = app.descendants(matching: .any)["player-channel"]
            XCTAssertTrue(player.waitForExistence(timeout: 20), "no player\n\(app.debugDescription)")
            XCTAssertTrue(until(15) { (player.value as? String) == number }, "player on \(player.value ?? "nil"), wanted \(number)")
            shot("spotlight-player")
            print("i4 spotlight \(title) opened player \(player.value ?? "")")
        #endif
    }

    private func field(in app: XCUIApplication) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(
            format: "elementType == %d OR elementType == %d",
            XCUIElement.ElementType.searchField.rawValue, XCUIElement.ElementType.textField.rawValue
        )).firstMatch
    }

    private func channelNumber(_ server: String, _ name: String) throws -> String {
        let result = try exchange(URLRequest(url: URL(string: server + "/api/v1/channels")!))
        XCTAssertEqual(result.status, 200, result.text)
        let body = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any]
        let rows = body?["channels"] as? [[String: Any]] ?? []
        let row = rows.first { ($0["displayName"] as? String) == name }
        return try XCTUnwrap(row?["displayNumber"] as? String, "no channel \(name)\n\(result.text)")
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

    private func shot(_ name: String) {
        let screen = XCUIScreen.main.screenshot()
        let attachment = XCTAttachment(screenshot: screen)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
        let dir = lane("BROADWAVE_SHOT_DIR")
        guard !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        try? screen.pngRepresentation.write(to: folder.appending(path: "\(name).png"))
    }

    private func lane(_ name: String) -> String {
        let env = ProcessInfo.processInfo.environment
        if let direct = env[name], !direct.isEmpty {
            return direct
        }
        return env["TEST_RUNNER_\(name)"] ?? ""
    }
}
