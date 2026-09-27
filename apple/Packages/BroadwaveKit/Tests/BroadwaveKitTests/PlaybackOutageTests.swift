@testable import BroadwaveKit
import Foundation
import Testing

private func fineSnap() -> RecoverySnap {
    RecoverySnap(health: true, freeTuner: true, tunerAnswers: true, online: true, signalLost: false)
}

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
    let down = RecoverySnap(health: false, freeTuner: false, tunerAnswers: false, online: true, signalLost: false)
    let named = clock.resolve(at: start.addingTimeInterval(8), snap: down, fatal: false)
    #expect(named?.message == PlaybackOutage.serverStopped)
    #expect(named?.recovery == .server)
    #expect(clock.resolve(at: start.addingTimeInterval(9), snap: fineSnap(), fatal: true) == nil)
    #expect(!clock.shouldProbe(at: start.addingTimeInterval(30), fatal: true))
}

@Test func serverUpLeavesAStallWithThePlayer() {
    var clock = ServerOutage()
    let start = Date(timeIntervalSince1970: 2000)
    clock.noteWaiting(at: start)
    #expect(clock.resolve(at: start.addingTimeInterval(8), snap: fineSnap(), fatal: false) == nil)
    #expect(!clock.shouldProbe(at: start.addingTimeInterval(10), fatal: false))
    #expect(clock.shouldProbe(at: start.addingTimeInterval(16), fatal: false))
}

@Test func fatalWhileTheServerIsUpSaysThePictureStopped() {
    var clock = ServerOutage()
    let now = Date(timeIntervalSince1970: 3000)
    let named = clock.resolve(at: now, snap: fineSnap(), fatal: true)
    #expect(named?.message == PlaybackOutage.pictureStopped)
    #expect(named?.recovery == nil)
}

@Test func aStartFailureSurfacesOnce() {
    var clock = ServerOutage()
    #expect(clock.surface(PlaybackOutage.serverStopped) == PlaybackOutage.serverStopped)
    #expect(clock.surface(PlaybackOutage.serverStopped) == nil)
}

@Test func aLostSignalAndASilentTunerAreNamedFromTheStall() {
    var signal = ServerOutage()
    let start = Date(timeIntervalSince1970: 4000)
    signal.noteWaiting(at: start)
    let lost = RecoverySnap(health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: true)
    let antenna = signal.resolve(at: start.addingTimeInterval(8), snap: lost, fatal: false)
    #expect(antenna?.message == PlaybackOutage.noSignal)
    #expect(antenna?.recovery == .signal)

    var tuner = ServerOutage()
    tuner.noteWaiting(at: start)
    let silent = RecoverySnap(health: true, freeTuner: true, tunerAnswers: false, online: true, signalLost: true)
    let named = tuner.resolve(at: start.addingTimeInterval(8), snap: silent, fatal: false)
    #expect(named?.message == PlaybackOutage.tunerStopped)
    #expect(named?.recovery == .tuner)
}

@Test func aFatalErrorWhileTheSignalIsLostNamesTheAntenna() {
    var clock = ServerOutage()
    let lost = RecoverySnap(health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: true)
    let named = clock.resolve(at: Date(timeIntervalSince1970: 5000), snap: lost, fatal: true)
    #expect(named?.message == PlaybackOutage.noSignal)
    #expect(named?.recovery == .signal)
}

@Test func aPhoneWithNoConnectionIsOfflineAndAClosedPortIsTheServer() {
    #expect(!PlaybackOutage.deviceOnline(URLError(.notConnectedToInternet)))
    #expect(!PlaybackOutage.deviceOnline(URLError(.dataNotAllowed)))
    #expect(!PlaybackOutage.deviceOnline(URLError(.internationalRoamingOff)))
    #expect(PlaybackOutage.deviceOnline(URLError(.cannotConnectToHost)))
    #expect(PlaybackOutage.deviceOnline(URLError(.timedOut)))
    #expect(PlaybackOutage.deviceOnline(URLError(.networkConnectionLost)))
}

