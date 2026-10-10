@testable import BroadwaveKit
import Foundation
import Testing

private func playing(_ ids: [Int64] = [4, 12]) -> MosaicAirPlay.State {
    MosaicAirPlay.State(
        generation: 1,
        itemReady: true,
        channelIDs: ids,
        phase: .playing
    )
}

@Test func anAirPlayRouteThatWasAlreadyOnIsRememberedSoTurningItOffStops() {
    let playing = playing()
    let (seen, held) = MosaicAirPlay.noteExternal(playing, active: true, generation: playing.generation)
    #expect(seen.routed)
    #expect(held == .cancelWait)
    let (off, step) = MosaicAirPlay.noteExternal(seen, active: false, generation: seen.generation)
    #expect(off.routed == false)
    #expect(step == .end(generation: seen.generation))
    // A callback from the previous player does not stop the new one.
    let ignored = MosaicAirPlay.noteExternal(off, active: false, generation: seen.generation - 1)
    #expect(ignored.1 == .none)
}

@Test func anIdleCheckThatFindsAirPlayAlreadyOnRemembersIt() {
    var state = playing()
    state.external = true
    let (next, step) = MosaicAirPlay.idleFired(state, generation: state.generation)
    #expect(next.routed)
    #expect(step == .cancelWait)
    let stopped = MosaicAirPlay.noteExternal(next, active: false, generation: next.generation)
    #expect(stopped.1 == .end(generation: next.generation))

    // A wait armed for the previous start does not drop the new one.
    var newer = next
    newer.generation = 2
    #expect(MosaicAirPlay.idleFired(newer, generation: 1).1 == .none)
}

@Test func tileChangesWhileRoutedRestartAndTheSameChannelsDoNot() {
    var state = playing([4, 12])
    state.routed = true
    let same = MosaicAirPlay.channelsChanged(state, channelIDs: [4, 12])
    #expect(same.1 == .none)
    #expect(same.0.generation == state.generation)

    let moved = MosaicAirPlay.channelsChanged(state, channelIDs: [12, 4])
    guard case let .begin(ids, onDevice, generation) = moved.1 else {
        Issue.record("expected a restart")
        return
    }
    #expect(ids == [12, 4])
    #expect(onDevice == false)
    #expect(generation == state.generation + 1)

    let dropped = MosaicAirPlay.channelsChanged(state, channelIDs: [4])
    #expect(dropped.1 == .end(generation: state.generation))

    // No mosaic: a tile change does not start one.
    #expect(MosaicAirPlay.channelsChanged(MosaicAirPlay.State(), channelIDs: [12, 4]).1 == .none)
}

@Test func aMosaicStillStartingFollowsTheTiles() {
    var state = playing([4, 12])
    state.routed = true
    // The sound tile moved: a restart, which clears routed until the route is seen again.
    let (restarting, _) = MosaicAirPlay.channelsChanged(state, channelIDs: [12, 4])
    #expect(restarting.routed == false)
    #expect(restarting.phase == .starting)
    // A tile removed during that restart still leaves the mosaic.
    let (next, step) = MosaicAirPlay.channelsChanged(restarting, channelIDs: [12, 9])
    #expect(step == .begin(channelIDs: [12, 9], onDevice: false, generation: restarting.generation + 1))
    #expect(next.channelIDs == [12, 9])
    #expect(MosaicAirPlay.channelsChanged(next, channelIDs: [12]).1 == .end(generation: next.generation))
}

@Test func openingThePickerAgainDoesNotRestartTheSameMosaic() {
    var state = playing([4, 12])
    state.routed = true
    let (next, step) = MosaicAirPlay.openPicker(state, channelIDs: [4, 12])
    #expect(step == .none)
    #expect(next.pickerOpen)
    #expect(next.generation == state.generation)
    #expect(next.routed)
}

@Test func aCancelledPickLeavesAtOnceAndAConnectingOneWaits() {
    let state = playing()
    let cancelled = MosaicAirPlay.closePicker(state, routeIsAirPlay: false)
    #expect(cancelled.0.pickerOpen == false)
    #expect(cancelled.1 == .end(generation: state.generation))

    let connecting = MosaicAirPlay.closePicker(state, routeIsAirPlay: true)
    #expect(connecting.1 == .wait(generation: state.generation, seconds: MosaicAirPlay.routeWait))

    var on = state
    on.external = true
    let staying = MosaicAirPlay.closePicker(on, routeIsAirPlay: true)
    #expect(staying.0.routed)
    #expect(staying.1 == .cancelWait)
}

@Test func aNewerStartIsNotUndoneByTheStopItInterrupted() {
    var state = playing()
    state.message = "AirPlay did not start. The server log says why."
    let (stopped, generation) = MosaicAirPlay.beginStop(state)
    #expect(stopped.message.isEmpty)
    #expect(stopped.phase == .idle)
    #expect(MosaicAirPlay.isCurrent(stopped, generation: generation))

    let (newer, step) = MosaicAirPlay.startDirect(stopped, channelIDs: [4, 12], onDevice: false)
    #expect(step == .begin(channelIDs: [4, 12], onDevice: false, generation: newer.generation))
    #expect(newer.generation > generation)
    // The end decided before the newer start is not applied to it.
    #expect(MosaicAirPlay.isCurrent(newer, generation: generation) == false)
}

@Test func aFailedItemFallsBackToTheTiles() {
    let state = playing()
    let (next, step) = MosaicAirPlay.playbackEnded(state, generation: state.generation)
    #expect(step == .end(generation: state.generation))
    #expect(next.phase == .playing)
    #expect(MosaicAirPlay.playbackEnded(state, generation: state.generation + 1).1 == .none)

    let (idle, _) = MosaicAirPlay.beginStop(state)
    #expect(MosaicAirPlay.playbackEnded(idle, generation: idle.generation).1 == .none)
}

@Test func pauseReachesTheMosaicPlayer() {
    let state = playing()
    let (paused, step) = MosaicAirPlay.setPaused(state, paused: true)
    #expect(paused.paused)
    #expect(step == .pause)
    let resumed = MosaicAirPlay.setPaused(paused, paused: false)
    #expect(resumed.1 == .play)

    let idle = MosaicAirPlay.setPaused(MosaicAirPlay.State(), paused: true)
    #expect(idle.0.paused)
    #expect(idle.1 == .none)
}

@Test func unmuteWaitsWhenThePictureHasNoSound() {
    #expect(MosaicAirPlay.unmuteNow(wantsSound: true, hasPicture: true, audio: .stereo))
    #expect(MosaicAirPlay.unmuteNow(wantsSound: true, hasPicture: true, audio: .auto))
    #expect(MosaicAirPlay.unmuteNow(wantsSound: true, hasPicture: true, audio: .none) == false)
    #expect(MosaicAirPlay.unmuteNow(wantsSound: true, hasPicture: false, audio: .stereo) == false)
    #expect(MosaicAirPlay.unmuteNow(wantsSound: false, hasPicture: true, audio: .stereo) == false)
}
