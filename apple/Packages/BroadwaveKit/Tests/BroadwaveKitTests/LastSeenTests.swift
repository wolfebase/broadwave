@testable import BroadwaveKit
import Foundation
import Testing

private let now = ISO8601DateFormatter().date(from: "2026-10-04T12:00:00Z")!

@Test func lastSeenIsAShortRelativeTime() {
    #expect(LastSeen.phrase(nil, now: now) == "")
    #expect(LastSeen.phrase("not-a-time", now: now) == "")
    #expect(LastSeen.phrase("2026-10-04T11:59:40Z", now: now) == "just now")
    #expect(LastSeen.phrase("2026-10-04T11:55:00Z", now: now) == "5 minutes ago")
    #expect(LastSeen.phrase("2026-10-04T11:59:00Z", now: now) == "1 minute ago")
    #expect(LastSeen.phrase("2026-10-04T11:00:00Z", now: now) == "1 hour ago")
    #expect(LastSeen.phrase("2026-10-04T09:00:00Z", now: now) == "3 hours ago")
    #expect(LastSeen.phrase("2026-10-03T12:00:00Z", now: now) == "1 day ago")
    #expect(LastSeen.phrase("2026-10-01T12:00:00Z", now: now) == "3 days ago")
}

@Test func anOfflineCardNamesWhenTheDeviceWasLastSeen() {
    #expect(LastSeen.offline("5 minutes ago") == "Offline. Last seen 5 minutes ago.")
    #expect(LastSeen.offline("") == "Offline.")
}
