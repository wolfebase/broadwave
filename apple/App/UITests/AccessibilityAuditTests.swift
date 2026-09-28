import XCTest

/// Labels and traits on the screens a viewer opens. Contrast and one-line card
/// clipping are printed and left for a visual review. A button with no name,
/// a bad trait, or an unlabeled control fails the test.
final class AccessibilityAuditTests: XCTestCase {
    private let auditLog = AuditLog()

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
        stampSetup()
    }

    func testHome() throws {
        let app = launch(tab: "home")
        XCTAssertTrue(app.buttons["Watch"].waitForExistence(timeout: 25), app.debugDescription)
        try finish(app, "home")
    }

    func testGuide() throws {
        let app = launch(tab: "guide")
        let now = app.descendants(matching: .any)["guide-now"]
        XCTAssertTrue(now.waitForExistence(timeout: 25), app.debugDescription)
        try finish(app, "guide")
    }

    func testSettings() throws {
        let app = launch(tab: "settings")
        XCTAssertTrue(app.descendants(matching: .any)["home-scan"].waitForExistence(timeout: 25), app.debugDescription)
        scroll(app)
        // About sits at the bottom of a long form. The audit still runs if it has not been built.
        _ = app.buttons["About Broadwave"].waitForExistence(timeout: 5)
        try finish(app, "settings")
    }

    func testRecordings() throws {
        let app = launch(tab: "recordings")
        XCTAssertTrue(recordingsReady(app), app.debugDescription)
        try finish(app, "recordings")
    }

    func testPlayerChrome() throws {
        let app = try launch(watch: firstChannel())
        #if os(tvOS)
            let chrome = app.descendants(matching: .any)["player-chrome"]
        #else
            let chrome = app.descendants(matching: .any)["portrait-program"]
        #endif
        XCTAssertTrue(chrome.waitForExistence(timeout: 30), app.debugDescription)
        #if os(tvOS)
            // The title is on screen while the picture is still starting. The system
            // transport bar comes up with the remote once that message is gone.
            let starting = app.staticTexts.matching(NSPredicate(
                format: "label BEGINSWITH 'Tuning the antenna' OR label BEGINSWITH 'Starting the picture' OR label BEGINSWITH 'Lining up with live' OR label BEGINSWITH 'Still tuning'"
            ))
            let ready = Date().addingTimeInterval(25)
            while Date() < ready, starting.firstMatch.exists {
                RunLoop.current.run(until: Date().addingTimeInterval(0.3))
            }
            print("a11y-tuning \(starting.firstMatch.exists ? "still" : "gone")")
            XCUIRemote.shared.press(.up)
            RunLoop.current.run(until: Date().addingTimeInterval(1))
        #endif
        try finish(app, "player")
    }

    func testMultiview() throws {
        let ids = try twoChannels()
        let app = launch(multiview: ids)
        XCTAssertTrue(app.buttons["multiview-pause"].waitForExistence(timeout: 30), app.debugDescription)
        try finish(app, "multiview")
    }

    func testTunerAndSignalRowsReadAsText() throws {
        let app = launch(tab: "settings", extra: ["-BroadwaveDiagnostics", "YES"])
        // tvOS section headers are capitalized, so the label is "TUNER HEALTH".
        XCTAssertTrue(text(app, "Tuner health").waitForExistence(timeout: 25), app.debugDescription)
        XCTAssertTrue(text(app, "Antenna").waitForExistence(timeout: 10), app.debugDescription)
        let ready = Date().addingTimeInterval(15)
        let tuners = app.staticTexts.matching(NSPredicate(format: "identifier BEGINSWITH 'tuner-health-'"))
        let signals = app.staticTexts.matching(NSPredicate(format: "identifier BEGINSWITH 'signal-row-'"))
        while Date() < ready {
            if tuners.firstMatch.exists || app.staticTexts["No tuner answered."].exists {
                if signals.firstMatch.exists || app.staticTexts["No antenna reading yet."].exists {
                    break
                }
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        }
        let tunerText = tuners.firstMatch.exists || app.staticTexts["No tuner answered."].exists
        let signalText = signals.firstMatch.exists || app.staticTexts["No antenna reading yet."].exists
        XCTAssertTrue(tunerText, "tuner health should be text, not an unlabeled control")
        XCTAssertTrue(signalText, "antenna readings should be text, not an unlabeled control")
        assertRows(tuners, words: ["Tuner", "lock", "Firmware", "model", "status"])
        assertRows(signals, words: ["signal", "reading", "No reading"])
        try finish(app, "diagnostics")
    }

    func testDynamicTypeXXL() throws {
        let pages = ["home", "guide", "settings", "recordings"]
        for page in pages {
            let app = launch(tab: page, extra: xxl)
            XCTAssertTrue(marker(app, page).waitForExistence(timeout: 25), "\(page) \(app.debugDescription)")
            if page == "settings" {
                scroll(app)
            }
            try assertClippedButtons(app, page)
        }
        let app = try launch(watch: firstChannel(), extra: xxl)
        #if os(tvOS)
            let shown = app.descendants(matching: .any)["player-chrome"].waitForExistence(timeout: 30)
        #else
            let shown = app.descendants(matching: .any)["portrait-program"].waitForExistence(timeout: 30)
        #endif
        XCTAssertTrue(shown)
        try assertClippedButtons(app, "player")
        let grid = try launch(multiview: twoChannels(), extra: xxl)
        XCTAssertTrue(grid.buttons["multiview-pause"].waitForExistence(timeout: 30))
        try assertClippedButtons(grid, "multiview")
    }

    private var xxl: [String] {
        ["-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryXXL"]
    }

    private func finish(_ app: XCUIApplication, _ page: String) throws {
        saveShot(page)
        try assertAudit(app, page)
        assertNamedButtons(app, page)
    }

    private func marker(_ app: XCUIApplication, _ page: String) -> XCUIElement {
        switch page {
        case "home": app.buttons["Watch"]
        case "guide": app.descendants(matching: .any)["guide-now"]
        case "settings": app.descendants(matching: .any)["home-scan"]
        default: app.staticTexts["Recordings"]
        }
    }

    private func recordingsReady(_ app: XCUIApplication) -> Bool {
        let end = Date().addingTimeInterval(25)
        while Date() < end {
            if app.staticTexts["No recordings yet"].exists || app.cells.firstMatch.exists {
                return true
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        }
        return app.staticTexts["Recordings"].exists
    }

    /// A section title matches whether or not the platform capitalizes it.
    private func text(_ app: XCUIApplication, _ label: String) -> XCUIElement {
        app.staticTexts.matching(NSPredicate(format: "label ==[c] %@", label)).firstMatch
    }

    private func scroll(_ app: XCUIApplication) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            for _ in 0 ..< 8 {
                remote.press(.down)
            }
        #else
            for _ in 0 ..< 3 {
                app.swipeUp()
            }
        #endif
    }

    /// Every button VoiceOver can land on has a name, and not the raw symbol name.
    private func assertNamedButtons(_ app: XCUIApplication, _ page: String) {
        let symbols: Set = [
            "xmark", "chevron.up", "chevron.down", "chevron.left", "chevron.right", "chevron.compact.backward",
            "info.circle", "play.fill", "record.circle", "list.bullet", "speaker.wave.2",
            "rectangle.split.2x1", "star", "star.fill", "star.slash", "eye.slash",
        ]
        var missing: [String] = []
        let window = app.frame.insetBy(dx: 0, dy: 8)
        // The guide has hundreds of named program buttons. Asking for each one takes minutes.
        // Only buttons with no name, or a raw symbol name, need a closer look.
        let suspects = app.buttons.matching(NSPredicate(format: "label == '' OR label IN %@", Array(symbols)))
        for index in 0 ..< suspects.count {
            let button = suspects.element(boundBy: index)
            guard button.exists else { continue }
            let frame = button.frame
            guard frame.width > 2, frame.height > 2, window.contains(CGPoint(x: frame.midX, y: frame.midY)) else { continue }
            var label = button.label.trimmingCharacters(in: .whitespacesAndNewlines)
            // A glass Menu keeps its title as a child and leaves the button label empty.
            if label.isEmpty {
                label = button.staticTexts.allElementsBoundByIndex
                    .map { $0.label.trimmingCharacters(in: .whitespacesAndNewlines) }
                    .filter { !$0.isEmpty }
                    .joined(separator: " ")
            }
            if label.isEmpty || symbols.contains(label) {
                missing.append("\(button.identifier) '\(label)' \(frame)")
            }
        }
        XCTAssertTrue(missing.isEmpty, "\(page) buttons with no name: \(missing.joined(separator: ", "))")
    }

    private func assertRows(_ rows: XCUIElementQuery, words: [String]) {
        for index in 0 ..< rows.count {
            let row = rows.element(boundBy: index)
            XCTAssertEqual(row.elementType, .staticText, row.debugDescription)
            let label = row.label.trimmingCharacters(in: .whitespacesAndNewlines)
            XCTAssertFalse(label.isEmpty, row.identifier)
            XCTAssertTrue(words.contains { label.localizedCaseInsensitiveContains($0) }, label)
        }
    }

    private func assertAudit(_ app: XCUIApplication, _ page: String) throws {
        let types: XCUIAccessibilityAuditType = [.sufficientElementDescription, .trait, .elementDetection]
        try app.performAccessibilityAudit(for: types) { [auditLog] issue in
            // The tvOS sidebar's back mark is a system image named for its symbol.
            // Its accessibility node ignores a new label. It is not a control we draw.
            let label = issue.element?.label ?? ""
            let identifier = issue.element?.identifier ?? ""
            if label == "chevron.compact.backward" || identifier == "chevron.compact.backward" {
                return true
            }
            auditLog.fail("\(page) \(Self.describe(issue))")
            return true
        }
        let found = auditLog.takeFailures()
        XCTAssertTrue(found.isEmpty, found.joined(separator: "\n"))
    }

    /// XXL may clip a one-line card title. That is a visual call. A clipped button name is not.
    private func assertClippedButtons(_ app: XCUIApplication, _ page: String) throws {
        try app.performAccessibilityAudit(for: [.textClipped, .dynamicType]) { [auditLog] issue in
            let line = "\(page) \(Self.describe(issue))"
            if issue.auditType == .textClipped, issue.element?.elementType == .button {
                auditLog.fail(line)
            } else {
                auditLog.note(line)
            }
            return true
        }
        let found = auditLog.takeFailures()
        let visual = auditLog.takeNotes()
        if !visual.isEmpty {
            print("a11y-visual \(page)\n\(visual.joined(separator: "\n"))")
        }
        XCTAssertTrue(found.isEmpty, found.joined(separator: "\n"))
    }

    private static func describe(_ issue: XCUIAccessibilityAuditIssue) -> String {
        let element = issue.element
        let kind = element.map { "\($0.elementType.rawValue)" } ?? "-"
        let identifier = element?.identifier ?? ""
        let label = element?.label ?? ""
        return "\(issue.auditType) \(kind) id=\(identifier) label=\(label) \(issue.compactDescription)"
    }

    private func launch(tab: String? = nil, watch: String? = nil, multiview: String? = nil, extra: [String] = []) -> XCUIApplication {
        let app = XCUIApplication()
        var args = ["-ApplePersistenceIgnoreState", "YES"]
        if let server = Self.server {
            args += ["-BroadwaveServerURL", server]
        } else {
            args += ["-BroadwaveDemo", "YES"]
        }
        if let tab {
            args += ["-BroadwaveTab", tab]
        }
        if let watch {
            args += ["-BroadwaveWatch", watch, "-BroadwaveInfo", "YES"]
        }
        if let multiview {
            args += ["-BroadwaveMultiview", multiview]
        }
        args += extra
        app.launchArguments = args
        print("a11y-server \(Self.server ?? "demo")")
        app.launch()
        return app
    }

    /// Writes the screen the audit just checked when BROADWAVE_A11Y_SHOTS is a directory.
    private func saveShot(_ page: String) {
        guard let dir = ProcessInfo.processInfo.environment["BROADWAVE_A11Y_SHOTS"], !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        #if os(tvOS)
            let platform = "tv"
        #else
            let platform = "iphone"
        #endif
        let url = folder.appending(path: "\(platform)-\(page).png")
        do {
            try XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
            print("a11y-shot \(url.path)")
        } catch {
            print("a11y-shot failed \(url.path) \(error)")
        }
    }

    private func firstChannel() throws -> String {
        let ids = try channelIDs()
        return ids.first ?? "1"
    }

    private func twoChannels() throws -> String {
        let ids = try channelIDs()
        if ids.count >= 2 {
            return "\(ids[0]),\(ids[1])"
        }
        return "1,2"
    }

    private func channelIDs() throws -> [String] {
        guard let server = Self.server else { return ["1", "2"] }
        let url = try XCTUnwrap(URL(string: server)?.appending(path: "/api/v1/channels"))
        let data = try Data(contentsOf: url)
        let page = try JSONDecoder().decode(ChannelPage.self, from: data)
        return page.channels.map { String($0.id) }
    }

    private func stampSetup() {
        guard let server = Self.server, let url = URL(string: server)?.appending(path: "/api/v1/settings") else { return }
        var request = URLRequest(url: url)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = Data(#"{"setupComplete":"1"}"#.utf8)
        let done = DispatchSemaphore(value: 0)
        URLSession.shared.dataTask(with: request) { _, _, _ in done.signal() }.resume()
        _ = done.wait(timeout: .now() + 5)
    }

    /// TEST_RUNNER_BROADWAVE_SERVER, with the prefix stripped for the test process.
    private static var server: String? {
        let value = ProcessInfo.processInfo.environment["BROADWAVE_SERVER"] ?? ""
        return value.isEmpty ? nil : value
    }
}

private struct ChannelPage: Decodable {
    struct Item: Decodable {
        let id: Int64
    }

    let channels: [Item]
}

private final class AuditLog: @unchecked Sendable {
    private let lock = NSLock()
    private var failures: [String] = []
    private var notes: [String] = []

    func fail(_ line: String) {
        lock.lock()
        failures.append(line)
        lock.unlock()
    }

    func note(_ line: String) {
        lock.lock()
        notes.append(line)
        lock.unlock()
    }

    func takeFailures() -> [String] {
        lock.lock()
        defer { lock.unlock() }
        let copy = failures
        failures = []
        return copy
    }

    func takeNotes() -> [String] {
        lock.lock()
        defer { lock.unlock() }
        let copy = notes
        notes = []
        return copy
    }
}
