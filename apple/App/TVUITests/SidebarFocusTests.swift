import XCTest

/// The Apple TV sidebar stays open over a tab until that tab's content takes focus.
/// Guide, Settings, and Search used to leave it open, which covered the page.
final class SidebarFocusTests: XCTestCase {
    func testGuideTakesFocus() {
        assertFocused("guide-now", "guide")
    }

    func testSettingsTakesFocus() {
        assertFocused("home-scan", "settings")
    }

    func testSearchTakesFocus() {
        assertFocused("search-field", "search")
    }

    /// Diagnostics is pushed from Settings. The sidebar stays open until the first row takes focus.
    func testDiagnosticsCollapsesTheSidebar() {
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
        let settings = app.buttons["Settings"].firstMatch
        XCTAssertTrue(settings.exists, app.debugDescription)
        XCTAssertFalse(settings.isEnabled, "sidebar still covers the page\n\(focused(app))\n\(app.debugDescription)")
        shot(app)
    }

    private func focused(_ app: XCUIApplication) -> String {
        let element = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        guard element.exists else { return "nothing" }
        return "\(element.identifier) \(element.label)"
    }

    private func shot(_: XCUIApplication) {
        guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let url = folder.appending(path: "tv-diagnostics.png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
        print("l49-shot \(url.path)")
    }

    private func assertFocused(_ identifier: String, _ page: String) {
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveFocus", page, "-ApplePersistenceIgnoreState", "YES"]
        app.launch()
        let control = app.descendants(matching: .any)[identifier]
        XCTAssertTrue(control.waitForExistence(timeout: 15), app.debugDescription)
        let deadline = Date().addingTimeInterval(6)
        while Date() < deadline {
            if control.hasFocus {
                return
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        }
        XCTFail("\(identifier) should take focus so the sidebar closes")
    }
}
