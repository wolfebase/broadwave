@testable import BroadwaveKit
import Foundation
import Testing

private func game(_ partial: GameSwitch.Game) -> GameSwitch.Game {
    var next = partial
    if next.state == nil {
        next.state = "in"
    }
    if next.teams == nil {
        next.teams = []
    }
    return next
}

private let zone = game(GameSwitch.Game(
    id: "nfl-redzone",
    league: "nfl",
    detail: "3rd 8:12",
    clock: "8:12",
    period: 3,
    redZone: true,
    teams: [
        GameSwitch.Team(name: "Raiders", abbr: "LV", score: "14", home: true),
        GameSwitch.Team(name: "Bears", abbr: "CHI", score: "17"),
    ]
))

private let power = game(GameSwitch.Game(
    id: "nhl-pp",
    league: "nhl",
    detail: "2nd 12:04",
    clock: "12:04",
    period: 2,
    powerPlay: true,
    teams: [
        GameSwitch.Team(name: "Rangers", abbr: "NYR", score: "1", home: true),
        GameSwitch.Team(name: "Bruins", abbr: "BOS", score: "1"),
    ]
))

private let close = game(GameSwitch.Game(
    id: "nfl-close",
    league: "nfl",
    detail: "4th 3:20",
    clock: "3:20",
    period: 4,
    teams: [
        GameSwitch.Team(name: "Dolphins", abbr: "MIA", score: "24", home: true),
        GameSwitch.Team(name: "Bills", abbr: "BUF", score: "21"),
    ]
))

private let lead = game(GameSwitch.Game(
    id: "nfl-lead",
    league: "nfl",
    detail: "2nd 10:00",
    clock: "10:00",
    period: 2,
    teams: [
        GameSwitch.Team(name: "Eagles", abbr: "PHI", score: "10", home: true),
        GameSwitch.Team(name: "Cowboys", abbr: "DAL", score: "14"),
    ]
))

private var now: Date {
    ISO8601DateFormatter.plain.date(from: "2026-09-25T20:00:00Z")!
}

@Test func redZoneWinsOverACloseFinish() {
    let got = GameSwitch.pickFocus(now: now, manualAt: .distantPast, games: [close, zone], previous: [])
    #expect(!got.keepManual)
    #expect(got.gameId == "nfl-redzone")
    #expect(got.banner == "Red zone: CHI at LV")
}

@Test func powerPlayWinsOverALeadChange() {
    var prev = lead
    prev.teams = [
        GameSwitch.Team(name: "Eagles", abbr: "PHI", score: "14", home: true),
        GameSwitch.Team(name: "Cowboys", abbr: "DAL", score: "7"),
    ]
    let got = GameSwitch.pickFocus(now: now, manualAt: .distantPast, games: [lead, power], previous: [prev])
    #expect(got.gameId == "nhl-pp")
    #expect(got.banner == "Power play: BOS at NYR")
}

@Test func aFlippedLeadBeatsTheFinalMinutes() {
    var prev = lead
    prev.teams = [
        GameSwitch.Team(name: "Eagles", abbr: "PHI", score: "14", home: true),
        GameSwitch.Team(name: "Cowboys", abbr: "DAL", score: "7"),
    ]
    let got = GameSwitch.pickFocus(now: now, manualAt: .distantPast, games: [close, lead], previous: [prev])
    #expect(got.gameId == "nfl-lead")
    #expect(got.banner == "Lead change: DAL at PHI")
}

@Test func finalMinutesUseTheClockOrTheDetailLine() {
    let got = GameSwitch.pickFocus(now: now, manualAt: .distantPast, games: [close], previous: [])
    #expect(got.banner == "Final minutes: BUF at MIA")
    var named = close
    named.period = 0
    named.clock = ""
    named.detail = "4th 2:05"
    #expect(GameSwitch.pickFocus(now: now, manualAt: .distantPast, games: [named], previous: []).banner == "Final minutes: BUF at MIA")
    var five = close
    five.clock = "5:00"
    five.detail = "4th 5:00"
    #expect(GameSwitch.pickFocus(now: now, manualAt: .distantPast, games: [five], previous: []).gameId == "")
}

@Test func aManualChoiceHoldsForUnder2Minutes() {
    let held = GameSwitch.pickFocus(now: now, manualAt: now.addingTimeInterval(-119), games: [close, zone], previous: [])
    #expect(held.keepManual)
    #expect(held.gameId == "")
    #expect(held.banner == "")
    let free = GameSwitch.pickFocus(now: now, manualAt: now.addingTimeInterval(-120), games: [close, zone], previous: [])
    #expect(!free.keepManual)
    #expect(free.gameId == "nfl-redzone")
}

@Test func autoPollsOnlyWhileATileIsAGame() {
    #expect(!GameSwitch.shouldPoll(auto: false, gameIDs: ["nfl-redzone"]))
    #expect(!GameSwitch.shouldPoll(auto: true, gameIDs: []))
    #expect(!GameSwitch.shouldPoll(auto: true, gameIDs: ["", ""]))
    #expect(GameSwitch.shouldPoll(auto: true, gameIDs: ["", "nfl-redzone"]))
}
