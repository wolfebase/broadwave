@testable import BroadwaveKit
import Foundation
import Testing

@Test func guideOriginIsTheHalfHourBeforeNow() throws {
    var cal = Calendar(identifier: .gregorian)
    cal.timeZone = try #require(TimeZone(identifier: "America/Chicago"))
    func at(_ hour: Int, _ minute: Int, _ second: Int = 0) throws -> Date {
        try #require(cal.date(from: DateComponents(year: 2026, month: 9, day: 26, hour: hour, minute: minute, second: second)))
    }
    #expect(try guideOrigin(for: at(2, 57, 41), calendar: cal) == at(2, 0))
    #expect(try guideOrigin(for: at(2, 30), calendar: cal) == at(2, 0))
    #expect(try guideOrigin(for: at(2, 10, 5), calendar: cal) == at(1, 30))
    #expect(try guideOrigin(for: at(0, 5), calendar: cal) == (at(23, 30).addingTimeInterval(-86400)))
    for minute in stride(from: 0, to: 60, by: 7) {
        let now = try at(14, minute, 30)
        let origin = guideOrigin(for: now, calendar: cal)
        #expect(origin <= now.addingTimeInterval(-30 * 60))
        #expect(now.timeIntervalSince(origin) < 60 * 60)
    }
}
