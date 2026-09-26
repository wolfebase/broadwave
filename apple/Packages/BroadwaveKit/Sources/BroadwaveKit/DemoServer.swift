import CryptoKit
import Foundation
import Network

/// Loopback player for Try the demo. It never opens a public interface and it
/// does not register Bonjour. A second screen on this Mac joins the same port
/// so two simulators share one room. The listener stays up after Leave the demo
/// so that other screen keeps playing.
public final class DemoServer: @unchecked Sendable {
    public static let shared = DemoServer()
    public static let port: UInt16 = 18649

    private let lock = NSLock()
    private let queue = DispatchQueue(label: "broadwave.demo")
    private var listener: NWListener?
    private var origin: URL?
    private var mediaRoot: URL?
    private var favorites: [Int64: Bool] = [:]
    private var hidden: [Int64: Bool] = [:]
    private var sockets: [ObjectIdentifier: DemoSocket] = [:]
    private var rooms: [String: DemoRoom] = [:]

    public func prepare(port: UInt16 = DemoServer.port, media: URL? = nil) async -> URL? {
        if let media {
            setMedia(media)
        } else if mediaDirectory() == nil {
            setMedia(Self.bundledMedia())
        }
        if let existing = currentOrigin() {
            return existing
        }
        if port == Self.port, let other = await Self.probe(port) {
            setOrigin(other)
            return other
        }
        return await listen(port)
    }

    public func stop() {
        lock.lock()
        let listener = listener
        let sockets = Array(sockets.values)
        self.listener = nil
        self.sockets.removeAll()
        rooms.removeAll()
        origin = nil
        lock.unlock()
        listener?.cancel()
        for socket in sockets {
            socket.cancel()
        }
    }

    func currentOrigin() -> URL? {
        lock.lock()
        defer { lock.unlock() }
        return origin
    }

    func mediaDirectory() -> URL? {
        lock.lock()
        defer { lock.unlock() }
        return mediaRoot
    }

    private func listen(_ port: UInt16) async -> URL? {
        let params = NWParameters.tcp
        params.requiredInterfaceType = .loopback
        params.allowLocalEndpointReuse = true
        guard let endpoint = NWEndpoint.Port(rawValue: port) else { return nil }
        guard let listener = try? NWListener(using: params, on: endpoint) else { return nil }
        listener.newConnectionHandler = { [weak self] conn in
            self?.accept(conn)
        }
        let origin: URL? = await withCheckedContinuation { cont in
            let once = DemoOnce()
            listener.stateUpdateHandler = { state in
                switch state {
                case .ready:
                    let bound = listener.port?.rawValue ?? port
                    once.run { cont.resume(returning: URL(string: "http://127.0.0.1:\(bound)")) }
                case .failed:
                    once.run { cont.resume(returning: nil) }
                default:
                    break
                }
            }
            listener.start(queue: queue)
            queue.asyncAfter(deadline: .now() + 3) {
                once.run { cont.resume(returning: nil) }
            }
        }
        guard let origin else {
            listener.cancel()
            if port == Self.port {
                return await Self.probe(port)
            }
            return nil
        }
        storeListener(listener, origin)
        return origin
    }

    private func setMedia(_ url: URL?) {
        lock.lock()
        mediaRoot = url
        lock.unlock()
    }

    private func setOrigin(_ url: URL?) {
        lock.lock()
        origin = url
        lock.unlock()
    }

    private func storeListener(_ listener: NWListener, _ url: URL) {
        lock.lock()
        self.listener = listener
        origin = url
        lock.unlock()
    }

    private static func probe(_ port: UInt16) async -> URL? {
        guard let url = URL(string: "http://127.0.0.1:\(port)/api/v1/server") else { return nil }
        var request = URLRequest(url: url)
        request.timeoutInterval = 0.4
        guard let (data, response) = try? await URLSession.shared.data(for: request),
              (response as? HTTPURLResponse)?.statusCode == 200,
              let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              (obj["id"] as? String) == "demo"
        else { return nil }
        return URL(string: "http://127.0.0.1:\(port)")
    }

    static func bundledMedia() -> URL? {
        let bundled = Bundle.module.resourceURL?.appendingPathComponent("DemoMedia")
        if let bundled, FileManager.default.fileExists(atPath: bundled.path) {
            return bundled
        }
        return Bundle.module.url(forResource: "DemoMedia", withExtension: nil)
    }

