import XCTest

/// Pauses and resumes live TV with the remote against a real server. Opt-in:
/// run with TEST_RUNNER_BROADWAVE_SERVER=http://host:port (and optionally
/// TEST_RUNNER_BROADWAVE_CHANNEL). The app's sync log shows whether the
/// screen locks back onto its room after the pause.
final class PauseResumeTests: XCTestCase {
    func testPauseAndResumeKeepPlaying() throws {
        let env = ProcessInfo.processInfo.environment
        guard let server = env["BROADWAVE_SERVER"], !server.isEmpty else {
            throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER to run against a server")
        }
        let app = XCUIApplication()
        app.launchArguments = [
            "-BroadwaveServerURL", server, "-BroadwaveWatch", env["BROADWAVE_CHANNEL"] ?? "1",
            "-BroadwaveSyncLog", "1", "-ApplePersistenceIgnoreState", "YES",
        ]
        app.launch()
        let remote = XCUIRemote.shared
        wait(40)
        remote.press(.playPause)
        wait(8)
        remote.press(.playPause)
        wait(60)
        XCTAssertEqual(app.state, .runningForeground)
    }

    /// Leaving for the Home screen pauses the player; that is not the viewer's pause.
    func testHomeAndBackKeepsSync() throws {
        let env = ProcessInfo.processInfo.environment
        guard let server = env["BROADWAVE_SERVER"], !server.isEmpty else {
            throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER to run against a server")
        }
        let app = XCUIApplication()
        app.launchArguments = [
            "-BroadwaveServerURL", server, "-BroadwaveWatch", env["BROADWAVE_CHANNEL"] ?? "1",
            "-BroadwaveSyncLog", "1", "-ApplePersistenceIgnoreState", "YES",
        ]
        app.launch()
        wait(40)
        XCUIRemote.shared.press(.home)
        wait(6)
        app.activate()
        wait(40)
        XCTAssertEqual(app.state, .runningForeground)
    }

    private func wait(_ seconds: TimeInterval) {
        RunLoop.current.run(until: Date().addingTimeInterval(seconds))
    }
}
