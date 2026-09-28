import XCTest

/// Home for several minutes, then back. The offline note stays hidden when the
/// socket returns quickly, and shows once the server has been gone for 3 seconds.
/// Opt-in: TEST_RUNNER_BROADWAVE_SERVER, CHANNEL, LOG, SHOTS, MARKS, HOME_SECONDS.
final class SleepWakeTests: XCTestCase {
    private var server: String {
        env("BROADWAVE_SERVER")
    }

    private var channel: String {
        env("BROADWAVE_CHANNEL")
    }

    private var logPath: String {
        env("BROADWAVE_LOG")
    }

    private var shots: String {
        env("BROADWAVE_SHOTS")
    }

    private var marks: String {
        env("BROADWAVE_MARKS")
    }

    private var homeSeconds: TimeInterval {
        let raw = env("BROADWAVE_HOME_SECONDS")
        return TimeInterval(raw).flatMap { $0 > 0 ? $0 : nil } ?? 300
    }

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    func testHomeThenBackHidesAFastReconnectAndShowsADeadServer() throws {
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveWatch", channel.isEmpty ? "1" : channel,
            "-BroadwaveSyncLog", "1",
        ]
        app.launch()
        XCTAssertTrue(waitForLog("state=locked", timeout: 90), "playback did not lock\n\(logTail())")
        shot("tv-playing")
        let banner = app.descendants(matching: .any)["offline-banner"]
        XCTAssertFalse(banner.exists, "offline note was up while the server answered")

        mark("home")
        XCUIRemote.shared.press(.home)
        Thread.sleep(forTimeInterval: 1)
        let away = app.state != .runningForeground
        mark("home-state \(app.state.rawValue) away=\(away)")
        shot("tv-home")
        Thread.sleep(forTimeInterval: max(homeSeconds - 1, 0))

        let since = logCount()
        mark("activate")
        app.activate()
        let back = Date()
        var showedWhileReturning = false
        while Date().timeIntervalSince(back) < 8 {
            if banner.exists {
                showedWhileReturning = true
                break
            }
            Thread.sleep(forTimeInterval: 0.25)
        }
        let fresh = logSuffix(since)
        let gap = reconnectGapMS(fresh)
        if showedWhileReturning {
            XCTAssertGreaterThanOrEqual(gap ?? 0, 3000, "offline note during a \(gap.map { "\($0)ms" } ?? "still-open") reconnect\n\(logTail())")
        }
        XCTAssertTrue(waitForLog("state=locked", since: since, timeout: 45), "did not lock after Home\n\(logTail())")
        shot("tv-back")
        mark("returned gap=\(gap.map(String.init) ?? "up") banner=\(showedWhileReturning)")

        mark("stop-server")
        let killed = try waitForStopped()
        let early = killed.addingTimeInterval(2.5)
        while Date() < early {
            XCTAssertFalse(banner.exists, "offline note before 3s (\(Date().timeIntervalSince(killed))s)")
            Thread.sleep(forTimeInterval: 0.1)
        }
        var shownAt: TimeInterval?
        let deadline = killed.addingTimeInterval(15)
        while Date() < deadline {
            if banner.exists {
                shownAt = Date().timeIntervalSince(killed)
                break
            }
            Thread.sleep(forTimeInterval: 0.1)
        }
        XCTAssertNotNil(shownAt, "offline note never appeared after the server stopped\n\(logTail())")
        mark(String(format: "banner %.2f", shownAt ?? -1))
        shot("tv-offline")
    }

    private func env(_ key: String) -> String {
        ProcessInfo.processInfo.environment[key] ?? ""
    }

    private func mark(_ name: String) {
        let line = "\(name) \(Int(Date().timeIntervalSince1970 * 1000))\n"
        guard !marks.isEmpty, let data = line.data(using: .utf8) else {
            print("broadwave-test \(name)")
            return
        }
        if FileManager.default.fileExists(atPath: marks), let handle = FileHandle(forWritingAtPath: marks) {
            handle.seekToEndOfFile()
            handle.write(data)
            try? handle.close()
        } else {
            try? data.write(to: URL(fileURLWithPath: marks))
        }
        print("broadwave-test \(name)")
    }

    /// The host writes `stopped <epoch ms>` after it kills the server.
    private func waitForStopped() throws -> Date {
        let deadline = Date().addingTimeInterval(20)
        while Date() < deadline {
            let text = (try? String(contentsOfFile: marks, encoding: .utf8)) ?? ""
            let line = text.split(separator: "\n").last { $0.hasPrefix("stopped ") }
            if let line, let ms = Int(line.split(separator: " ").last ?? "") {
                return Date(timeIntervalSince1970: Double(ms) / 1000)
            }
            Thread.sleep(forTimeInterval: 0.1)
        }
        XCTFail("the host did not stop the server")
        return Date()
    }

    private func shot(_ name: String) {
        guard !shots.isEmpty else { return }
        let image = XCUIScreen.main.screenshot()
        let url = URL(fileURLWithPath: shots).appendingPathComponent("\(name).png")
        try? image.pngRepresentation.write(to: url)
    }

    private func logText() -> String {
        guard !logPath.isEmpty else { return "" }
        return (try? String(contentsOfFile: logPath, encoding: .utf8)) ?? ""
    }

    private func logCount() -> Int {
        logText().count
    }

    private func logSuffix(_ since: Int) -> String {
        let text = logText()
        guard text.count > since else { return "" }
        return String(text.dropFirst(since))
    }

    private func logTail() -> String {
        let text = logText()
        return String(text.suffix(1500))
    }

    private func waitForLog(_ needle: String, since: Int = 0, timeout: TimeInterval) -> Bool {
        let end = Date().addingTimeInterval(timeout)
        while Date() < end {
            if logSuffix(since).contains(needle) {
                return true
            }
            Thread.sleep(forTimeInterval: 0.25)
        }
        return logSuffix(since).contains(needle)
    }

    /// Milliseconds from the first socket drop after this slice to the next open.
    /// Nil when the socket never dropped.
    private func reconnectGapMS(_ text: String) -> Int? {
        let pattern = #"broadwave socket (down|open) (\d+)"#
        guard let regex = try? NSRegularExpression(pattern: pattern) else { return nil }
        let ns = text as NSString
        var down: Int?
        for match in regex.matches(in: text, range: NSRange(location: 0, length: ns.length)) {
            let kind = ns.substring(with: match.range(at: 1))
            let ms = Int(ns.substring(with: match.range(at: 2))) ?? 0
            if kind == "down", down == nil {
                down = ms
            } else if kind == "open", let down, ms >= down {
                return ms - down
            }
        }
        return nil
    }
}
