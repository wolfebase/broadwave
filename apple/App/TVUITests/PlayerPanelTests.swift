import XCTest

/// Channels and Stream, plus a clickpad swipe while the transport bar is hidden.
/// Opt-in: TEST_RUNNER_BROADWAVE_SERVER and TEST_RUNNER_BROADWAVE_SHOTS.
final class PlayerPanelTests: XCTestCase {
    private var server: String {
        ProcessInfo.processInfo.environment["BROADWAVE_SERVER"] ?? ""
    }

    private var shots: String {
        ProcessInfo.processInfo.environment["BROADWAVE_SHOTS"] ?? ""
    }

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    func testPanelsAndTheTransportMenu() throws {
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        let app = try launch()
        let started = try waitChannel(app)
        XCTAssertFalse(started.isEmpty, "no channel")
        let menu = app.staticTexts["menuProbe"]
        XCTAssertTrue(menu.waitForExistence(timeout: 8), app.debugDescription)
        XCTAssertTrue(menu.label.contains("Record"), menu.label)
        XCTAssertTrue(menu.label.contains("Multiview"), menu.label)
        XCTAssertFalse(menu.label.contains("Start over"), "a fresh picture does not hold the start: \(menu.label)")

        try showTabs(app)
        try showContent(app, "panel-channels", tab: "Channels")
        shot(app, "channels")
        let rows = app.descendants(matching: .any).matching(NSPredicate(format: "identifier BEGINSWITH %@", "panel-channel-"))
        XCTAssertGreaterThan(rows.count, 1, app.debugDescription)
        let before = app.staticTexts["channel-now"].label
        try changeChannel(app, from: before)
        shot(app, "channel-changed")

        try showContent(app, "panel-stream", tab: "Stream")
        XCTAssertTrue(app.staticTexts["Rendition"].waitForExistence(timeout: 5), app.debugDescription)
        XCTAssertTrue(app.staticTexts["Drift"].exists, app.debugDescription)
        shot(app, "stream")
    }

    func testSwipeChangesChannelWhenTheBarIsHidden() throws {
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        let app = try launch()
        let before = try waitChannel(app)
        let probe = app.staticTexts["transportProbe"]
        XCTAssertTrue(until(20) { probe.label == "hidden" }, "transport stayed \(probe.label)\n\(focused(app))\n\(app.debugDescription)")
        // tvOS XCTest has no swipe. A down press while the bar is hidden takes the same step.
        XCUIRemote.shared.press(.down)
        let moved = until(8) {
            let now = app.staticTexts["channel-now"].label
            return !now.isEmpty && now != before
        }
        shot(app, "swipe")
        XCTAssertTrue(moved, "stayed on \(app.staticTexts["channel-now"].label)")
    }

    func testRestingOnAChannelRowStartsItsPicture() throws {
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        let app = try launch()
        let started = try waitChannel(app)
        let playing = try XCTUnwrap(Int64(firstChannelID()))
        let sibling = try XCTUnwrap(sameFrequency(as: playing), "no second channel on the playing frequency")
        XCTAssertNil(try relayFeed(sibling), "\(sibling) was running before the row had focus")

        try showTabs(app)
        try showContent(app, "panel-channels", tab: "Channels")
        let row = "panel-channel-\(sibling)"
        for _ in 0 ..< 8 where !focused(app).hasPrefix(row + " ") {
            XCUIRemote.shared.press(.down)
            RunLoop.current.run(until: Date().addingTimeInterval(0.4))
        }
        XCTAssertTrue(focused(app).hasPrefix(row + " "), "focus \(focused(app))\n\(app.debugDescription)")
        shot(app, "warm-row")

        var feed: RelayFeed?
        XCTAssertTrue(until(8) {
            feed = try? self.relayFeed(sibling)
            return feed?.renditions?.contains { $0.viewers == 0 } == true
        }, "no guessed picture for \(sibling): \(String(describing: feed))")
        print("warm-relay \(String(describing: feed))")
        XCTAssertEqual(app.staticTexts["channel-now"].label, started, "resting on a row changed the channel")
    }

    private func sameFrequency(as channel: Int64) -> Int64? {
        var match: Int64?
        _ = until(10) {
            guard let signals = try? self.fetch(SignalPage.self, "/api/v1/signals").channels,
                  let freq = signals.first(where: { $0.channelId == channel })?.frequencyHz, freq > 0
            else { return false }
            match = signals.first { $0.channelId != channel && $0.frequencyHz == freq }?.channelId
            return true
        }
        return match
    }

    private func relayFeed(_ channel: Int64) throws -> RelayFeed? {
        try fetch(Diagnostics.self, "/api/v1/diagnostics").relay?.first { $0.channelId == channel }
    }

    private func fetch<T: Decodable>(_: T.Type, _ path: String) throws -> T {
        let url = try XCTUnwrap(URL(string: server + path))
        return try JSONDecoder().decode(T.self, from: Data(contentsOf: url))
    }

