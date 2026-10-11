import BroadwaveKit
import CoreGraphics
import Testing

@Test func quadRowsSitOneGapApart() {
    let frames = MosaicFrames.equalGrid(columns: 2, rows: 2, count: 4, in: CGSize(width: 800, height: 1200), gap: 8)
    #expect(frames.count == 4)
    #expect(abs(frames[2].minY - frames[0].maxY - 8) < 0.01)
    #expect(abs(frames[3].minY - frames[1].maxY - 8) < 0.01)
    for frame in frames {
        #expect(abs(frame.width / frame.height - 16.0 / 9.0) < 0.01)
    }
    #expect(abs(frames[0].minX - (800 - frames[1].maxX)) < 0.01)
    #expect(abs(frames[0].minY - (1200 - frames[2].maxY)) < 0.01)
}

@Test func oneRowStaysCenteredInATallCell() {
    let frames = MosaicFrames.equalGrid(columns: 2, rows: 1, count: 2, in: CGSize(width: 800, height: 1200), gap: 8)
    #expect(abs(frames[0].minY - (1200 - frames[0].height) / 2) < 0.01)
    #expect(frames[0].minY > 8)
}

@Test func aShortBoxLimitsTheQuadByHeight() {
    let frames = MosaicFrames.equalGrid(columns: 2, rows: 2, count: 4, in: CGSize(width: 1000, height: 400), gap: 8)
    for frame in frames {
        #expect(frame.maxX <= 1000)
        #expect(frame.maxY <= 400)
    }
    #expect(abs(frames[2].minY - frames[0].maxY - 8) < 0.01)
}
