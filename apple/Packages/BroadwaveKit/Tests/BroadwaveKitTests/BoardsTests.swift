@testable import BroadwaveKit
import Foundation
import Testing

private let t0 = Date(timeIntervalSince1970: 1_790_000_000)

private func airing(
    _ id: Int64, channel: Int64 = 1, title: String, subtitle: String? = nil, category: String? = nil,
    gameId: String? = nil, live: Bool? = nil, start: Double = 0, minutes: Double = 60
) -> Airing {
    Airing(
        id: id, channelId: channel, title: title, subtitle: subtitle, category: category, live: live, gameId: gameId,
        start: t0.addingTimeInterval(start * 60), end: t0.addingTimeInterval((start + minutes) * 60)
    )
}

private func channel(_ id: Int64, _ number: String, favorite: Bool = false, twin: Int64? = nil) -> Channel {
    Channel(
        id: id, deviceId: "d", guideNumber: number, guideName: "C\(id)", displayNumber: number, displayName: "C\(id)",
        hd: true, favorite: favorite, enabled: true, hidden: false, present: true, twinId: twin
    )
}

@Test func theSportsPageListsGamesNotShowsAboutSports() {
    #expect(SportsBoard.isGame(airing(1, title: "College Football", subtitle: "Washington at Purdue", category: "Sport, Football", gameId: "401")))
    #expect(SportsBoard.isGame(airing(2, title: "High School Football", subtitle: "Lee's Summit at Blue Springs", category: "Sport, Football", live: true)))
    #expect(SportsBoard.isGame(airing(3, title: "NASCAR Cup Series", category: "Sport, Auto racing", live: true)))
    // A sitcom about football, a talk show, a magazine, a highlights show.
    #expect(!SportsBoard.isGame(airing(4, title: "The Game", subtitle: "Punk Ass Chauncey", category: "Series, Comedy drama, Football")))
    #expect(!SportsBoard.isGame(airing(5, title: "SportsPulse", category: "Series, Sports talk")))
    #expect(!SportsBoard.isGame(airing(6, title: "The Short List", subtitle: "Likeable", category: "Sport, Entertainment")))
    #expect(!SportsBoard.isGame(airing(7, title: "KC End Zone", category: "Sport, Football")))
    #expect(!SportsBoard.isGame(airing(8, title: "Fight Night", category: "Sports talk", live: true)))
}

@Test func oneGameOnTwoStationsIsOneEntryOnTheFavorite() {
    let wdaf = channel(73, "104.1")
    let fox = channel(82, "27.2", favorite: true)
    let other = channel(22, "38.1")
    let pairs: [(Channel, Airing)] = [
        (wdaf, airing(1, channel: 73, title: "College Football", subtitle: "Washington at Purdue", category: "Sport, Football", gameId: "401")),
        (fox, airing(2, channel: 82, title: "College Football", subtitle: "Washington at Purdue", category: "Sport, Football", gameId: "401")),
        (other, airing(3, channel: 22, title: "High School Football", subtitle: "A at B", category: "Sport, Football", live: true)),
        (wdaf, airing(4, channel: 73, title: "SportsPulse", category: "Series, Sports talk")),
    ]
    let games = SportsBoard.games(pairs)
    #expect(games.map(\.airing.id) == [2, 3])
    #expect(games[0].channel.id == 82)
    #expect(games[0].also.map(\.id) == [73])
    #expect(games[1].also.isEmpty)
}

@Test func aReplayOfAGameIsNotListedAgain() {
    let kmci = channel(5, "38.1")
    let pairs: [(Channel, Airing)] = [
        (kmci, airing(1, channel: 5, title: "High School Football", subtitle: "Lee's Summit at Blue Springs", category: "Sport, Football", live: true)),
        (kmci, airing(2, channel: 5, title: "High School Football", subtitle: "Lee's Summit at Blue Springs", category: "Sport, Football", live: true, start: 13 * 60)),
        (kmci, airing(3, channel: 5, title: "High School Football", subtitle: "Raytown at Belton", category: "Sport, Football", live: true, start: 14 * 60)),
    ]
    #expect(SportsBoard.games(pairs).map(\.airing.id) == [1, 3])
}

@Test func aTwinIsNotListedAsAnotherStation() {
    let three = channel(73, "104.1", twin: 28)
    let one = channel(28, "4.1", twin: 73)
    let pairs: [(Channel, Airing)] = [
        (three, airing(1, channel: 73, title: "NFL Football", subtitle: "SEA at DEN", category: "Sport, Football")),
        (one, airing(2, channel: 28, title: "NFL Football", subtitle: "SEA at DEN", category: "Sport, Football")),
    ]
    let games = SportsBoard.games(pairs)
    #expect(games.count == 1)
    #expect(games[0].also.isEmpty)
}

@Test func theLeagueAndStatusAreShortWords() {
    #expect(SportsBoard.league(airing(1, title: "College Football", subtitle: "Washington at Purdue")) == "College Football")
    #expect(SportsBoard.league(airing(2, title: "NFL: Seahawks at Broncos")) == "NFL")
    #expect(SportsBoard.league(airing(3, title: "Seahawks at Broncos")) == nil)
    let live = Game(id: "1", league: "nfl", name: "x", start: t0, state: "in", detail: "9:28 - 4th")
    let done = Game(id: "2", league: "nfl", name: "x", start: t0, state: "post", completed: true, detail: "Final/OT")
    #expect(SportsBoard.status(live) == "9:28 - 4th")
    #expect(SportsBoard.status(done) == "Final")
}

@Test func searchShowsEachAiringOnceOnAShownChannelOnNowFirst() {
    let now = t0.addingTimeInterval(10 * 60)
    let list = [
        airing(1, channel: 28, title: "News at 9", start: 0),
        airing(2, channel: 73, title: "News at 9", start: 0),
        airing(3, channel: 5, title: "News at 10", start: 60),
        airing(4, channel: 5, title: "News at 6", start: -240),
        airing(5, channel: 5, title: "Morning News", start: 600),
    ]
    let groups = SearchBoard.group(list, visible: [73, 5], now: now)
    #expect(groups.onNow.map(\.id) == [2])
    #expect(groups.later.map(\.id) == [3, 5])
    #expect(!groups.isEmpty)
}

@Test func suggestionsStartWithWhatWasTypedThenContainIt() {
    let titles = ["The Evening News", "News at 9", "Newsmax Tonight", "News at 9", "Wheel of Fortune", "news"]
    #expect(SearchBoard.suggestions("news", titles: titles) == ["News at 9", "Newsmax Tonight", "The Evening News"])
    #expect(SearchBoard.suggestions("n", titles: titles).isEmpty)
}

@Test func recentSearchesKeepTheNewestEightOnce() throws {
    let defaults = try #require(UserDefaults(suiteName: "recent-\(UUID().uuidString)"))
    var recent = RecentSearches.load(defaults)
    #expect(recent.items.isEmpty)
    for word in ["chiefs", "news", "x", "  ", "Chiefs"] {
        recent.note(word)
    }
    #expect(recent.items == ["Chiefs", "news"])
    for n in 0 ..< 10 {
        recent.note("show \(n)")
    }
    #expect(recent.items.count == RecentSearches.limit)
    #expect(recent.items.first == "show 9")
    recent.save(defaults)
    #expect(RecentSearches.load(defaults).items == recent.items)
    recent.clear()
    #expect(recent.items.isEmpty)
}
