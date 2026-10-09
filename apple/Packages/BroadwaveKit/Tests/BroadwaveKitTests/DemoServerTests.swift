@testable import BroadwaveKit
import Foundation
import Network
import Testing

/// The films the app bundles (`apple/App/DemoMedia`).
let demoMedia = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
    .deletingLastPathComponent().deletingLastPathComponent()
    .appendingPathComponent("App/DemoMedia")

@Test func demoServerAnswersOnLoopback() async throws {
    let server = DemoServer()
    let port = UInt16.random(in: 20000 ... 45000)
    let origin = try #require(await server.prepare(port: port, media: demoMedia))
    defer { server.stop() }
    #expect(origin.host == "127.0.0.1")

    let (data, response) = try await URLSession.shared.data(from: demoURL(origin, "/api/v1/server"))
    let http = try #require(response as? HTTPURLResponse)
    #expect(http.statusCode == 200)
    let info = try APIClient.decoder.decode(ServerInfo.self, from: data)
    #expect(info.id == "demo")
    #expect(info.features.contains("wholeHomeSync"))
    #expect(info.minAppVersion == "1.0")

    let (channelData, _) = try await URLSession.shared.data(from: demoURL(origin, "/api/v1/channels?guide=1"))
    struct Channels: Decodable { var channels: [Channel] }
    let channels = try APIClient.decoder.decode(Channels.self, from: channelData).channels
    #expect(channels.map(\.guideNumber) == ["4.1", "5.1", "9.1", "11.1"])

    let now = Date()
    var parts = try #require(URLComponents(url: demoURL(origin, "/api/v1/airings"), resolvingAgainstBaseURL: false))
    parts.queryItems = [
        URLQueryItem(name: "from", value: ISO8601DateFormatter.plain.string(from: now.addingTimeInterval(-1800))),
        URLQueryItem(name: "to", value: ISO8601DateFormatter.plain.string(from: now.addingTimeInterval(3600))),
    ]
    let (airingData, _) = try await URLSession.shared.data(from: #require(parts.url))
    struct Airings: Decodable { var airings: [Airing] }
    let airings = try APIClient.decoder.decode(Airings.self, from: airingData).airings
    #expect(airings.contains { $0.channelId == 1 && $0.title == "Big Buck Bunny" && $0.isOn(at: now) })

    var watch = URLRequest(url: demoURL(origin, "/api/v1/watch"))
    watch.httpMethod = "POST"
    watch.setValue("application/json", forHTTPHeaderField: "Content-Type")
    watch.httpBody = Data("{\"channelId\":1}".utf8)
    let (watchData, watchResponse) = try await URLSession.shared.data(for: watch)
    #expect((watchResponse as? HTTPURLResponse)?.statusCode == 200)
    let session = try APIClient.decoder.decode(WatchSession.self, from: watchData)
    #expect(session.playlist == "/live/1/index.m3u8")

    let playlistURL = try #require(URL(string: session.playlist, relativeTo: origin)?.absoluteURL)
    let (playlistData, _) = try await URLSession.shared.data(from: playlistURL)
    let playlist = String(bytes: playlistData, encoding: .utf8) ?? ""
    #expect(playlist.contains("#EXT-X-PROGRAM-DATE-TIME:"))
    #expect(!playlist.contains("#EXT-X-ENDLIST"))
    let dates = playlist.split(separator: "\n").compactMap { line -> Date? in
        guard line.hasPrefix("#EXT-X-PROGRAM-DATE-TIME:") else { return nil }
        return ISO8601DateFormatter.fractional.date(from: String(line.dropFirst("#EXT-X-PROGRAM-DATE-TIME:".count)))
    }
    #expect(dates.contains { abs($0.timeIntervalSinceNow) < 30 })

    let planBody = Data("{\"channelIds\":[1,3]}".utf8)
    var planRequest = URLRequest(url: demoURL(origin, "/api/v1/multiview/plan"))
    planRequest.httpMethod = "POST"
    planRequest.setValue("application/json", forHTTPHeaderField: "Content-Type")
    planRequest.httpBody = planBody
    let (planData, _) = try await URLSession.shared.data(for: planRequest)
    let plan = try APIClient.decoder.decode(MultiviewPlan.self, from: planData)
    #expect(plan.playable.map(\.channelId) == [1, 3])
    #expect(plan.blocked.isEmpty)

    let client = APIClient(base: origin)
    let scan = try await client.home(fresh: true)
    #expect(scan.places.isEmpty)
    #expect(scan.sharing == false)

    if let lan = lanIPv4() {
        let refused = await tcpRefused(host: lan, port: port)
        #expect(refused)
    }
}

@Test func demoSettingsRoundTrip() async throws {
    let server = DemoServer()
    let port = UInt16.random(in: 20000 ... 45000)
    let origin = try #require(await server.prepare(port: port, media: demoMedia))
    defer { server.stop() }
    let client = APIClient(base: origin)
    let before = try await client.settings()
    #expect(before["pictureMode"] == "broadcast")
    #expect(before["bufferMinutes"] == "60")
    #expect(before["autoplay"] == "1")
    #expect(before["hideScores"] == "0")
    #expect(before["hdhrEmulate"] == "0")
    #expect(before["writeNfo"] == "0")
    try await client.saveSettings(["pictureMode": "film", "hideScores": "1", "autoplay": "0", "hdhrEmulate": "1"])
    let after = try await client.settings()
    #expect(after["pictureMode"] == "film")
    #expect(after["hideScores"] == "1")
    #expect(after["autoplay"] == "0")
    #expect(after["hdhrEmulate"] == "1")
    try await client.saveSettings(["pictureMode": "nope"])
    let kept = try await client.settings()
    #expect(kept["pictureMode"] == "film")
    let secret = "demo-secret-value"
    try await client.saveSettings([
        "sdUser": "guide@example.com",
        "sdPassword": secret,
        "tmdbKey": secret,
        "watermarkGB": "12",
        "guideUrl": "https://example.com/guide.xml",
    ])
    let guide = try await client.settings()
    #expect(guide["sdUser"] == "guide@example.com")
    #expect(guide["watermarkGB"] == "12")
    #expect(guide["guideUrl"] == "https://example.com/guide.xml")
    #expect(guide["sdPasswordSet"] == "1")
    #expect(guide["tmdbKeySet"] == "1")
    #expect(guide["sdPassword"] == nil)
    #expect(guide["tmdbKey"] == nil)
    #expect(!guide.values.contains(secret))
    try await client.saveSettings(["watermarkGB": "nope", "sdPassword": "  ", "bufferMinutes": "15"])
    let rejected = try await client.settings()
    #expect(rejected["watermarkGB"] == "12")
    #expect(rejected["sdPassword"] == nil)
    #expect(rejected["bufferMinutes"] == "60")
    try await client.saveSettings(["bufferMinutes": "120"])
    #expect(try await (client.settings())["bufferMinutes"] == "120")
    try await client.saveSettings(["bufferMinutes": "0"])
    #expect(try await (client.settings())["bufferMinutes"] == "0")
    try await client.saveSettings(["writeNfo": "1"])
    #expect(try await (client.settings())["writeNfo"] == "1")
    try await client.saveSettings(["writeNfo": "yes"])
    #expect(try await (client.settings())["writeNfo"] == "1")
    #expect(before["makeRoom"] == "0")
    #expect(before["deleteWatchedDays"] == "0")
    try await client.saveSettings(["makeRoom": "1", "deleteWatchedDays": "14"])
    let room = try await client.settings()
    #expect(room["makeRoom"] == "1")
    #expect(room["deleteWatchedDays"] == "14")
    try await client.saveSettings(["makeRoom": "yes", "deleteWatchedDays": "week"])
    let refused = try await client.settings()
    #expect(refused["makeRoom"] == "1")
    #expect(refused["deleteWatchedDays"] == "14")
    try await client.saveSettings(["deleteWatchedDays": "3651"])
    #expect(try await client.settings()["deleteWatchedDays"] == "14")
}

@Test func demoKeepRoundTrip() async throws {
    let server = DemoServer()
    let port = UInt16.random(in: 20000 ... 45000)
    let origin = try #require(await server.prepare(port: port, media: demoMedia))
    defer { server.stop() }
    let client = APIClient(base: origin)
    #expect(try await client.setKeep(recordingID: 7, true))
    let listed = try await client.recordings()
    #expect(listed.first { $0.id == 7 }?.keep == true)
    #expect(listed.first { $0.id == 7 }?.title == "Kept")
    #expect(try await client.setKeep(recordingID: 7, false) == false)
    let again = try await client.recordings()
    #expect(again.first { $0.id == 7 }?.keep == nil)
}

@Test func demoHiddenChannelLeavesTheGuideOnly() async throws {
    let server = DemoServer()
    let port = UInt16.random(in: 20000 ... 45000)
    let origin = try #require(await server.prepare(port: port, media: demoMedia))
    defer { server.stop() }
    let client = APIClient(base: origin)
    let hidden = try await client.patchChannel(3, ChannelPatch(hidden: true))
    #expect(hidden.hidden)
    #expect(try await client.channels().map(\.id) == [1, 2, 4])
    let lineup = try await client.lineup()
    #expect(lineup.map(\.id) == [1, 2, 3, 4])
    #expect(lineup.first { $0.id == 3 }?.hidden == true)
    _ = try await client.patchChannel(3, ChannelPatch(hidden: false))
    #expect(try await client.channels().count == 4)
    #expect(try await client.passes().isEmpty)
}

@Test func demoRoomCountsBothScreens() async throws {
    let server = DemoServer()
    let port = UInt16.random(in: 45001 ... 49151)
    let origin = try #require(await server.prepare(port: port, media: demoMedia))
    defer { server.stop() }
    var comps = try #require(URLComponents(url: origin, resolvingAgainstBaseURL: false))
    comps.scheme = "ws"
    comps.path = "/api/v1/ws"
    let url = try #require(comps.url)
    let first = URLSession.shared.webSocketTask(with: url)
    let second = URLSession.shared.webSocketTask(with: url)
    first.resume()
    second.resume()
    defer {
        first.cancel(with: .goingAway, reason: nil)
        second.cancel(with: .goingAway, reason: nil)
    }
    let join = "{\"type\":\"sync.join\",\"data\":{\"room\":\"channel:1\",\"channelId\":1}}"
    try await first.send(.string(join))
    try await second.send(.string(join))
    let members = try await withThrowingTaskGroup(of: Int.self) { group in
        group.addTask { try await maxMembers(second) }
        group.addTask {
            try await Task.sleep(for: .seconds(5))
            return -1
        }
        let value = try await group.next() ?? -1
        group.cancelAll()
        return value
    }
    #expect(members == 2)
}

@Test func demoFilmsStayUnder25MB() {
    let root = demoMedia
    var total: Int64 = 0
    let files = FileManager.default.enumerator(at: root, includingPropertiesForKeys: [.fileSizeKey])
    while let url = files?.nextObject() as? URL {
        total += Int64((try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0)
    }
    #expect(total <= 25 * 1024 * 1024)
    for channel in 1 ... 4 {
        let initURL = root.appendingPathComponent("\(channel)/init.mp4")
        #expect(FileManager.default.fileExists(atPath: initURL.path))
    }
}

private func memberCount(_ text: String) -> Int? {
    guard text.contains("sync.state"), let data = text.data(using: .utf8) else { return nil }
    guard let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return nil }
    guard let body = obj["data"] as? [String: Any] else { return nil }
    return (body["members"] as? NSNumber)?.intValue
}

private func demoURL(_ origin: URL, _ path: String) -> URL {
    URL(string: path, relativeTo: origin)!.absoluteURL
}

private func maxMembers(_ task: URLSessionWebSocketTask) async throws -> Int {
    var best = 0
    for _ in 0 ..< 8 {
        let message = try await task.receive()
        guard case let .string(text) = message else { continue }
        guard let members = memberCount(text) else { continue }
        best = max(best, members)
        if best >= 2 {
            return best
        }
    }
    return best
}

private func lanIPv4() -> String? {
    var pointer: UnsafeMutablePointer<ifaddrs>?
    guard getifaddrs(&pointer) == 0, let first = pointer else { return nil }
    defer { freeifaddrs(pointer) }
    var cursor: UnsafeMutablePointer<ifaddrs>? = first
    while let current = cursor {
        defer { cursor = current.pointee.ifa_next }
        guard let address = current.pointee.ifa_addr, address.pointee.sa_family == UInt8(AF_INET) else { continue }
        let name = String(cString: current.pointee.ifa_name)
        guard name.hasPrefix("en") else { continue }
        var host = [CChar](repeating: 0, count: Int(NI_MAXHOST))
        let length = socklen_t(address.pointee.sa_len)
        guard getnameinfo(address, length, &host, socklen_t(host.count), nil, 0, NI_NUMERICHOST) == 0 else { continue }
        let bytes = host.prefix { $0 != 0 }.map { UInt8(bitPattern: $0) }
        let ip = String(bytes: bytes, encoding: .utf8) ?? ""
        if ip.hasPrefix("192.168.") || ip.hasPrefix("10.") {
            return ip
        }
    }
    return nil
}

private func tcpRefused(host: String, port: UInt16) async -> Bool {
    guard let endpoint = NWEndpoint.Port(rawValue: port) else { return false }
    let conn = NWConnection(host: NWEndpoint.Host(host), port: endpoint, using: .tcp)
    return await withCheckedContinuation { cont in
        let once = RefusedOnce()
        conn.stateUpdateHandler = { state in
            switch state {
            case .ready:
                once.run { cont.resume(returning: false) }
                conn.cancel()
            case .failed, .cancelled:
                once.run { cont.resume(returning: true) }
            default:
                break
            }
        }
        conn.start(queue: .global())
        DispatchQueue.global().asyncAfter(deadline: .now() + 1) {
            once.run {
                conn.cancel()
                cont.resume(returning: true)
            }
        }
    }
}

private final class RefusedOnce: @unchecked Sendable {
    private let lock = NSLock()
    private var done = false
    func run(_ body: () -> Void) {
        lock.lock()
        if done {
            lock.unlock()
            return
        }
        done = true
        lock.unlock()
        body()
    }
}

@Test func demoHasNothingComingUp() async throws {
    let server = DemoServer()
    let port = UInt16.random(in: 20000 ... 45000)
    let origin = try #require(await server.prepare(port: port, media: demoMedia))
    defer { server.stop() }
    let client = APIClient(base: origin)
    let plan = try await client.schedule()
    #expect(plan.items.isEmpty)
    #expect(try await client.events().isEmpty)
}

@Test func reconnectWaitBacksOffWithJitterUnderFiveSeconds() {
    #expect(EventSocket.reconnectWait(0, random: 0) == 0.25)
    #expect(EventSocket.reconnectWait(0, random: 1) == 0.5)
    #expect(EventSocket.reconnectWait(3, random: 0.5) == 3)
    for retry in 0 ..< 40 {
        #expect(EventSocket.reconnectWait(retry, random: 1) <= 5)
        #expect(EventSocket.reconnectWait(retry, random: 0) >= 0.25)
    }
}

@MainActor
@Test func aRestartedServerIsNamedAndTheRoomIsJoinedAgain() async throws {
    // Below the system's ephemeral range (49152 up), which an outgoing
    // connection can hold when the second server binds the same port.
    let port = UInt16.random(in: 45001 ... 49151)
    let first = DemoServer()
    let origin = try #require(await first.prepare(port: port, media: demoMedia))
    let socket = EventSocket(base: origin)
    defer { socket.disconnect() }
    var seen: [String] = []
    socket.on("reconnected") { _ in seen.append("reconnected") }
    socket.on("restarted") { _ in seen.append("restarted") }
    var members = 0
    socket.on("sync.state") { data in
        let body = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
        members = (body?["members"] as? NSNumber)?.intValue ?? members
    }
    socket.connect()
    socket.join(room: "channel:1", channelID: 1)
    try await waitFor { socket.connected && members == 1 }
    #expect(seen.isEmpty)

    first.stop()
    try await waitFor { !socket.connected }
    members = 0
    let second = DemoServer()
    _ = try #require(await second.prepare(port: port, media: demoMedia))
    defer { second.stop() }
    let back = Date()
    try await waitFor(seconds: 8) { socket.connected && members == 1 }
    #expect(Date().timeIntervalSince(back) < 7)
    try await waitFor { seen.contains("restarted") }
    #expect(seen == ["reconnected", "restarted"])
}

@MainActor
private func waitFor(seconds: Double = 5, _ done: () -> Bool) async throws {
    let end = Date().addingTimeInterval(seconds)
    while !done() {
        if Date() > end {
            throw CancellationError()
        }
        try await Task.sleep(for: .milliseconds(50))
    }
}