    private func accept(_ conn: NWConnection) {
        let socket = DemoSocket(conn: conn, server: self)
        lock.lock()
        sockets[ObjectIdentifier(socket)] = socket
        lock.unlock()
        socket.start(on: queue)
    }

    fileprivate func drop(_ socket: DemoSocket) {
        let id = ObjectIdentifier(socket)
        lock.lock()
        sockets.removeValue(forKey: id)
        var changed: [DemoRoom] = []
        for name in rooms.keys where rooms[name]?.members.contains(id) == true {
            guard var room = rooms[name] else { continue }
            room.members.remove(id)
            if room.members.isEmpty {
                rooms.removeValue(forKey: name)
            } else {
                room.version += 1
                rooms[name] = room
                changed.append(room)
            }
        }
        let open = sockets
        lock.unlock()
        for room in changed {
            send(room, to: open)
        }
    }

    fileprivate func receive(_ text: String, from socket: DemoSocket) {
        guard let data = text.data(using: .utf8),
              let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let type = obj["type"] as? String
        else { return }
        let body = obj["data"] as? [String: Any] ?? [:]
        switch type {
        case "clock":
            let t0 = (body["t0"] as? NSNumber)?.doubleValue ?? 0
            reply(socket, "clock", ["t0": t0, "t1": Self.nowMS()])
        case "sync.join":
            guard let room = body["room"] as? String, Self.validRoom(room) else { return }
            let channel = (body["channelId"] as? NSNumber)?.int64Value ?? 0
            join(room, channel: channel, socket: socket)
        case "sync.leave":
            guard let room = body["room"] as? String else { return }
            leave(room, socket: socket)
        case "here":
            break
        case "sync.command":
            guard let room = body["room"] as? String else { return }
            command(room, body: body, socket: socket)
        default:
            break
        }
    }

    private func join(_ name: String, channel: Int64, socket: DemoSocket) {
        lock.lock()
        var room = rooms[name] ?? DemoRoom(name: name, channelId: channel)
        let id = ObjectIdentifier(socket)
        if !room.members.contains(id) {
            room.members.insert(id)
            room.version += 1
        }
        rooms[name] = room
        let open = sockets
        lock.unlock()
        send(room, to: open)
    }

    private func leave(_ name: String, socket: DemoSocket) {
        lock.lock()
        guard var room = rooms[name] else {
            lock.unlock()
            return
        }
        room.members.remove(ObjectIdentifier(socket))
        room.version += 1
        if room.members.isEmpty {
            rooms.removeValue(forKey: name)
            lock.unlock()
            return
        }
        rooms[name] = room
        let open = sockets
        lock.unlock()
        send(room, to: open)
    }

    private func command(_ name: String, body: [String: Any], socket: DemoSocket) {
        lock.lock()
        guard var room = rooms[name], room.members.contains(ObjectIdentifier(socket)) else {
            lock.unlock()
            return
        }
        guard room.grouped else {
            lock.unlock()
            reply(socket, "error", ["code": "sync", "message": "Only a group pauses every screen."])
            return
        }
        let action = body["action"] as? String ?? ""
        let now = Self.nowMS()
        switch action {
        case "pause":
            let at = (body["mediaTime"] as? NSNumber)?.doubleValue ?? room.target(at: now)
            room.anchorServer = now
            room.anchorMedia = at
            room.rate = 0
        case "play", "live":
            room.anchorServer = now
            room.anchorMedia = now - 10000
            room.rate = 1
        case "seek":
            let at = (body["mediaTime"] as? NSNumber)?.doubleValue ?? room.anchorMedia
            room.anchorServer = now
            room.anchorMedia = at
        default:
            lock.unlock()
            return
        }
        room.version += 1
        rooms[name] = room
        let open = sockets
        lock.unlock()
        send(room, to: open)
    }

    private func send(_ room: DemoRoom, to sockets: [ObjectIdentifier: DemoSocket]) {
        let frame = Self.textFrame("sync.state", room.payload())
        for id in room.members {
            sockets[id]?.send(frame)
        }
    }

    private func reply(_ socket: DemoSocket, _ type: String, _ data: [String: Any]) {
        socket.send(Self.textFrame(type, data))
    }

    fileprivate func hello() -> String {
        Self.textFrame("hello", ["serverTime": Self.nowMS()])
    }

