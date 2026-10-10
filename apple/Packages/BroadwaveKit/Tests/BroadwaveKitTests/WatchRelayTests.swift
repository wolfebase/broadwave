@testable import BroadwaveKit
import Foundation
import Testing

@Test func aWatchRequestAndItsReplyCrossAsMessages() {
    let press = WatchRelay.Request.press(screen: "TV-1", .down)
    let sent = WatchRelay.message(press)
    // WatchConnectivity carries property-list values only.
    #expect(PropertyListSerialization.propertyList(sent, isValidFor: .binary))
    #expect(WatchRelay.request(sent) == press)
    #expect(WatchRelay.request(WatchRelay.message(.screens)) == .screens)

    let reply = WatchRelay.Reply(screens: [.init(id: "TV-1", name: "Living Room", kind: "appletv", playing: "4.1 Big Buck Bunny")])
    #expect(WatchRelay.reply(WatchRelay.message(reply)) == reply)
    #expect(WatchRelay.reply(WatchRelay.message(WatchRelay.Reply(error: "Your server isn't answering."))).map(\.error) == "Your server isn't answering.")

    // Something a newer app sent is not guessed at.
    #expect(WatchRelay.request(["json": Data(#"{"eject":{}}"#.utf8)]) == nil)
    #expect(WatchRelay.request(["other": 1]) == nil)
}

@Test func televisionsComeFirstAndThePhoneAskingIsLeftOut() {
    let rows: [WatchRelay.Row] = [
        .init(id: "p", name: "Sam's iPhone", kind: "iphone", playing: ""),
        .init(id: "k", name: "Kitchen", kind: "web", playing: "5.1 Sintel"),
        .init(id: "b", name: "Bedroom", kind: "appletv", playing: ""),
        .init(id: "l", name: "Living Room", kind: "appletv", playing: "4.1 Big Buck Bunny"),
        .init(id: "i", name: "iPad", kind: "ipad", playing: ""),
    ]
    #expect(WatchRelay.ordered(rows, without: "p").map(\.id) == ["b", "l", "k", "i"])
    #expect(WatchRelay.ordered(rows, without: nil).count == 5)
}

@Test func theCrownMovesOneChannelPerDetent() {
    #expect(WatchRelay.crownSteps(from: 0, to: 0.4) == nil)
    #expect(WatchRelay.crownSteps(from: 0, to: 1) ?? (.play, 0) == (.up, 1))
    #expect(WatchRelay.crownSteps(from: 3, to: 1) ?? (.play, 0) == (.down, 2))
    // A fast spin does not page through the whole lineup.
    #expect(WatchRelay.crownSteps(from: 0, to: -40) ?? (.play, 0) == (.down, 5))
}
