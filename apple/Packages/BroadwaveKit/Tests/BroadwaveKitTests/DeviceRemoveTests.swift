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

@Test func aDeviceDecodesOfflineWhenTheServerSendsItAndWhenItDoesNot() throws {
    let stopped = Data(#"""
    {"deviceId":"A3E00001","friendlyName":"HDHomeRun DUAL","baseUrl":"http://127.0.0.1:1","tunerCount":2,"lastSeen":"2026-10-05T03:00:00Z","offline":true}
    """#.utf8)
    let gone = try APIClient.decoder.decode(Device.self, from: stopped)
    #expect(gone.offline == true)
    #expect(gone.deviceId == "A3E00001")
    #expect(gone.lastSeen == "2026-10-05T03:00:00Z")
    #expect(LastSeen.offline(LastSeen.phrase(gone.lastSeen, now: Date(timeIntervalSince1970: 1_791_170_160))) == "Offline. Last seen 16 minutes ago.")

    let answering = Data(#"""
    {"deviceId":"FAKEHDHR","friendlyName":"Fake HDHomeRun","baseUrl":"http://127.0.0.1:2","tunerCount":2,"offline":false}
    """#.utf8)
    let live = try APIClient.decoder.decode(Device.self, from: answering)
    #expect(live.offline == false)

    let older = Data(#"""
    {"deviceId":"FAKEHDHR","friendlyName":"Fake HDHomeRun","baseUrl":"http://127.0.0.1:2","tunerCount":2}
    """#.utf8)
    let plain = try APIClient.decoder.decode(Device.self, from: older)
    #expect(plain.offline == nil)
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
