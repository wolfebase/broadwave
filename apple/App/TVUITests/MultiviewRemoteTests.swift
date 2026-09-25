import XCTest

/// Drives the Siri Remote through multiview. The app is launched offline with two tiles.
final class MultiviewRemoteTests: XCTestCase {
    func testSwipeClickPauseMenu() {
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveMultiviewTest", "YES", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()

        let first = app.buttons["tile-1"]
        XCTAssertTrue(first.waitForExistence(timeout: 15), app.debugDescription)
        let second = app.buttons["tile-3"]
        XCTAssertTrue(second.exists, app.debugDescription)

        let remote = XCUIRemote.shared
        focusFirst(remote, first: first, second: second)
        XCTAssertTrue(value(first).contains("Focused"), "first \(value(first)) second \(value(second))")
        remote.press(.right)
        XCTAssertTrue(value(second).contains("Focused"), "second \(value(second)) first \(value(first))")
        XCTAssertFalse(value(first).contains("Focused"))

        remote.press(.select)
        XCTAssertTrue(value(second).contains("Sound on"), value(second))

        remote.press(.playPause)
        XCTAssertTrue(app.staticTexts["Paused"].waitForExistence(timeout: 3), app.debugDescription)

        remote.press(.menu)
        let deadline = Date().addingTimeInterval(5)
        while first.exists, Date() < deadline {
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        }
        XCTAssertFalse(first.exists, "Menu should leave multiview")
    }

    func testLongPressOpensTheTileMenu() {
        let app = XCUIApplication()
        app.launchArguments = ["-BroadwaveMultiviewTest", "YES", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()
        let first = app.buttons["tile-1"]
        XCTAssertTrue(first.waitForExistence(timeout: 15), app.debugDescription)
        let second = app.buttons["tile-3"]
        let remote = XCUIRemote.shared
        focusFirst(remote, first: first, second: second)
        XCTAssertTrue(value(first).contains("Focused"), "first \(value(first))")
        remote.press(.select, forDuration: 1.5)
        let remove = app.descendants(matching: .any)["Remove"]
        XCTAssertTrue(remove.waitForExistence(timeout: 3), app.debugDescription)
    }

    private func focusFirst(_ remote: XCUIRemote, first: XCUIElement, second: XCUIElement) {
        for _ in 0 ..< 4 where !value(first).contains("Focused") && !value(second).contains("Focused") {
            remote.press(.down)
        }
        if !value(first).contains("Focused") {
            remote.press(.left)
        }
    }

    private func value(_ element: XCUIElement) -> String {
        element.value as? String ?? ""
    }
}
