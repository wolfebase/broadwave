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

@Test func aStallThatStaysWithNoNamedCauseIsTheStoppedPicture() {
    var clock = ServerOutage()
    let start = Date(timeIntervalSince1970: 2000)
    clock.noteWaiting(at: start)
    #expect(clock.resolve(at: start.addingTimeInterval(8), snap: fineSnap(), fatal: false, afterPicture: true) == nil)
    let named = clock.resolve(at: start.addingTimeInterval(16), snap: fineSnap(), fatal: false, afterPicture: true)
    #expect(named?.message == PlaybackOutage.pictureStopped)
    #expect(named?.recovery == nil)
    #expect(clock.resolve(at: start.addingTimeInterval(20), snap: fineSnap(), fatal: false, afterPicture: true) == nil)
}

@Test func playingBetweenProbesKeepsTheStallWithThePlayer() {
    var clock = ServerOutage()
    let start = Date(timeIntervalSince1970: 2000)
    clock.noteWaiting(at: start)
    #expect(clock.resolve(at: start.addingTimeInterval(8), snap: fineSnap(), fatal: false) == nil)
    clock.notePlaying()
    clock.noteWaiting(at: start.addingTimeInterval(9))
    #expect(clock.resolve(at: start.addingTimeInterval(17), snap: fineSnap(), fatal: false) == nil)
}

@Test func fatalWhileTheServerIsUpSaysThePictureStopped() {
    var clock = ServerOutage()
    let now = Date(timeIntervalSince1970: 3000)
    let named = clock.resolve(at: now, snap: fineSnap(), fatal: true)
    #expect(named?.message == PlaybackOutage.pictureStopped)
    #expect(named?.recovery == nil)
}

@Test func aServerThatLostThePictureStartsItAgain() {
    let facts = RecoveryFacts(health: true, online: true, channelID: 1, assumeLost: false)
    let lost = [ChannelSignal(channelId: 1, number: "4.1", name: "WDAF", verdict: "Lost", live: true)]
    let gone = PlaybackOutage.snap(facts, lists: RecoveryLists(tuners: [], devices: [], signals: lost, playlistFound: false))
    #expect(gone.watchGone)

    var stall = ServerOutage()
    let start = Date(timeIntervalSince1970: 6000)
    stall.noteWaiting(at: start)
    let named = stall.resolve(at: start.addingTimeInterval(8), snap: gone, fatal: false)
    #expect(named?.message == PlaybackOutage.pictureRestarting)
    #expect(named?.recovery == .restart)

    var fatal = ServerOutage()
    #expect(fatal.resolve(at: start, snap: gone, fatal: true)?.recovery == .restart)

    #expect(PlaybackOutage.recoveryReady(.restart, fineSnap()))
    let down = RecoverySnap(health: false, freeTuner: false, tunerAnswers: false, online: true, signalLost: false)
    #expect(!PlaybackOutage.recoveryReady(.restart, down))
}

@Test func anUnreadPlaylistIsNotALostPicture() {
    let facts = RecoveryFacts(health: true, online: true, channelID: 1, assumeLost: false)
    #expect(!PlaybackOutage.snap(facts, lists: RecoveryLists(tuners: [], devices: [], signals: [])).watchGone)
    #expect(!PlaybackOutage.snap(facts, lists: RecoveryLists(tuners: [], devices: [], signals: [], playlistFound: true)).watchGone)
    let down = RecoveryFacts(health: false, online: true, channelID: 1, assumeLost: false)
    #expect(!PlaybackOutage.snap(down, lists: RecoveryLists(playlistFound: false)).watchGone)
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

@Test func aFullPictureBudgetIsAskedUntilASlotFrees() {
    let full = "This server can play 4 pictures at once. Stop one."
    #expect(PlaybackOutage.startAttempts(code: "pictures_full", message: full) == 4)
    #expect(PlaybackOutage.startAttempts(code: "internal", message: "tuner 0 did not lock") == 2)
    #expect(PlaybackOutage.startAttempts(code: "no_signal", message: "This channel isn't coming in. Check the antenna.") == 1)
    #expect(PlaybackOutage.startAttempts(
        code: "tuners_busy",
        message: "Every tuner is busy. Stop a recording or watch something already on."
    ) == 1)
    #expect(PlaybackOutage.startAttempts(code: "internal", message: "This channel did not start.") == 1)
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

    let dark = PlaybackOutage.viewerFailure(code: "no_signal", status: 503, message: "", online: true)
    #expect(dark.message == PlaybackOutage.noSignal)
    #expect(dark.recovery == nil)

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

