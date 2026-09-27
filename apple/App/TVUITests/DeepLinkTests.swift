import XCTest

/// Links sent while the player is up. A page link used to change the tab under
/// the full-screen player and leave the player on top.
final class DeepLinkTests: XCTestCase {
    func testWatchLinkChangesTheChannelWhilePlaying() throws {
        let app = launchPlaying()
        let link = try XCTUnwrap(URL(string: "broadwave://watch/2"))
        app.open(link)
        XCTAssertTrue(app.staticTexts["5.1  Sintel"].waitForExistence(timeout: 15), app.debugDescription)
    }

    func testGuideLinkClosesThePlayer() throws {
        let app = launchPlaying()
        let link = try XCTUnwrap(URL(string: "broadwave://guide"))
        app.open(link)
        XCTAssertTrue(app.descendants(matching: .any)["guide-now"].waitForExistence(timeout: 15), app.debugDescription)
        XCTAssertTrue(app.staticTexts["4.1  Big Buck Bunny"].waitForNonExistence(timeout: 10))
    }

    private func launchPlaying() -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = [
            "-BroadwaveDemo", "YES", "-BroadwaveInfo", "YES", "-BroadwaveWatch", "1",
            "-ApplePersistenceIgnoreState", "YES",
        ]
        app.launch()
        XCTAssertTrue(app.staticTexts["4.1  Big Buck Bunny"].waitForExistence(timeout: 60), app.debugDescription)
        return app
    }
}