    private func launch() throws -> XCUIApplication {
        let channel = try firstChannelID()
        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveWatch", channel,
            "-BroadwaveChannelNow", "YES",
            "-BroadwaveSyncLog", "1",
        ]
        app.launch()
        return app
    }

    private func firstChannelID() throws -> String {
        let url = try XCTUnwrap(URL(string: server + "/api/v1/channels"))
        let data = try Data(contentsOf: url)
        let page = try JSONDecoder().decode(ChannelPage.self, from: data)
        return try String(XCTUnwrap(page.channels.first?.id, "no channels"))
    }

    private func waitChannel(_ app: XCUIApplication) throws -> String {
        let now = app.staticTexts["channel-now"]
        XCTAssertTrue(now.waitForExistence(timeout: 20), app.debugDescription)
        let ready = until(15) { !now.label.isEmpty }
        XCTAssertTrue(ready, "channel never arrived")
        return now.label
    }

    /// The info tabs are cells in the transport strip. Their names sit on the inner text, not the cell.
    private func showTabs(_ app: XCUIApplication) throws {
        if until(8, { self.tabsReady(app) }) {
            shot(app, "tabs")
            return
        }
        // Select brings the bar back. Up and down would change the channel.
        if app.staticTexts["transportProbe"].label != "shown" {
            XCUIRemote.shared.press(.select)
        }
        let opened = until(8) { self.tabsReady(app) }
        shot(app, "tabs")
        XCTAssertTrue(
            opened,
            "tabs did not appear\ntransport \(app.staticTexts["transportProbe"].label)\nfocus \(focused(app))\n\(app.debugDescription)"
        )
    }

    /// Focuses the named tab. Focusing it shows the page.
    private func showContent(_ app: XCUIApplication, _ identifier: String, tab label: String) throws {
        let body = app.descendants(matching: .any)[identifier]
        if body.exists {
            return
        }
        if !tabsReady(app) {
            try showTabs(app)
        }
        focusTab(app, label: label)
        XCTAssertTrue(
            until(3) { body.exists },
            "\(identifier) did not appear\nfocus \(focused(app))\n\(app.debugDescription)"
        )
    }

    private func tabsReady(_ app: XCUIApplication) -> Bool {
        let strip = app.collectionViews["AVInfoMenuCollection"]
        return strip.exists && strip.staticTexts["Channels"].exists && strip.staticTexts["Stream"].exists
    }

    /// The tab cells carry no label of their own. The remote walks until the focused cell sits on the named text.
    /// A select while this panel is up does not open a tab, so this never clicks.
    private func focusTab(_ app: XCUIApplication, label: String) {
        let target = app.collectionViews["AVInfoMenuCollection"]
            .staticTexts.matching(NSPredicate(format: "label == %@", label))
            .firstMatch
        for _ in 0 ..< 8 where !stripHasFocus(app) {
            XCUIRemote.shared.press(.down)
            RunLoop.current.run(until: Date().addingTimeInterval(0.4))
        }
        guard target.waitForExistence(timeout: 2) else { return }
        for _ in 0 ..< 6 {
            if tabCovers(app, target) {
                return
            }
            guard stripHasFocus(app) else { break }
            let towardLeft = target.frame.midX < focusedCell(app).frame.midX - 20
            XCUIRemote.shared.press(towardLeft ? .left : .right)
            RunLoop.current.run(until: Date().addingTimeInterval(0.35))
        }
    }

    private func tabCovers(_ app: XCUIApplication, _ text: XCUIElement) -> Bool {
        guard stripHasFocus(app), text.exists else { return false }
        return abs(focusedCell(app).frame.midX - text.frame.midX) < 40
    }

    private func focusedCell(_ app: XCUIApplication) -> XCUIElement {
        app.collectionViews["AVInfoMenuCollection"].cells.element(matching: NSPredicate(format: "hasFocus == true"))
    }

    private func stripHasFocus(_ app: XCUIApplication) -> Bool {
        app.collectionViews["AVInfoMenuCollection"].cells.element(matching: NSPredicate(format: "hasFocus == true")).exists
    }

    /// The channel list sits under the tab. Down moves into it, then onto a row that is not playing.
    private func changeChannel(_ app: XCUIApplication, from current: String) throws {
        for _ in 0 ..< 6 where !focused(app).contains("panel-channel-") {
            XCUIRemote.shared.press(.down)
            RunLoop.current.run(until: Date().addingTimeInterval(0.4))
        }
        for _ in 0 ..< 4 {
            let focus = focused(app)
            if focus.contains("panel-channel-"), !focus.contains(current) {
                break
            }
            XCUIRemote.shared.press(.down)
            RunLoop.current.run(until: Date().addingTimeInterval(0.4))
        }
        XCUIRemote.shared.press(.select)
        let moved = until(8) {
            let now = app.staticTexts["channel-now"].label
            return !now.isEmpty && now != current
        }
        XCTAssertTrue(moved, "channel stayed \(app.staticTexts["channel-now"].label), focus \(focused(app))")
    }

    private func focused(_ app: XCUIApplication) -> String {
        let element = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        guard element.exists else { return "nothing" }
        return "\(element.identifier) \(element.label)"
    }

    private func shot(_ app: XCUIApplication, _ name: String) {
        guard !shots.isEmpty else { return }
        let folder = URL(fileURLWithPath: shots, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let url = folder.appending(path: "tv-\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
        print("l52-shot \(url.path)")
        _ = app
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

private struct ChannelPage: Decodable {
    struct Item: Decodable {
        let id: Int64
    }

    let channels: [Item]
}

private struct SignalPage: Decodable {
    struct Item: Decodable {
        let channelId: Int64
        let frequencyHz: Int?
    }

    let channels: [Item]
}

private struct Diagnostics: Decodable {
    let relay: [RelayFeed]?
}

private struct RelayFeed: Decodable {
    struct Rendition: Decodable {
        let key: String
        let viewers: Int
    }

    let channelId: Int64
    let renditions: [Rendition]?
}
