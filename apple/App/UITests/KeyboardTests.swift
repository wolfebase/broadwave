import XCTest

/// iPad with a hardware keyboard: the player keys match the web player, Command-number
/// changes the page, and the guide keeps the docked picture on screen.
/// Opt-in: BROADWAVE_SERVER, the lineup from `scripts/demo-lineup.sh` (4.1, then 5.1
/// with Sintel on now; no tuner needed).
@MainActor
final class KeyboardTests: XCTestCase {
    func testKeyboardOnIPad() throws {
        #if os(tvOS)
            throw XCTSkip("a hardware keyboard is iPhone and iPad")
        #else
            executionTimeAllowance = 240
            let server = lane("BROADWAVE_SERVER")
            try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
            try XCTSkipIf(UIDevice.current.userInterfaceIdiom != .pad, "the guide preview is iPad only")
            let channel = lane("BROADWAVE_CHANNEL").isEmpty ? "1" : lane("BROADWAVE_CHANNEL")

            let app = XCUIApplication()
            app.launchArguments = [
                "-ApplePersistenceIgnoreState", "YES",
                "-BroadwaveServerURL", server,
                "-BroadwaveWatch", channel,
                "-BroadwaveChannelNow", "YES",
            ]
            app.launch()
            let now = app.staticTexts["channel-now"]
            XCTAssertTrue(now.waitForExistence(timeout: 30), app.debugDescription)
            let first = now.label

            app.typeKey(.downArrow, modifierFlags: [])
            XCTAssertTrue(wait { now.label != first }, "Down did not change channel from \(first)")
            shot("01-next-channel")
            app.typeKey(.upArrow, modifierFlags: [])
            XCTAssertTrue(wait { now.label == first }, "Up did not come back to \(first), on \(now.label)")

            app.typeKey("g", modifierFlags: [])
            XCTAssertTrue(app.buttons["Close guide"].waitForExistence(timeout: 5), "G opened no channel list")
            shot("02-channels")
            // The Simulator never hands Escape to the app, so this checks Command-period, its stand-in.
            app.typeKey(".", modifierFlags: .command)
            XCTAssertTrue(wait { !app.buttons["Close guide"].exists }, "Command-period left the channel list open")

            app.typeKey(".", modifierFlags: .command)
            XCTAssertTrue(app.descendants(matching: .any)["miniPlayer"].waitForExistence(timeout: 10), "Command-period did not minimize")

            app.typeKey("2", modifierFlags: .command)
            let preview = app.descendants(matching: .any)["guide-preview"]
            XCTAssertTrue(preview.waitForExistence(timeout: 10), "the guide shows no docked picture\n\(app.debugDescription)")
            shot("03-guide-preview")

            app.typeKey("3", modifierFlags: .command)
            XCTAssertTrue(app.navigationBars["Sports"].waitForExistence(timeout: 10), "Command-3 did not open Sports")
            app.typeKey("2", modifierFlags: .command)
            XCTAssertTrue(preview.waitForExistence(timeout: 10))
            preview.tap()
            XCTAssertTrue(now.waitForExistence(timeout: 10), "the preview did not open the player")

            app.typeKey("m", modifierFlags: [])
            let back = app.buttons["Back to one channel"]
            XCTAssertTrue(back.waitForExistence(timeout: 10), "M did not open multiview")
            shot("04-multiview")
            app.typeKey(".", modifierFlags: .command)
            XCTAssertTrue(wait { !back.exists } && now.waitForExistence(timeout: 10), "Command-period did not leave multiview")

            // A program picked in the guide shows its channel there, and Preview plays it in place.
            app.typeKey(".", modifierFlags: .command)
            app.typeKey("2", modifierFlags: .command)
            XCTAssertTrue(preview.waitForExistence(timeout: 10))
            let other = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", "Sintel")).firstMatch
            XCTAssertTrue(other.waitForExistence(timeout: 10), "no Sintel program in the guide")
            other.tap()
            let start = app.buttons["guide-preview-start"]
            XCTAssertTrue(start.waitForExistence(timeout: 10), "a picked channel offers no Preview\n\(app.debugDescription)")
            shot("05-picked")
            start.tap()
            let mini = app.descendants(matching: .any)["miniPlayer"]
            XCTAssertTrue(wait(15) { mini.exists && mini.label.contains("5.1") && preview.exists }, "Preview did not play 5.1 in the guide")
            XCTAssertFalse(start.exists, "Preview still offered for the channel playing")
            shot("06-previewing")
        #endif
    }

    private func wait(_ seconds: Double = 8, _ done: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while Date() < end {
            if done() {
                return true
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.25))
        }
        return done()
    }

    private func shot(_ name: String) {
        let dir = lane("BROADWAVE_SHOT_DIR")
        guard !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: folder.appending(path: "keys-\(name).png"))
    }

    private func lane(_ name: String) -> String {
        let env = ProcessInfo.processInfo.environment
        if let direct = env[name], !direct.isEmpty {
            return direct
        }
        return env["TEST_RUNNER_\(name)"] ?? ""
    }
}
