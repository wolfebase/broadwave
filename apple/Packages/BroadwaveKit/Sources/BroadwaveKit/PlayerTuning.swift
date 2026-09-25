import AVFoundation
import Foundation

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
}
