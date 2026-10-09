@testable import BroadwaveKit
import Foundation
import Testing

@MainActor
@Test func aFailedFavoriteLeavesTheChannelAndSaysWhy() async {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [FavoriteStub.self]
    let store = AppStore(session: URLSession(configuration: config))
    store.persistServers = false
    defer { store.forget() }
    let channel = Channel(
        id: 1,
        deviceId: "lab",
        guideNumber: "11.1",
        guideName: "HBR",
        displayNumber: "11.1",
        displayName: "Harbor News",
        hd: true,
        favorite: false,
        enabled: true,
        hidden: false,
        present: true
    )
    store.previewLineup([channel])
    await store.toggleFavorite(channel)
    #expect(store.channels.first?.favorite == false)
    #expect(store.error == "Favorites are off.")
}

private final class FavoriteStub: URLProtocol, @unchecked Sendable {
    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        let url = request.url ?? URL(string: "http://127.0.0.1:19057")!
        let body = Data(#"{"code":"off","message":"Favorites are off."}"#.utf8)
        let response = HTTPURLResponse(url: url, statusCode: 500, httpVersion: "HTTP/1.1", headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: body)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}
