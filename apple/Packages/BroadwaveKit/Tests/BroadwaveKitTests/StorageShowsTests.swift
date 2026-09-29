@testable import BroadwaveKit
import Foundation
import Testing

private func fixture(_ name: String) throws -> Data {
    var url = URL(fileURLWithPath: #filePath)
    for _ in 0 ..< 6 {
        url.deleteLastPathComponent()
    }
    return try Data(contentsOf: url.appendingPathComponent("api/fixtures/\(name).json"))
}

@Test func decodesStorageShows() throws {
    let list = try APIClient.decoder.decode(StorageShows.self, from: fixture("storage-shows"))
    #expect(list.shows.count == 2)

    let news = list.shows[0]
    #expect(news.title == "Evening News")
    #expect(news.count == 2)
    #expect(news.bytes == 2_400_000_000)
    #expect(news.pass?.id == 4)
    #expect(news.pass?.keep == 3)
    #expect(news.newest > news.oldest)

    let game = list.shows[1]
    #expect(game.title == "The Afternoon Game")
    #expect(game.count == 1)
    #expect(game.bytes == 800_000_000)
    #expect(game.pass == nil)
    #expect(game.oldest == game.newest)
    #expect(news.bytes > game.bytes)
}

@Test func aShowTitleMatchesIgnoringCaseAndSpace() {
    #expect(sameShowTitle("Evening News", "Evening News"))
    #expect(sameShowTitle("  evening news ", "Evening News"))
    #expect(sameShowTitle("Evening News", " evening news "))
    #expect(!sameShowTitle("Night Talk", "Evening News"))
    #expect(!sameShowTitle("Evening News Extra", "Evening News"))
    #expect(sameShowTitle("Night Talk", ""))
}
