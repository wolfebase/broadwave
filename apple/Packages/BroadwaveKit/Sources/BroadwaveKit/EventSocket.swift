import Foundation

/// The server's event socket: live updates, clock sync, and Whole-Home Sync rooms.
@MainActor
public final class EventSocket {
    public private(set) var connected = false
    /// Server clock minus local clock, in milliseconds.
    public private(set) var offset: Double = 0

    private let url: URL
    private var task: URLSessionWebSocketTask?
    private var handlers: [String: [UUID: (Data) -> Void]] = [:]
    private var rooms: [String: Int64] = [:]
    private var refs: [String: Int] = [:]
    private var latest: [String: Data] = [:]
    private var bestRTT = Double.infinity
    private var retry = 0
    private var clockTimer: Timer?

    public init(base: URL) {
        var comps = URLComponents(url: base, resolvingAgainstBaseURL: false)!
        comps.scheme = base.scheme == "https" ? "wss" : "ws"
        comps.path = "/api/v1/ws"
        url = comps.url!
    }

    public static func nowMS() -> Double {
        Date().timeIntervalSince1970 * 1000
    }

    public func serverNow() -> Double {
        Self.nowMS() + offset
    }

    public func connect() {
        guard task == nil else { return }
        let task = URLSession.shared.webSocketTask(with: url)
        self.task = task
        task.resume()
        receive(task)
        bestRTT = .infinity
        for i in 0 ..< 5 {
            DispatchQueue.main.asyncAfter(deadline: .now() + .milliseconds(200 + i * 300)) { [weak self] in self?.sampleClock() }
        }
        clockTimer?.invalidate()
        clockTimer = Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.sampleClock() }
        }
        for (room, channel) in rooms {
            send("sync.join", ["room": room, "channelId": channel])
        }
    }

    public func disconnect() {
        clockTimer?.invalidate()
        task?.cancel(with: .goingAway, reason: nil)
        task = nil
        connected = false
    }

    @discardableResult
    public func on(_ type: String, _ handler: @escaping (Data) -> Void) -> UUID {
        let id = UUID()
        handlers[type, default: [:]][id] = handler
        return id
    }

    public func off(_ type: String, _ id: UUID) {
        handlers[type]?[id] = nil
    }

    public func join(room: String, channelID: Int64) {
        let n = (refs[room] ?? 0) + 1
        refs[room] = n
        rooms[room] = channelID
        if n == 1 {
            send("sync.join", ["room": room, "channelId": channelID])
        }
    }

    public func leave(room: String) {
        let n = (refs[room] ?? 0) - 1
        if n > 0 {
            refs[room] = n
            return
        }
        refs[room] = nil
        latest[room] = nil
        guard rooms.removeValue(forKey: room) != nil else { return }
        send("sync.leave", ["room": room])
    }

    /// How many tiles in this process are in the room.
    public func membership(of room: String) -> Int {
        refs[room] ?? 0
    }

    /// Last `sync.state` for the room. A second tile joins without a new push, so it reads this.
    public func roomState(_ room: String) -> Data? {
        latest[room]
    }

    public func command(room: String, action: String, mediaTime: Double? = nil) {
        var body: [String: Any] = ["room": room, "action": action]
        if let mediaTime {
            body["mediaTime"] = mediaTime
        }
        send("sync.command", body)
    }

    private func sampleClock() {
        send("clock", ["t0": Self.nowMS()])
    }

    private func send(_ type: String, _ data: [String: Any]) {
        guard let task else { return }
        let frame: [String: Any] = ["type": type, "data": data]
        guard let json = try? JSONSerialization.data(withJSONObject: frame), let text = String(data: json, encoding: .utf8) else { return }
        task.send(.string(text)) { _ in }
    }

    private func receive(_ task: URLSessionWebSocketTask) {
        task.receive { [weak self] result in
            Task { @MainActor in
                guard let self, self.task === task else { return }
                switch result {
                case let .success(message):
                    self.connected = true
                    self.retry = 0
                    if case let .string(text) = message, let data = text.data(using: .utf8) {
                        self.dispatch(data)
                    }
                    self.receive(task)
                case .failure:
                    self.connected = false
                    self.task = nil
                    let wait = min(15.0, 0.5 * pow(2, Double(self.retry)))
                    self.retry += 1
                    DispatchQueue.main.asyncAfter(deadline: .now() + wait) { [weak self] in self?.connect() }
                }
            }
        }
    }

    private func dispatch(_ data: Data) {
        guard let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any], let type = obj["type"] as? String else { return }
        let payload = obj["data"].flatMap { try? JSONSerialization.data(withJSONObject: $0, options: [.fragmentsAllowed]) } ?? Data()
        if type == "sync.state", let d = obj["data"] as? [String: Any], let room = d["room"] as? String {
            latest[room] = payload
        }
        if type == "clock", let d = obj["data"] as? [String: Any], let t0 = d["t0"] as? Double, let t1 = d["t1"] as? Double {
            let t2 = Self.nowMS()
            let rtt = t2 - t0
            if rtt <= bestRTT * 1.2 {
                bestRTT = min(rtt, bestRTT)
                offset = t1 - (t0 + t2) / 2
            }
            bestRTT *= 1.01
        }
        handlers[type]?.values.forEach { $0(payload) }
    }
}
