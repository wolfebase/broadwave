@testable import BroadwaveKit
import Foundation
import Testing

private func channel(_ number: String, name: String, hd: Bool = true, favorite: Bool = false) -> Channel {
    Channel(
        id: Int64(number.hashValue & 0xFFFF), deviceId: "d", guideNumber: number, guideName: name,
        displayNumber: number, displayName: name, hd: hd, favorite: favorite, enabled: true, hidden: false, present: true
    )
}

private func airing(_ title: String, category: String? = nil, subtitle: String? = nil, seconds: TimeInterval = 3600) -> Airing {
    let start = Date(timeIntervalSince1970: 1_790_000_000)
    return Airing(id: 1, channelId: 1, title: title, subtitle: subtitle, category: category, start: start, end: start.addingTimeInterval(seconds))
}

@Test func channelNumbersSortAsNumbers() {
    #expect(Channel.guideOrder(channel("14.2", name: "Early"), channel("14.10", name: "Late")))
    #expect(!Channel.guideOrder(channel("14.10", name: "Late"), channel("14.2", name: "Early")))
    #expect(Channel.guideOrder(channel("2", name: "Two"), channel("10", name: "Ten")))
    #expect(Channel.guideOrder(channel("4.1", name: "Alpha"), channel("4.1", name: "Zed")))
    #expect(!Channel.guideOrder(channel("4.1", name: "Zed"), channel("4.1", name: "Alpha")))
    let ordered = ["10", "2", "14.10", "14.2", "9"].map { channel($0, name: $0) }.sorted(by: Channel.guideOrder)
    #expect(ordered.map(\.displayNumber) == ["2", "9", "10", "14.2", "14.10"])
}

@Test func listingsUseTheSameCategoriesAsTheWebGuide() {
    #expect(airing("Harbor at Valley", category: "Sports").kind == .sports)
    #expect(airing("NFL: Harbor at Valley").kind == .sports)
    #expect(airing("Game Day: Harbor at Valley").kind == .sports)
    #expect(airing("College Football: Harbor at Valley").kind == .sports)
    #expect(airing("College Basketball: Larks at Crows").kind == .sports)
    #expect(airing("Football").kind == .other)
    #expect(airing("Evening News").kind == .news)
    #expect(airing("The Newsroom").kind == .other)
    #expect(airing("The Newsroom", category: "Drama").kind == .series)
    #expect(airing("Quiz", category: "   ").kind == .other)
    #expect(airing("Quiz", category: "Drama").kind == .series)
    #expect(airing("Feature", category: "Movie").kind == .movies)
    #expect(airing("Cartoon", category: "Kids").kind == .kids)
    #expect(airing("Feature", category: "Family movie").kind == .movies)
}

@Test func aMatchupDropsTheLeaguePrefixOnEitherSide() {
    #expect(airing("Harbor at Valley").matchup.map { [$0.0, $0.1] } == ["Harbor", "Valley"])
    #expect(airing("Larks vs. Crows").matchup.map { [$0.0, $0.1] } == ["Larks", "Crows"])
    #expect(airing("NFL: Harbor at Valley").matchup.map { [$0.0, $0.1] } == ["Harbor", "Valley"])
    #expect(airing("Harbor at NBA: Valley").matchup.map { [$0.0, $0.1] } == ["Harbor", "Valley"])
    #expect(airing("NFL: NFC: Harbor at Valley").matchup?.0 == "NFC: Harbor")
    #expect(airing("Evening News").matchup == nil)
    let fromSubtitle = airing("Primetime", subtitle: "Harbor at Valley")
    #expect(fromSubtitle.matchup.map { [$0.0, $0.1] } == ["Harbor", "Valley"])
}

@Test func minutesLeftRoundsToTheNearestMinute() {
    let start = Date(timeIntervalSince1970: 1_790_000_000)
    let show = airing("News", seconds: 3600)
    #expect(show.minutesLeft(at: start.addingTimeInterval(3600 - 59)) == "1m left")
    #expect(show.minutesLeft(at: start.addingTimeInterval(3600 - 29)) == "0m left")
    #expect(show.minutesLeft(at: start.addingTimeInterval(3600 - 90 * 60)) == "1h 30m left")
    #expect(show.minutesLeft(at: start.addingTimeInterval(3600 - 61 * 60)) == "1h 1m left")
    #expect(show.minutesLeft(at: start.addingTimeInterval(3600 + 30)) == "0m left")
}

#if DEBUG
    @Test @MainActor func theFeaturedChannelPrefersAFavorite() {
        let store = AppStore()
        store.previewLineup([
            channel("4.1", name: "Plain", hd: true),
            channel("5.1", name: "Favorite", hd: false, favorite: true),
        ])
        #expect(store.featured()?.0.displayName == "Favorite")
        store.previewLineup([])
        #expect(store.featured() == nil)
    }
#endif

@Test @MainActor func homeNoticesWaitTheirTurn() {
    let store = AppStore()
    store.noteHome("   ")
    #expect(store.homeNotice == nil)
    store.noteHome("New Apple TV found: Living Room.")
    store.noteHome("New tuner found: Den.")
    #expect(store.homeNotice == "New Apple TV found: Living Room.")
    store.dismissHome()
    #expect(store.homeNotice == "New tuner found: Den.")
    store.dismissHome()
    #expect(store.homeNotice == nil)
}

@Test func aSavedCatalogStaysInsideItsDirectory() throws {
    let dir = FileManager.default.temporaryDirectory.appendingPathComponent("broadwave-kit-\(UUID().uuidString)", isDirectory: true)
    defer { try? FileManager.default.removeItem(at: dir) }
    let snap = CatalogSnapshot(channels: [], airings: [], recordings: [])
    CatalogCache.save(snap, serverID: "../outside", directory: dir)
    let inside = dir.appendingPathComponent("broadwave-outside.json")
    let escaped = dir.deletingLastPathComponent().appendingPathComponent("broadwave-outside.json")
    #expect(FileManager.default.fileExists(atPath: inside.path))
    #expect(!FileManager.default.fileExists(atPath: escaped.path))
    let written = try #require(CatalogCache.savedAt(serverID: "../outside", directory: dir))
    #expect(abs(written.timeIntervalSinceNow) < 30)
    #expect(CatalogCache.load(serverID: "../outside", directory: dir)?.channels.isEmpty == true)
}

@Test func aFailedGuideTailKeepsListingsPastTheWindow() {
    let now = Date(timeIntervalSince1970: 1_790_500_000)
    let horizon = now.addingTimeInterval(4 * 3600)
    let near = Airing(id: 1, channelId: 4, title: "Harbor Report", start: now, end: now.addingTimeInterval(1800))
    let far = Airing(id: 2, channelId: 4, title: "Valley News", start: horizon.addingTimeInterval(3600), end: horizon.addingTimeInterval(7200))
    let gone = Airing(id: 3, channelId: 4, title: "Night Desk", start: horizon.addingTimeInterval(-60), end: horizon.addingTimeInterval(1800))
    let kept = GuideMerge.listings(previous: [near, far, gone], window: [near], tail: nil, horizon: horizon)
    #expect(kept.map(\.id) == [1, 2])
    let cleared = GuideMerge.listings(previous: [near, far], window: [near], tail: [], horizon: horizon)
    #expect(cleared.map(\.id) == [1])
}