    private static func textFrame(_ type: String, _ data: [String: Any]) -> String {
        let obj: [String: Any] = ["type": type, "data": data]
        guard JSONSerialization.isValidJSONObject(obj),
              let bytes = try? JSONSerialization.data(withJSONObject: obj),
              let text = String(data: bytes, encoding: .utf8)
        else { return "{\"type\":\"\(type)\"}" }
        return text
    }

    fileprivate func http(_ method: String, _ target: String, body: Data) -> Data {
        let parts = target.split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false)
        let path = String(parts.first ?? "/")
        let query = parts.count > 1 ? String(parts[1]) : ""
        switch (method, path) {
        case ("GET", "/api/v1/server"):
            return Self.ok(Self.json(Self.serverInfo()))
        case ("GET", "/api/v1/clock"):
            return Self.ok(Self.json(["serverTime": Self.nowMS()]))
        case ("GET", "/api/v1/channels"):
            return Self.ok(Self.json(["channels": channels()]))
        case ("GET", "/api/v1/airings"):
            return Self.ok(Self.json(["airings": airings(query: query)]))
        case ("GET", "/api/v1/recordings"):
            return Self.ok(Data("{\"recordings\":[]}".utf8))
        case ("GET", "/api/v1/settings"):
            return Self.ok(Data("{\"needsSetup\":\"0\",\"setupComplete\":\"1\",\"liveScores\":\"0\",\"checkUpdates\":\"0\"}".utf8))
        case ("GET", "/api/v1/teams"):
            return Self.ok(Data("{\"teams\":[]}".utf8))
        case ("GET", "/api/v1/sports/scoreboard"):
            return Self.ok(Data("{\"games\":[]}".utf8))
        case ("GET", "/api/v1/home"):
            return Self.ok(Data("{\"places\":[],\"tunerAddress\":\"\",\"sharing\":false}".utf8))
        case ("GET", "/api/v1/search"):
            return Self.ok(Self.json(search(query)))
        case ("POST", "/api/v1/watch"):
            return watch(body)
        case ("POST", "/api/v1/multiview/plan"):
            return plan(body)
        case ("POST", _) where path.hasPrefix("/api/v1/watch/") && path.hasSuffix("/stop"):
            return Self.ok(Data("{}".utf8))
        case ("PATCH", _) where path.hasPrefix("/api/v1/channels/"):
            return patchChannel(path, body: body)
        case ("GET", _) where path.hasPrefix("/live/"):
            return media(path)
        case ("GET", _) where path.hasPrefix("/media/art/"):
            return art(path)
        case ("GET", _) where path.hasPrefix("/api/v1/channels/") && path.hasSuffix("/frame"):
            return art(path)
        case ("POST", "/api/v1/recordings"):
            return Self.fail(409, "demo", "The demo plays samples. It does not record.")
        default:
            return Self.fail(404, "missing", "The demo has no \(path).")
        }
    }

    private func channels() -> [Channel] {
        lock.lock()
        let fav = favorites
        let hide = hidden
        lock.unlock()
        return DemoFilm.all.map { film in
            var channel = film.channel()
            if let on = fav[film.id] {
                channel.favorite = on
            }
            if let on = hide[film.id] {
                channel.hidden = on
            }
            return channel
        }
    }

    private func airings(query: String) -> [Airing] {
        let items = URLComponents(string: "http://demo/?\(query)")?.queryItems ?? []
        let from = items.first { $0.name == "from" }?.value.flatMap { ISO8601DateFormatter.plain.date(from: $0) }
        let to = items.first { $0.name == "to" }?.value.flatMap { ISO8601DateFormatter.plain.date(from: $0) }
        return DemoFilm.airings(from: from, to: to)
    }

    private func search(_ query: String) -> SearchResult {
        let raw = query.split(separator: "&").first { $0.hasPrefix("q=") }.map { String($0.dropFirst(2)) } ?? ""
        let q = raw.removingPercentEncoding ?? raw
        let hits = DemoFilm.airings(from: nil, to: nil).filter {
            q.isEmpty || $0.title.localizedCaseInsensitiveContains(q)
        }
        return SearchResult(query: q, airings: hits, recordings: [])
    }

    private func watch(_ body: Data) -> Data {
        let channel = Self.intField(body, "channelId") ?? 1
        guard DemoFilm.all.contains(where: { $0.id == channel }) else {
            return Self.fail(404, "missing", "That channel is not in the demo.")
        }
        let session = WatchSession(
            channelId: channel,
            playlist: "/live/\(channel)/index.m3u8",
            rendition: "720.aac2",
            stream: StreamInfo(
                rendition: "720.aac2",
                video: "720",
                audio: "aac2",
                mode: "broadcast",
                reason: "Original picture",
                sourceVideo: "H264",
                sourceAudio: "AAC",
                encoder: "copy",
                scan: "progressive",
                sourceWidth: 1280,
                sourceHeight: 720,
                sourceFps: "30",
                outputWidth: 1280,
                outputHeight: 720,
                outputFps: "30",
                bitrate: "1 Mb/s",
                decode: "cpu"
            ),
            encoder: "copy",
            picture: "broadcast",
            shared: false,
            viewers: 1,
            frequencyHz: 500_000_000 + Int(channel)
        )
        return Self.ok(Self.json(session))
    }

    private func plan(_ body: Data) -> Data {
        let ids = Self.intList(body, "channelIds")
        var playable: [MultiviewPlanPlayable] = []
        var blocked: [MultiviewPlanBlocked] = []
        for id in ids {
            if DemoFilm.all.contains(where: { $0.id == id }) {
                playable.append(MultiviewPlanPlayable(channelId: id, frequencyHz: 500_000_000 + Int(id), shared: false))
            } else {
                blocked.append(MultiviewPlanBlocked(channelId: id, holders: [], reason: "That channel is not in the demo."))
            }
        }
        let plan = MultiviewPlan(
            playable: playable,
            blocked: blocked,
            tunersNeeded: 0,
            tunersFree: DemoFilm.all.count,
            note: ""
        )
        return Self.ok(Self.json(plan))
    }

    private func patchChannel(_ path: String, body: Data) -> Data {
        let id = Int64(path.split(separator: "/").last ?? "") ?? 0
        guard var channel = channels().first(where: { $0.id == id }) else {
            return Self.fail(404, "missing", "That channel is not in the demo.")
        }
        if let fav = Self.boolField(body, "favorite") {
            lock.lock()
            favorites[id] = fav
            lock.unlock()
            channel.favorite = fav
        }
        if let hide = Self.boolField(body, "hidden") {
            lock.lock()
            hidden[id] = hide
            lock.unlock()
            channel.hidden = hide
        }
        return Self.ok(Self.json(channel))
    }

    private func media(_ path: String) -> Data {
        let bits = path.split(separator: "/").map(String.init)
        guard bits.count >= 3, let channel = Int(bits[1]) else {
            return Self.fail(404, "missing", "That recording is not in the demo.")
        }
        let name = bits[2]
        if name == "index.m3u8" {
            return Self.ok(Data(DemoFilm.playlist(channel: channel).utf8), type: "application/vnd.apple.mpegurl")
        }
        let file = name.split(separator: "?").first.map(String.init) ?? name
        guard let root = mediaDirectory() else {
            return Self.fail(404, "missing", "The demo films are not on this device.")
        }
        let url = root.appendingPathComponent("\(channel)").appendingPathComponent(file)
        guard let data = try? Data(contentsOf: url) else {
            return Self.fail(404, "missing", "The demo films are not on this device.")
        }
        return Self.ok(data, type: "video/mp4")
    }

    private func art(_ path: String) -> Data {
        let parts = path.split(separator: "/").map(String.init)
        let raw = if parts.last == "frame", parts.count >= 2 {
            Int64(parts[parts.count - 2]) ?? 1
        } else {
            Int64(parts.last?.split(separator: "?").first ?? "") ?? 1
        }
        let channel = DemoFilm.channelID(for: raw)
        if let root = mediaDirectory() {
            let url = root.appendingPathComponent("\(channel)").appendingPathComponent("art.jpg")
            if let data = try? Data(contentsOf: url) {
                return Self.ok(data, type: "image/jpeg")
            }
        }
        return Self.fail(404, "missing", "That picture is not in the demo.")
    }

    private static func serverInfo() -> ServerInfo {
        ServerInfo(
            id: "demo",
            name: "Demo",
            version: "1.0.0",
            apiVersion: 1,
            encoder: "copy",
            tunerCount: 0,
            features: ["live", "dvr", "passes", "wholeHomeSync", "multiview"],
            minAppVersion: "1.0"
        )
    }

    static func nowMS() -> Double {
        Date().timeIntervalSince1970 * 1000
    }

    static func validRoom(_ room: String) -> Bool {
        room.count <= 64 && (room.hasPrefix("channel:") || room.hasPrefix("group:") || room.hasPrefix("multiview:"))
    }

    private static func encoder() -> JSONEncoder {
        let enc = JSONEncoder()
        enc.dateEncodingStrategy = .custom { date, encoder in
            var box = encoder.singleValueContainer()
            try box.encode(ISO8601DateFormatter.fractional.string(from: date))
        }
        return enc
    }

    private static func json(_ value: some Encodable) -> Data {
        (try? encoder().encode(value)) ?? Data("{}".utf8)
    }

    private static func ok(_ body: Data, type: String = "application/json") -> Data {
        let head = "HTTP/1.1 200 OK\r\nContent-Type: \(type)\r\nContent-Length: \(body.count)\r\nConnection: close\r\nCache-Control: no-store\r\n\r\n"
        var out = Data(head.utf8)
        out.append(body)
        return out
    }

    private static func fail(_ status: Int, _ code: String, _ message: String) -> Data {
        let body = json(["code": code, "message": message])
        let reason = status == 404 ? "Not Found" : "Conflict"
        let head = "HTTP/1.1 \(status) \(reason)\r\nContent-Type: application/json\r\nContent-Length: \(body.count)\r\nConnection: close\r\n\r\n"
        var out = Data(head.utf8)
        out.append(body)
        return out
    }

    private static func intField(_ body: Data, _ key: String) -> Int64? {
        guard let obj = try? JSONSerialization.jsonObject(with: body) as? [String: Any] else { return nil }
        return (obj[key] as? NSNumber)?.int64Value
    }

    private static func boolField(_ body: Data, _ key: String) -> Bool? {
        guard let obj = try? JSONSerialization.jsonObject(with: body) as? [String: Any] else { return nil }
        return (obj[key] as? NSNumber)?.boolValue
    }

    private static func intList(_ body: Data, _ key: String) -> [Int64] {
        guard let obj = try? JSONSerialization.jsonObject(with: body) as? [String: Any],
              let list = obj[key] as? [Any]
        else { return [] }
        return list.compactMap { ($0 as? NSNumber)?.int64Value }
    }
}

