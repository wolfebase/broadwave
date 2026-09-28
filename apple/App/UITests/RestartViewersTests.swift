import XCTest

/// One viewer per screen after a server restart. Opt-in: the harness sets
/// TEST_RUNNER_BROADWAVE_SERVER, _CONTROL, and _SHOTS. A slow restart stays
/// down until the outage is on screen. A fast one is /start alone.
final class RestartViewersTests: XCTestCase {
    private let stopped = "The server stopped. It will try again when it's back."

    private var server: String {
        lane("BROADWAVE_SERVER")
    }

    private var control: String {
        lane("BROADWAVE_CONTROL")
    }

    private var shots: String {
        lane("BROADWAVE_SHOTS")
    }

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    func testOneChannelKeepsOneViewer() throws {
        try runLayouts(["single"])
    }

    func testMultiviewKeepsOneViewerPerTile() throws {
        try runLayouts(["2up"])
    }

    private func runLayouts(_ layouts: [String]) throws {
        executionTimeAllowance = 700
        try XCTSkipIf(server.isEmpty || control.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER and _CONTROL")
        let lineup = try channels()
        let primary = try XCTUnwrap(lineup.first { $0.guideNumber == "4.1" }, "no 4.1 in \(lineup)")
        let second = try XCTUnwrap(lineup.first { $0.guideNumber == "5.1" }, "no 5.1 in \(lineup)")
        var notes: [String] = []
        for layout in layouts {
            for mode in ["slow", "fast"] {
                let name = "\(device)-\(layout)-\(mode)"
                let tiles = layout == "2up" ? 2 : 1
                let note = scenario(name: name, tiles: tiles, slow: mode == "slow", primary: primary.id, second: second.id)
                notes.append(note)
                print("broadwave-l40 \(note)")
                appendSummary(note)
            }
        }
        let failed = notes.filter { $0.contains(" FAIL ") }
        XCTAssertTrue(failed.isEmpty, failed.joined(separator: "\n"))
    }

    /// Plays, restarts, checks the viewer count, then leaves. Returns one line of numbers.
    private func scenario(name: String, tiles: Int, slow: Bool, primary: Int64, second: Int64) -> String {
        post(control, "/start", timeout: 90)
        let app = XCUIApplication()
        var args = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveSyncLog", "1",
        ]
        if tiles == 2 {
            args += ["-BroadwaveMultiview", "\(primary),\(second)", "-BroadwaveMultiviewLayout", "2up"]
        } else {
            args += ["-BroadwaveWatch", "\(primary)"]
        }
        app.launchArguments = args
        app.launch()

        guard let before = waitViewers(tiles, timeout: 70) else {
            shot(name + "-playing")
            app.terminate()
            return "\(name) FAIL did not settle on \(tiles) viewer(s): \(viewerLine())"
        }
        shot(name + "-playing")

        if slow {
            post(control, "/stop", timeout: 20)
            let showed = until(50) { self.outage(app) }
            shot(name + "-outage")
            guard showed else {
                leave(app)
                return "\(name) FAIL outage never showed before=\(before) now=\(viewerLine())"
            }
        }
        post(control, "/start", timeout: 90)
        let restarted = Date()
        var after = ""
        let back = until(50) {
            let line = self.viewerLine()
            if line == before, !self.outage(app) {
                after = line
                return true
            }
            return false
        }
        guard back else {
            shot(name + "-back")
            leave(app)
            return "\(name) FAIL picture did not return before=\(before) now=\(viewerLine()) outage=\(outage(app))"
        }
        let pictureMs = Int(Date().timeIntervalSince(restarted) * 1000)
        Thread.sleep(forTimeInterval: 30)
        let held = viewerLine()
        shot(name + "-back")
        let matched = held == after && held == before
        leave(app)
        let leftAt = Date()
        var early = ""
        if until(8, { self.viewerLine().isEmpty }) {
            early = "free in \(Int(Date().timeIntervalSince(leftAt)))s"
        }
        let remain = max(0, 60 - Date().timeIntervalSince(leftAt))
        Thread.sleep(forTimeInterval: remain)
        let gone = viewerLine()
        shot(name + "-left")
        app.terminate()
        let ok = matched && gone.isEmpty
        return "\(name) \(ok ? "ok" : "FAIL") before=\(before) after=\(after) held=\(held) pictureMs=\(pictureMs) left=\(gone.isEmpty ? "none" : gone) \(early)"
    }

