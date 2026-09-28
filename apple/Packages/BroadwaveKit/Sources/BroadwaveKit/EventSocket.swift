import Foundation
import os
#if canImport(UIKit)
    import UIKit
#endif

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
    private var screenName = ""
    private var screenKind = ""
    private var stopped = false
    private var opened = false
    private var boot = ""
    private var heard = 0
    private var retryWork: DispatchWorkItem?
    private var observers: [NSObjectProtocol] = []
    /// Set on wake until the next `sync.state`, so a return can be timed.
    private var awaitingState = false
    private static let debugLog = Logger(subsystem: "com.wolfeup.broadwave", category: "socket")
    /// First drop of the socket. The app looks for the same server at a new address.
    public var onFailure: (() -> Void)?
    /// Called when `connected` changes.
    public var onConnected: ((Bool) -> Void)?

    public init(base: URL) {
        var comps = URLComponents(url: base, resolvingAgainstBaseURL: false)!
        comps.scheme = base.scheme == "https" ? "wss" : "ws"
        comps.path = "/api/v1/ws"
        url = comps.url!
        var names: [Notification.Name] = [.NSSystemClockDidChange]
        #if canImport(UIKit)
            names += [UIApplication.didBecomeActiveNotification, UIApplication.significantTimeChangeNotification]
        #endif
        for name in names {
            observers.append(NotificationCenter.default.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated { self?.wake() }
            })
        }
    }

    /// Backoff with jitter, so a restarted server is not hit by every screen in
    /// the same instant. Capped at 5 s so a screen is back within 5 s of the server.
    public nonisolated static func reconnectWait(_ retry: Int, random: Double = .random(in: 0 ... 1)) -> Double {
        let ceiling = min(5.0, 0.5 * pow(2, Double(min(retry, 16))))
        return ceiling / 2 + random * ceiling / 2
    }

    public static func nowMS() -> Double {
        Date().timeIntervalSince1970 * 1000
    }

    public func serverNow() -> Double {
        Self.nowMS() + offset
    }

    public func connect() {
        guard !stopped, task == nil else { return }
        retryWork?.cancel()
        retryWork = nil
        let task = URLSession.shared.webSocketTask(with: url)
        self.task = task
        task.resume()
        receive(task)
        burst()
        clockTimer?.invalidate()
        clockTimer = Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.sampleClock() }
        }
        // The server starts a room by the kind of screen that joins it, so the
        // screen says what it is first.
        if !screenName.isEmpty, !screenKind.isEmpty {
            send("here", ["name": screenName, "kind": screenKind])
        }
        for (room, channel) in rooms {
            send("sync.join", ["room": room, "channelId": channel])
        }
    }

    /// Tells the server which screen this app is. Sent again after each reconnect.
    public func announce(name: String, kind: String) {
        screenName = name
        screenKind = kind
        guard !name.isEmpty, !kind.isEmpty else { return }
        send("here", ["name": name, "kind": kind])
    }

    /// After a sleep, a return to the foreground, or a clock change: reconnect
    /// now and measure the clock again. A socket that stayed quiet across a
    /// sleep can look open and be dead, so one that does not answer is replaced.
    public func wake() {
        guard !stopped else { return }
        debugLine("socket wake")
        awaitingState = true
        guard let task else {
            retry = 0
            connect()
            return
        }
        // Already open. A later "socket open" means this one was replaced.
        if connected {
            debugLine("socket open")
        }
        burst()
        let before = heard
        DispatchQueue.main.asyncAfter(deadline: .now() + 4) { [weak self] in
            guard let self, self.task === task, heard == before else { return }
            debugLine("socket quiet")
            task.cancel(with: .goingAway, reason: nil)
            self.task = nil
            setConnected(false)
            retry = 0
            connect()
        }
    }

    public func disconnect() {
        stopped = true
        retryWork?.cancel()
        observers.forEach(NotificationCenter.default.removeObserver)
        observers = []
        clockTimer?.invalidate()
        task?.cancel(with: .goingAway, reason: nil)
        task = nil
        connected = false
    }

    private func setConnected(_ value: Bool) {
        guard connected != value else { return }
        connected = value
        if !value {
            debugLine("socket down")
        }
        onConnected?(value)
    }

    /// `-BroadwaveSyncLog 1` only. The epoch ms is the clock a soak lines up.
    private func debugLine(_ name: String) {
        guard UserDefaults.standard.bool(forKey: "BroadwaveSyncLog") else { return }
        let line = "broadwave \(name) \(Int(Date().timeIntervalSince1970 * 1000))"
        print(line)
        fflush(stdout)
        Self.debugLog.notice("\(line, privacy: .public)")
    }

    private func burst() {
        bestRTT = .infinity
        for i in 0 ..< 5 {
            DispatchQueue.main.asyncAfter(deadline: .now() + .milliseconds(200 + i * 300)) { [weak self] in self?.sampleClock() }
        }
    }

    private func emit(_ type: String) {
        handlers[type]?.values.forEach { $0(Data()) }
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

    /// A screen's playback health for the server log.
    public func report(_ fields: [String: Any]) {
        send("sync.report", fields)
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
                    self.heard += 1
                    self.retry = 0
                    if !self.connected {
                        self.setConnected(true)
                        self.debugLine("socket open")
                        // Events sent while this screen was away are gone.
                        if self.opened {
                            self.emit("reconnected")
                        }
                        self.opened = true
                    }
                    if case let .string(text) = message, let data = text.data(using: .utf8) {
                        self.dispatch(data)
                    }
                    self.receive(task)
                case .failure:
                    self.task = nil
                    self.setConnected(false)
                    guard !self.stopped else { return }
                    self.onFailure?()
                    let wait = Self.reconnectWait(self.retry)
                    self.retry += 1
                    let work = DispatchWorkItem { [weak self] in
                        guard self?.stopped == false else { return }
                        self?.connect()
                    }
                    self.retryWork = work
                    DispatchQueue.main.asyncAfter(deadline: .now() + wait, execute: work)
                }
            }
        }
    }

    private func dispatch(_ data: Data) {
        guard let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any], let type = obj["type"] as? String else { return }
        let payload = obj["data"].flatMap { try? JSONSerialization.data(withJSONObject: $0, options: [.fragmentsAllowed]) } ?? Data()
        if type == "sync.state", let d = obj["data"] as? [String: Any], let room = d["room"] as? String {
            latest[room] = payload
            if awaitingState {
                awaitingState = false
                debugLine("sync.state")
            }
        }
        if type == "hello", let d = obj["data"] as? [String: Any], let next = d["boot"] as? String, !next.isEmpty {
            // Every watch and room the old process had is gone.
            let restarted = !boot.isEmpty && next != boot
            boot = next
            if restarted {
                emit("restarted")
            }
        }
        if type == "clock", let d = obj["data"] as? [String: Any], let t0 = d["t0"] as? Double, let t1 = d["t1"] as? Double {
            let t2 = Self.nowMS()
            let rtt = t2 - t0
            // A negative or huge round trip means the local clock moved while the sample was out.
            if rtt >= 0, rtt < 10000, rtt <= bestRTT * 1.2 {
                bestRTT = min(rtt, bestRTT)
                offset = t1 - (t0 + t2) / 2
            }
            bestRTT *= 1.01
        }
        handlers[type]?.values.forEach { $0(payload) }
    }
}
