import CoreGraphics

/// Equal 16:9 tiles. One row stays centered in each cell. Two or more rows pack
/// with exactly `gap` between the pictures and the group centered in the box,
/// so a tall iPad portrait does not split the leftover space between the rows.
public enum MosaicFrames {
    private static let wide = 16.0 / 9.0

    public static func equalGrid(columns: Int, rows: Int, count: Int, in box: CGSize, gap: CGFloat) -> [CGRect] {
        guard columns > 0, rows > 0, count > 0, box.width > 1, box.height > 1 else { return [] }
        if rows <= 1 {
            return centeredCells(columns: columns, rows: rows, count: count, in: box, gap: gap)
        }
        let byWidth = (box.width - gap * CGFloat(columns - 1)) / CGFloat(columns)
        var tile = CGSize(width: byWidth, height: byWidth / wide)
        let stacked = tile.height * CGFloat(rows) + gap * CGFloat(rows - 1)
        if stacked > box.height {
            let byHeight = (box.height - gap * CGFloat(rows - 1)) / CGFloat(rows)
            tile = CGSize(width: byHeight * wide, height: byHeight)
        }
        let group = CGSize(
            width: tile.width * CGFloat(columns) + gap * CGFloat(columns - 1),
            height: tile.height * CGFloat(rows) + gap * CGFloat(rows - 1)
        )
        let origin = CGPoint(x: (box.width - group.width) / 2, y: (box.height - group.height) / 2)
        return (0 ..< count).map { index in
            let col = index % columns
            let row = index / columns
            return CGRect(
                x: origin.x + CGFloat(col) * (tile.width + gap),
                y: origin.y + CGFloat(row) * (tile.height + gap),
                width: tile.width,
                height: tile.height
            )
        }
    }

    private static func centeredCells(columns: Int, rows: Int, count: Int, in box: CGSize, gap: CGFloat) -> [CGRect] {
        let cellW = (box.width - gap * CGFloat(columns - 1)) / CGFloat(columns)
        let cellH = (box.height - gap * CGFloat(rows - 1)) / CGFloat(rows)
        return (0 ..< count).map { index in
            let col = index % columns
            let row = index / columns
            let region = CGRect(
                x: CGFloat(col) * (cellW + gap),
                y: CGFloat(row) * (cellH + gap),
                width: cellW,
                height: cellH
            )
            return fit(region)
        }
    }

    private static func fit(_ region: CGRect) -> CGRect {
        guard region.width > 1, region.height > 1 else { return .zero }
        let byWidth = CGSize(width: region.width, height: region.width / wide)
        let size = byWidth.height <= region.height
            ? byWidth
            : CGSize(width: region.height * wide, height: region.height)
        return CGRect(
            x: region.minX + (region.width - size.width) / 2,
            y: region.minY + (region.height - size.height) / 2,
            width: size.width,
            height: size.height
        )
    }
}
