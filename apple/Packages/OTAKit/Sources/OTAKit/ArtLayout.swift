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
}
