import Foundation

/// What the player says when live TV cannot keep going, and when a stall is
/// long enough to ask. A short stall is the player's to ride out. The words
/// match the web player.
public enum PlaybackOutage {
    public static let serverStopped = "The server stopped. It will try again when it's back."
    public static let connectionDropped = "The connection dropped. It will try again when it's back."
    public static let tunerStopped = "This tuner did not answer. Check that it is on."
    public static let noSignal = "This channel isn't coming in. Check the antenna."
    public static let pictureStopped = "The picture stopped. Trying again usually fixes it."
    public static let tunersBusy = "Every tuner is busy. Stop a recording or watch something already on."
    /// A stall is named only after this long, and only when something is actually wrong.
    public static let stallSeconds: TimeInterval = 8

    public enum Recovery: Equatable, Sendable {
        case busy
        case server
        case tuner
        case signal
    }

    /// A closed port is the server. These codes are the phone itself.
    public static func deviceOnline(_ error: URLError) -> Bool {
        switch error.code {
        case .notConnectedToInternet, .dataNotAllowed, .internationalRoamingOff:
            false
        default:
            true
        }
    }

    public static func unreachable(online: Bool) -> OutageDecision {
        classify(RecoverySnap(health: false, freeTuner: false, tunerAnswers: false, online: online, signalLost: false))
    }

    public static func viewerFailure(code: String, status: Int, message: String, online: Bool) -> OutageDecision {
        if code == "tuners_busy" {
            let text = message.isEmpty ? tunersBusy : message
            return OutageDecision(message: text, recovery: .busy)
        }
        let lowered = message.lowercased()
        let network = status == 0
            || lowered.contains("failed to fetch")
            || lowered.contains("networkerror")
            || lowered.contains("load failed")
        if network {
            return unreachable(online: online)
        }
        if lowered.contains("did not answer") {
            return OutageDecision(message: tunerStopped, recovery: .tuner)
        }
        let text = message.isEmpty ? "This channel did not start." : message
        return OutageDecision(message: text, recovery: nil)
    }

    public static func classify(_ snap: RecoverySnap) -> OutageDecision {
        if !snap.health {
            if !snap.online {
                return OutageDecision(message: connectionDropped, recovery: .server)
            }
            return OutageDecision(message: serverStopped, recovery: .server)
        }
        if !snap.tunerAnswers {
            return OutageDecision(message: tunerStopped, recovery: .tuner)
        }
        if snap.signalLost {
            return OutageDecision(message: noSignal, recovery: .signal)
        }
        return OutageDecision(message: pictureStopped, recovery: nil)
    }

    public static func recoveryReady(_ kind: Recovery, _ snap: RecoverySnap) -> Bool {
        switch kind {
        case .server:
            snap.health && snap.online
        case .busy:
            snap.freeTuner
        case .tuner:
            snap.tunerAnswers
        case .signal:
            snap.health && !snap.signalLost
        }
    }

    public static func aTunerIsFree(_ tuners: [Tuner]) -> Bool {
        tuners.contains { tuner in
            (tuner.target ?? "").isEmpty && (tuner.guide ?? "").isEmpty
        }
    }

    /// A nil list means that read failed. A failed read does not count as fixed.
    /// `assumeLost` applies only when the signal read itself failed, so a blip
    /// does not look like the antenna came back.
    public static func snap(_ facts: RecoveryFacts, lists: RecoveryLists = RecoveryLists()) -> RecoverySnap {
        if !facts.health {
            return RecoverySnap(health: false, freeTuner: false, tunerAnswers: false, online: facts.online, signalLost: false)
        }
        let freeTuner = lists.tuners.map { aTunerIsFree($0) } ?? false
        let tunerAnswers = lists.devices?.contains { ($0.error ?? "").isEmpty } ?? false
        var signalLost = facts.assumeLost
        if let signals = lists.signals {
            if let row = signals.first(where: { $0.channelId == facts.channelID }) {
                signalLost = row.live == true && row.verdict == "Lost"
            } else {
                signalLost = false
            }
        }
        return RecoverySnap(
            health: true,
            freeTuner: freeTuner,
            tunerAnswers: tunerAnswers,
            online: facts.online,
            signalLost: signalLost
        )
    }
}

/// The flags around one recovery read. The lists are separate so a failed
/// fetch stays nil instead of looking like an empty, healthy home.
public struct RecoveryFacts: Equatable, Sendable {
    public var health: Bool
    public var online: Bool
    public var channelID: Int64
    public var assumeLost: Bool

    public init(health: Bool, online: Bool, channelID: Int64, assumeLost: Bool) {
        self.health = health
        self.online = online
        self.channelID = channelID
        self.assumeLost = assumeLost
    }
}

public struct RecoveryLists: Equatable, Sendable {
    public var tuners: [Tuner]?
    public var devices: [DeviceHealth]?
    public var signals: [ChannelSignal]?

    public init(tuners: [Tuner]? = nil, devices: [DeviceHealth]? = nil, signals: [ChannelSignal]? = nil) {
        self.tuners = tuners
        self.devices = devices
        self.signals = signals
    }
}

public struct OutageDecision: Equatable, Sendable {
    public var message: String
    public var recovery: PlaybackOutage.Recovery?

    public init(message: String, recovery: PlaybackOutage.Recovery?) {
        self.message = message
        self.recovery = recovery
    }
}

/// One look at the server, the tuners, and the channel, for the outage clock.
public struct RecoverySnap: Equatable, Sendable {
    public var health: Bool
    public var freeTuner: Bool
    public var tunerAnswers: Bool
    public var online: Bool
    public var signalLost: Bool

    public init(health: Bool, freeTuner: Bool, tunerAnswers: Bool, online: Bool, signalLost: Bool) {
        self.health = health
        self.freeTuner = freeTuner
        self.tunerAnswers = tunerAnswers
        self.online = online
        self.signalLost = signalLost
    }
}

/// Clock for one playback. A stall starts when the picture stops advancing.
/// Eight seconds later the caller asks whether anything is actually wrong.
public struct ServerOutage: Equatable, Sendable {
    public enum Phase: Equatable, Sendable {
        case playing
        case waiting(since: Date)
        case surfaced
    }

    public private(set) var phase: Phase = .playing

    public init() {}

    public mutating func notePlaying() {
        if case .surfaced = phase {
            return
        }
        phase = .playing
    }

    /// The first stalled sample starts the clock. Later ones leave it alone.
    public mutating func noteWaiting(at now: Date) {
        if case .playing = phase {
            phase = .waiting(since: now)
        }
    }

    public func shouldProbe(at now: Date, fatal: Bool) -> Bool {
        if case .surfaced = phase {
            return false
        }
        if fatal {
            return true
        }
        if case let .waiting(since) = phase {
            return now.timeIntervalSince(since) >= PlaybackOutage.stallSeconds
        }
        return false
    }

    /// A stall while the server, the tuners, and the signal are fine stays
    /// with the player, and the clock starts over. Anything else is named once.
    public mutating func resolve(at now: Date, snap: RecoverySnap, fatal: Bool) -> OutageDecision? {
        if case .surfaced = phase {
            return nil
        }
        let decision = PlaybackOutage.classify(snap)
        if fatal || decision.recovery != nil {
            phase = .surfaced
            return decision
        }
        phase = .waiting(since: now)
        return nil
    }

    /// A start that never reached the server. Once, until the clock is reset.
    public mutating func surface(_ message: String) -> String? {
        if case .surfaced = phase {
            return nil
        }
        phase = .surfaced
        return message
    }
}
