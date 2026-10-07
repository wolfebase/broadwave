import BroadwaveKit
import XCTest

/// Find commercials says it is working, a second press does not start another scan, then it says how many.
/// Opt-in: TEST_RUNNER_BROADWAVE_SERVER on a harness that already lists a finished recording titled "Night Desk".
@MainActor
final class CommercialScanTests: XCTestCase {
    private let title = "Night Desk"

    func testFindingCommercialsSaysItIsWorking() throws {
        continueAfterFailure = false
        executionTimeAllowance = 180
        let server = lane("BROADWAVE_SERVER")
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        let id = try XCTUnwrap(try recordingID(server), "plant \(title) before the test")
        defer { delete(server, id) }

        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveTab", "recordings",
            "-BroadwaveDetectHold", "45",
        ]
        app.launch()

        let row = app.descendants(matching: .any).matching(NSPredicate(format: "identifier == 'recording-row' AND label CONTAINS %@", title)).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 25), app.debugDescription)
        XCTAssertTrue(openMenu(app, on: row, showing: "Find commercials"), "no menu\n\(app.debugDescription)")
        choose(app, "Find commercials")

        let working = app.descendants(matching: .any).matching(NSPredicate(format: "identifier == 'recordings-notice' AND label == %@", CommercialScan.finding)).firstMatch
        XCTAssertTrue(working.waitForExistence(timeout: 5), app.debugDescription)
        shot(app, "finding")

        // The working item replaces Find commercials, so a second press has nothing to start.
        // tvOS menu rows do not report focus; they are found by label.
        let offeredAgain = openMenu(app, on: row, showing: "Find commercials", refocus: false)
        XCTAssertFalse(offeredAgain, "a second press could start another scan\n\(app.debugDescription)")
        let held = entry(app, CommercialScan.finding)
        if held.exists {
            XCTAssertFalse(held.isEnabled)
        }
        dismissMenu(app)

        let done = app.descendants(matching: .any).matching(NSPredicate(format: "identifier == 'recordings-notice' AND label == %@", CommercialScan.found(0))).firstMatch
        XCTAssertTrue(done.waitForExistence(timeout: 90), "notice stayed \(working.label)\n\(app.debugDescription)")
        shot(app, "found")
    }

    private func openMenu(_ app: XCUIApplication, on row: XCUIElement, showing item: String, refocus: Bool = true) -> Bool {
        #if os(tvOS)
            if refocus {
                focus(app)
            }
            for _ in 0 ..< 2 {
                RunLoop.current.run(until: Date().addingTimeInterval(1))
                XCUIRemote.shared.press(.select, forDuration: 1.5)
                if entry(app, item).waitForExistence(timeout: 4) {
                    return true
                }
            }
            return false
        #else
            if row.isHittable {
                row.press(forDuration: 1.2)
            } else {
                row.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).press(forDuration: 1.2)
            }
            return entry(app, item).waitForExistence(timeout: 4)
        #endif
    }

    private func choose(_ app: XCUIApplication, _ label: String) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            // Menu rows do not report focus. Mark watched, Make a channel, then Find commercials.
            remote.press(.down)
            RunLoop.current.run(until: Date().addingTimeInterval(0.5))
            remote.press(.down)
            RunLoop.current.run(until: Date().addingTimeInterval(0.5))
            remote.press(.select)
            _ = label
        #else
            let item = entry(app, label)
            if item.isHittable {
                item.tap()
            } else {
                item.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
        #endif
    }

    private func dismissMenu(_ app: XCUIApplication) {
        #if os(tvOS)
            XCUIRemote.shared.press(.menu)
        #else
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.08)).tap()
        #endif
        _ = until(3) { !self.entry(app, CommercialScan.finding).exists || self.entry(app, "recordings-notice").exists }
    }

    private func focus(_ app: XCUIApplication) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            if focused(app).contains(title) {
                return
            }
            for _ in 0 ..< 12 {
                remote.press(.down)
                RunLoop.current.run(until: Date().addingTimeInterval(0.35))
                if focused(app).contains(title) {
                    return
                }
            }
        #else
            _ = app
        #endif
    }

    private func entry(_ app: XCUIApplication, _ label: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", label)).firstMatch
    }

    private func focused(_ app: XCUIApplication) -> String {
        let element = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        return element.exists ? element.label : ""
    }

    private func recordingID(_ server: String) throws -> Int? {
        let result = try exchange(URLRequest(url: URL(string: server + "/api/v1/recordings")!))
        guard result.status == 200,
              let root = try JSONSerialization.jsonObject(with: Data(result.text.utf8)) as? [String: Any],
              let rows = root["recordings"] as? [[String: Any]]
        else { return nil }
        return rows.compactMap { row -> Int? in
            guard row["title"] as? String == title else { return nil }
            return (row["id"] as? NSNumber)?.intValue
        }.last
    }

    private func delete(_ server: String, _ id: Int) {
        var request = URLRequest(url: URL(string: server + "/api/v1/recordings/\(id)")!)
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
