import XCTest

/// Broadwave's App Shortcuts are listed in the Shortcuts app, and "What's on" answers
/// from the saved server. Opt-in: BROADWAVE_SERVER.
@MainActor
final class AppShortcutsTests: XCTestCase {
    func testWhatsOnFromShortcuts() throws {
        #if os(tvOS)
            throw XCTSkip("Shortcuts is iPhone and iPad only")
        #else
            executionTimeAllowance = 180
            let server = lane("BROADWAVE_SERVER")
            try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")

            // Connecting saves the server where the intents read it.
            let app = XCUIApplication()
            app.launchArguments = ["-ApplePersistenceIgnoreState", "YES", "-BroadwaveServerURL", server]
            app.launch()
            XCTAssertTrue(app.buttons["Home"].waitForExistence(timeout: 25), app.debugDescription)

            let shortcuts = XCUIApplication(bundleIdentifier: "com.apple.shortcuts")
            shortcuts.terminate()
            shortcuts.activate()
            let library = shortcuts.tabBars.buttons["Library"].firstMatch
            if library.waitForExistence(timeout: 10) {
                library.tap()
            }
            // Library's Apps list, then Broadwave's page, which lists every App Shortcut.
            let page = shortcuts.buttons["Broadwave"].firstMatch
            if page.waitForExistence(timeout: 5) {
                page.tap()
            }
            let tile = shortcuts.descendants(matching: .any)
                .matching(NSPredicate(format: "label BEGINSWITH %@", "What")).matching(NSPredicate(format: "label CONTAINS %@", "on")).firstMatch
            XCTAssertTrue(tile.waitForExistence(timeout: 15), "no What's on tile\n\(shortcuts.debugDescription)")
            for title in ["Watch a channel", "Watch a game", "Record a show", "Start multiview"] {
                let other = shortcuts.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", title)).firstMatch
                XCTAssertTrue(other.exists, "no \(title) tile")
            }
            shot("shortcuts-library")
            tile.tap()

            // The answer shows as a snippet over Shortcuts.
            let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
            let answers = ["Nothing in the guide right now.", "Open Broadwave to pick your server.", "Can't reach your server."]
            let pattern = NSPredicate(format: "label MATCHES %@ OR label IN %@", #".*\d+\.\d+, .+\."#, answers)
            var found: XCUIElement?
            let end = Date().addingTimeInterval(20)
            while found == nil, Date() < end {
                for host in [shortcuts, springboard] {
                    let hit = host.descendants(matching: .any).matching(pattern).firstMatch
                    if hit.exists {
                        found = hit
                        break
                    }
                }
                RunLoop.current.run(until: Date().addingTimeInterval(0.5))
            }
            shot("shortcuts-whats-on")
            let answer = try XCTUnwrap(found, "no answer\n\(shortcuts.debugDescription)\n\(springboard.debugDescription)")
            print("i4 whats-on \(answer.label)")
            XCTAssertNotEqual(answer.label, "Open Broadwave to pick your server.")
            XCTAssertNotEqual(answer.label, "Can't reach your server.")
        #endif
    }

    /// The links the intents hand to OpenURLIntent open the player and multiview.
    func testIntentLinks() throws {
        #if os(tvOS)
            throw XCTSkip("covered by the tvOS link tests")
        #else
            executionTimeAllowance = 180
            let server = lane("BROADWAVE_SERVER")
            try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
            let app = XCUIApplication()
            app.launchArguments = ["-ApplePersistenceIgnoreState", "YES", "-BroadwaveServerURL", server]
            app.launch()
            XCTAssertTrue(app.buttons["Home"].waitForExistence(timeout: 25), app.debugDescription)

            follow(app, "broadwave://watch/2")
            let player = app.descendants(matching: .any)["player-channel"]
            XCTAssertTrue(player.waitForExistence(timeout: 20), "no player\n\(app.debugDescription)")
            let end = Date().addingTimeInterval(15)
            while (player.value as? String) != "4.2", Date() < end {
                RunLoop.current.run(until: Date().addingTimeInterval(0.25))
            }
            let watched = player.value as? String
            XCTAssertEqual(watched, "4.2")
            shot("link-watch-2")

            follow(app, "broadwave://multiview?ch=1,3")
            let left = app.descendants(matching: .any)["tile-1"]
            let right = app.descendants(matching: .any)["tile-3"]
            XCTAssertTrue(left.waitForExistence(timeout: 20), "no tile 1\n\(app.debugDescription)")
            XCTAssertTrue(right.waitForExistence(timeout: 5), "no tile 3\n\(app.debugDescription)")
            RunLoop.current.run(until: Date().addingTimeInterval(4))
            shot("link-multiview-1-3")
            print("i4 links watch=\(watched ?? "") multiview tiles=\(left.exists),\(right.exists)")
        #endif
    }

    /// Opens a link the way the system does, answering the simulator's "Open in" prompt.
    private func follow(_ app: XCUIApplication, _ link: String) {
        app.open(URL(string: link)!)
        let open = XCUIApplication(bundleIdentifier: "com.apple.springboard").buttons["Open"]
        if open.waitForExistence(timeout: 3) {
            open.tap()
        }
    }

    private func shot(_ name: String) {
        let screen = XCUIScreen.main.screenshot()
        let attachment = XCTAttachment(screenshot: screen)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
        let dir = lane("BROADWAVE_SHOT_DIR")
        guard !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        try? screen.pngRepresentation.write(to: folder.appending(path: "\(name).png"))
    }

    private func lane(_ name: String) -> String {
        let env = ProcessInfo.processInfo.environment
        if let direct = env[name], !direct.isEmpty {
            return direct
        }
        return env["TEST_RUNNER_\(name)"] ?? ""
    }
}
