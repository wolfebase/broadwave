@testable import BroadwaveKit
import Foundation
import Testing

private let start = Date(timeIntervalSince1970: 1000)

/// Plays `seconds` of picture, one sample a second, from `time`.
private func play(_ frozen: inout FrozenPicture, from time: Double, seconds: Int) -> Double {
    for i in 0 ... seconds {
        #expect(frozen.note(time: time + Double(i), playing: true, at: start.addingTimeInterval(Double(i))) == nil)
    }
    return time + Double(seconds)
}

@Test func aStuckPictureReloadsThenRetunes() {
    var frozen = FrozenPicture()
    let at = play(&frozen, from: 100, seconds: 5)
    var steps: [FrozenPicture.Step] = []
    for i in 6 ... 30 {
        if let step = frozen.note(time: at, playing: true, at: start.addingTimeInterval(Double(i))) {
            steps.append(step)
            #expect(frozen.reconnecting)
        }
    }
    #expect(steps == [.reload, .retune, .reload])
}

@Test func aPauseThePlayerMadeReloadsAfterASecond() {
    var frozen = FrozenPicture()
    let at = play(&frozen, from: 100, seconds: 5)
    #expect(frozen.note(time: at, playing: false, stoppedItself: true, at: start.addingTimeInterval(5.5)) == nil)
    #expect(frozen.note(time: at, playing: false, stoppedItself: true, at: start.addingTimeInterval(6)) == .reload)
}

@Test func stepsStopAtTheCap() {
    var frozen = FrozenPicture()
    let at = play(&frozen, from: 100, seconds: 5)
    var steps = 0
    for i in 6 ... 200 where frozen.note(time: at, playing: true, at: start.addingTimeInterval(Double(i))) != nil {
        steps += 1
    }
    #expect(steps == FrozenPicture.maxSteps)
    #expect(!frozen.reconnecting)
}

@Test func aViewersPauseIsNotAFreeze() {
    var frozen = FrozenPicture()
    let at = play(&frozen, from: 100, seconds: 5)
    for i in 6 ... 30 {
        #expect(frozen.note(time: at, playing: false, at: start.addingTimeInterval(Double(i))) == nil)
    }
    #expect(!frozen.reconnecting)
}

@Test func aPictureThatNeverMovedIsStillStarting() {
    var frozen = FrozenPicture()
    for i in 0 ... 30 {
        #expect(frozen.note(time: 0, playing: true, at: start.addingTimeInterval(Double(i))) == nil)
    }
}

@Test func movingAgainEndsReconnecting() {
    var frozen = FrozenPicture()
    var at = play(&frozen, from: 100, seconds: 5)
    var step: FrozenPicture.Step?
    for i in 6 ... 14 {
        step = frozen.note(time: at, playing: true, at: start.addingTimeInterval(Double(i))) ?? step
    }
    #expect(step == .reload)
    // The reloaded item lands at the live edge, then plays.
    at += 20
    #expect(frozen.note(time: at, playing: true, at: start.addingTimeInterval(15)) == nil)
    #expect(frozen.reconnecting)
    #expect(frozen.note(time: at + 1, playing: true, at: start.addingTimeInterval(16)) == nil)
    #expect(!frozen.reconnecting)
}

@Test func aShortRunOfPictureDoesNotStartTheCountOver() {
    var frozen = FrozenPicture()
    var at = play(&frozen, from: 100, seconds: 5)
    var steps: [FrozenPicture.Step] = []
    var clock = 5.0
    // Stuck, then the reload plays 3 s of what the server still lists, then stuck again.
    for _ in 0 ..< 2 {
        for _ in 0 ..< 9 {
            clock += 1
            if let step = frozen.note(time: at, playing: true, at: start.addingTimeInterval(clock)) {
                steps.append(step)
            }
        }
        for _ in 0 ..< 3 {
            clock += 1
            at += 1
            _ = frozen.note(time: at, playing: true, at: start.addingTimeInterval(clock))
        }
    }
    #expect(steps == [.reload, .retune])
}

@Test func twentySecondsOfPictureStartsTheCountOver() {
    var frozen = FrozenPicture()
    var at = play(&frozen, from: 100, seconds: 5)
    var first: FrozenPicture.Step?
    for i in 6 ... 14 {
        first = frozen.note(time: at, playing: true, at: start.addingTimeInterval(Double(i))) ?? first
    }
    #expect(first == .reload)
    for i in 15 ... 40 {
        at += 1
        _ = frozen.note(time: at, playing: true, at: start.addingTimeInterval(Double(i)))
    }
    var again: FrozenPicture.Step?
    for i in 41 ... 50 {
        again = frozen.note(time: at, playing: true, at: start.addingTimeInterval(Double(i))) ?? again
    }
    #expect(again == .reload)
}

@Test func aSeekIsNotAFreeze() {
    var frozen = FrozenPicture()
    var at = play(&frozen, from: 100, seconds: 5)
    // Stands still for 6 s, then a sync seek, then 6 s more: never 8 s in a row.
    for i in 6 ... 11 {
        #expect(frozen.note(time: at, playing: true, at: start.addingTimeInterval(Double(i))) == nil)
    }
    at += 10
    for i in 12 ... 18 {
        #expect(frozen.note(time: at, playing: true, at: start.addingTimeInterval(Double(i))) == nil)
    }
}

/// A reloaded item pauses itself while it starts. That is not the next freeze.
@Test func aReloadGetsTimeToStart() {
    var frozen = FrozenPicture()
    let at = play(&frozen, from: 100, seconds: 5)
    #expect(frozen.note(time: at, playing: false, stoppedItself: true, at: start.addingTimeInterval(5.5)) == nil)
    #expect(frozen.note(time: at, playing: false, stoppedItself: true, at: start.addingTimeInterval(6)) == .reload)
    // The new item opens at the live edge and pauses itself for a few seconds.
    let edge = at + 20
    for i in 7 ... 13 {
        #expect(frozen.note(time: edge, playing: false, stoppedItself: true, at: start.addingTimeInterval(Double(i))) == nil)
    }
    // Eight seconds that never start are the next step.
    #expect(frozen.note(time: edge, playing: false, stoppedItself: true, at: start.addingTimeInterval(15)) == .retune)
}

@Test func anEndedItemIsAStepAtOnce() {
    var frozen = FrozenPicture()
    // It never moved: the sync engine held its first frame when it died.
    #expect(frozen.note(time: 0, playing: false, at: start) == nil)
    #expect(frozen.note(time: 0, playing: false, ended: true, at: start.addingTimeInterval(1)) == .reload)
    #expect(frozen.reconnecting)
    #expect(frozen.note(time: nil, playing: false, ended: true, at: start.addingTimeInterval(2)) == .retune)
}

@Test func aReloadThatPlaysGetsTheShortWaitBack() {
    var frozen = FrozenPicture()
    var at = play(&frozen, from: 100, seconds: 5)
    _ = frozen.note(time: at, playing: false, stoppedItself: true, at: start.addingTimeInterval(5.5))
    #expect(frozen.note(time: at, playing: false, stoppedItself: true, at: start.addingTimeInterval(6)) == .reload)
    at += 20
    #expect(frozen.note(time: at, playing: true, at: start.addingTimeInterval(7)) == nil)
    #expect(frozen.note(time: at + 1, playing: true, at: start.addingTimeInterval(8)) == nil)
    #expect(frozen.note(time: at + 1, playing: false, stoppedItself: true, at: start.addingTimeInterval(8.5)) == nil)
    #expect(frozen.note(time: at + 1, playing: false, stoppedItself: true, at: start.addingTimeInterval(9)) == .retune)
}
