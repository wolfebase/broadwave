import XCTest

/// One room per tile, on a real server. A playlist tile beside a tuner tile:
/// stopping the playlist must not pause the tuner, the playlist comes back on
/// its own, Play/Pause moves both, and each tile is within 50 ms of its room.
/// Opt-in. The harness sets TEST_RUNNER_BROADWAVE_SERVER, _ORIGIN, _TUNER,
/// _PLAYLIST, _LOG, and _SHOTS.
final class MultiviewRoomTests: XCTestCase {
    private let picture = "The picture stopped. Trying again usually fixes it."

    func testPlaylistTileDoesNotPauseTheTuner() throws {
        executionTimeAllowance = 600
        continueAfterFailure = false
        let env = ProcessInfo.processInfo.environment
        func lane(_ name: String) -> String {
            let direct = env[name] ?? ""
            if !direct.isEmpty {
                return direct
            }
            return env["TEST_RUNNER_\(name)"] ?? ""
        }
        let server = lane("BROADWAVE_SERVER")
        let origin = lane("BROADWAVE_ORIGIN")
        print("broadwave-l32 server=\(server.isEmpty ? "-" : server) origin=\(origin.isEmpty ? "-" : origin)")
        guard !server.isEmpty, !origin.isEmpty,
              let tuner = Int(lane("BROADWAVE_TUNER")),
              let playlist = Int(lane("BROADWAVE_PLAYLIST")),
              !lane("BROADWAVE_LOG").isEmpty
        else {
            throw XCTSkip("set TEST_RUNNER_BROADWAVE_SERVER, _ORIGIN, _TUNER, _PLAYLIST, and _LOG")
        }
        let logPath = lane("BROADWAVE_LOG")
        let shots = lane("BROADWAVE_SHOTS")
        let app = XCUIApplication()
        app.launchArguments = [
            "-BroadwaveServerURL", server,
            "-BroadwaveMultiview", "\(playlist),\(tuner)",
            "-BroadwaveMultiviewLayout", "2up",
            "-BroadwaveSyncLog", "1",
            "-ApplePersistenceIgnoreState", "YES",
        ]
        app.launch()

        // tvOS can expose the focused tile twice while its picture message updates.
        let tunerTile = app.buttons.matching(identifier: "tile-\(tuner)").firstMatch
        let playlistTile = app.buttons.matching(identifier: "tile-\(playlist)").firstMatch
        XCTAssertTrue(tunerTile.waitForExistence(timeout: 40), app.debugDescription)
        XCTAssertTrue(playlistTile.exists, app.debugDescription)

        XCTAssertTrue(until(90) { self.settled(tunerTile) && self.settled(playlistTile) }, "tiles did not lock tuner=\(shown(tunerTile)) playlist=\(shown(playlistTile))")
        XCTAssertTrue(until(15) { self.separateRooms(tuner: tuner, playlist: playlist, in: logPath) }, "tiles did not join their own rooms \(roomNames(in: logPath))\n\(tail(logPath))")
        shot(shots, "playing")

        post(origin, "/stop")
        let stopped = Date()
        var tunerRates: [Double] = []
        var showed = false
        var shotStopped = false
        let hold = until(40) {
            if let rate = self.live(tunerTile)?.rate {
                tunerRates.append(rate)
            }
            if self.outageCount(tunerTile, playlistTile) >= 1 {
                showed = true
                if !shotStopped {
                    self.shot(shots, "stopped")
                    shotStopped = true
                }
            }
            return Date().timeIntervalSince(stopped) >= 30
        }
        XCTAssertTrue(hold)
        XCTAssertTrue(showed, "the playlist tile did not name the picture \(shown(playlistTile))\n\(tail(logPath))")
        XCTAssertEqual(outageCount(tunerTile, playlistTile), 1, "the tuner tile showed the picture message \(shown(tunerTile))")
        XCTAssertFalse(tunerRates.contains { $0 < 0.05 }, "the tuner paused while the playlist was down \(tunerRates)")
        XCTAssertTrue(tunerRates.contains { $0 >= 0.5 }, "no tuner samples during the stop")
        XCTAssertEqual(outageCount(tunerTile, playlistTile), 1, "the picture message left, or the tuner showed it too")
        XCTAssertTrue(until(8) { self.logText(logPath).contains(self.picture) }, "the log did not name the picture\n\(tail(logPath))")

        post(origin, "/start")
        let started = Date()
        let back = until(30) {
            self.outageCount(tunerTile, playlistTile) == 0 && self.settled(playlistTile)
        }
        XCTAssertTrue(back, "the playlist picture was not back on its own \(shown(playlistTile))\n\(tail(logPath))")
        print("broadwave-l32 back \(String(format: "%.1f", Date().timeIntervalSince(started)))s")
        shot(shots, "back")

        let soakStart = Date()
        XCTAssertTrue(until(130) { Date().timeIntervalSince(soakStart) >= 120 })
        let since = soakStart.addingTimeInterval(90)
        let spread = mediaSpread(tuner: tuner, playlist: playlist, in: logPath, since: since)
        let tunerLock = lockDrift(tuner, in: logPath, since: since)
        let playlistLock = lockDrift(playlist, in: logPath, since: since)
        print("broadwave-l32 drift tuner \(tunerLock) playlist \(playlistLock) spread \(spread)")
        XCTAssertTrue(observed(tuner, in: logPath, since: since), "no tuner drift during the soak")
        XCTAssertTrue(observed(playlist, in: logPath, since: since), "no playlist drift during the soak")
        XCTAssertLessThanOrEqual(tunerLock, 50, "tuner drift \(tunerLock)")
        XCTAssertLessThanOrEqual(playlistLock, 50, "playlist drift \(playlistLock)")
        shot(shots, "soak")

        let remote = XCUIRemote.shared
        remote.press(.playPause)
        let pausedLabel = app.staticTexts.matching(identifier: "multiview-paused").firstMatch
        var didPause = until(4) { pausedLabel.exists }
        if !didPause {
            remote.press(.playPause)
            didPause = until(4) { pausedLabel.exists }
        }
        XCTAssertTrue(didPause, "Play/Pause did not pause the grid \(focused(app))")
        XCTAssertTrue(until(6) {
            (self.live(tunerTile)?.rate ?? 1) < 0.05 && (self.live(playlistTile)?.rate ?? 1) < 0.05
        }, "one tile kept playing tuner=\(shown(tunerTile)) playlist=\(shown(playlistTile))")
        shot(shots, "paused")

        remote.press(.playPause)
        XCTAssertTrue(until(8) { !pausedLabel.exists }, "Play did not resume")
        XCTAssertTrue(until(12) {
            self.settled(tunerTile) && self.settled(playlistTile)
        }, "one tile stayed paused tuner=\(shown(tunerTile)) playlist=\(shown(playlistTile))")
        shot(shots, "resumed")
        XCTAssertEqual(app.state, .runningForeground)
    }

