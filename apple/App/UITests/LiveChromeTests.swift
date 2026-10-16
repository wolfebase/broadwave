#if os(iOS)
    import XCTest

    /// The live player's controls on iPhone and iPad: one panel at a time, a swipe that
    /// puts a panel away does not change the channel, a tap outside closes it, and over the
    /// picture the controls hide after 3 s and come back with a tap.
    /// Opt-in: TEST_RUNNER_BROADWAVE_SERVER (a local harness is enough) and
    /// TEST_RUNNER_BROADWAVE_SHOT_DIR for screenshots.
    @MainActor
    final class LiveChromeTests: XCTestCase {
        private var server: String {
            ProcessInfo.processInfo.environment["BROADWAVE_SERVER"] ?? ""
        }

        override func setUp() {
            super.setUp()
            continueAfterFailure = false
            XCUIDevice.shared.orientation = .portrait
        }

        func testASwipeThatClosesAPanelKeepsTheChannel() throws {
            // On iPad the controls float and hide; this test is about panels, so they stay.
            let app = try launch(stays: true)
            let now = app.staticTexts["channel-now"]
            XCTAssertTrue(until(30) { now.exists && !now.label.isEmpty }, "no channel")
            let channel = now.label
            let channels = app.buttons["portrait-channels"]
            XCTAssertTrue(channels.waitForExistence(timeout: 30), app.debugDescription)
            pause(4)

            channels.tap()
            let list = app.descendants(matching: .any)["portrait-guide"]
            XCTAssertTrue(list.waitForExistence(timeout: 5), "the channel list did not open")
            XCTAssertTrue(app.buttons["Close guide"].exists, "the list has no close button")
            shot("panel-open")

            // A swipe down on the picture, as one would put a sheet away.
            let pad = app.descendants(matching: .any)["player-gesture"]
            let top = pad.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.3))
            top.press(forDuration: 0.05, thenDragTo: pad.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.55)))
            XCTAssertTrue(until(3) { !list.exists }, "the swipe left the list open")
            pause(1.5)
            XCTAssertEqual(now.label, channel, "the swipe that closed the list changed the channel")
            shot("panel-swiped-away")

            // A tap on the picture outside the list closes it too.
            channels.tap()
            XCTAssertTrue(list.waitForExistence(timeout: 5))
            pad.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.32)).tap()
            XCTAssertTrue(until(3) { !list.exists }, "a tap outside left the list open")
            XCTAssertEqual(now.label, channel)

            // One panel at a time: stream details replace the list.
            channels.tap()
            XCTAssertTrue(list.waitForExistence(timeout: 5))
            app.buttons["player-more"].tap()
            app.buttons["portrait-stream"].tap()
            XCTAssertTrue(until(3) { !list.exists }, "two panels were open at once")
            XCTAssertTrue(app.staticTexts["Stream"].waitForExistence(timeout: 3), "no stream details")
            shot("stream-details")

            // With no panel open, a swipe still changes the channel.
            pad.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.3)).tap()
            XCTAssertTrue(until(3) { !app.staticTexts["Stream"].exists }, "a tap outside left the stream details open")
            pad.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.55))
                .press(forDuration: 0.05, thenDragTo: pad.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.25)))
            XCTAssertTrue(until(8) { now.label != channel }, "a swipe with no panel stayed on \(channel)")
        }

        func testControlsOverThePictureHideAndComeBack() throws {
            let app = try launch(landscape: true)
            let probe = app.staticTexts["chromeProbe"]
            XCTAssertTrue(probe.waitForExistence(timeout: 30), app.debugDescription)
            XCTAssertTrue(until(10) { onScreen(app, "Minimize") != nil })
            XCTAssertTrue(app.buttons["player-play"].waitForExistence(timeout: 30), "no play button")
            shot("controls-shown")
            XCTAssertTrue(until(15) { probe.label == "hidden" }, "the controls stayed up")
            XCTAssertNil(onScreen(app, "Minimize"), "Minimize is still on screen")
            shot("controls-hidden")

            let pad = app.descendants(matching: .any)["player-gesture"]
            pad.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            XCTAssertTrue(until(4) { probe.label == "shown" }, "a tap did not bring the controls back")
            for id in ["Minimize", "player-play", "player-back", "portrait-channels", "portrait-record", "player-more"] {
                let found = onScreen(app, id)
                XCTAssertNotNil(found, "no \(id)")
                guard let button = found else { continue }
                XCTAssertGreaterThanOrEqual(min(button.frame.width, button.frame.height), 44, "\(id) is smaller than 44 pt")
            }
            XCTAssertFalse(app.descendants(matching: .any)["portrait-program"].label.isEmpty, "no title")

            app.buttons["player-play"].tap()
            XCTAssertTrue(app.buttons["Play"].waitForExistence(timeout: 3), "pause did not show Play")
            shot("paused")
            // Paused, the controls stay.
            pause(5)
            XCTAssertEqual(probe.label, "shown")
            app.buttons["Play"].tap()
            XCTAssertTrue(app.buttons["Pause"].waitForExistence(timeout: 3))

            pad.doubleTap()
            XCTAssertTrue(until(3) { (pad.value as? String) == "Fill" }, "double tap did not fill")
            shot("filled")
        }

        func testTheMiniPlayerPausesAndOffersPictureInPicture() throws {
            let app = try launch(stays: true)
            XCTAssertTrue(until(30) { onScreen(app, "Minimize") != nil }, app.debugDescription)
            pause(6)
            tap(app, "Minimize")
            let mini = app.descendants(matching: .any)["miniPlayer"]
            XCTAssertTrue(mini.waitForExistence(timeout: 10), "no mini player")
            let play = app.buttons["mini-play"]
            XCTAssertTrue(play.waitForExistence(timeout: 5), "no play button in the mini player")
            XCTAssertEqual(play.label, "Pause")
            pause(2)
            shot("mini-playing")
            play.tap()
            XCTAssertTrue(until(3) { play.label == "Play" }, "the mini player did not pause")
            XCTAssertEqual(mini.value as? String, "Paused")
            play.tap()
            XCTAssertTrue(until(3) { play.label == "Pause" })
            // The iPad simulator has Picture in Picture; the iPhone simulator says it has not.
            if UIDevice.current.userInterfaceIdiom == .pad {
                let pip = app.buttons["mini-pip"]
                XCTAssertTrue(pip.waitForExistence(timeout: 10), "no Picture in Picture in the mini player")
                pip.tap()
                pause(3)
                shot("mini-pip")
            }
        }

        private func launch(landscape: Bool = false, stays: Bool = false) throws -> XCUIApplication {
            try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
            let app = XCUIApplication()
            app.launchArguments = try [
                "-ApplePersistenceIgnoreState", "YES",
                "-BroadwaveServerURL", server,
                "-BroadwaveWatch", firstChannelID(),
                "-BroadwaveChannelNow", "YES",
            ] + (landscape ? ["-BroadwaveLandscape", "YES"] : []) + (stays ? ["-BroadwaveChromeStays", "YES"] : [])
            app.launch()
            return app
        }

        private func firstChannelID() throws -> String {
            struct Page: Decodable {
                struct Item: Decodable { let id: Int64 }
                let channels: [Item]
            }
            let url = try XCTUnwrap(URL(string: server + "/api/v1/channels"))
            let page = try JSONDecoder().decode(Page.self, from: Data(contentsOf: url))
            return try String(XCTUnwrap(page.channels.first?.id, "no channels"))
        }

        /// The button with this name or identifier that is drawn on screen; the zoom leaves a copy behind.
        private func onScreen(_ app: XCUIApplication, _ id: String) -> XCUIElement? {
            let all = app.buttons.matching(NSPredicate(format: "identifier == %@ OR label == %@", id, id)).allElementsBoundByIndex
            return all.first { $0.exists && $0.frame.width > 0 && app.frame.intersects($0.frame) && ($0.isHittable || !$0.isEnabled) }
        }

        /// Taps where the on-screen copy is drawn. An index-bound element can go stale before the tap lands.
        private func tap(_ app: XCUIApplication, _ id: String) {
            guard let frame = onScreen(app, id)?.frame else {
                XCTFail("no \(id) on screen")
                return
            }
            app.coordinate(withNormalizedOffset: .zero).withOffset(CGVector(dx: frame.midX, dy: frame.midY)).tap()
        }

        private func shot(_ name: String) {
            guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_SHOT_DIR"], !dir.isEmpty else { return }
            let folder = URL(fileURLWithPath: dir, isDirectory: true)
            try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            let device = UIDevice.current.userInterfaceIdiom == .pad ? "ipad" : "iphone"
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: folder.appending(path: "\(device)-\(name).png"))
        }

        private func pause(_ seconds: TimeInterval) {
            RunLoop.current.run(until: Date().addingTimeInterval(seconds))
        }

        private func until(_ seconds: TimeInterval, _ check: () -> Bool) -> Bool {
            let deadline = Date().addingTimeInterval(seconds)
            while Date() < deadline {
                if check() {
                    return true
                }
                RunLoop.current.run(until: Date().addingTimeInterval(0.2))
            }
            return check()
        }
    }
#endif
