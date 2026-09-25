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
            next.refreshRate = rate
        }
        return next
    }

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
