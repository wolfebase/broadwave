@testable import BroadwaveKit
import Foundation
import Testing

@Test func stallWaitsEightSecondsBeforeAProbe() {
    var clock = ServerOutage()
    let start = Date(timeIntervalSince1970: 1000)
    clock.noteWaiting(at: start)
    #expect(!clock.shouldProbe(at: start.addingTimeInterval(7.9), fatal: false))
    #expect(clock.shouldProbe(at: start.addingTimeInterval(8), fatal: false))
}

@Test func playingCancelsTheWait() {
    var clock = ServerOutage()
    let start = Date(timeIntervalSince1970: 1000)
    clock.noteWaiting(at: start)
    clock.notePlaying()
    #expect(!clock.shouldProbe(at: start.addingTimeInterval(30), fatal: false))
}

@Test func serverDownNamesTheOutageOnce() {
    var clock = ServerOutage()
    let start = Date(timeIntervalSince1970: 1000)
    clock.noteWaiting(at: start)
    #expect(clock.resolve(at: start.addingTimeInterval(8), health: false, fatal: false) == PlaybackOutage.serverStopped)
    #expect(clock.resolve(at: start.addingTimeInterval(9), health: true, fatal: true) == nil)
    #expect(!clock.shouldProbe(at: start.addingTimeInterval(30), fatal: true))
}

@Test func serverUpLeavesAStallWithThePlayer() {
    var clock = ServerOutage()
    let start = Date(timeIntervalSince1970: 2000)
    clock.noteWaiting(at: start)
    #expect(clock.resolve(at: start.addingTimeInterval(8), health: true, fatal: false) == nil)
    #expect(!clock.shouldProbe(at: start.addingTimeInterval(10), fatal: false))
    #expect(clock.shouldProbe(at: start.addingTimeInterval(16), fatal: false))
}

@Test func fatalWhileTheServerIsUpSaysThePictureStopped() {
    var clock = ServerOutage()
    let now = Date(timeIntervalSince1970: 3000)
    #expect(clock.resolve(at: now, health: true, fatal: true) == PlaybackOutage.pictureStopped)
}

@Test func aStartFailureSurfacesOnce() {
    var clock = ServerOutage()
    #expect(clock.surface(PlaybackOutage.serverStopped) == PlaybackOutage.serverStopped)
    #expect(clock.surface(PlaybackOutage.serverStopped) == nil)
}
