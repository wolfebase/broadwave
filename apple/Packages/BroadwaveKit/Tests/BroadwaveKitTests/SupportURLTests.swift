@testable import BroadwaveKit
import Foundation
import Testing

@Test func supportURLIsOnTheServerOrigin() throws {
    let base = try #require(URL(string: "http://127.0.0.1:9/api/v1"))
    let support = APIClient(base: base).supportURL()
    #expect(support.path == "/api/v1/support")
    #expect(support.host() == "127.0.0.1")
}
