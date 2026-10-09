@testable import BroadwaveKit
import Foundation
import Testing

private let now = Date(timeIntervalSince1970: 1_790_500_000)

private func channel(
    _ id: Int64, _ number: String, _ name: String, network: String? = nil, hidden: Bool = false, playsAs: Int64? = nil
) -> Channel {
    var channel = Channel(
        id: id, deviceId: "d", guideNumber: number, guideName: name, displayNumber: number, displayName: name,
        hd: true, favorite: false, enabled: true, hidden: hidden, present: true
    )
    channel.network = network
    channel.playsAs = playsAs
    return channel
}

private let lineup = [
    channel(1, "9.2", "KBWV2"),
    channel(2, "9.1", "KBWV", network: "ABC"),
    channel(3, "4.1", "WTST", network: "FOX"),
    channel(4, "109.1", "KBWV 3.0", hidden: true, playsAs: 2),
    channel(5, "14.1", "Rivertown Weather"),
]

@Test func aChannelIsFoundByNumberNameOrNetwork() {
    #expect(Voice.channel("9", in: lineup)?.id == 2)
    #expect(Voice.channel("channel 9", in: lineup)?.id == 2)
    #expect(Voice.channel("9.2", in: lineup)?.id == 1)
    #expect(Voice.channel("9 2", in: lineup)?.id == 1)
    #expect(Voice.channel("9-2", in: lineup)?.id == 1)
    #expect(Voice.channel("wtst", in: lineup)?.id == 3)
    #expect(Voice.channel("Fox", in: lineup)?.id == 3)
    #expect(Voice.channel("weather", in: lineup)?.id == 5)
    #expect(Voice.channel("4 point 1", in: lineup)?.id == 3)
    #expect(Voice.channel("channel nine dot two", in: lineup) == nil)
    // No channel 41: Siri heard "four one".
    #expect(Voice.channel("41", in: lineup)?.id == 3)
    #expect(Voice.channel("14", in: lineup)?.id == 5)
    #expect(Voice.channel("7", in: lineup) == nil)
    #expect(Voice.channel("cbs", in: lineup) == nil)
}

@Test func anEncryptedStationAskedForByNumberPlaysItsTwin() {
    #expect(Voice.channel("109.1", in: lineup)?.id == 2)
    #expect(Voice.channel("109", in: lineup)?.id == 2)
}

private func airing(_ id: Int64, on channel: Int64, _ title: String, sports: Bool = true, from: Double, to: Double) -> Airing {
    Airing(
        id: id, channelId: channel, title: title, category: sports ? "Sports event" : "News", guideNumber: nil,
        start: now.addingTimeInterval(from), end: now.addingTimeInterval(to)
    )
}

@Test func theGameIsTheOneOnNowElseTheNextToday() {
    let airings = [
        airing(1, on: 3, "Chiefs at Broncos", from: 3600, to: 14400),
        airing(2, on: 2, "Royals at Twins", from: -600, to: 9000),
        airing(3, on: 3, "Chiefs Kingdom Report", sports: false, from: -600, to: 1200),
        airing(4, on: 4, "Royals Classic", from: -600, to: 1200),
        airing(5, on: 2, "Chiefs at Raiders", from: 86400, to: 97200),
    ]
    #expect(Voice.game("the Chiefs game", airings: airings, channels: lineup, now: now)?.id == 1)
    #expect(Voice.game("Royals", airings: airings, channels: lineup, now: now)?.id == 2)
    #expect(Voice.game("Jets", airings: airings, channels: lineup, now: now) == nil)
    #expect(Voice.game("KC", airings: airings, channels: lineup, now: now) == nil)
}

@Test func aTeamIsMatchedByWholeWordsInTheTitleOrSubtitle() {
    var vikings = airing(1, on: 3, "NFL Football", from: -600, to: 9000)
    vikings.subtitle = "Vikings at Packers"
    let airings = [vikings, airing(2, on: 2, "Hornets at Celtics", from: 600, to: 9000)]
    #expect(Voice.game("Kings", airings: airings, channels: lineup, now: now) == nil)
    #expect(Voice.game("Nets", airings: airings, channels: lineup, now: now) == nil)
    #expect(Voice.game("the Vikings game", airings: airings, channels: lineup, now: now)?.id == 1)
    #expect(Voice.team("the Theater Kids game") == "theater kids")
}

@Test func theGameOnNowBeatsAnEarlierListingAndTwelveHoursIsTheEdge() {
    let airings = [
        airing(1, on: 3, "Chiefs at Broncos", from: 1800, to: 9000),
        airing(2, on: 2, "Chiefs Classic: Chiefs at Raiders", from: -3600, to: 600),
        airing(3, on: 5, "Royals at Twins", from: 13 * 3600, to: 16 * 3600),
    ]
    #expect(Voice.game("Chiefs", airings: airings, channels: lineup, now: now)?.id == 2)
    #expect(Voice.game("Royals", airings: airings, channels: lineup, now: now) == nil)
}

@Test func theShowToRecordPrefersTheExactTitleAiringSoonest() {
    let airings = [
        airing(1, on: 3, "Jeopardy! Masters", sports: false, from: 600, to: 2400),
        airing(2, on: 2, "Jeopardy!", sports: false, from: 7200, to: 9000),
        airing(3, on: 2, "Jeopardy!", sports: false, from: -7200, to: -5400),
    ]
    // Siri hears no exclamation mark.
    #expect(Voice.show("jeopardy", in: airings, now: now)?.airing.id == 2)
    #expect(Voice.show("jeopardy", in: airings, now: now)?.exact == true)
    #expect(Voice.show("Masters", in: airings, now: now)?.airing.id == 1)
    #expect(Voice.show("Masters", in: airings, now: now)?.exact == false)
    #expect(Voice.show("Wheel", in: airings, now: now) == nil)
}

@Test func whatsOnReadsTheChannelsWithAListing() {
    let snap = TopShelf.Snapshot(
        channels: [lineup[1], lineup[2]],
        airings: [airing(1, on: 2, "Evening News", sports: false, from: -600, to: 1200)]
    )
    #expect(Voice.onNow(snap, now: now) == "9.1, Evening News.")
    #expect(Voice.onNow(TopShelf.Snapshot(channels: [lineup[1]]), now: now) == "Nothing in the guide right now.")
}

@Test func whatsOnFallsBackToEveryChannelWhenNoFavoriteHasAListing() {
    var favorite = lineup[2]
    favorite.favorite = true
    let snap = TopShelf.Snapshot(
        channels: [favorite, lineup[1], lineup[4]],
        airings: [
            airing(1, on: 2, "Evening News", sports: false, from: -600, to: 1200),
            airing(2, on: 5, "Forecast", sports: false, from: -600, to: 1200),
        ]
    )
    #expect(Voice.onNow(snap, now: now) == "9.1, Evening News. 14.1, Forecast.")
    #expect(Voice.onNow(snap, now: now, limit: 1) == "9.1, Evening News.")
}
