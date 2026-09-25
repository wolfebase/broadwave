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

@Test func aBlankSampleDoesNotInvent1080p60() {
    let match = PlayerTuning.matchingDisplay(width: 0, height: 0, rate: 0, previous: DisplayMatch())
    #expect(match == DisplayMatch())
    #expect(!PlayerTuning.displayReady(match))
}

@Test func aLateFilmRateReplaces59_94() {
    let earlyRate = Float(59.94)
    let filmRate = Float(23.976)
    let early = PlayerTuning.matchingDisplay(width: 1920, height: 1080, rate: earlyRate, previous: DisplayMatch())
    let film = PlayerTuning.matchingDisplay(width: 1920, height: 1080, rate: filmRate, previous: early)
    #expect(film.refreshRate == filmRate)
    #expect(film.width == 1920)
    #expect(film.height == 1080)
    #expect(PlayerTuning.displayReady(film))
}

@Test func a720pPictureReplaces1080p() {
    let rate = Float(59.94)
    let previous = DisplayMatch(width: 1920, height: 1080, refreshRate: rate)
    let match = PlayerTuning.matchingDisplay(width: 1280, height: 720, rate: rate, previous: previous)
    #expect(match.width == 1280)
    #expect(match.height == 720)
    #expect(match.refreshRate == rate)
}

@Test func aServerHintFillsTheRateAndKeepsTheAssetSize() {
    let rate = Float(59.94)
    let film = Float(23.976)
    let asset = PlayerTuning.matchingDisplay(width: 1280, height: 720, rate: 0, previous: DisplayMatch())
    let hinted = PlayerTuning.hintedDisplay(DisplayMatch(width: 1920, height: 1080, refreshRate: rate), current: asset)
    #expect(hinted.width == 1280)
    #expect(hinted.height == 720)
    #expect(hinted.refreshRate == rate)
    let corrected = PlayerTuning.matchingDisplay(width: 1280, height: 720, rate: film, previous: hinted)
    #expect(corrected.refreshRate == film)
    #expect(corrected.width == 1280)
}

@Test func anUnknownSampleKeepsThePictureAlreadyMatched() {
    let known = DisplayMatch(width: 1280, height: 720, refreshRate: Float(23.976))
    let held = PlayerTuning.matchingDisplay(width: 0, height: 0, rate: 0, previous: known)
    #expect(held == known)
}
