import XCTest

/// The live transport bar names the program and shows where it is in its slot, and the
/// Info tab leads with the program. Production listings (TEST_RUNNER_BROADWAVE_SERVER);
/// nothing is recorded.
final class ProgramBarTests: WatchingCase {
    func testTheBarShowsTheProgram() throws {
        try openBar([])
        let timeline = element("program-timeline")
        XCTAssertTrue(until(5) { timeline.exists }, "no program timeline with the bar up")
        XCTAssertTrue(timeline.label.contains(" to "), "timeline says \(timeline.label)")
        let program = try XCTUnwrap(timeline.label.components(separatedBy: " on ").first)
        shot("program-0-bar")
        // Info: the program leads, the channel is under it.
        press(.down, wait: 2.5)
        shot("program-1-info")
        XCTAssertTrue(until(3) { !timeline.exists }, "the timeline stays over the Info tab")
        // The guide under the player has the same words in its grid; the Info heading is
        // the one at the panel's top left (about 650 pt down a 1080 pt screen).
        let named = app.staticTexts.matching(NSPredicate(format: "label == %@", program))
        let heading = until(3) {
            named.allElementsBoundByIndex.contains { (600 ... 720).contains($0.frame.minY) && $0.frame.minX < 200 }
        }
        XCTAssertTrue(heading, "the Info tab does not lead with \(program): \(named.allElementsBoundByIndex.map(\.frame))")
    }

    private func openBar(_ extra: [String]) throws {
        try launch(["-BroadwaveWatch", "28", "-BroadwaveChannelNow", "YES"] + extra)
        let player = element("player-channel")
        let tuning = element("tuning")
        XCTAssertTrue(until(30) { player.exists && !tuning.exists }, "the picture never started")
        let probe = element("transportProbe")
        until(15) { probe.label == "hidden" }
        wait(2)
        press(.select, wait: 1.5)
        XCTAssertTrue(until(5) { probe.label == "shown" }, "the bar did not come up")
    }
}
