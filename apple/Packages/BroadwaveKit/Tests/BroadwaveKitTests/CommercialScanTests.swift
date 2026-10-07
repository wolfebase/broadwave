import BroadwaveKit
import Foundation
import Testing

@Test func aCommercialScanSaysHowManyBreaksItFound() {
    #expect(CommercialScan.finding == "Finding commercials…")
    #expect(CommercialScan.found(0) == "No breaks found.")
    #expect(CommercialScan.found(-1) == "No breaks found.")
    #expect(CommercialScan.found(1) == "Found 1 break.")
    #expect(CommercialScan.found(3) == "Found 3 breaks.")
    #expect(CommercialScan.failed == "Could not look for commercials. Try again.")
}

@Test func aSecondPressDoesNotStartAnotherScan() {
    var running: Set<Int64> = []
    #expect(CommercialScan.start(4, running: &running))
    #expect(running.contains(4))
    #expect(!CommercialScan.start(4, running: &running))
    #expect(CommercialScan.start(5, running: &running))
    running.remove(4)
    #expect(CommercialScan.start(4, running: &running))
}
