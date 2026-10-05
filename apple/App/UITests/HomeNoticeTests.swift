#if os(iOS)
    import XCTest

    /// An arrival line never covers the tab bar or the mini player, and a screen is not
    /// told about itself. Opt-in: TEST_RUNNER_BROADWAVE_SERVER on the local harness.
    @MainActor
    final class HomeNoticeTests: XCTestCase {
        func testTheLineLeavesTheTabsAndTheMiniPlayerFree() throws {
            try XCTSkipUnless(UIDevice.current.userInterfaceIdiom == .pad, "the tab bar is at the top on iPad only")
            let server = try XCTUnwrap(ProcessInfo.processInfo.environment["BROADWAVE_SERVER"].flatMap { $0.isEmpty ? nil : $0 }, "set TEST_RUNNER_BROADWAVE_SERVER")
            let app = XCUIApplication()
            app.launchArguments = [
                "-BroadwaveServerURL", server, "-BroadwaveWatch", "1",
                "-BroadwaveHomeNotice", "New Apple TV found: Den.", "-ApplePersistenceIgnoreState", "YES",
            ]
            app.launch()
            let minimize = app.buttons["Minimize"].firstMatch
            XCTAssertTrue(minimize.waitForExistence(timeout: 20), app.debugDescription)
            minimize.tap()

            let notNow = app.buttons["Not now"]
            XCTAssertTrue(notNow.waitForExistence(timeout: 10), app.debugDescription)
            let mini = app.descendants(matching: .any)["miniPlayer"].firstMatch
            XCTAssertTrue(mini.waitForExistence(timeout: 10), app.debugDescription)
            // Compare rows, not buttons: the line spans the screen.
            XCTAssertTrue(notNow.frame.maxY < mini.frame.minY - 8 || notNow.frame.minY > mini.frame.maxY + 8, "the line \(notNow.frame) on the mini player \(mini.frame)")
            let recordings = app.buttons["Recordings"].firstMatch
            XCTAssertTrue(recordings.isHittable, app.debugDescription)
            XCTAssertTrue(notNow.frame.minY > recordings.frame.maxY, "the line \(notNow.frame) over the tabs \(recordings.frame)")
            if let shot = ProcessInfo.processInfo.environment["BROADWAVE_SHOT"] {
                try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: shot))
            }
            notNow.tap()
            sleep(1)
            if let shot = ProcessInfo.processInfo.environment["BROADWAVE_SHOT"] {
                try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: shot.replacingOccurrences(of: ".png", with: "-after.png")))
            }
            recordings.tap()
            XCTAssertTrue(app.buttons["recording-row"].firstMatch.waitForExistence(timeout: 10), "Recordings did not open: \(app.debugDescription)")
        }
    }
#endif
