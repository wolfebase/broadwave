import Foundation

/// A recording cut at its commercial breaks, for the Apple TV player's chapter list:
/// the parts of the show between breaks, each break, and the credits.
public enum BreakChapters {
    public struct Chapter: Equatable, Sendable {
        public var title: String
        public var start: Double
        /// Nil when the recording's length is not known yet.
        public var end: Double?
        public var isBreak: Bool
    }

    /// Breaks shorter than a second or past the end are dropped, and overlapping ones merged.
    public static func breaks(_ markers: [Marker], duration: Double?) -> [ClosedRange<Double>] {
        let limit = duration.flatMap { $0 > 0 && $0.isFinite ? $0 : nil } ?? .infinity
        var out: [ClosedRange<Double>] = []
        for marker in markers.sorted(by: { $0.start < $1.start }) {
            let start = max(0, marker.start), end = min(marker.end, limit)
            guard end - start >= 1 else { continue }
            if let last = out.last, start <= last.upperBound {
                out[out.count - 1] = last.lowerBound ... max(last.upperBound, end)
            } else {
                out.append(start ... end)
            }
        }
        return out
    }

    public static func make(_ markers: [Marker], duration: Double?, creditsStart: Double? = nil) -> [Chapter] {
        let cuts = breaks(markers, duration: duration)
        let length = duration.flatMap { $0 > 0 && $0.isFinite ? $0 : nil }
        guard !cuts.isEmpty else { return [] }
        var out: [Chapter] = []
        var cursor = 0.0
        var part = 1
        func addPart(until end: Double?) {
            if let end, end - cursor < 1 {
                return
            }
            out.append(Chapter(title: String(localized: "Part \(part)", bundle: .module), start: cursor, end: end, isBreak: false))
            part += 1
        }
        for cut in cuts {
            addPart(until: cut.lowerBound)
            out.append(Chapter(
                title: String(localized: "Commercial break · \(Marker.clock(cut.upperBound - cut.lowerBound))", bundle: .module),
                start: cut.lowerBound, end: cut.upperBound, isBreak: true
            ))
            cursor = cut.upperBound
        }
        if let credits = creditsStart, credits > cursor + 1, length.map({ credits < $0 }) ?? true {
            addPart(until: credits)
            out.append(Chapter(title: String(localized: "Credits", bundle: .module), start: credits, end: length, isBreak: false))
        } else {
            addPart(until: length)
        }
        return out
    }
}