    private func leave(_ app: XCUIApplication) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            for _ in 0 ..< 4 {
                remote.press(.menu)
                Thread.sleep(forTimeInterval: 1)
                if viewerLine().isEmpty {
                    return
                }
            }
        #else
            let back = app.buttons["Back to one channel"]
            if back.exists {
                back.tap()
                _ = app.buttons["Minimize"].waitForExistence(timeout: 8)
            }
            let minimize = app.buttons["Minimize"]
            if minimize.exists {
                minimize.tap()
            }
            let close = app.buttons["Close"]
            if close.waitForExistence(timeout: 3) {
                close.tap()
            }
        #endif
    }

    private func outage(_ app: XCUIApplication) -> Bool {
        let query = app.descendants(matching: .any).matching(
            NSPredicate(format: "label CONTAINS %@ OR value CONTAINS %@", stopped, stopped)
        )
        return query.firstMatch.exists
    }

    /// Viewer rows that are ours, as `guide:count` sorted. Empty when none are.
    private func viewerLine() -> String {
        guard let rows = tuners() else { return "?" }
        return rows
            .filter { ($0.ours == true) || ($0.viewers ?? 0) > 0 }
            .map { "\($0.guide ?? "-"):\($0.viewers ?? 0)" }
            .sorted()
            .joined(separator: ",")
    }

    /// Waits until there are `tiles` of our tuners and each has one viewer.
    private func waitViewers(_ tiles: Int, timeout: TimeInterval) -> String? {
        var stable = ""
        var hits = 0
        let ok = until(timeout) {
            let line = self.viewerLine()
            let parts = line.split(separator: ",").filter { !$0.isEmpty }
            let oneEach = parts.count == tiles && parts.allSatisfy { $0.hasSuffix(":1") }
            if oneEach, line == stable {
                hits += 1
                return hits >= 2
            }
            stable = oneEach ? line : ""
            hits = 0
            return false
        }
        return ok ? stable : nil
    }

    private func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while Date() < end {
            if done() {
                return true
            }
            Thread.sleep(forTimeInterval: 0.5)
        }
        return done()
    }

    private struct ChannelRow: Decodable {
        var id: Int64
        var guideNumber: String?
    }

    private func channels() throws -> [ChannelRow] {
        struct Body: Decodable { var channels: [ChannelRow] }
        let data = try get(server + "/api/v1/channels?guide=1")
        return try JSONDecoder().decode(Body.self, from: data).channels
    }

    private struct TunerRow: Decodable {
        var guide: String?
        var ours: Bool?
        var viewers: Int?
    }

    private func tuners() -> [TunerRow]? {
        guard let data = try? get(server + "/api/v1/tuners") else { return nil }
        struct Body: Decodable { var tuners: [TunerRow] }
        return try? JSONDecoder().decode(Body.self, from: data).tuners
    }

    private func get(_ raw: String) throws -> Data {
        let url = try XCTUnwrap(URL(string: raw))
        var request = URLRequest(url: url)
        request.timeoutInterval = 10
        let exp = expectation(description: raw)
        final class Box: @unchecked Sendable { var data = Data(); var error: Error? }
        let box = Box()
        URLSession.shared.dataTask(with: request) { data, response, error in
            let status = (response as? HTTPURLResponse)?.statusCode ?? 0
            if let error {
                box.error = error
            } else if !(200 ..< 300).contains(status) {
                box.error = NSError(domain: "broadwave", code: status)
            } else {
                box.data = data ?? Data()
            }
            exp.fulfill()
        }.resume()
        wait(for: [exp], timeout: 15)
        if let error = box.error {
            throw error
        }
        return box.data
    }

    private func post(_ base: String, _ path: String, timeout: TimeInterval) {
        guard let url = URL(string: base + path) else {
            XCTFail("bad url \(base)\(path)")
            return
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.timeoutInterval = timeout
        let exp = expectation(description: path)
        final class Box: @unchecked Sendable { var status = 0; var error: String? }
        let box = Box()
        URLSession.shared.dataTask(with: request) { _, response, error in
            box.status = (response as? HTTPURLResponse)?.statusCode ?? 0
            box.error = error?.localizedDescription
            exp.fulfill()
        }.resume()
        wait(for: [exp], timeout: timeout + 15)
        XCTAssertNil(box.error, box.error ?? "")
        XCTAssertEqual(box.status, 204, "\(path) status \(box.status)")
    }

    private func appendSummary(_ line: String) {
        guard !shots.isEmpty else { return }
        let url = URL(fileURLWithPath: shots).appendingPathComponent("summary.txt")
        let text = line + "\n"
        if FileManager.default.fileExists(atPath: url.path), let handle = try? FileHandle(forWritingTo: url) {
            handle.seekToEndOfFile()
            handle.write(Data(text.utf8))
            try? handle.close()
        } else {
            try? text.write(to: url, atomically: true, encoding: .utf8)
        }
    }

    private func shot(_ name: String) {
        guard !shots.isEmpty else { return }
        let url = URL(fileURLWithPath: shots).appendingPathComponent("\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
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
