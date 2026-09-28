@testable import BroadwaveKit
import Testing

@Test func homeKeepsTheFullPlayerBehindTheSmallWindow() {
    #expect(PictureHandoff.dismissWhenPictureInPictureStarts == false)
}

@Test func closingTheSmallWindowAwayStopsTheWatch() {
    #expect(PictureHandoff.stopWhenClosed(restored: false, away: true))
    #expect(!PictureHandoff.stopWhenClosed(restored: true, away: true))
    #expect(!PictureHandoff.stopWhenClosed(restored: false, away: false))
    #expect(!PictureHandoff.stopWhenClosed(restored: true, away: false))
}

@Test func theRouteButtonSpeaksAirPlay() {
    #expect(PictureHandoff.airPlayLabel == "AirPlay")
}
