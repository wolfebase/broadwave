@testable import BroadwaveKit
import Foundation
import Testing

@Test func searchAndDevicePathsKeepReservedCharacters() async throws {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [QueryStub.self]
    let api = try APIClient(base: #require(URL(string: "http://stub.invalid")), session: URLSession(configuration: config))
    let asked = "C++ & news = tonight?"
    _ = try await api.search(asked)
    let url = try #require(QueryStub.lastURL)
    #expect(url.path == "/api/v1/search")
    let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems
    #expect(items?.count == 1)
    #expect(items?.first?.name == "q")
    #expect(items?.first?.value == asked)

    _ = try await api.search("100%")
    let percent = try #require(QueryStub.lastURL)
    #expect(URLComponents(url: percent, resolvingAgainstBaseURL: false)?.queryItems?.first?.value == "100%")

    // queryItems turns %2B back into +, so the wire string is what the server reads.
    _ = try await api.search("C++")
    #expect(QueryStub.lastURL?.absoluteString.hasSuffix("q=C%2B%2B") == true)

    _ = try await api.search("a;b")
    #expect(QueryStub.lastURL?.absoluteString.hasSuffix("q=a%3Bb") == true)

    try await api.startScan(deviceID: "ab/cd")
    #expect(QueryStub.lastURL?.absoluteString == "http://stub.invalid/api/v1/devices/ab%2Fcd/scan")
    #expect(QueryStub.lastMethod == "POST")
    _ = try await api.scanStatus(deviceID: "ab/cd")
    #expect(QueryStub.lastURL?.absoluteString == "http://stub.invalid/api/v1/devices/ab%2Fcd/scan")
    #expect(QueryStub.lastMethod == "GET")
}

private final class QueryStub: URLProtocol, @unchecked Sendable {
    nonisolated(unsafe) static var lastURL: URL?
    nonisolated(unsafe) static var lastMethod: String?

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        Self.lastURL = request.url
        Self.lastMethod = request.httpMethod
        let path = request.url?.path ?? ""
        let json = path.hasSuffix("/scan") ? #"{"scanning":true,"found":0}"# : #"{"airings":[],"recordings":[]}"#
        let res = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: res, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(json.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}
