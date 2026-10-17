@testable import BroadwaveKit
import Foundation
import Testing

@Test func aLiveWindowComesFromTheItemsClock() throws {
    let now = Date(timeIntervalSince1970: 1_800_000_000)
    let window = try #require(LiveWindow.make(currentDate: now, currentSeconds: 600, seekable: [0 ... 614]))
    #expect(window.start == now.addingTimeInterval(-600))
    #expect(window.edge == now.addingTimeInterval(14))
    #expect(window.isLive)
    #expect(abs(window.fraction - 600.0 / 614.0) < 0.0001)
    #expect(LiveWindow.make(currentDate: nil, currentSeconds: 1, seekable: [0 ... 2]) == nil)
    #expect(LiveWindow.make(currentDate: now, currentSeconds: 1, seekable: []) == nil)
}

@Test func backTenStopsAtTheOldestFrameAndForwardAtLive() {
    let now = Date(timeIntervalSince1970: 1_800_000_000)
    let window = LiveWindow(start: now.addingTimeInterval(-5), position: now, edge: now.addingTimeInterval(4))
    #expect(window.moved(by: -10) == window.start)
    #expect(window.moved(by: 10) == window.edge)
    #expect(window.date(at: 0.5) == window.start.addingTimeInterval(4.5))
    #expect(window.holds(now.addingTimeInterval(-3)))
    #expect(!window.holds(now.addingTimeInterval(-6)))
}

@Test func farBehindLiveSaysHowFar() {
    let now = Date(timeIntervalSince1970: 1_800_000_000)
    let behind = LiveWindow(start: now.addingTimeInterval(-3600), position: now, edge: now.addingTimeInterval(245))
    #expect(!behind.isLive)
    #expect(LiveWindow.behindText(behind.behind) == "\u{2212}4:05")
    #expect(LiveWindow.behindText(3729) == "\u{2212}1:02:09")
    #expect(LiveWindow.behindText(-3) == "\u{2212}0:00")
}
