import XCTest

/// Walks the pass editor: new pass, rename, days, the half-hour window, Stop at, delete,
/// Edit+drag on iPhone, Move up/down on Apple TV. Opt-in: TEST_RUNNER_BROADWAVE_SERVER.
@MainActor
final class PassEditorTests: XCTestCase {
    func testPassEditorSavesEachRule() throws {
        continueAfterFailure = false
        executionTimeAllowance = 240
        let server = lane("BROADWAVE_SERVER")
        try XCTSkipIf(server.isEmpty, "set TEST_RUNNER_BROADWAVE_SERVER")
        try put(server, #"{"setupComplete":"1"}"#)
        try clearPasses(server)
        let alpha = try addPass(server, title: "Alpha News")
        let beta = try addPass(server, title: "Beta News")
        defer {
            for pass in (try? passes(server)) ?? [] {
                deletePass(server, pass.id)
            }
        }

        let app = XCUIApplication()
        app.launchArguments = [
            "-ApplePersistenceIgnoreState", "YES",
            "-BroadwaveServerURL", server,
            "-BroadwaveTab", "settings",
            "-BroadwavePasses", "YES",
        ]
        app.launch()

        XCTAssertTrue(
            app.navigationBars["Passes"].waitForExistence(timeout: 8)
                || app.staticTexts["Passes"].waitForExistence(timeout: 20)
                || app.descendants(matching: .any)["new-pass"].waitForExistence(timeout: 2),
            app.debugDescription
        )
        XCTAssertTrue(row(app, alpha.id).waitForExistence(timeout: 10), app.debugDescription)
        XCTAssertTrue(row(app, beta.id).exists, app.debugDescription)
        shot(app, "list")

        #if os(tvOS)
            try reorderOnTV(app, server: server, alpha: alpha.id, beta: beta.id)
            openNewPass(app)
            shot(app, "new")
        #else
            try reorderOnPhone(app, server: server, alpha: alpha.id, beta: beta.id)
            try addAndEdit(app, server: server)
        #endif
        print("l101 \(device) ok")
    }

    #if os(iOS)
        private func reorderOnPhone(_ app: XCUIApplication, server: String, alpha: Int, beta: Int) throws {
            let edit = app.navigationBars["Passes"].buttons["Edit"]
            XCTAssertTrue(edit.waitForExistence(timeout: 8), "no Edit\n\(app.debugDescription)")
            edit.tap()
            shot(app, "edit")
            let handle = app.buttons.matching(NSPredicate(format: "label CONTAINS 'Reorder' AND label CONTAINS 'Alpha News'")).firstMatch
            let target = app.buttons.matching(NSPredicate(format: "label CONTAINS 'Reorder' AND label CONTAINS 'Beta News'")).firstMatch
            if handle.waitForExistence(timeout: 5), target.exists {
                handle.press(forDuration: 0.8, thenDragTo: target)
            } else {
                // The reorder control can sit on the cell; drag the Alpha row onto Beta.
                let from = row(app, alpha)
                let to = row(app, beta)
                from.press(forDuration: 0.8, thenDragTo: to)
            }
            XCTAssertTrue(until(8) { (try? self.passIDs(server)) == [beta, alpha] }, "order stayed \((try? passIDs(server)) ?? [])")
            shot(app, "reordered")
            let done = app.navigationBars["Passes"].buttons["Done"]
            if done.waitForExistence(timeout: 3) {
                done.tap()
            }
        }
    #endif

    #if os(tvOS)
        private func reorderOnTV(_ app: XCUIApplication, server: String, alpha: Int, beta: Int) throws {
            focusRow(app, containing: "Alpha News")
            XCUIRemote.shared.press(.select)
            XCTAssertTrue(
                app.navigationBars["Alpha News"].waitForExistence(timeout: 8)
                    || app.descendants(matching: .any)["pass-move-down"].waitForExistence(timeout: 8),
                app.debugDescription
            )
            reveal(app, identifier: "pass-move-down", containing: "Move down")
            if focusedText(app).contains("Delete") {
                for _ in 0 ..< 8 {
                    XCUIRemote.shared.press(.up)
                    RunLoop.current.run(until: Date().addingTimeInterval(0.2))
                    if focusedText(app).contains("Move down") {
                        break
                    }
                }
            }
            XCTAssertTrue(focusedText(app).contains("Move down"), "wanted Move down, at \(focusedText(app))")
            shot(app, "move")
            XCUIRemote.shared.press(.select)
            XCTAssertTrue(until(8) { (try? self.passIDs(server)) == [beta, alpha] }, "order stayed \((try? passIDs(server)) ?? [])")
            XCTAssertTrue(app.descendants(matching: .any)["pass-move-up"].waitForExistence(timeout: 5), "left the pass page\n\(app.debugDescription)")
            XCTAssertTrue(
                app.descendants(matching: .any)["pass-move-up"].exists
                    || app.navigationBars["Alpha News"].exists,
                "focus left the pass page"
            )
            shot(app, "moved")
            XCUIRemote.shared.press(.menu)
            XCTAssertTrue(
                app.staticTexts["Passes"].waitForExistence(timeout: 8)
                    || app.descendants(matching: .any)["new-pass"].waitForExistence(timeout: 2),
                app.debugDescription
            )
        }
    #endif

    private func addAndEdit(_ app: XCUIApplication, server: String) throws {
        openNewPass(app)
        XCTAssertTrue(
            app.navigationBars["New pass"].waitForExistence(timeout: 8)
                || app.descendants(matching: .any)["pass-title"].waitForExistence(timeout: 8)
                || app.descendants(matching: .any)["add-pass"].waitForExistence(timeout: 2),
            app.debugDescription
        )
        shot(app, "new")

        let title = app.descendants(matching: .any)["pass-title"]
        XCTAssertTrue(title.waitForExistence(timeout: 8), app.debugDescription)
        typeTitle(app, title, "Desk News")
        let add = app.buttons["add-pass"].firstMatch
        XCTAssertTrue(add.waitForExistence(timeout: 5), app.debugDescription)
        choose(app, add, label: "Add pass")
        XCTAssertTrue(until(10) { (try? self.passes(server).contains { $0.title == "Desk News" }) == true }, "Desk News was not saved")
        shot(app, "added")

        let desk = try XCTUnwrap(try (passes(server)).first { $0.title == "Desk News" })
        #if os(tvOS)
            focusRow(app, containing: "Desk News")
            XCUIRemote.shared.press(.select)
        #else
            let deskRow = row(app, desk.id)
            XCTAssertTrue(deskRow.waitForExistence(timeout: 8), app.debugDescription)
            tap(deskRow)
        #endif
        XCTAssertTrue(
            app.navigationBars["Desk News"].waitForExistence(timeout: 8)
                || app.staticTexts["Desk News"].waitForExistence(timeout: 8)
                || app.descendants(matching: .any)["pass-title"].waitForExistence(timeout: 2),
            app.debugDescription
        )

        let field = app.descendants(matching: .any)["pass-title"]
        XCTAssertTrue(field.waitForExistence(timeout: 8), app.debugDescription)
        #if os(iOS)
            field.coordinate(withNormalizedOffset: CGVector(dx: 0.95, dy: 0.5)).tap()
            field.typeText(" Late")
        #else
            typeTitle(app, field, "Desk News Late", replace: true)
        #endif
        submitTitle(app, field)
        XCTAssertTrue(until(8) { (try? self.pass(server, desk.id)?.title) == "Desk News Late" }, "rename stayed \((try? pass(server, desk.id)?.title) ?? "")")
        shot(app, "renamed")

        let wednesday = app.descendants(matching: .any)["pass-day-3"].firstMatch
        XCTAssertTrue(wednesday.waitForExistence(timeout: 6), app.debugDescription)
        #if os(iOS)
            wednesday.coordinate(withNormalizedOffset: CGVector(dx: 0.9, dy: 0.5)).tap()
        #else
            reveal(app, identifier: "pass-day-3")
            XCUIRemote.shared.press(.select)
        #endif
        XCTAssertTrue(until(8) { (try? self.pass(server, desk.id)?.days)?.contains(3) == true }, "days stayed \((try? pass(server, desk.id)?.days) ?? [])")
        shot(app, "days")

        try setFrom(app, server: server, id: desk.id)
        shot(app, "window")

        try setStopAt(app, server: server, id: desk.id)
        shot(app, "stop-at")

        #if os(iOS)
            app.swipeUp()
            app.swipeUp()
        #endif
        let del = app.buttons["delete-pass"].firstMatch
        XCTAssertTrue(del.waitForExistence(timeout: 8), "no Delete pass\n\(app.debugDescription)")
        choose(app, del, label: "Delete pass")
        let confirm = app.buttons["Delete"].firstMatch
        XCTAssertTrue(confirm.waitForExistence(timeout: 6), app.debugDescription)
        choose(app, confirm, label: "Delete")
        XCTAssertTrue(until(10) { (try? self.pass(server, desk.id)) == nil }, "Desk News Late stayed")
        shot(app, "deleted")
    }

    private func openNewPass(_ app: XCUIApplication) {
        let button = app.descendants(matching: .any)["new-pass"]
        if button.waitForExistence(timeout: 5) {
            choose(app, button, label: "New pass")
            return
        }
        let labeled = app.buttons["New pass"]
        XCTAssertTrue(labeled.waitForExistence(timeout: 5), app.debugDescription)
        choose(app, labeled, label: "New pass")
    }

    private func typeTitle(_ app: XCUIApplication, _ field: XCUIElement, _ value: String, replace: Bool = false) {
        #if os(tvOS)
            reveal(app, identifier: "pass-title")
            if !focusedText(app).contains("Title"), !focusedText(app).contains("pass-title") {
                // Keep looking; the field may already hold the keyboard.
            }
            XCUIRemote.shared.press(.select)
            RunLoop.current.run(until: Date().addingTimeInterval(0.6))
            if replace {
                field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 24))
            }
            field.typeText(value)
        #else
            tap(field)
            if replace {
                field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 40))
            }
            field.typeText(value)
        #endif
        _ = app
    }

    private func submitTitle(_ app: XCUIApplication, _ field: XCUIElement) {
        #if os(tvOS)
            field.typeText("\n")
        #else
            if app.keyboards.buttons["return"].exists {
                app.keyboards.buttons["return"].tap()
            } else {
                field.typeText("\n")
            }
        #endif
    }

    private func toggleDay(_ app: XCUIApplication, _ name: String) {
        let day = app.switches[name].firstMatch
        if day.waitForExistence(timeout: 4) {
            reveal(app, identifier: day.identifier.isEmpty ? name : day.identifier)
            #if os(tvOS)
                if !focusedText(app).contains(name) {
                    focusNamed(app, name)
                }
                XCUIRemote.shared.press(.select)
            #else
                tap(day)
            #endif
            return
        }
        let any = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", name)).firstMatch
        XCTAssertTrue(any.waitForExistence(timeout: 6), "no \(name)\n\(app.debugDescription)")
        choose(app, any, label: name)
    }

    private func setFrom(_ app: XCUIApplication, server: String, id: Int) throws {
        #if os(iOS)
            app.swipeUp()
        #endif
        let from = app.buttons["pass-time-from"].firstMatch
        XCTAssertTrue(from.waitForExistence(timeout: 6), "no From picker\n\(app.debugDescription)")
        #if os(iOS)
            if !from.isHittable {
                app.swipeUp()
            }
            tap(from)
        #else
            reveal(app, identifier: "pass-time-from")
            XCUIRemote.shared.press(.select)
        #endif
        // 12:00 AM is next to Any time, so it is on screen without scrolling the menu.
        pickLabeled(app, containing: "12:00", also: "AM")
        let timed = until(8) { (try? self.pass(server, id)?.timeStart)?.isEmpty == false }
        shot(app, "window")
        XCTAssertTrue(timed, "time window stayed \((try? pass(server, id)?.timeStart) ?? "empty")")
    }

    private func setStopAt(_ app: XCUIApplication, server: String, id: Int) throws {
        #if os(iOS)
            app.swipeUp()
        #endif
        let picker = app.buttons["pass-stop-at"].firstMatch
        XCTAssertTrue(picker.waitForExistence(timeout: 6) || app.buttons.matching(NSPredicate(format: "label CONTAINS 'Stop at'")).firstMatch.exists, "no Stop at\n\(app.debugDescription)")
        let hit = picker.exists ? picker : app.buttons.matching(NSPredicate(format: "label CONTAINS 'Stop at'")).firstMatch
        choose(app, hit, label: "Stop at")
        pickLabeled(app, containing: "3 unwatched")
        XCTAssertTrue(until(8) { (try? self.pass(server, id)?.limitCount) == 3 }, "Stop at stayed \((try? pass(server, id)?.limitCount).map(String.init) ?? "nil")")
    }

    private func pickLabeled(_ app: XCUIApplication, containing: String, also: String? = nil) {
        let pred = if let also {
            NSPredicate(format: "label CONTAINS %@ AND label CONTAINS %@", containing, also)
        } else {
            NSPredicate(format: "label CONTAINS %@", containing)
        }
        #if os(iOS)
            let option = app.descendants(matching: .any).matching(pred).firstMatch
            if option.waitForExistence(timeout: 5) {
                tap(option)
                return
            }
            if app.pickerWheels.firstMatch.exists {
                app.pickerWheels.element.adjust(toPickerWheelValue: containing)
            }
        #else
            for _ in 0 ..< 40 {
                if focusedText(app).contains(containing) {
                    XCUIRemote.shared.press(.select)
                    return
                }
                XCUIRemote.shared.press(.down)
                RunLoop.current.run(until: Date().addingTimeInterval(0.15))
            }
        #endif
        _ = app
    }

    private func row(_ app: XCUIApplication, _ id: Int) -> XCUIElement {
        app.descendants(matching: .any)["pass-row-\(id)"]
    }

    #if os(iOS)
        private func tap(_ el: XCUIElement) {
            if el.isHittable {
                el.tap()
            } else {
                el.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
        }
    #endif

    private func choose(_ app: XCUIApplication, _ el: XCUIElement, label: String) {
        #if os(tvOS)
            if !focusedText(app).contains(label), !focusedText(app).contains(el.identifier) {
                focusNamed(app, label)
            }
            XCUIRemote.shared.press(.select)
        #else
            tap(el)
        #endif
        _ = app
    }

    private func reveal(_ app: XCUIApplication, identifier: String? = nil, containing: String? = nil) {
        #if os(tvOS)
            let remote = XCUIRemote.shared
            for _ in 0 ..< 40 {
                let here = focusedText(app)
                if let identifier, here.contains(identifier) {
                    return
                }
                if let containing, here.contains(containing) {
                    return
                }
                remote.press(.down)
                RunLoop.current.run(until: Date().addingTimeInterval(0.2))
            }
        #else
            let target = containing ?? identifier ?? ""
            for _ in 0 ..< 8 {
                if target.isEmpty {
                    return
                }
                let hit = app.descendants(matching: .any).matching(NSPredicate(format: "identifier == %@ OR label CONTAINS %@", target, target)).firstMatch
                if hit.exists, hit.isHittable {
                    return
                }
                app.swipeUp()
            }
        #endif
    }

    #if os(tvOS)
        private func focusRow(_ app: XCUIApplication, containing: String) {
            let remote = XCUIRemote.shared
            for _ in 0 ..< 20 {
                if focusedText(app).contains(containing) {
                    return
                }
                remote.press(.down)
                RunLoop.current.run(until: Date().addingTimeInterval(0.25))
            }
            XCTFail("never focused \(containing), at \(focusedText(app))")
        }

        private func focusNamed(_ app: XCUIApplication, _ name: String) {
            let remote = XCUIRemote.shared
            for _ in 0 ..< 8 {
                if focusedText(app).contains(name) {
                    return
                }
                remote.press(.up)
                RunLoop.current.run(until: Date().addingTimeInterval(0.15))
            }
            for _ in 0 ..< 40 {
                if focusedText(app).contains(name) {
                    return
                }
                remote.press(.down)
                RunLoop.current.run(until: Date().addingTimeInterval(0.2))
            }
        }
    #endif

    private func focusedText(_ app: XCUIApplication) -> String {
        let focused = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        guard focused.exists else { return "" }
        let value = focused.value as? String ?? ""
        return "\(focused.identifier) \(focused.label) \(value)"
    }

    private func shot(_ app: XCUIApplication, _ name: String) {
        let dir = lane("BROADWAVE_SHOT_DIR")
        guard !dir.isEmpty else { return }
        let folder = URL(fileURLWithPath: dir, isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let url = folder.appending(path: "\(device)-\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
        print("l101-shot \(url.path)")
        _ = app
    }

    private var device: String {
        #if os(tvOS)
            "tv"
        #else
            "iphone"
        #endif
    }

    private func lane(_ name: String) -> String {
        let env = ProcessInfo.processInfo.environment
        let direct = env[name] ?? ""
        if !direct.isEmpty {
            return direct
        }
        return env["TEST_RUNNER_\(name)"] ?? ""
    }

    private func put(_ server: String, _ body: String) throws {
        var request = URLRequest(url: URL(string: server + "/api/v1/settings")!)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = Data(body.utf8)
        let result = try exchange(request)
        XCTAssertTrue((200 ..< 300).contains(result.status), "put \(result.status) \(result.text)")
    }

    private func clearPasses(_ server: String) throws {
        for pass in try passes(server) {
            deletePass(server, pass.id)
        }
    }

    @discardableResult
    private func addPass(_ server: String, title: String) throws -> PassJSON {
        var request = URLRequest(url: URL(string: server + "/api/v1/passes")!)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = Data(#"{"title":"\#(title)","matchKind":"title","padBefore":1,"padAfter":2}"#.utf8)
        let result = try exchange(request)
        XCTAssertTrue((200 ..< 300).contains(result.status), "add \(result.status) \(result.text)")
        return try XCTUnwrap(decodeList(result.text).first { $0.title == title }, result.text)
    }

    private func passes(_ server: String) throws -> [PassJSON] {
        let result = try exchange(URLRequest(url: URL(string: server + "/api/v1/passes")!))
        XCTAssertEqual(result.status, 200, result.text)
        return decodeList(result.text)
    }

    private func pass(_ server: String, _ id: Int) throws -> PassJSON? {
        try passes(server).first { $0.id == id }
    }

    private func passIDs(_ server: String) throws -> [Int] {
        try passes(server).map(\.id)
    }

    private func deletePass(_ server: String, _ id: Int) {
        var request = URLRequest(url: URL(string: server + "/api/v1/passes/\(id)")!)
        request.httpMethod = "DELETE"
        _ = try? exchange(request)
    }

    private func decodeList(_ text: String) -> [PassJSON] {
        guard let data = text.data(using: .utf8),
              let body = try? JSONDecoder().decode(PassListJSON.self, from: data)
        else { return [] }
        return body.passes
    }

    private func exchange(_ request: URLRequest) throws -> (status: Int, text: String) {
        var output: (Int, String)?
        var failed: Error?
        let done = expectation(description: request.url?.absoluteString ?? "request")
        URLSession.shared.dataTask(with: request) { data, response, error in
            if let error {
                failed = error
            } else {
                let status = (response as? HTTPURLResponse)?.statusCode ?? 0
                output = (status, String(data: data ?? Data(), encoding: .utf8) ?? "")
            }
            done.fulfill()
        }.resume()
        wait(for: [done], timeout: 10)
        if let failed {
            throw failed
        }
        return try XCTUnwrap(output)
    }

    private func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while Date() < end {
            if done() {
                return true
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.25))
        }
        return done()
    }
}

private struct PassListJSON: Decodable {
    var passes: [PassJSON]
}

private struct PassJSON: Decodable {
    var id: Int
    var title: String
    var days: [Int]?
    var timeStart: String?
    var limitCount: Int?
}

private enum PassClock {
    static var sixPM: String {
        var cal = Calendar.current
        cal.locale = .current
        let date = cal.date(bySettingHour: 18, minute: 0, second: 0, of: Date()) ?? Date()
        return date.formatted(date: .omitted, time: .shortened)
    }
}
