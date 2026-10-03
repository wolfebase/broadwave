@testable import BroadwaveKit
import Foundation
import Testing

@Test func eachTileHasItsOwnRoom() {
    let base = "multiview:ab12cd34"
    let tuner = MultiviewRooms.tile(base, channelID: 4)
    let playlist = MultiviewRooms.tile(base, channelID: 801)
    #expect(tuner == "multiview:ab12cd34:4")
    #expect(playlist == "multiview:ab12cd34:801")
    #expect(tuner != playlist)
    #expect(tuner.count <= MultiviewRooms.limit)
    #expect(playlist.count <= MultiviewRooms.limit)
    #expect(DemoServer.validRoom(tuner))
    #expect(DemoServer.validRoom(playlist))
    // An 8-character session id still fits the longest channel id.
    let longest = MultiviewRooms.tile("multiview:abcdefgh", channelID: Int64.max)
    #expect(longest.count <= MultiviewRooms.limit)
    #expect(DemoServer.validRoom(longest))
    #expect(longest.hasSuffix(":\(Int64.max)"))
}

@Test @MainActor func pauseReachesEveryTile() {
    let commands = TileCommands()
    var heard: [Int64: [String]] = [:]
    commands.bind(4) { heard[4, default: []].append($0) }
    commands.bind(801) { heard[801, default: []].append($0) }
    commands.send("pause")
    #expect(heard[4] == ["pause"])
    #expect(heard[801] == ["pause"])

    // A tile that starts again replaces its sender. One that left is dropped.
    commands.bind(4) { heard[4, default: []].append("new:" + $0) }
    commands.unbind(801)
    commands.send("play")
    #expect(heard[4] == ["pause", "new:play"])
    #expect(heard[801] == ["pause"])
}

private func stuckSnap(_ edit: (inout TilePlaybackSnap) -> Void = { _ in }) -> TilePlaybackSnap {
    var snap = TilePlaybackSnap()
    snap.syncWaiting = true
    snap.hasItem = true
    snap.stuckFor = 10
    edit(&snap)
    return snap
}

@Test func aStuckTileReplays() {
    #expect(TilePlayback.shouldReplay(stuckSnap { $0.stuckFor = 3 }))
    #expect(!TilePlayback.shouldReplay(stuckSnap { $0.stuckFor = 2.9 }))
    #expect(TilePlayback.isStuck(stuckSnap { $0.stuckFor = 0 }))
    #expect(!TilePlayback.shouldReplay(stuckSnap { $0.syncWaiting = false }))
    #expect(!TilePlayback.shouldReplay(stuckSnap { $0.rate = 1 }))
    #expect(!TilePlayback.shouldReplay(stuckSnap { $0.itemFailed = true }))
    #expect(!TilePlayback.shouldReplay(stuckSnap { $0.hasItem = false }))
    #expect(!TilePlayback.shouldReplay(stuckSnap { $0.waitingToPlay = true }))
    #expect(!TilePlayback.shouldReplay(stuckSnap { $0.viewerPaused = true }))
}

private func deadSnap(_ edit: (inout TilePlaybackSnap) -> Void = { _ in }) -> TilePlaybackSnap {
    var snap = TilePlaybackSnap()
    snap.syncWaiting = true
    snap.hasItem = true
    snap.rate = 1
    snap.waitingToPlay = true
    snap.primed = true
    snap.deadFor = 5
    edit(&snap)
    return snap
}

@Test func aTileThatStoppedFetchingReloads() {
    #expect(TilePlayback.shouldReload(deadSnap()))
    #expect(TilePlayback.shouldReload(deadSnap { $0.rate = 0; $0.waitingToPlay = false }))
    #expect(TilePlayback.isDead(deadSnap { $0.deadFor = 0 }))
    #expect(!TilePlayback.shouldReload(deadSnap { $0.deadFor = 4.9 }))
    // A fresh encode that has not sent its first segment is still starting.
    #expect(!TilePlayback.shouldReload(deadSnap { $0.primed = false }))
    #expect(!TilePlayback.shouldReload(deadSnap { $0.buffered = 2 }))
    #expect(!TilePlayback.shouldReload(deadSnap { $0.syncWaiting = false }))
    #expect(!TilePlayback.shouldReload(deadSnap { $0.itemFailed = true }))
    #expect(!TilePlayback.shouldReload(deadSnap { $0.viewerPaused = true }))
    #expect(!TilePlayback.shouldReload(deadSnap { $0.hasItem = false }))
    #expect(TilePlayback.shouldReload(deadSnap { $0.reloads = 1 }))
    #expect(!TilePlayback.shouldReload(deadSnap { $0.reloads = TilePlayback.maxReloads }))
}

/// tvOS posts failedToPlayToEndTime on an item that refused a playlist. It
/// still lists its buffer, and the sync engine may be holding it.
@Test func aTileWhoseItemEndedReloadsAtOnce() {
    let ended = deadSnap {
        $0.ended = true
        $0.buffered = 8
        $0.syncWaiting = false
        $0.rate = 0
        $0.waitingToPlay = false
        $0.deadFor = 1
    }
    #expect(TilePlayback.shouldReload(ended))
    #expect(!TilePlayback.shouldReload(deadSnap { $0.ended = true; $0.buffered = 8; $0.deadFor = 0.9 }))
    #expect(!TilePlayback.shouldReload(deadSnap { $0.ended = true; $0.viewerPaused = true }))
    #expect(!TilePlayback.shouldReload(deadSnap { $0.ended = true; $0.itemFailed = true }))
    // Without the notice, a buffer it still lists is not dead.
    #expect(!TilePlayback.isDead(deadSnap { $0.buffered = 8 }))
}

@Test @MainActor func aTileThatJoinsWhilePausedStartsPaused() {
    let commands = TileCommands()
    var heard: [String] = []
    commands.bind(4, { heard.append($0) }, whenPaused: true)
    #expect(heard == ["pause"])
    commands.bind(9, { heard.append("other:" + $0) }, whenPaused: false)
    #expect(heard == ["pause"])
}
