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
