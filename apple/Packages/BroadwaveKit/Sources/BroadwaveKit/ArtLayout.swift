import Foundation

/// How a picture can be shown. Full-bleed needs landscape art at least 1280 px wide,
/// and the slot must stay within 1.25× that width.
public enum ArtLayout {
    public static func choose(width: Int, height: Int, slot: Int) -> String {
        if width >= 1280, height > 0, width > height, slot > 0, slot * 4 <= width * 5 {
            return "bleed"
        }
        return "composed"
    }

    /// Longest side to draw, in pixels. Zero means the picture's own size.
    public static func displayEdge(native: Int, slot: Int) -> Int {
        guard native > 0, slot > 0 else { return 0 }
        let limit = native + native / 4
        return slot < limit ? slot : limit
    }

    /// Wide guide cells keep a thumbnail beside the title. 220 matches the web grid.
    public static func showsCellArt(slot: Int) -> Bool {
        slot > 220
    }

    /// Points to draw so this side stays within 1.25× its native pixels.
    /// Zero when the size is unknown: the caller draws at the slot.
    public static func cappedPoints(native: Int, slotPoints: Double, scale: Double) -> Double {
        guard native > 0, slotPoints > 0 else { return 0 }
        let scale = max(scale, 1)
        let slot = Int((slotPoints * scale).rounded())
        let edge = displayEdge(native: native, slot: slot)
        guard edge > 0 else { return 0 }
        return Double(edge) / scale
    }
}
