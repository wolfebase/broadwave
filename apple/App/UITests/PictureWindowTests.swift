import XCTest

#if os(iOS)
    /// Home starts Picture in Picture, the picture keeps moving, and a tap returns
    /// to the same channel. The AirPlay button is named and opens the picker.
    final class PictureWindowTests: XCTestCase {
        private var server: String {
            ProcessInfo.processInfo.environment["BROADWAVE_SERVER"] ?? ""
        }

        private var channel: String {
            ProcessInfo.processInfo.environment["BROADWAVE_CHANNEL"] ?? ""
        }

        private var logPath: String {
            ProcessInfo.processInfo.environment["BROADWAVE_LOG"] ?? ""
        }

        private var shots: String {
            ProcessInfo.processInfo.environment["BROADWAVE_SHOTS"] ?? ""
        }

        private var holdSeconds: TimeInterval {
            let raw = ProcessInfo.processInfo.environment["BROADWAVE_PIP_SECONDS"] ?? ""
            return TimeInterval(raw) ?? 120
        }

        override func setUp() {
            super.setUp()
            continueAfterFailure = false
            // A previous run can leave the phone on its side. Portrait is where the named controls are.
            XCUIDevice.shared.orientation = .portrait
        }

        func testHomeKeepsThePictureAndAirPlayOpens() {
            XCTAssertFalse(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
            XCTAssertFalse(channel.isEmpty, "set TEST_RUNNER_BROADWAVE_CHANNEL")
            let app = XCUIApplication()
            app.launchArguments = [
                "-ApplePersistenceIgnoreState", "YES",
                "-BroadwaveServerURL", server,
                "-BroadwaveWatch", channel,
                "-BroadwaveSyncLog", "1",
                "-BroadwaveExercise", "YES",
            ]
            app.launch()

            let program = app.descendants(matching: .any)["portrait-program"]
            XCTAssertTrue(program.waitForExistence(timeout: 30), app.debugDescription)
            let title = program.label
            XCTAssertFalse(title.isEmpty, "the player should name the channel")

            XCTAssertTrue(waitForLog { $0.contains("state=locked") || $0.contains("state=syncing") }, "playback did not start")
            let supported = logText().contains("pip supported yes")
            shot("playing")

            let airplay = app.buttons["airplay"]
            XCTAssertTrue(airplay.waitForExistence(timeout: 5), "AirPlay button missing\n\(app.debugDescription)")
            XCTAssertEqual(airplay.label, "AirPlay")
            airplay.tap()
            XCTAssertTrue(waitForLog(timeout: 8) { $0.contains("airplay open") }, "the route picker did not open")
            let picker = routePicker(app)
            let showedRoutes = picker.waitForExistence(timeout: 6)
            shot(showedRoutes ? "airplay" : "airplay-missing")
            if showedRoutes {
                dismissPicker(app)
            } else {
                // A stuck presentation would keep Home from starting Picture in Picture.
                app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.35)).tap()
            }
            shot("playing-after-airplay")

            XCUIDevice.shared.press(.home)
            if supported {
                // The iPad simulator can show the window. The iPhone simulator reports it unsupported.
                XCTAssertTrue(waitForLog(timeout: 12) { $0.contains("pip start") }, "Picture in Picture did not start\n\(logTail())")
                shot("pip")

                XCTAssertTrue(waitForLog(timeout: 8) { $0.contains("clock media=") }, "no picture clock\n\(logTail())")
                let began = Date()
                let start = lastClock()
                // Most of the hold stays as the window started, so a shot during
                // it is the picture and not the rotation. Device rotation only:
                // asking the app to rotate pulls the player forward.
                let quiet = max(holdSeconds - 8, 4)
                Thread.sleep(forTimeInterval: quiet)
                rotate("landscape", app: false)
                Thread.sleep(forTimeInterval: 2)
                shot("pip-landscape")
                let remaining = holdSeconds - Date().timeIntervalSince(began)
                if remaining > 0 {
                    Thread.sleep(forTimeInterval: remaining)
                }
                rotate("portrait", app: false)
                Thread.sleep(forTimeInterval: 2)
                shot("pip-held")

                let end = lastClock()
                if let start, let end {
                    let need = max(holdSeconds - 25, 5) * 1000
                    XCTAssertGreaterThan(end.media - start.media, need, "the picture did not keep moving (\(start.media) to \(end.media))\n\(logTail())")
                    XCTAssertEqual(end.stalls, start.stalls, "stalls \(start.stalls) to \(end.stalls)")
                } else {
                    XCTFail("no picture clock after Home\n\(logTail())")
                }

                openPictureAgain(app)
                XCTAssertTrue(program.waitForExistence(timeout: 15), app.debugDescription)
                XCTAssertEqual(program.label, title, "came back on \(program.label), left on \(title)")
                XCTAssertTrue(waitForLog(timeout: 15) { $0.contains("pip restore") }, "a tap did not restore the player\n\(logTail())")
            } else {
                XCTAssertTrue(logText().contains("pip supported no"), "Picture in Picture did not start\n\(logTail())")
                shot("home")
                app.activate()
                XCTAssertTrue(program.waitForExistence(timeout: 15), app.debugDescription)
                XCTAssertEqual(program.label, title, "came back on \(program.label), left on \(title)")
            }
            shot("restored")
            // The button asks the system. A simulator with no route view still
            // counts: the list is an empty window there, and forcing it on screen
            // leaves the small picture black.
            if !showedRoutes {
                XCTAssertTrue(logText().contains("airplay open"), "the route button did not open\n\(logTail())")
            }
        }

        /// The system route list. It can sit in the app or in SpringBoard.
        private func routePicker(_ app: XCUIApplication) -> XCUIElement {
            func match(_ root: XCUIApplication) -> XCUIElement {
                let query = NSPredicate(
                    format: "identifier != 'airplay' AND (label == 'Cancel' OR label == 'Close' OR label CONTAINS[c] 'iPhone' OR label CONTAINS[c] 'iPad' OR label CONTAINS 'Office' OR label CONTAINS 'Living Room' OR label CONTAINS 'MacBook' OR label CONTAINS 'Apple TV' OR label CONTAINS 'Speaker' OR label CONTAINS 'Control Other')"
                )
                return root.descendants(matching: .any).matching(query).firstMatch
            }
            let inApp = match(app)
            if inApp.exists {
                return inApp
            }
            return match(XCUIApplication(bundleIdentifier: "com.apple.springboard"))
        }

        private func dismissPicker(_ app: XCUIApplication) {
            if app.buttons["Cancel"].waitForExistence(timeout: 2) {
                app.buttons["Cancel"].tap()
                return
            }
            if app.buttons["Close"].waitForExistence(timeout: 1) {
                app.buttons["Close"].tap()
                return
            }
            // An iPad popover closes when the picture is tapped.
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.35)).tap()
        }

        /// The small window sits on the Home screen. A tap shows its controls.
        /// Close is the leading one. The trailing one returns to the full player
        /// and is what logs the restore. Its spoken name is not "return".
        private func openPictureAgain(_ app: XCUIApplication) {
            let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
            springboard.coordinate(withNormalizedOffset: CGVector(dx: 0.78, dy: 0.16)).tap()
            Thread.sleep(forTimeInterval: 0.8)
            shot("pip-tap")
            if !shots.isEmpty {
                try? controlReport(springboard).write(toFile: (shots as NSString).appendingPathComponent("pip-buttons.txt"), atomically: true, encoding: .utf8)
            }
            if let button = restoreControl(springboard) {
                button.tap()
            }
            if !waitForLog(timeout: 8, where: { $0.contains("pip restore") }) {
                app.activate()
            }
        }

        /// Every control on the Home screen, so a missed restore says where it was.
        private func controlReport(_ root: XCUIApplication) -> String {
            root.buttons.allElementsBoundByIndex.map { button in
                let frame = button.frame
                return "\(button.label) id=\(button.identifier) \(Int(frame.minX)),\(Int(frame.minY)) \(Int(frame.width))x\(Int(frame.height))"
            }.joined(separator: "\n")
        }

        /// The control at the trailing edge of the small window. Close sits at the
        /// leading edge, so the rightmost control in the top of the screen is restore.
        private func restoreControl(_ root: XCUIApplication) -> XCUIElement? {
            let screen = root.frame
            let named = NSPredicate(
                format: "(label CONTAINS[c] 'return' OR label CONTAINS[c] 'expand' OR label CONTAINS[c] 'fullscreen' OR label CONTAINS[c] 'full screen') AND NOT (label CONTAINS[c] 'close')"
            )
            let spoken = root.buttons.matching(named).firstMatch
            if spoken.exists {
                return spoken
            }
            let top = root.buttons.allElementsBoundByIndex.filter { button in
                let frame = button.frame
                guard frame.width > 8, frame.minY < screen.height * 0.3, frame.midX > screen.width * 0.5 else { return false }
                return !button.label.lowercased().contains("close")
            }
            return top.max { $0.frame.midX < $1.frame.midX }
        }

        private func rotate(_ verb: String, app: Bool = true) {
            let mask: UIDeviceOrientation = verb == "landscape" ? .landscapeRight : .portrait
            XCUIDevice.shared.orientation = mask
            if app, let path = exercisePath() {
                try? "\(verb) \(Int(Date().timeIntervalSince1970))\n".write(toFile: path, atomically: true, encoding: .utf8)
            }
        }

        private func exercisePath() -> String? {
            guard let text = try? String(contentsOfFile: logPath, encoding: .utf8) else { return nil }
            for line in text.split(separator: "\n").reversed() {
                if let range = line.range(of: "exercise-file ") {
                    return String(line[range.upperBound...]).trimmingCharacters(in: .whitespaces)
                }
            }
            return nil
        }

        private func waitForLog(timeout: TimeInterval = 40, where match: (String) -> Bool) -> Bool {
            let end = Date().addingTimeInterval(timeout)
            while Date() < end {
                if match(logText()) {
                    return true
                }
                Thread.sleep(forTimeInterval: 0.4)
            }
            return false
        }

        private func logText() -> String {
            guard !logPath.isEmpty else { return "" }
            let url = URL(fileURLWithPath: logPath)
            guard let data = try? Data(contentsOf: url, options: [.uncached]) else { return "" }
            return String(bytes: data, encoding: .utf8) ?? ""
        }

        private func logTail() -> String {
            let lines = logText().split(separator: "\n")
            return lines.suffix(30).joined(separator: "\n")
        }

        /// The play log stops growing while the app is away. The clock file does not.
        private func lastClock() -> (media: Double, stalls: Int)? {
            guard let path = clockPath() else { return nil }
            let url = URL(fileURLWithPath: path)
            guard let data = try? Data(contentsOf: url, options: [.uncached]),
                  let text = String(bytes: data, encoding: .utf8) else { return nil }
            for line in text.split(separator: "\n").reversed() {
                var media: Double?
                var stalls: Int?
                for part in line.split(separator: " ") {
                    if part.hasPrefix("media="), let value = Double(part.dropFirst("media=".count)) {
                        media = value
                    }
                    if part.hasPrefix("stalls="), let value = Int(part.dropFirst("stalls=".count)) {
                        stalls = value
                    }
                }
                if let media, let stalls {
                    return (media, stalls)
                }
            }
            return nil
        }

        private func clockPath() -> String? {
            for line in logText().split(separator: "\n").reversed() {
                guard let range = line.range(of: "exercise-file ") else { continue }
                let exercise = String(line[range.upperBound...]).trimmingCharacters(in: .whitespaces)
                return URL(fileURLWithPath: exercise).deletingLastPathComponent().appendingPathComponent("broadwave-clock.txt").path
            }
            return nil
        }

        private func shot(_ name: String) {
            guard !shots.isEmpty else { return }
            try? FileManager.default.createDirectory(atPath: shots, withIntermediateDirectories: true)
            let url = URL(fileURLWithPath: shots).appendingPathComponent("\(name).png")
            try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
        }
    }
#endif