private struct DemoRoom {
    var name: String
    var channelId: Int64
    var members: Set<ObjectIdentifier> = []
    var version = 0
    var anchorServer: Double
    var anchorMedia: Double
    var rate: Double = 1

    init(name: String, channelId: Int64) {
        self.name = name
        self.channelId = channelId
        let now = DemoServer.nowMS()
        anchorServer = now
        anchorMedia = now - 10000
        version = 1
    }

    var grouped: Bool {
        name.hasPrefix("group:") || name.hasPrefix("multiview:")
    }

    func target(at now: Double) -> Double {
        anchorMedia + (now - anchorServer) * rate
    }

    func payload() -> [String: Any] {
        [
            "room": name,
            "channelId": NSNumber(value: channelId),
            "mode": grouped ? "group" : "follow",
            "anchorServer": anchorServer,
            "anchorMedia": anchorMedia,
            "rate": rate,
            "latency": "balanced",
            "version": version,
            "members": members.count,
        ]
    }
}

private struct DemoFilm {
    var id: Int64
    var number: String
    var title: String

    static let all: [DemoFilm] = [
        DemoFilm(id: 1, number: "4.1", title: "Big Buck Bunny"),
        DemoFilm(id: 2, number: "5.1", title: "Sintel"),
        DemoFilm(id: 3, number: "9.1", title: "Tears of Steel"),
        DemoFilm(id: 4, number: "11.1", title: "Elephants Dream"),
    ]