@Test func aBusyTunerTellsTheViewerWhatToStopAndComesBackWhenOneIsFree() {
    let got = PlaybackOutage.viewerFailure(
        code: "tuners_busy",
        status: 409,
        message: "Every tuner is busy. Stop a recording or watch something already on.",
        online: true
    )
    #expect(got.recovery == .busy)
    #expect(got.message.contains("tuner is busy"))
    let blank = PlaybackOutage.viewerFailure(code: "tuners_busy", status: 409, message: "", online: true)
    #expect(blank.message == PlaybackOutage.tunersBusy)

    let held = RecoverySnap(health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: false)
    let free = RecoverySnap(health: true, freeTuner: true, tunerAnswers: true, online: true, signalLost: false)
    #expect(!PlaybackOutage.recoveryReady(.busy, held))
    #expect(PlaybackOutage.recoveryReady(.busy, free))

    let both = [
        Tuner(index: 0, guide: "4.1", target: "127.0.0.1", ours: true),
        Tuner(index: 1, guide: "5.1", target: "127.0.0.1", ours: false),
    ]
    let one = [
        Tuner(index: 0, guide: "", target: "", ours: false),
        Tuner(index: 1, guide: "5.1", target: "127.0.0.1", ours: true),
    ]
    #expect(!PlaybackOutage.aTunerIsFree(both))
    #expect(PlaybackOutage.aTunerIsFree(one))
    #expect(PlaybackOutage.aTunerIsFree([Tuner(index: 0, ours: false)]))
}

@Test func aServerOrTunerThatStopsAnsweringWaitsUntilItAnswers() {
    let down = PlaybackOutage.viewerFailure(code: "", status: 0, message: "Failed to fetch", online: true)
    #expect(down.message == PlaybackOutage.serverStopped)
    #expect(down.recovery == .server)
    let offlineStart = PlaybackOutage.viewerFailure(code: "", status: 0, message: "Load failed", online: false)
    #expect(offlineStart.message == PlaybackOutage.connectionDropped)

    let gone = RecoverySnap(health: false, freeTuner: false, tunerAnswers: false, online: true, signalLost: false)
    let back = RecoverySnap(health: true, freeTuner: false, tunerAnswers: false, online: true, signalLost: false)
    #expect(!PlaybackOutage.recoveryReady(.server, gone))
    #expect(PlaybackOutage.recoveryReady(.server, back))
    let phoneBackServerUp = RecoverySnap(health: true, freeTuner: true, tunerAnswers: true, online: true, signalLost: false)
    let phoneStillGone = RecoverySnap(health: true, freeTuner: true, tunerAnswers: true, online: false, signalLost: false)
    #expect(PlaybackOutage.recoveryReady(.server, phoneBackServerUp))
    #expect(!PlaybackOutage.recoveryReady(.server, phoneStillGone))

    let tuner = PlaybackOutage.viewerFailure(code: "internal", status: 500, message: "the tuner did not answer", online: true)
    #expect(tuner.message == PlaybackOutage.tunerStopped)
    #expect(tuner.recovery == .tuner)
    let quiet = RecoverySnap(health: true, freeTuner: true, tunerAnswers: false, online: true, signalLost: false)
    let answering = RecoverySnap(health: true, freeTuner: true, tunerAnswers: true, online: true, signalLost: false)
    #expect(!PlaybackOutage.recoveryReady(.tuner, quiet))
    #expect(PlaybackOutage.recoveryReady(.tuner, answering))

    let other = PlaybackOutage.viewerFailure(code: "nope", status: 500, message: "", online: true)
    #expect(other.message == "This channel did not start.")
    #expect(other.recovery == nil)
}