    private struct Sample {
        var at: Date
        var drift: Int
        var rate: Double
    }

    private struct TileReading {
        var channel: Int
        var drift: Int
        var rate: Double
    }

    private func post(_ origin: String, _ path: String) {
        let exp = expectation(description: path)
        guard let url = URL(string: origin + path) else {
            XCTFail("bad url \(origin)\(path)")
            return
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.timeoutInterval = 5
        URLSession.shared.dataTask(with: request) { _, _, error in
            XCTAssertNil(error)
            exp.fulfill()
        }.resume()
        wait(for: [exp], timeout: 8)
    }

    /// The outage is drawn over the tile. The tile is one accessibility
    /// element, so the sentence is on the button's value, not a separate text.
    private func outageCount(_ tuner: XCUIElement, _ playlist: XCUIElement) -> Int {
        [tuner, playlist].filter { ($0.value as? String)?.contains(picture) == true }.count
    }

    private struct Live {
        var drift: Int
        var rate: Double
        var state: String
    }

    /// The tile button's value, written while the sync log is on.
    private func live(_ element: XCUIElement) -> Live? {
        let text = element.value as? String ?? ""
        let pattern = #"drift (-?\d+), rate ([0-9.]+), ([A-Za-z]+)"#
        guard let regex = try? NSRegularExpression(pattern: pattern),
              let match = regex.firstMatch(in: text, range: NSRange(text.startIndex..., in: text)),
              let drift = Int(group(text, match, 1)),
              let rate = Double(group(text, match, 2))
        else { return nil }
        return Live(drift: drift, rate: rate, state: group(text, match, 3))
    }

    private func settled(_ element: XCUIElement) -> Bool {
        guard let live = live(element) else { return false }
        return live.state == "locked" && live.rate >= 0.5 && abs(live.drift) <= 80
    }

    private func shown(_ element: XCUIElement) -> String {
        element.value as? String ?? ""
    }

    private func observed(_ channel: Int, in path: String, since: Date) -> Bool {
        if !syncDrifts(for: channel, in: path, since: since).isEmpty {
            return true
        }
        return !samples(for: channel, in: path).filter { $0.at >= since }.isEmpty
    }

    private func lockDrift(_ channel: Int, in path: String, since: Date) -> Int {
        let drifts = syncDrifts(for: channel, in: path, since: since)
        if !drifts.isEmpty {
            return median(drifts.map(abs))
        }
        return median(samples(for: channel, in: path).filter { $0.at >= since }.map { abs($0.drift) })
    }

    /// `log stream` ends lines with a carriage return. In a Swift string that
    /// return and the following newline are one character, so splitting on
    /// "\n" leaves the whole file as a single line.
    private func lines(in path: String) -> [String] {
        logText(path).split(whereSeparator: \.isNewline).map(String.init)
    }

    private func samples(for channel: Int, in path: String) -> [Sample] {
        var out: [Sample] = []
        for line in lines(in: path) {
            guard let at = stamp(String(line)),
                  let match = line.range(of: #"tile channel=(\d+) drift=(-?\d+).*rate=([0-9.]+)"#, options: .regularExpression)
            else { continue }
            let parts = String(line[match])
            guard let parsed = tileParts(parts), parsed.channel == channel else { continue }
            out.append(Sample(at: at, drift: parsed.drift, rate: parsed.rate))
        }
        return out
    }

    private func tileParts(_ text: String) -> TileReading? {
        let pattern = #"tile channel=(\d+) drift=(-?\d+).*rate=([0-9.]+)"#
        guard let regex = try? NSRegularExpression(pattern: pattern),
              let match = regex.firstMatch(in: text, range: NSRange(text.startIndex..., in: text)),
              let channel = Int(group(text, match, 1)),
              let drift = Int(group(text, match, 2)),
              let rate = Double(group(text, match, 3))
        else { return nil }
        return TileReading(channel: channel, drift: drift, rate: rate)
    }

    private func syncDrifts(for channel: Int, in path: String, since: Date) -> [Int] {
        var out: [Int] = []
        for line in lines(in: path) {
            let text = String(line)
            guard let at = stamp(text), at >= since, let room = roomName(text), owns(room, channel) else { continue }
            guard let drift = value(text, #"drift=(-?\d+)"#) else { continue }
            out.append(drift)
        }
        return out
    }

    /// `sync room=multiview:<id>:<channel> wall=…`. The channel is the last field,
    /// so a session id that starts with a digit is not that channel.
    private func roomName(_ text: String) -> String? {
        guard text.contains("sync room=multiview:"),
              let range = text.range(of: #"room=multiview:\S+"#, options: .regularExpression)
        else { return nil }
        return String(text[range]).replacingOccurrences(of: "room=", with: "")
    }

    private func owns(_ room: String, _ channel: Int) -> Bool {
        room.split(separator: ":").last == Substring(String(channel))
    }

    private func separateRooms(tuner: Int, playlist: Int, in path: String) -> Bool {
        let rooms = roomNames(in: path)
        guard rooms.contains(where: { $0.hasSuffix(":\(tuner)") }),
              rooms.contains(where: { $0.hasSuffix(":\(playlist)") })
        else { return false }
        return rooms.allSatisfy { $0.split(separator: ":").count >= 3 }
    }

    private func roomNames(in path: String) -> [String] {
        var names: [String] = []
        let pattern = #"tile room=(\S+)"#
        for line in lines(in: path) {
            let text = String(line)
            guard let name = text.range(of: pattern, options: .regularExpression).map({ String(text[$0]) }) else { continue }
            let room = name.replacingOccurrences(of: "tile room=", with: "")
            if !names.contains(room) {
                names.append(room)
            }
        }
        return names
    }

    private func mediaSpread(tuner: Int, playlist: Int, in path: String, since: Date) -> Int {
        var tunerMedia: [(Date, Int)] = []
        var playlistMedia: [(Date, Int)] = []
        for line in lines(in: path) {
            let text = String(line)
            guard let at = stamp(text), at >= since, text.contains("sync room=multiview:") else { continue }
            guard let media = value(text, #"media=(-?\d+)"#), let room = roomName(text) else { continue }
            if owns(room, tuner) {
                tunerMedia.append((at, media))
            } else if owns(room, playlist) {
                playlistMedia.append((at, media))
            }
        }
        var gaps: [Int] = []
        for (at, media) in tunerMedia {
            guard let other = playlistMedia.min(by: { abs($0.0.timeIntervalSince(at)) < abs($1.0.timeIntervalSince(at)) }),
                  abs(other.0.timeIntervalSince(at)) < 1
            else { continue }
            gaps.append(abs(media - other.1))
        }
        return median(gaps)
    }

    private func value(_ text: String, _ pattern: String) -> Int? {
        guard let regex = try? NSRegularExpression(pattern: pattern),
              let match = regex.firstMatch(in: text, range: NSRange(text.startIndex..., in: text))
        else { return nil }
        return Int(group(text, match, 1))
    }

    private func group(_ text: String, _ match: NSTextCheckingResult, _ index: Int) -> String {
        guard let range = Range(match.range(at: index), in: text) else { return "" }
        return String(text[range])
    }

    /// Log lines start with `2026-09-27 19:34:32.389`. A date formatter in the
    /// simulator missed them, so the fields are read directly.
    private func stamp(_ line: String) -> Date? {
        let pattern = #"(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})\.(\d{3})"#
        guard let regex = try? NSRegularExpression(pattern: pattern),
              let match = regex.firstMatch(in: line, range: NSRange(line.startIndex..., in: line)),
              let year = Int(group(line, match, 1)),
              let month = Int(group(line, match, 2)),
              let day = Int(group(line, match, 3)),
              let hour = Int(group(line, match, 4)),
              let minute = Int(group(line, match, 5)),
              let second = Int(group(line, match, 6)),
              let millis = Int(group(line, match, 7))
        else { return nil }
        var parts = DateComponents()
        parts.calendar = Calendar(identifier: .gregorian)
        parts.timeZone = .current
        parts.year = year
        parts.month = month
        parts.day = day
        parts.hour = hour
        parts.minute = minute
        parts.second = second
        parts.nanosecond = millis * 1_000_000
        return parts.date
    }

    private func median(_ values: [Int]) -> Int {
        guard !values.isEmpty else { return 0 }
        let sorted = values.sorted()
        return sorted[sorted.count / 2]
    }

    private func logText(_ path: String) -> String {
        (try? String(contentsOfFile: path, encoding: .utf8)) ?? ""
    }

    private func tail(_ path: String) -> String {
        lines(in: path).suffix(30).joined(separator: "\n")
    }

    private func shot(_ dir: String, _ name: String) {
        guard !dir.isEmpty else { return }
        let url = URL(fileURLWithPath: dir).appendingPathComponent("\(name).png")
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: url)
    }

    private func focused(_ app: XCUIApplication) -> String {
        let element = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        return element.exists ? "\(element.identifier) \(element.label)" : "nothing"
    }

    private func until(_ seconds: TimeInterval, _ done: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while Date() < end {
            if done() {
                return true
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.4))
        }
        return done()
    }
}
