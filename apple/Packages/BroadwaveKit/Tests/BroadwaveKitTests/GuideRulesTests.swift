@testable import BroadwaveKit
import Foundation
import Testing

private func airing(_ title: String, category: String) -> Airing {
    let start = Date(timeIntervalSince1970: 1_000_000)
    return Airing(id: 1, channelId: 1, title: title, category: category, start: start, end: start.addingTimeInterval(3600))
}

@Test func newsInATitleNeedsAWordBoundary() {
    #expect(airing("Newsroom Weekly", category: "Series").kind == .series)
    #expect(airing("Evening News", category: "Series").kind == .news)
    #expect(airing("Desk Notes", category: "News magazine").kind == .news)
}

@Test func gameDayWithAMatchupIsSports() {
    #expect(airing("Game Day: North at South", category: "Special").kind == .sports)
    #expect(airing("Game Day", category: "Special").kind == .series)
}

@Test func minutesLeftRoundsTheSameWayAsTheWeb() {
    let start = Date(timeIntervalSince1970: 1_000_000)
    let show = Airing(id: 1, channelId: 9, title: "Evening News", start: start, end: start.addingTimeInterval(3600))
    #expect(show.minutesLeft(at: show.end.addingTimeInterval(-30)) == "1m left")
    #expect(show.minutesLeft(at: show.end.addingTimeInterval(-90)) == "2m left")
    #expect(show.minutesLeft(at: show.end.addingTimeInterval(-59.5 * 60)) == "1h 0m left")
    #expect(show.minutesLeft(at: show.end.addingTimeInterval(-90 * 60)) == "1h 30m left")
    #expect(show.minutesLeft(at: show.end) == "0m left")
}
