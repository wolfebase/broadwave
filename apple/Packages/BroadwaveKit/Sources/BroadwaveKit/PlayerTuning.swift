import AVFoundation
import Foundation

/// Size and rate to give the TV. Zeros mean the asset has not reported them yet.
public struct DisplayMatch: Equatable, Sendable {
    public var width: Int
    public var height: Int
    public var refreshRate: Float

    public init(width: Int = 0, height: Int = 0, refreshRate: Float = 0) {
        self.width = width
        self.height = height
        self.refreshRate = refreshRate
    }
}

/// Live buffer and bitrate choices. The room target sits about 10 seconds behind
/// the broadcast, so the forward buffer has to cover that plus a segment.
public enum PlayerTuning {
    public static func forwardBuffer(network: String, tile: Bool) -> TimeInterval {
        if tile {
            return 8
        }
        return network == "cellular" ? 12 : 8
    }

    /// Zero is no cap. A cellular link stays at 4 Mb/s so a 1080p60 rendition does not fill the pipe.
    public static func peakBitRate(network: String) -> Double {
        network == "cellular" ? 4_000_000 : 0
    }

    public static func apply(_ item: AVPlayerItem, network: String, tile: Bool) {
        item.preferredForwardBufferDuration = forwardBuffer(network: network, tile: tile)
        item.preferredPeakBitRate = peakBitRate(network: network)
        // A sync trim plays at 0.98x or 1.02x. The time-domain default cuts
        // and repeats slices of speech to hold pitch, which sounds like the
        // voice catching; spectral stretches it smoothly.
        item.audioTimePitchAlgorithm = .spectral
    }

    /// Folds one measurement into the display match.
    /// A zero width, height, or rate is "not known yet" and is ignored, so a
    /// late 23.976 can replace an early 59.94 and a blank sample cannot invent
    /// 1920×1080. Closing the player resets `previous` to zeros.
    public static func matchingDisplay(width: Int, height: Int, rate: Float, previous: DisplayMatch) -> DisplayMatch {
        var next = previous
        if width > 1, height > 1 {
            next.width = width
            next.height = height
        }
        if rate > 1 {
            next.refreshRate = canonicalRefreshRate(rate)
        }
        return next
    }

    /// The picture to ask the TV for. The asset's size wins. A measurement
    /// within a hundredth of a broadcast rate uses that rate, so 60000/1001
    /// asks for 59.94 and 30000/1001 asks for 29.97. When the asset reports
    /// the integer beside the rate the server named (60 beside 59.94), the
    /// broadcast rate is what we ask for. A true 60 stays 60.
    public static func displayAsked(asset: DisplayMatch, hint: DisplayMatch) -> DisplayMatch {
        var next = hintedDisplay(hint, current: asset)
        next.refreshRate = chooseRefreshRate(
            measured: canonicalRefreshRate(asset.refreshRate),
            hinted: canonicalRefreshRate(hint.refreshRate)
        )
        return next
    }

    /// The words the display log uses, so 59.94 is not printed as 59.94006.
    public static func refreshRateName(_ rate: Float) -> String {
        let asked = canonicalRefreshRate(rate)
        for item in broadcastRates where abs(item.rate - asked) < 0.001 {
            return item.name
        }
        if asked <= 1 {
            return "0"
        }
        return String(format: "%.3f", asked)
    }

    /// Snaps a measurement onto a broadcast rate. Zero stays zero. A rate
    /// that is not near one of the names is kept, so an odd source is not
    /// relabeled.
    public static func canonicalRefreshRate(_ rate: Float) -> Float {
        guard rate > 1 else { return 0 }
        var best: Float = 0
        var gap: Float = 0.01
        for item in broadcastRates {
            let distance = abs(item.rate - rate)
            if distance < gap {
                gap = distance
                best = item.rate
            }
        }
        if best > 0 {
            return best
        }
        return rate
    }

    private static func chooseRefreshRate(measured: Float, hinted: Float) -> Float {
        if measured <= 1 {
            return hinted
        }
        if hinted <= 1 || abs(measured - hinted) < 0.001 {
            return measured
        }
        if let broadcast = broadcastPair(measured, hinted) {
            return broadcast
        }
        return measured
    }

    /// 23.976/24, 29.97/30, and 59.94/60. The fractional rate is the one to ask for.
    private static func broadcastPair(_ a: Float, _ b: Float) -> Float? {
        let pairs: [(Float, Float)] = [(23.976, 24), (29.97, 30), (59.94, 60)]
        for (fraction, integer) in pairs {
            let hit = (near(a, fraction) && near(b, integer)) || (near(a, integer) && near(b, fraction))
            if hit {
                return fraction
            }
        }
        return nil
    }

    private static func near(_ a: Float, _ b: Float) -> Bool {
        abs(a - b) < 0.001
    }

    private static let broadcastRates: [(rate: Float, name: String)] = [
        (23.976, "23.976"),
        (24, "24"),
        (25, "25"),
        (29.97, "29.97"),
        (30, "30"),
        (50, "50"),
        (59.94, "59.94"),
        (60, "60"),
    ]

    public static func displayReady(_ match: DisplayMatch) -> Bool {
        match.width > 1 && match.height > 1 && match.refreshRate > 1
    }

    /// A server hint fills only fields the asset has not reported, so a
    /// 1280×720 picture is not put back to 1920×1080.
    public static func hintedDisplay(_ hint: DisplayMatch, current: DisplayMatch) -> DisplayMatch {
        var next = current
        if next.width <= 1, hint.width > 1 {
            next.width = hint.width
        }
        if next.height <= 1, hint.height > 1 {
            next.height = hint.height
        }
        if next.refreshRate <= 1, hint.refreshRate > 1 {
            next.refreshRate = hint.refreshRate
        }
        return next
    }
}
