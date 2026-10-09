@testable import BroadwaveKit
import Foundation
import Testing

private func alert(_ id: String, channel: Int64 = 1) -> GameAlert {
    GameAlert(id: id, kind: "start", gameId: "g", channelId: channel, channel: "4.1", text: "Starting now: CHI at LV")
}

@Test func aGameAlertDecodesTheSocketPayload() throws {
    let json = """
    {"id":"g:close","kind":"close","gameId":"g","channelId":4,"channel":"5.1","text":"Close game: BUF at MIA","detail":"BUF 21, MIA 24 · 4th 3:20"}
    """
    let decoded = try JSONDecoder().decode(GameAlert.self, from: Data(json.utf8))
    #expect(decoded.id == "g:close")
    #expect(decoded.kind == "close")
    #expect(decoded.channelId == 4)
    #expect(decoded.channel == "5.1")
    #expect(decoded.detail == "BUF 21, MIA 24 · 4th 3:20")

    let plain = """
    {"id":"g:start","kind":"start","gameId":"g","channelId":1,"channel":"4.1","text":"Starting now: CHI at LV"}
    """
    let start = try JSONDecoder().decode(GameAlert.self, from: Data(plain.utf8))
    #expect(start.detail == nil)
}

@Test func aRepeatedGameAlertIsDropped() {
    var queue = GameAlerts()
    let now = Date(timeIntervalSince1970: 1_700_000_000)
    let first = alert("g:start")
    let took = queue.receive(first, at: now)
    let again = queue.receive(first, at: now.addingTimeInterval(1))
    let empty = queue.receive(alert(""), at: now)
    let noChannel = queue.receive(alert("none", channel: 0), at: now)
    let head = queue.current(at: now)
    #expect(took)
    #expect(!again)
    #expect(!empty)
    #expect(!noChannel)
    #expect(head?.alert.id == "g:start")
    queue.dismiss()
    let after = queue.current(at: now)
    #expect(after == nil)
    // A server restart sends the same id again. This screen already showed it.
    let replay = queue.receive(first, at: now.addingTimeInterval(2))
    let still = queue.current(at: now)
    #expect(!replay)
    #expect(still == nil)
}

@Test func aGameAlertOlderThanTenMinutesIsDropped() {
    var queue = GameAlerts()
    let now = Date(timeIntervalSince1970: 1_700_000_000)
    let old = queue.receive(alert("old:start"), at: now)
    let next = queue.receive(alert("next:close", channel: 2), at: now.addingTimeInterval(100))
    let before = queue.current(at: now.addingTimeInterval(599))
    let atTen = queue.current(at: now.addingTimeInterval(600))
    let later = queue.current(at: now.addingTimeInterval(700))
    #expect(old)
    #expect(next)
    #expect(before?.alert.id == "old:start")
    #expect(atTen?.alert.id == "next:close")
    #expect(later == nil)
}

@MainActor @Test func theStoreQueuesAGameAlert() {
    let store = AppStore()
    let now = Date(timeIntervalSince1970: 1_700_000_000)
    let first = alert("g:start")
    store.noteGameAlert(first, now: now)
    #expect(store.gameAlert?.text == "Starting now: CHI at LV")
    #expect(store.gameAlertAt == now)
    store.noteGameAlert(first, now: now.addingTimeInterval(1))
    store.noteGameAlert(alert("h:close", channel: 2), now: now.addingTimeInterval(2))
    store.dismissGameAlert(now: now.addingTimeInterval(3))
    #expect(store.gameAlert?.id == "h:close")
    store.refreshGameAlerts(now: now.addingTimeInterval(2 + 600))
    #expect(store.gameAlert == nil)
    store.noteGameAlert(first, now: now.addingTimeInterval(700))
    #expect(store.gameAlert == nil)
}
