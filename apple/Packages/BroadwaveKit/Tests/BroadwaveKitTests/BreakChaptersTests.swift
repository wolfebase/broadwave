@testable import BroadwaveKit
import Foundation
import Testing

private func marker(_ id: Int64, _ start: Double, _ end: Double) -> Marker {
    Marker(id: id, recordingId: 1, start: start, end: end)
}

@Test func chaptersAlternatePartsAndBreaks() {
    let chapters = BreakChapters.make([marker(2, 900, 1050), marker(1, 300, 420)], duration: 1800)
    #expect(chapters.map(\.title) == [
        "Part 1", "Commercial break · 2:00", "Part 2", "Commercial break · 2:30", "Part 3",
    ])
    #expect(chapters.map(\.start) == [0, 300, 420, 900, 1050])
    #expect(chapters.last?.end == 1800)
    #expect(chapters.filter(\.isBreak).count == 2)
}

@Test func chaptersMergeOverlapsAndDropSlivers() {
    let cuts = BreakChapters.breaks([marker(1, 100, 200), marker(2, 180, 260), marker(3, 500, 500.5), marker(4, 1700, 1900)], duration: 1800)
    #expect(cuts == [100 ... 260, 1700 ... 1800])
}

@Test func chaptersEndOnCreditsAndNoneWithoutBreaks() {
    #expect(BreakChapters.make([], duration: 1800).isEmpty)
    let chapters = BreakChapters.make([marker(1, 0, 60)], duration: 1800, creditsStart: 1740)
    // A break at the very start leaves no empty first part.
    #expect(chapters.map(\.title) == ["Commercial break · 1:00", "Part 1", "Credits"])
    #expect(chapters[1].end == 1740)
}

@Test func chaptersWithoutALengthLeaveTheLastPartOpen() {
    let chapters = BreakChapters.make([marker(1, 300, 400)], duration: nil)
    #expect(chapters.last?.title == "Part 2")
    #expect(chapters.last?.end == nil)
}