    static let blurb = "A Blender Foundation film, under CC BY."

    func channel() -> Channel {
        Channel(
            id: id, deviceId: "demo", guideNumber: number, guideName: title,
            displayNumber: number, displayName: title,
            videoCodec: "H264", audioCodec: "AAC",
            hd: true, favorite: true, enabled: true, hidden: false, present: true,
            artUrl: "demo", artWidth: 1280, artHeight: 720
        )
    }

    static func channelID(for raw: Int64) -> Int64 {
        if (1 ... 4).contains(raw) {
            return raw
        }
        let channel = raw / 1_000_000
        return (1 ... 4).contains(channel) ? channel : 1
    }

    static func airings(from: Date?, to: Date?) -> [Airing] {
        let now = Date()
        var start = now.addingTimeInterval(-3600)
        let step: TimeInterval = 1800
        start = Date(timeIntervalSince1970: floor(start.timeIntervalSince1970 / step) * step)
        let until = now.addingTimeInterval(48 * 3600)
        var out: [Airing] = []
        for film in all {
            var cursor = start
            while cursor < until {
                let end = cursor.addingTimeInterval(step)
                let afterFrom = from.map { end > $0 } ?? true
                let beforeTo = to.map { cursor < $0 } ?? true
                if afterFrom, beforeTo {
                    let slot = Int64(cursor.timeIntervalSince1970 / step)
                    out.append(Airing(
                        id: film.id * 1_000_000 + slot,
                        channelId: film.id,
                        title: film.title,
                        subtitle: "Open movie",
                        description: blurb,
                        imageUrl: "demo",
                        imageWidth: 1280,
                        imageHeight: 720,
                        guideSource: "demo",
                        guideNumber: film.number,
                        channelName: film.title,
                        start: cursor,
                        end: end
                    ))
                }
                cursor = end
            }
        }
        return out
    }

