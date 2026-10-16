#if os(iOS)
    import XCTest

    /// Screens for the iOS 26 and 27 touches: the caption style preview, the player's
    /// glass controls, swipe actions in the guide, the zoom into the player, and
    /// multiview's subtitles menu. Opt-in: TEST_RUNNER_BROADWAVE_SERVER (a server with
    /// listings) and TEST_RUNNER_BROADWAVE_SHOT_DIR. Tunes channels; records nothing.
    @MainActor
    final class ShowcaseTests: XCTestCase {
        func testCaptionStylePreview() throws {
            let app = try launch(["-BroadwaveTab", "settings"])
            let preview = app.descendants(matching: .any)["caption-preview"]
            XCTAssertTrue(scrollTo(preview, in: app), app.debugDescription)
            // Center it, then let the still load.
            app.swipeUp(velocity: .slow)
            if !preview.isHittable {
                app.swipeDown(velocity: .slow)
            }
            pause(3)
            shot("ios-captions")
        }

        func testGuideSwipeActions() throws {
            let app = try launch(["-BroadwaveTab", "guide"])
            let row = app.cells.firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 30), app.debugDescription)
            pause(2)
            row.swipeRight()
            XCTAssertTrue(app.buttons["Record"].waitForExistence(timeout: 5) || app.buttons["Stop"].exists, app.debugDescription)
            shot("ios-guide-swipe-record")
            row.swipeLeft()
            row.swipeLeft()
            pause(1)
            shot("ios-guide-swipe-trailing")
        }

        func testZoomIntoThePlayer() throws {
            let app = try launch(["-BroadwaveTab", "home"])
            let card = app.buttons["Watch"].firstMatch
            XCTAssertTrue(card.waitForExistence(timeout: 30), app.debugDescription)
            pause(2)
            shot("ios-home")
            card.tap()
            // Mid-zoom, then settled.
            RunLoop.current.run(until: Date().addingTimeInterval(0.18))
            shot("ios-zoom-mid")
            XCTAssertTrue(app.descendants(matching: .any)["portrait-program"].waitForExistence(timeout: 20), app.debugDescription)
            pause(12)
            shot("ios-player-portrait")
        }

        func testLandscapeControls() throws {
            let app = try launch(["-BroadwaveWatch", "28", "-BroadwaveLandscape", "YES"])
            XCTAssertTrue(app.buttons["Minimize"].waitForExistence(timeout: 30), app.debugDescription)
            pause(12)
            shot("ios-player-landscape")
        }

        func testMultiviewSubtitlesMenu() throws {
            let app = try launch(["-BroadwaveMultiview", "28,73", "-BroadwaveLandscape", "YES"])
            let menu = app.buttons["multiview-subtitles"]
            XCTAssertTrue(menu.waitForExistence(timeout: 40), app.debugDescription)
            pause(10)
            shot("ios-multiview")
            menu.tap()
            pause(2)
            shot("ios-multiview-subtitles")
        }

        /// iOS 27: the live channel in Control Center's Now Playing, and its next button as channel up.
        func testNowPlayingInControlCenter() throws {
            let app = try launch(["-BroadwaveWatch", "28", "-BroadwaveSyncLog", "1"])
            XCTAssertTrue(app.buttons["Minimize"].waitForExistence(timeout: 30), app.debugDescription)
            pause(15)
            let board = XCUIApplication(bundleIdentifier: "com.apple.springboard")
            let corner = board.coordinate(withNormalizedOffset: CGVector(dx: 0.92, dy: 0.005))
            corner.press(forDuration: 0.1, thenDragTo: board.coordinate(withNormalizedOffset: CGVector(dx: 0.92, dy: 0.6)))
            pause(3)
            shot("ios-nowplaying-cc")
            let player = board.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS[c] %@", "Now Playing")).firstMatch
            if player.exists {
                player.press(forDuration: 1)
                pause(3)
                shot("ios-nowplaying-cc-open")
            }
            let next = board.buttons.matching(NSPredicate(format: "label CONTAINS[c] %@", "next")).firstMatch
            if next.exists {
                next.tap()
                pause(6)
                shot("ios-nowplaying-cc-next")
            }
            note("ios-nowplaying-tree", board.debugDescription)
        }

        private func note(_ name: String, _ text: String) {
            guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
            try? text.write(toFile: dir + "/\(name).txt", atomically: true, encoding: .utf8)
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

        private func scrollTo(_ element: XCUIElement, in app: XCUIApplication) -> Bool {
            if element.waitForExistence(timeout: 15), element.isHittable {
                return true
            }
            for _ in 0 ..< 8 {
                app.swipeUp(velocity: .slow)
                if element.exists, element.isHittable {
                    return true
                }
            }
            return element.exists
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
#endif
