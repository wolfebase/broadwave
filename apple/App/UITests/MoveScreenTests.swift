import XCTest

/// The player's Move to another screen lists the other open screens, and
/// choosing one closes the player here.
/// Opt-in: TEST_RUNNER_BROADWAVE_SERVER (a local harness), TEST_RUNNER_BROADWAVE_WATCH
/// (a channel id), and TEST_RUNNER_BROADWAVE_MOVE_TO (the other screen's name, open
/// on the same server). TEST_RUNNER_BROADWAVE_MOVE_SEND=1 also sends the channel there.
@MainActor
final class MoveScreenTests: XCTestCase {
    func testTheOtherScreenIsListedAndTakesTheChannel() throws {
        #if os(iOS)
            executionTimeAllowance = 180
            let server = lane("BROADWAVE_SERVER")
            let channel = lane("BROADWAVE_WATCH")
            let target = lane("BROADWAVE_MOVE_TO")
            try XCTSkipIf(server.isEmpty || channel.isEmpty || target.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER, _WATCH, and _MOVE_TO")

            let app = XCUIApplication()
            app.launchArguments = [
                "-ApplePersistenceIgnoreState", "YES",
                "-BroadwaveServerURL", server,
                "-BroadwaveWatch", channel,
            ]
            app.launch()

            let move = app.buttons["portrait-move"]
            XCTAssertTrue(move.waitForExistence(timeout: 30), app.debugDescription)
            move.tap()
            let row = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", target)).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 15), "no \(target) in the list\n\(app.debugDescription)")
            XCTAssertTrue(row.identifier.hasPrefix("move-screen-"), row.identifier)
            shot(app, "move-list")
            print("i5 listed \(row.label)")

            guard lane("BROADWAVE_MOVE_SEND") == "1" else { return }
            row.tap()
            // The sender waits until the other screen plays it.
            let starting = app.descendants(matching: .any)["move-screens-status"]
            if starting.waitForExistence(timeout: 3) {
                shot(app, "move-starting")
                print("i5 status \(starting.label)")
            }
            let gone = NSPredicate(format: "exists == false")
            expectation(for: gone, evaluatedWith: app.buttons["portrait-move"])
            waitForExpectations(timeout: 25)
            shot(app, "moved")
            print("i5 moved to \(target)")
        #else
            throw XCTSkip("Apple TV sends from its Move to another screen panel")
        #endif
    }

    private func shot(_ app: XCUIApplication, _ name: String) {
        let dir = lane("BROADWAVE_SHOT_DIR")
        guard !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let url = folder.appending(path: "iphone-\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
        print("i5-shot \(url.path)")
        _ = app
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
