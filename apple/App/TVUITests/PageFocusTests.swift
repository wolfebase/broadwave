import XCTest

/// Apple TV has no tab sidebar. The guide opens on a channel, its icons open the other
/// pages, and each page takes focus when it opens. Back returns to the guide.
/// Offline: `-BroadwaveFocus` shows a two-channel preview lineup.
final class PageFocusTests: XCTestCase {
    func testGuideTakesFocus() {
        assertFocused("4.1 One", "guide", byLabel: true)
    }

    /// Up reaches the filter row, Right the icons. Settings and Search open over the
    /// guide and Back returns to the guide, never to a menu or the Home screen.
    func testIconsOpenPagesAndBackReturns() {
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveFocus", "guide", "-ApplePersistenceIgnoreState", "YES", "-lastChannel", "0"]
        app.launch()
        let channel = app.buttons["4.1 One"].firstMatch
        XCTAssertTrue(channel.waitForExistence(timeout: 15), app.debugDescription)
        XCTAssertTrue(until(8) { channel.hasFocus }, "the guide did not open on a channel: \(focused(app))")
        shot(app, "tv-guide-launch")
        XCTAssertFalse(app.buttons["Guide"].exists, "a Guide tab is still on screen")

        for (icon, page) in [("guide-go-settings", "home-scan"), ("guide-go-search", "search-field")] {
            let button = app.buttons[icon]
            XCTAssertTrue(button.exists, "no \(icon)\n\(app.debugDescription)")
            var trail: [String] = []
            let moves = Array(repeating: XCUIRemote.Button.up, count: 3) + Array(repeating: .right, count: 14) + Array(repeating: .left, count: 6)
            for move in moves where !button.hasFocus {
                XCUIRemote.shared.press(move)
                pause(0.4)
                trail.append(focused(app))
            }
            XCTAssertTrue(button.hasFocus, "could not reach \(icon): \(trail)")
            XCUIRemote.shared.press(.select)
            let target = app.descendants(matching: .any)[page]
            XCTAssertTrue(target.waitForExistence(timeout: 10), "\(icon) did not open its page\n\(app.debugDescription)")
            XCTAssertTrue(until(6) { target.hasFocus }, "\(page) did not take focus: \(focused(app))")
            shot(app, icon)
            XCUIRemote.shared.press(.menu)
            // Back returns to the guide where the viewer was: on the icon.
            XCTAssertTrue(until(8) { channel.isHittable && button.hasFocus }, "Back from \(icon) did not return to the guide: \(focused(app))")
            XCTAssertEqual(app.state, .runningForeground)
        }
    }

    func testSettingsTakesFocus() {
        assertFocused("home-scan", "settings")
    }

    func testSearchTakesFocus() {
        assertFocused("search-field", "search")
    }

    /// Diagnostics is pushed from Settings; its first row takes focus.
    func testDiagnosticsTakesFocus() {
        let app = XCUIApplication()
        app.launchArguments = [
            "-BroadwaveFocus", "settings",
            "-BroadwaveDiagnostics", "YES",
            "-ApplePersistenceIgnoreState", "YES",
        ]
        app.launch()
        let row = app.descendants(matching: .any)["diagnostics-first"]
        XCTAssertTrue(row.waitForExistence(timeout: 15), app.debugDescription)
        let deadline = Date().addingTimeInterval(8)
        while Date() < deadline, !row.hasFocus {
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        }
        XCTAssertTrue(row.hasFocus, "first row should have focus\n\(focused(app))\n\(app.debugDescription)")
        shot(app, "tv-diagnostics")
    }

    private func focused(_ app: XCUIApplication) -> String {
        let element = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        guard element.exists else { return "nothing" }
        return "\(element.identifier) \(element.label)"
    }

    private func shot(_: XCUIApplication, _ name: String) {
        guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let url = folder.appending(path: "\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
        print("l49-shot \(url.path)")
    }

    private func pause(_ seconds: TimeInterval) {
        RunLoop.current.run(until: Date().addingTimeInterval(seconds))
    }

    private func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while Date() < end {
            if done() {
                return true
            }
            pause(0.2)
        }
        return done()
    }

    private func assertFocused(_ identifier: String, _ page: String, byLabel: Bool = false) {
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveFocus", page, "-ApplePersistenceIgnoreState", "YES", "-lastChannel", "0"]
        app.launch()
        let control = byLabel ? app.buttons[identifier].firstMatch : app.descendants(matching: .any)[identifier]
        XCTAssertTrue(control.waitForExistence(timeout: 15), app.debugDescription)
        let deadline = Date().addingTimeInterval(6)
        while Date() < deadline {
            if control.hasFocus {
                return
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        }
        XCTFail("\(identifier) should take focus when \(page) opens: \(focused(app))")
    }
}