@Test func aDroppedConnectionAndALostSignalEachWaitForTheirOwnFix() {
    let offline = PlaybackOutage.classify(RecoverySnap(health: false, freeTuner: false, tunerAnswers: false, online: false, signalLost: false))
    #expect(offline.message == PlaybackOutage.connectionDropped)
    #expect(offline.recovery == .server)
    let stillOffline = RecoverySnap(health: false, freeTuner: false, tunerAnswers: false, online: false, signalLost: false)
    #expect(!PlaybackOutage.recoveryReady(.server, stillOffline))

    let down = PlaybackOutage.classify(RecoverySnap(health: false, freeTuner: false, tunerAnswers: false, online: true, signalLost: false))
    #expect(down.message == PlaybackOutage.serverStopped)

    let lost = PlaybackOutage.classify(RecoverySnap(health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: true))
    #expect(lost.message == PlaybackOutage.noSignal)
    #expect(lost.recovery == .signal)
    let dark = RecoverySnap(health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: true)
    let lit = RecoverySnap(health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: false)
    #expect(!PlaybackOutage.recoveryReady(.signal, dark))
    #expect(PlaybackOutage.recoveryReady(.signal, lit))
}

@Test func aSignalReadingFailsClosedAndALiveLostRowIsTheOnlyLoss() {
    let lost = ChannelSignal(channelId: 2, number: "5.1", name: "KCTV", verdict: "Lost", live: true)
    let stored = ChannelSignal(channelId: 2, number: "5.1", name: "KCTV", verdict: "Lost", live: false)
    let great = ChannelSignal(channelId: 2, number: "5.1", name: "KCTV", verdict: "Great", live: true)
    let answering = DeviceHealth(deviceId: "A", model: "HDHR", firmwareVersion: "1", tuners: [])
    let silent = DeviceHealth(deviceId: "B", model: "HDHR", firmwareVersion: "1", tuners: [], error: PlaybackOutage.tunerStopped)
    let idle = [Tuner(index: 0, ours: false)]

    let named = PlaybackOutage.snap(
        RecoveryFacts(health: true, online: true, channelID: 2, assumeLost: false),
        lists: RecoveryLists(tuners: idle, devices: [answering], signals: [lost])
    )
    #expect(named.signalLost)
    #expect(named.freeTuner)
    #expect(named.tunerAnswers)

    let remembered = PlaybackOutage.snap(
        RecoveryFacts(health: true, online: true, channelID: 2, assumeLost: true),
        lists: RecoveryLists(tuners: idle, devices: [answering], signals: [stored])
    )
    #expect(!remembered.signalLost)

    let elsewhere = PlaybackOutage.snap(
        RecoveryFacts(health: true, online: true, channelID: 9, assumeLost: true),
        lists: RecoveryLists(tuners: idle, devices: [answering], signals: [lost])
    )
    #expect(!elsewhere.signalLost)

    let unread = PlaybackOutage.snap(
        RecoveryFacts(health: true, online: true, channelID: 2, assumeLost: true)
    )
    #expect(!unread.freeTuner)
    #expect(!unread.tunerAnswers)
    #expect(unread.signalLost)

    let either = PlaybackOutage.snap(
        RecoveryFacts(health: true, online: true, channelID: 2, assumeLost: false),
        lists: RecoveryLists(tuners: idle, devices: [silent, answering], signals: [great])
    )
    #expect(either.tunerAnswers)
    #expect(!either.signalLost)

    let none = PlaybackOutage.snap(
        RecoveryFacts(health: true, online: true, channelID: 2, assumeLost: false),
        lists: RecoveryLists(tuners: [], devices: [silent], signals: [great])
    )
    #expect(!none.freeTuner)
    #expect(!none.tunerAnswers)

    let serverDown = PlaybackOutage.snap(
        RecoveryFacts(health: false, online: false, channelID: 2, assumeLost: true),
        lists: RecoveryLists(tuners: idle, devices: [answering], signals: [lost])
    )
    #expect(!serverDown.health)
    #expect(!serverDown.signalLost)
    #expect(!serverDown.freeTuner)
}
