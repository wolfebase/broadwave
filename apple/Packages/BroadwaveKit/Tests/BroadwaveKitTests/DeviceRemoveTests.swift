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

@Test func removeDeviceReadsTheDevicesLeftAndTheBusyRefusal() async throws {
    struct Devices: Decodable { var devices: [Device] }
    let left = try fixture("device-remove")
    let decoded = try APIClient.decoder.decode(Devices.self, from: left).devices
    #expect(decoded.map(\.deviceId) == ["FAKEHDHR", "src-1"])
    #expect(decoded[0].tunerCount == 2)
    #expect(decoded[1].friendlyName == "FastChannels")
    #expect(decoded[1].tunerCount == 0)

    // The contract fixture is the 200 body. A busy tuner answers this envelope.
    let busy = Data(#"{"code":"device_busy","message":"Something is playing or recording from it. Stop that first."}"#.utf8)
    let refusal = try APIClient.decoder.decode(APIErrorBody.self, from: busy)
    #expect(refusal.code == "device_busy")
    #expect(refusal.message == "Something is playing or recording from it. Stop that first.")

    RemoveDeviceStub.payload = left
    RemoveDeviceStub.status = 200
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [RemoveDeviceStub.self]
    let api = try APIClient(base: #require(URL(string: "http://stub.invalid")), session: URLSession(configuration: config))
    let devices = try await api.removeDevice("src-2")
    #expect(devices.map(\.deviceId) == ["FAKEHDHR", "src-1"])
    #expect(RemoveDeviceStub.lastMethod == "DELETE")
    #expect(RemoveDeviceStub.lastPath == "/api/v1/devices/src-2")

    RemoveDeviceStub.payload = busy
    RemoveDeviceStub.status = 409
    do {
        _ = try await api.removeDevice("FAKEHDHR")
        Issue.record("a busy device was removed")
    } catch let error as APIError {
        #expect(error.code == "device_busy")
        #expect(error.status == 409)
        #expect(error.message == "Something is playing or recording from it. Stop that first.")
    }
    #expect(RemoveDeviceStub.lastPath == "/api/v1/devices/FAKEHDHR")
}

private final class RemoveDeviceStub: URLProtocol, @unchecked Sendable {
    nonisolated(unsafe) static var payload = Data()
    nonisolated(unsafe) static var status = 200
    nonisolated(unsafe) static var lastMethod: String?
    nonisolated(unsafe) static var lastPath: String?

    override static func canInit(with _: URLRequest) -> Bool {
        true
    }

    override static func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        Self.lastMethod = request.httpMethod
        Self.lastPath = request.url?.path
        let res = HTTPURLResponse(url: request.url!, statusCode: Self.status, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: res, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Self.payload)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}