@Test func aStoppedPictureTriesAgainEveryTenSecondsForTwoMinutes() {
    var fires: [Int] = []
    var elapsed: TimeInterval = 0
    for _ in 0 ..< 20 {
        guard let wait = PlaybackOutage.pictureRetryDelay(
            message: PlaybackOutage.pictureStopped,
            recovery: nil,
            elapsed: elapsed
        ) else { break }
        elapsed += wait
        fires.append(Int((elapsed * 1000).rounded()))
    }
    #expect(fires == Array(stride(from: 10000, through: 120_000, by: 10000)))
    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.pictureStopped, recovery: nil, elapsed: elapsed) == nil)
    let later = PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.pictureStopped, recovery: nil, elapsed: 10.05)
    #expect(later.map { Int(($0 * 1000).rounded()) } == 9950)
    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.pictureStopped, recovery: nil, elapsed: 120) == nil)
    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.pictureStopped, recovery: nil, elapsed: -1) == nil)
    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.pictureStopped, recovery: nil, elapsed: .nan) == nil)
    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.pictureStopped, recovery: nil, elapsed: .infinity) == nil)

    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.noSignal, recovery: nil, elapsed: 0) == nil)
    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.noSignal, recovery: .signal, elapsed: 0) == nil)
    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.serverStopped, recovery: .server, elapsed: 0) == nil)
    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.tunerStopped, recovery: .tuner, elapsed: 0) == nil)
    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.tunersBusy, recovery: .busy, elapsed: 0) == nil)
    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.pictureStopped, recovery: .server, elapsed: 0) == nil)
    #expect(PlaybackOutage.pictureRetryDelay(message: PlaybackOutage.pictureRestarting, recovery: .restart, elapsed: 0) == nil)

    #expect(PlaybackOutage.holdPictureMessage(OutageDecision(message: PlaybackOutage.pictureStopped, recovery: nil)))
    #expect(PlaybackOutage.holdPictureMessage(OutageDecision(message: "stream returned 503", recovery: nil)))
    #expect(!PlaybackOutage.holdPictureMessage(OutageDecision(message: PlaybackOutage.noSignal, recovery: nil)))
    #expect(!PlaybackOutage.holdPictureMessage(OutageDecision(message: PlaybackOutage.noSignal, recovery: .signal)))
    #expect(!PlaybackOutage.holdPictureMessage(OutageDecision(message: PlaybackOutage.serverStopped, recovery: .server)))
    #expect(!PlaybackOutage.holdPictureMessage(OutageDecision(message: "Every tuner is busy.", recovery: .busy)))
}

@Test func aSourceThatStoppedSendingIsTheStoppedPicture() {
    let decision = PlaybackOutage.viewerFailure(code: "stream_down", status: 503, message: "stream returned 503 Service Unavailable", online: true)
    #expect(decision.message == PlaybackOutage.pictureStopped)
    #expect(decision.recovery == nil)
    #expect(PlaybackOutage.pictureRetryDelay(message: decision.message, recovery: decision.recovery, elapsed: 0) == 10)
}

@Test func aHomeWithOnlyPlaylistsNeverBlamesATuner() {
    let facts = RecoveryFacts(health: true, online: true, channelID: 1, assumeLost: false)
    let snap = PlaybackOutage.snap(facts, lists: RecoveryLists(tuners: [], devices: [], signals: []))
    #expect(snap.tunerAnswers)
    #expect(PlaybackOutage.classify(snap).message == PlaybackOutage.pictureStopped)
    let silent = DeviceHealth(deviceId: "B", model: "HDHR", firmwareVersion: "1", tuners: [], error: PlaybackOutage.tunerStopped)
    #expect(!PlaybackOutage.aTunerAnswers([silent]))
    #expect(!PlaybackOutage.snap(facts, lists: RecoveryLists(tuners: [], devices: nil, signals: [])).tunerAnswers)
}
