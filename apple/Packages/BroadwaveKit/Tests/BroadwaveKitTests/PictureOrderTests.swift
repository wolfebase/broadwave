@testable import BroadwaveKit
import Foundation
import Testing

@Test func theSoundTileAsksBeforeTheQuietOnes() {
    let t0 = Date(timeIntervalSince1970: 1_700_000_000)
    var round = PictureOrder.begin(4, now: t0)
    #expect(!PictureOrder.holdQuiet(round, channelID: 4, now: t0))
    #expect(PictureOrder.holdQuiet(round, channelID: 5, now: t0))
    #expect(PictureOrder.holdQuiet(round, channelID: 5, now: t0.addingTimeInterval(7.9)))
    #expect(!PictureOrder.holdQuiet(round, channelID: 5, now: t0.addingTimeInterval(PictureOrder.soundWait)))

    round = PictureOrder.soundDidAnswer(round, channelID: 4)
    #expect(round.answered)
    #expect(!PictureOrder.holdQuiet(round, channelID: 5, now: t0))

    // A quiet tile answering does not open the gate.
    let ignored = PictureOrder.soundDidAnswer(PictureOrder.begin(4, now: t0), channelID: 9)
    #expect(!ignored.answered)

    // No sound tile yet does not freeze the grid.
    #expect(!PictureOrder.holdQuiet(PictureOrder.begin(0, now: t0), channelID: 5, now: t0))
}

@Test func aDropOrRestartStartsTheOrderOver() {
    let t0 = Date(timeIntervalSince1970: 1_700_000_000)
    let answered = PictureOrder.soundDidAnswer(PictureOrder.begin(4, now: t0), channelID: 4)
    let again = PictureOrder.beginAgain(answered, now: t0.addingTimeInterval(30))
    #expect(!again.answered)
    #expect(again.soundID == 4)
    #expect(PictureOrder.holdQuiet(again, channelID: 5, now: t0.addingTimeInterval(30)))
    #expect(!PictureOrder.holdQuiet(again, channelID: 5, now: t0.addingTimeInterval(30 + PictureOrder.soundWait)))

    // Moving the sound makes that tile first and holds the one that had it.
    let moved = PictureOrder.soundChanged(again, soundID: 5, now: t0.addingTimeInterval(40))
    #expect(!PictureOrder.holdQuiet(moved, channelID: 5, now: t0.addingTimeInterval(40)))
    #expect(PictureOrder.holdQuiet(moved, channelID: 4, now: t0.addingTimeInterval(40)))
    let same = PictureOrder.soundChanged(moved, soundID: 5, now: t0.addingTimeInterval(41))
    #expect(same == moved)
}

@Test func aTileWithNoWatchAsksAfterTheOnesThatHadOne() {
    #expect(PictureOrder.askDelay(hadWatch: true, serverCameBack: true) == 0)
    #expect(PictureOrder.askDelay(hadWatch: false, serverCameBack: false) == 0)
    #expect(PictureOrder.askDelay(hadWatch: false, serverCameBack: true) == PictureOrder.lateAsk)
    #expect(PictureOrder.lateAsk == 3)
}
