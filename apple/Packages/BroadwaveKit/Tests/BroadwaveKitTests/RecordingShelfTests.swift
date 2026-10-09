@testable import BroadwaveKit
import Foundation
import Testing

@Test func recordingNowListsWhatIsRecordingOldestFirst() {
    let t = Date(timeIntervalSince1970: 1_790_500_000)
    func rec(_ id: Int64, status: String, ago: TimeInterval, subtitle: String? = nil, ends: TimeInterval? = nil) -> Recording {
        Recording(
            id: id, channelId: id, guideNumber: "\(id).1", title: "Show \(id)", subtitle: subtitle,
            status: status, startedAt: t.addingTimeInterval(-ago), endsAt: ends.map { t.addingTimeInterval($0) }
        )
    }
    let rows = WidgetFeed.recordingNow([
        rec(3, status: "recorded", ago: 10),
        rec(1, status: "recording", ago: 100, subtitle: "Part one", ends: 1000),
        rec(2, status: "recording", ago: 50),
        rec(4, status: "recording", ago: 40),
        rec(5, status: "recording", ago: 30),
        rec(6, status: "recording", ago: 20),
    ])
    #expect(rows.map(\.id) == ["rec-1", "rec-2", "rec-4", "rec-5"])
    #expect(rows[0].title == "Show 1")
    #expect(rows[0].detail == "Part one")
    #expect(rows[0].number == "1.1")
    #expect(rows[0].link == URL(string: "broadwave://watch/1"))
    #expect(rows[0].end == t.addingTimeInterval(1000))
    #expect(rows[1].detail == "")
    #expect(WidgetFeed.recordingNow([rec(9, status: "recording", ago: 1)], limit: 0).isEmpty)
}
