import AVFoundation
@testable import BroadwaveKit
import Foundation
import Testing

@Test func lanLiveHasNoPeakBitrateCap() throws {
    let url = try #require(URL(string: "https://example.invalid/index.m3u8"))
    let item = AVPlayerItem(url: url)
    PlayerTuning.apply(item, network: "lan", tile: false)
    #expect(item.preferredPeakBitRate == 0)
    #expect(item.preferredForwardBufferDuration == 8)
}

@Test func cellularCapsThePeakBitrate() throws {
    let url = try #require(URL(string: "https://example.invalid/index.m3u8"))
    let item = AVPlayerItem(url: url)
    PlayerTuning.apply(item, network: "cellular", tile: false)
    #expect(item.preferredPeakBitRate == 4_000_000)
    #expect(item.preferredForwardBufferDuration == 12)
}

@Test func aTileKeepsAShorterBuffer() {
    #expect(PlayerTuning.forwardBuffer(network: "lan", tile: true) == 8)
    #expect(PlayerTuning.peakBitRate(network: "lan") == 0)
}