    /// A live window whose program date-time is wall clock. The 12-second loop
    /// restarts its timestamps, so a wrap is a discontinuity.
    static func playlist(channel _: Int) -> String {
        let seg = 4.0
        let now = Date().timeIntervalSince1970
        let endSeq = Int((now - 2) / seg)
        let startSeq = endSeq - 5
        var lines = [
            "#EXTM3U",
            "#EXT-X-VERSION:7",
            "#EXT-X-TARGETDURATION:4",
            "#EXT-X-INDEPENDENT-SEGMENTS",
            "#EXT-X-MEDIA-SEQUENCE:\(startSeq)",
            "#EXT-X-START:TIME-OFFSET=-10.0",
            "#EXT-X-MAP:URI=\"init.mp4\"",
        ]
        for seq in startSeq ... endSeq {
            if seq > startSeq, seq % 3 == 0 {
                lines.append("#EXT-X-DISCONTINUITY")
                lines.append("#EXT-X-MAP:URI=\"init.mp4\"")
            }
            let pdt = ISO8601DateFormatter.fractional.string(from: Date(timeIntervalSince1970: Double(seq) * seg))
            lines.append("#EXT-X-PROGRAM-DATE-TIME:\(pdt)")
            lines.append("#EXTINF:4.000,")
            lines.append("seg\(seq % 3).m4s?n=\(seq)")
        }
        return lines.joined(separator: "\n") + "\n"
    }
}

private final class DemoOnce: @unchecked Sendable {
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

private final class DemoSocket: @unchecked Sendable {
    let conn: NWConnection
    private weak var server: DemoServer?
    private var buffer = Data()
    private var upgraded = false
    private var closed = false

    init(conn: NWConnection, server: DemoServer) {
        self.conn = conn
        self.server = server
    }

    func start(on queue: DispatchQueue) {
        conn.stateUpdateHandler = { [weak self] state in
            if case .failed = state {
                self?.finish()
            }
            if case .cancelled = state {
                self?.finish()
            }
        }
        conn.start(queue: queue)
        read()
    }

    func cancel() {
        conn.cancel()
    }

    func send(_ text: String) {
        conn.send(content: DemoSocket.encode(text), completion: .contentProcessed { _ in })
    }

    private func read() {
        conn.receive(minimumIncompleteLength: 1, maximumLength: 65536) { [weak self] data, _, complete, error in
            guard let self else { return }
            if let data {
                buffer.append(data)
            }
            if buffer.count > 1_048_576 {
                finish()
                return
            }
            if upgraded {
                pumpFrames()
            } else if let request = takeHTTP() {
                if request.upgrade {
                    upgraded = true
                    let accept = DemoSocket.accept(request.key)
                    let head = "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: \(accept)\r\n\r\n"
                    conn.send(content: Data(head.utf8), completion: .contentProcessed { [weak self] _ in
                        self?.send(self?.server?.hello() ?? "")
                    })
                    pumpFrames()
                } else if let server {
                    let response = server.http(request.method, request.target, body: request.body)
                    conn.send(content: response, completion: .contentProcessed { [weak self] _ in
                        self?.conn.cancel()
                    })
                    return
                }
            }
            if error != nil || complete {
                finish()
                return
            }
            read()
        }
    }

