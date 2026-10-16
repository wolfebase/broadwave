import XCTest

/// Apple TV screens for the tvOS 26 and 27 work, driven with remote presses only:
/// the Top Shelf carousel (Broadwave moved to the top row of the home screen), a
/// recording's breaks in the scrubber and its Chapters list, and the player.
/// Opt-in: TEST_RUNNER_BROADWAVE_SERVER, TEST_RUNNER_BROADWAVE_SHOT_DIR, and for the
/// chapters TEST_RUNNER_BROADWAVE_RECORDING (a recording id with breaks on that server).
@MainActor
final class ShowcaseTVTests: XCTestCase {
    private let remote = XCUIRemote.shared

    /// Plays a channel first, so the server has fresh stills and the Top Shelf leads with it.
    func testTopShelfCarousel() throws {
        let app = try launch(["-BroadwaveWatch", "28"])
        XCTAssertTrue(app.otherElements["player-channel"].waitForExistence(timeout: 40) || app.wait(for: .runningForeground, timeout: 5))
        pause(15)
        remote.press(.home)
        pause(3)
        let home = XCUIApplication(bundleIdentifier: "com.apple.HeadBoard")
        let icon = home.descendants(matching: .any).matching(NSPredicate(format: "label == %@", "Broadwave")).firstMatch
        XCTAssertTrue(icon.waitForExistence(timeout: 10), home.debugDescription)
        shot("tv-home-before")
        if !inTopRow(icon, home) {
            XCTAssertTrue(focus(icon), "could not focus Broadwave\n\(home.debugDescription)")
            moveToTopRow(home)
        }
        XCTAssertTrue(focus(icon), home.debugDescription)
        // The shelf loads after focus settles; give it the stills.
        pause(12)
        shot("tv-topshelf")
        remote.press(.up)
        pause(4)
        shot("tv-topshelf-focused")
        remote.press(.right)
        pause(4)
        shot("tv-topshelf-next")
    }

    func testRecordingChaptersAndBreaks() throws {
        guard let id = ProcessInfo.processInfo.environment["BROADWAVE_RECORDING"], !id.isEmpty else {
            throw XCTSkip("Set TEST_RUNNER_BROADWAVE_RECORDING")
        }
        let app = try launch([])
        pause(6)
        try app.open(XCTUnwrap(URL(string: "broadwave://recording/\(id)")))
        // A recording link plays it at once.
        pause(16)
        remote.press(.playPause)
        pause(2)
        shot("tv-recording-scrubber")
        remote.press(.down)
        pause(3)
        shot("tv-recording-info")
        remote.press(.right)
        pause(2)
        shot("tv-recording-chapters")
    }

    func testLivePlayer() throws {
        let app = try launch(["-BroadwaveWatch", "28", "-BroadwaveSyncLog", "1"])
        pause(20)
        remote.press(.playPause)
        pause(1)
        remote.press(.playPause)
        pause(2)
        shot("tv-live-player")
        remote.press(.down)
        pause(3)
        shot("tv-live-info")
        _ = app
    }

    private func inTopRow(_ icon: XCUIElement, _ home: XCUIApplication) -> Bool {
        icon.frame.minY < home.frame.height * 0.55 && icon.frame.minY > home.frame.height * 0.3
    }

    /// Walks focus across the home screen until the icon has it.
    private func focus(_ icon: XCUIElement) -> Bool {
        for direction in [XCUIRemote.Button.down, .right, .down, .right, .down, .right, .down, .right, .down, .right] {
            if icon.hasFocus {
                return true
            }
            for _ in 0 ..< 6 where !icon.hasFocus {
                remote.press(direction)
                pause(0.4)
            }
        }
        // Back to the top-left and across each row.
        for _ in 0 ..< 8 {
            remote.press(.up)
        }
        for _ in 0 ..< 6 {
            for _ in 0 ..< 8 where !icon.hasFocus {
                remote.press(.right)
                pause(0.3)
            }
            if icon.hasFocus {
                return true
            }
            for _ in 0 ..< 8 {
                remote.press(.left)
            }
            remote.press(.down)
            pause(0.4)
        }
        return icon.hasFocus
    }

    /// Hold Select, choose Move, then Up until it reaches the top row.
    private func moveToTopRow(_ home: XCUIApplication) {
        remote.press(.select, forDuration: 2)
        pause(1.5)
        shot("tv-home-menu")
        let move = home.buttons["Move"]
        if move.waitForExistence(timeout: 3) {
            for _ in 0 ..< 4 where !move.hasFocus {
                remote.press(.down)
                pause(0.3)
            }
            remote.press(.select)
            pause(1)
        }
        for _ in 0 ..< 10 {
            remote.press(.up)
            pause(0.5)
        }
        for _ in 0 ..< 8 {
            remote.press(.left)
            pause(0.3)
        }
        remote.press(.select)
        pause(2)
    }

    private func launch(_ extra: [String]) throws -> XCUIApplication {
        guard let server = ProcessInfo.processInfo.environment["BROADWAVE_SERVER"], !server.isEmpty else {
            throw XCTSkip("Set TEST_RUNNER_BROADWAVE_SERVER")
        }
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveServerURL", server, "-ApplePersistenceIgnoreState", "YES"] + extra
        app.launch()
        return app
    }

    private func pause(_ seconds: TimeInterval) {
        RunLoop.current.run(until: Date().addingTimeInterval(seconds))
    }

    private func shot(_ name: String) {
        guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: folder.appending(path: "\(name).png"))
    }
}
