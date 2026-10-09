@testable import BroadwaveKit
import Foundation
import Testing

private struct Stamped: Decodable {
    var t: Date
}

@Test func datesStayRightWhenManyResponsesDecodeAtOnce() async {
    // 2026-10-08 23:00:00 Z, and half a second later.
    let plainInstant = Date(timeIntervalSince1970: 1_791_500_400)
    let fractionInstant = plainInstant.addingTimeInterval(0.5)
    let plain = Data(#"{"t":"2026-10-08T23:00:00Z"}"#.utf8)
    let fraction = Data(#"{"t":"2026-10-08T23:00:00.500Z"}"#.utf8)
    let failed = await withTaskGroup(of: Int.self) { group in
        for _ in 0 ..< 8 {
            group.addTask {
                var misses = 0
                for _ in 0 ..< 40 {
                    let decodedPlain = try? APIClient.decoder.decode(Stamped.self, from: plain)
                    let decodedFraction = try? APIClient.decoder.decode(Stamped.self, from: fraction)
                    if decodedPlain?.t != plainInstant {
                        misses += 1
                    }
                    if decodedFraction?.t != fractionInstant {
                        misses += 1
                    }
                    if ISO8601DateFormatter.date(from: "2026-10-08T23:00:00Z") != plainInstant {
                        misses += 1
                    }
                    if ISO8601DateFormatter.plainString(from: plainInstant) != "2026-10-08T23:00:00Z" {
                        misses += 1
                    }
                    let written = ISO8601DateFormatter.fractionalString(from: fractionInstant)
                    if ISO8601DateFormatter.date(from: written) != fractionInstant {
                        misses += 1
                    }
                }
                return misses
            }
        }
        var total = 0
        for await misses in group {
            total += misses
        }
        return total
    }
    #expect(failed == 0)
}
