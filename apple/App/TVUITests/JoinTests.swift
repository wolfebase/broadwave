import XCTest

/// Joining a home server from away with an invite code, using only the remote.
/// Needs a live server: TEST_RUNNER_BROADWAVE_JOIN_SERVER (such as https://tv.example.com)
/// and TEST_RUNNER_BROADWAVE_JOIN_CODE, an unused invite. Each run spends the invite.
final class JoinTests: XCTestCase {
    override func setUp() {
        continueAfterFailure = false
    }

    func testInviteCodeJoinsAndOpensTheGuide() throws {
        let env = ProcessInfo.processInfo.environment
        guard let server = env["BROADWAVE_JOIN_SERVER"], !server.isEmpty,
              let code = env["BROADWAVE_JOIN_CODE"], !code.isEmpty
        else {
            throw XCTSkip("set TEST_RUNNER_BROADWAVE_JOIN_SERVER and TEST_RUNNER_BROADWAVE_JOIN_CODE")
        }
        let app = XCUIApplication()
        // BroadwaveDiscover skips the saved server and the network explainer, so this starts on the finder.
        app.launchArguments = ["-BroadwaveDiscover", "YES", "-ApplePersistenceIgnoreState", "YES"]
        app.launch()

        let open = app.buttons["join-open"]
        XCTAssertTrue(open.waitForExistence(timeout: 30), app.debugDescription)
        focus(open, in: app)
        XCUIRemote.shared.press(.select)

        let address = app.textFields["join-address"]
        XCTAssertTrue(address.waitForExistence(timeout: 10), app.debugDescription)
        type(server, into: address, in: app)
        type(code, into: app.textFields["join-code"], in: app)

        let join = app.buttons["join-submit"]
        focus(join, in: app)
        XCUIRemote.shared.press(.select)

        XCTAssertTrue(app.descendants(matching: .any)["guide-now"].waitForExistence(timeout: 60), app.debugDescription)
        XCTAssertFalse(app.descendants(matching: .any)["signed-out"].exists)
    }

    /// Moves focus with the remote until the element has it. Down first, then up.
    private func focus(_ element: XCUIElement, in app: XCUIApplication) {
        for direction in [XCUIRemote.Button.down, .up] {
            for _ in 0 ..< 20 {
                if element.hasFocus {
                    return
                }
                XCUIRemote.shared.press(direction)
            }
        }
        XCTAssertTrue(element.hasFocus, "Could not focus \(element). \(app.debugDescription)")
    }

    /// Selects the field, types on the keyboard, and returns to the form.
    private func type(_ text: String, into field: XCUIElement, in app: XCUIApplication) {
        focus(field, in: app)
        XCUIRemote.shared.press(.select)
        let keyboard = app.keyboards.firstMatch
        _ = keyboard.waitForExistence(timeout: 5)
        app.typeText(text)
        // Return closes the tvOS keyboard and keeps the text.
        app.typeText("\n")
        _ = keyboard.waitForNonExistence(timeout: 5)
    }
}
