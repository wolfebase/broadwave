import XCTest

/// Watching on Apple TV with the remote, against a real server: the live player,
/// the guide, and recordings. Opt-in: TEST_RUNNER_BROADWAVE_SERVER=http://host:port.
/// TEST_RUNNER_BROADWAVE_SHOTS=<dir> keeps a screenshot of every step.
@MainActor
class WatchingCase: XCTestCase {
    let app = XCUIApplication()

    var server: String {
        get throws {
            guard let server = ProcessInfo.processInfo.environment["BROADWAVE_SERVER"], !server.isEmpty else {
                throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER to run against a server")
            }
            return server
        }
    }

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    func launch(_ extra: [String] = []) throws {
        app.launchArguments = try ["-ApplePersistenceIgnoreState", "YES", "-BroadwaveServerURL", server, "-lastChannel", "0"] + extra
        app.launch()
    }

    func shot(_ name: String) {
        let env = ProcessInfo.processInfo.environment
        guard let dir = env["BROADWAVE_SHOTS"] ?? env["TEST_RUNNER_BROADWAVE_SHOTS"], !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: folder.appending(path: "\(name).png"))
    }

    func press(_ button: XCUIRemote.Button, _ times: Int = 1, wait seconds: TimeInterval = 0.5) {
        for _ in 0 ..< times {
            XCUIRemote.shared.press(button)
            wait(seconds)
        }
    }

    func focused() -> XCUIElement {
        app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
    }

    func focusedLabel() -> String {
        let element = focused()
        return element.exists ? element.label : "nothing"
    }

    func element(_ id: String) -> XCUIElement {
        app.descendants(matching: .any)[id]
    }

    func text(_ label: String) -> XCUIElement {
        app.staticTexts.matching(NSPredicate(format: "label == %@", label)).firstMatch
    }

    @discardableResult
    func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while Date() < end {
            if done() {
                return true
            }
            wait(0.1)
        }
        return done()
    }

    func wait(_ seconds: TimeInterval) {
        RunLoop.current.run(until: Date().addingTimeInterval(seconds))
    }

    // MARK: Server

    @discardableResult
    func call(_ method: String, _ path: String, _ body: [String: Any]? = nil) throws -> Any? {
        var request = try URLRequest(url: URL(string: server + path)!, timeoutInterval: 60)
        request.httpMethod = method
        if let body {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONSerialization.data(withJSONObject: body)
        }
        let done = expectation(description: path)
        nonisolated(unsafe) var result: (Data?, Int) = (nil, 0)
        URLSession.shared.dataTask(with: request) { data, response, _ in
            result = (data, (response as? HTTPURLResponse)?.statusCode ?? 0)
            done.fulfill()
        }.resume()
        wait(for: [done], timeout: 70)
        guard (200 ..< 300).contains(result.1) else {
            throw NSError(domain: "server", code: result.1, userInfo: [NSLocalizedDescriptionKey: "\(method) \(path): \(result.1)"])
        }
        return result.0.flatMap { try? JSONSerialization.jsonObject(with: $0) }
    }

    func recordings() throws -> [[String: Any]] {
        try (call("GET", "/api/v1/recordings") as? [String: Any])?["recordings"] as? [[String: Any]] ?? []
    }
}

/// Recordings play as on-demand video, and one Back leaves them.
final class RecordingPlaybackTests: WatchingCase {
    /// A finished recording: 0:00 to its length, time left, no LIVE badge or air-time clock.
    func testAFinishedRecordingPlaysOnDemand() throws {
        let rec = try XCTUnwrap(try recordings().first { $0["status"] as? String == "complete" && ($0["durationSec"] as? Double ?? 0) > 120 })
        let title = try XCTUnwrap(rec["title"] as? String)
        try launch(["-BroadwaveTab", "recordings", "-BroadwaveChannelNow", "YES"])
        try open(title)
        XCTAssertTrue(until(20) { self.playing() }, "the recording never started")
        wait(3)
        press(.select, wait: 1.5)
        shot("rec-finished-bar")
        let elapsed = element("AXElapsedTime")
        XCTAssertTrue(until(5) { elapsed.exists }, "no transport bar")
        XCTAssertFalse(elapsed.label.contains("AM") || elapsed.label.contains("PM"), "air-time clock: \(elapsed.label)")
        XCTAssertFalse(text("LIVE").exists && text("LIVE").frame.width > 1, "LIVE badge on a recording")
        let remaining = element("AXRemainingTime")
        if remaining.exists {
            XCTAssertTrue(remaining.label.hasPrefix("-"), "time left: \(remaining.label)")
        }
        press(.menu, wait: 2)
        shot("rec-finished-back")
        XCTAssertTrue(until(5) { !elapsed.exists }, "one Back left the player up")
        XCTAssertTrue(focusedLabel().contains(title), "focus on \(focusedLabel())")
    }

    /// A recording still on the air starts within a few seconds, plays on demand, and one Back leaves.
    func testARecordingStillOnTheAirStarts() throws {
        let title = "Watching Test \(Int(Date().timeIntervalSince1970) % 10000)"
        let channels = try (call("GET", "/api/v1/channels") as? [String: Any])?["channels"] as? [[String: Any]] ?? []
        let channel = try XCTUnwrap(channels.first { $0["guideNumber"] as? String == "5.1" } ?? channels.first)
        let made = try XCTUnwrap(try call("POST", "/api/v1/recordings", ["channelId": channel["id"] as Any, "minutes": 4, "title": title]) as? [String: Any])
        let id = try XCTUnwrap(made["id"] as? Int)
        addTeardownBlock { [self] in
            _ = try? call("POST", "/api/v1/recordings/\(id)/stop")
            _ = try? call("DELETE", "/api/v1/recordings/\(id)")
        }
        // A few segments on disk, as a viewer who opens it right after it starts.
        wait(25)
        try launch(["-BroadwaveTab", "recordings", "-BroadwaveChannelNow", "YES"])
        try open(title)
        let start = Date()
        XCTAssertTrue(until(20) { self.playing() }, "the recording never started")
        print("TIMING in-progress recording select->playing \(String(format: "%.2f", Date().timeIntervalSince(start)))s")
        wait(4)
        press(.select, wait: 1.5)
        shot("rec-growing-bar")
        let elapsed = element("AXElapsedTime")
        XCTAssertTrue(until(5) { elapsed.exists }, "no transport bar")
        XCTAssertFalse(elapsed.label.contains("AM") || elapsed.label.contains("PM"), "air-time clock: \(elapsed.label)")
        press(.menu, wait: 2)
        shot("rec-growing-back")
        XCTAssertTrue(until(5) { !elapsed.exists }, "one Back left the player up")
    }

    private func open(_ title: String) throws {
        let row = app.buttons.matching(NSPredicate(format: "identifier == 'recording-row' AND label CONTAINS %@", title)).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 20), "no row \(title)")
        for _ in 0 ..< 12 where !focusedLabel().contains(title) {
            press(.down, wait: 0.4)
        }
        XCTAssertTrue(focusedLabel().contains(title), "focus on \(focusedLabel())")
        shot("rec-row")
        press(.select, wait: 0.2)
    }

    /// The player's clock moves.
    private func playing() -> Bool {
        let probe = element("recording-time")
        guard probe.exists, let first = Double(probe.label), first > 0 else { return false }
        wait(1)
        return (Double(probe.label) ?? first) > first
    }
}