    private func pumpFrames() {
        while let frame = DemoSocket.decode(&buffer) {
            switch frame {
            case let .text(text):
                server?.receive(text, from: self)
            case let .ping(payload):
                conn.send(content: DemoSocket.pong(payload), completion: .contentProcessed { _ in })
            case .close:
                finish()
                return
            }
        }
    }

    private func finish() {
        guard !closed else { return }
        closed = true
        server?.drop(self)
        conn.cancel()
    }

    private struct Request {
        var method: String
        var target: String
        var upgrade: Bool
        var key: String
        var body: Data
    }

    private func takeHTTP() -> Request? {
        guard let headerEnd = buffer.range(of: Data("\r\n\r\n".utf8)) else { return nil }
        let head = String(data: buffer[..<headerEnd.lowerBound], encoding: .utf8) ?? ""
        let lines = head.components(separatedBy: "\r\n")
        let request = lines.first?.split(separator: " ") ?? []
        guard request.count >= 2 else { return nil }
        var headers: [String: String] = [:]
        for line in lines.dropFirst() {
            guard let split = line.firstIndex(of: ":") else { continue }
            let name = line[..<split].trimmingCharacters(in: .whitespaces).lowercased()
            let value = line[line.index(after: split)...].trimmingCharacters(in: .whitespaces)
            headers[name] = value
        }
        let length = Int(headers["content-length"] ?? "") ?? 0
        let bodyStart = headerEnd.upperBound
        guard buffer.count >= bodyStart + length else { return nil }
        let body = buffer[bodyStart ..< (bodyStart + length)]
        buffer.removeSubrange(..<(bodyStart + length))
        let upgrade = (headers["upgrade"] ?? "").lowercased() == "websocket"
        return Request(
            method: String(request[0]),
            target: String(request[1]),
            upgrade: upgrade,
            key: headers["sec-websocket-key"] ?? "",
            body: Data(body)
        )
    }

    private enum Frame {
        case text(String)
        case ping(Data)
        case close
    }

    private static func decode(_ data: inout Data) -> Frame? {
        guard data.count >= 2 else { return nil }
        let opcode = data[0] & 0x0F
        let masked = (data[1] & 0x80) != 0
        var len = Int(data[1] & 0x7F)
        var offset = 2
        if len == 126 {
            guard data.count >= 4 else { return nil }
            len = (Int(data[2]) << 8) | Int(data[3])
            offset = 4
        } else if len == 127 {
            guard data.count >= 10 else { return nil }
            len = 0
            for i in 0 ..< 8 {
                len = (len << 8) | Int(data[2 + i])
            }
            offset = 10
        }
        let maskLen = masked ? 4 : 0
        guard data.count >= offset + maskLen + len else { return nil }
        var payload = Data(data[(offset + maskLen) ..< (offset + maskLen + len)])
        if masked {
            for i in 0 ..< payload.count {
                payload[i] ^= data[offset + (i % 4)]
            }
        }
        data.removeSubrange(..<(offset + maskLen + len))
        switch opcode {
        case 0x1:
            return .text(String(data: payload, encoding: .utf8) ?? "")
        case 0x8:
            return .close
        case 0x9:
            return .ping(payload)
        default:
            return .text("")
        }
    }

    private static func encode(_ text: String) -> Data {
        let payload = Data(text.utf8)
        var frame = Data([0x81])
        if payload.count < 126 {
            frame.append(UInt8(payload.count))
        } else {
            frame.append(126)
            frame.append(UInt8((payload.count >> 8) & 0xFF))
            frame.append(UInt8(payload.count & 0xFF))
        }
        frame.append(payload)
        return frame
    }

    private static func pong(_ payload: Data) -> Data {
        var frame = Data([0x8A, UInt8(payload.count)])
        frame.append(payload)
        return frame
    }

    private static func accept(_ key: String) -> String {
        let magic = key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
        let digest = Insecure.SHA1.hash(data: Data(magic.utf8))
        return Data(digest).base64EncodedString()
    }
}
