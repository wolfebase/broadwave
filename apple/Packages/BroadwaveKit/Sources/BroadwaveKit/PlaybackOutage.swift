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
    public static let pictureRestarting = "The picture stopped. Starting it again."
    public static let tunersBusy = "Every tuner is busy. Stop a recording or watch something already on."
    public static let channelDidNotStart = "This channel did not start. Try again."
    /// A response that is not the error envelope. The same words as the web client.
    public static let requestFailed = "That did not work. Try again."

    /// What a failed button says: the server's sentence, or the generic one for
    /// anything else (a timeout, an unreachable server, a proxy page).
    public static func actionMessage(_ error: any Error) -> String {
        if let api = error as? APIError, !api.message.isEmpty, !unreadable(api.message) {
            return api.message
        }
        return requestFailed
    }

    /// A message the player can show. System and transport text is replaced.
    /// The server's own sentences pass through.
    public static func viewerMessage(_ message: String) -> String {
        let text = message.trimmingCharacters(in: .whitespacesAndNewlines)
        if text.isEmpty || unreadable(text) {
            return channelDidNotStart
        }
        return text
    }

    private static func unreadable(_ text: String) -> Bool {
        if text.range(of: #"^The server answered \d+"#, options: .regularExpression) != nil {
            return true
        }
        let lowered = text.lowercased()
        if ["internal server error", "bad gateway", "service unavailable", "gateway timeout"].contains(lowered) {
            return true
        }
        if lowered.contains("unexpected token") || lowered.contains("is not valid json") {
            return true
        }
        if lowered.contains("couldn't be read") || lowered.contains("couldn’t be read") {
            return true
        }
        if lowered.contains("couldn't be completed") || lowered.contains("couldn’t be completed") {
            return true
        }
        if lowered.contains("nsurlerror") {
            return true
        }
        return false
    }

    /// A stall is named only after this long, and only when something is actually wrong.
    public static let stallSeconds: TimeInterval = 8
    /// A stopped picture with no named cause has nothing to wait for. The player
    /// starts a new watch on this clock instead. The last one starts at two minutes.
    public static let pictureRetryEvery: TimeInterval = 10
    public static let pictureRetryFor: TimeInterval = 2 * 60

    public enum Recovery: Equatable, Sendable {
        case busy
        case server
        case tuner
        case signal
        /// The server no longer has this picture (it restarted, or let it go).
        case restart
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
        // Tried once and not coming in. Try again is the viewer's call.
        if code == "no_signal" {
            return OutageDecision(message: noSignal, recovery: nil)
        }
        // A source that stopped sending, like a playlist's upstream. The quiet
        // clock asks again, since nothing the player can read will change.
        if code == "stream_down" {
            return OutageDecision(message: pictureStopped, recovery: nil)
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
        return OutageDecision(message: viewerMessage(message), recovery: nil)
    }

    /// How many times to ask before the message stays. A tuner that missed its
    /// first answer, or refused the channel, gets one more try. A full picture
    /// budget often still holds the layout just left, and those encodes free
    /// one at a time. A page that left before its watch answered holds one
    /// until the server sees nobody fetching it (15 s), so ask for 20 s.
    public static func startAttempts(code: String, message: String) -> Int {
        if code == "pictures_full" {
            return 10
        }
        if code == "tuners_busy" || code == "no_signal" {
            return 1
        }
        if code == "tuner_refused" || message.localizedStandardContains("tuner") {
            return 2
        }
        return 1
    }

    /// Delay until the next quiet watch. Nil when this outage is not on that
    /// clock, or the two minutes are over.
    public static func pictureRetryDelay(message: String, recovery: Recovery?, elapsed: TimeInterval) -> TimeInterval? {
        guard message == pictureStopped, recovery == nil else { return nil }
        guard elapsed.isFinite, elapsed >= 0 else { return nil }
        let everyMs = Int(pictureRetryEvery * 1000)
        let forMs = Int(pictureRetryFor * 1000)
        let elapsedMs = Int((elapsed * 1000).rounded())
        guard elapsedMs < forMs else { return nil }
        let into = elapsedMs % everyMs
        let wait = into == 0 ? everyMs : everyMs - into
        if elapsedMs + wait > forMs {
            return nil
        }
        return TimeInterval(wait) / 1000
    }

    /// A quiet retry that fails without a named cause keeps the picture message.
    /// No signal stays the viewer's call, including a tune that never locked.
    public static func holdPictureMessage(_ decision: OutageDecision) -> Bool {
        decision.recovery == nil && decision.message != noSignal
    }

    public static func classify(_ snap: RecoverySnap) -> OutageDecision {
        if !snap.health {
            if !snap.online {
                return OutageDecision(message: connectionDropped, recovery: .server)
            }
            return OutageDecision(message: serverStopped, recovery: .server)
        }
        if snap.watchGone {
            return OutageDecision(message: pictureRestarting, recovery: .restart)
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
        case .restart:
            snap.health && snap.online
        }
    }

    public static func aTunerIsFree(_ tuners: [Tuner]) -> Bool {
        tuners.contains { tuner in
            (tuner.target ?? "").isEmpty && (tuner.guide ?? "").isEmpty
        }
    }

    /// A home with no HDHomeRun (only playlists) has no tuner to blame.
    public static func aTunerAnswers(_ devices: [DeviceHealth]) -> Bool {
        devices.isEmpty || devices.contains { ($0.error ?? "").isEmpty }
    }

    /// A nil list means that read failed. A failed read does not count as fixed.
    /// `assumeLost` applies only when the signal read itself failed, so a blip
    /// does not look like the antenna came back.
    public static func snap(_ facts: RecoveryFacts, lists: RecoveryLists = RecoveryLists()) -> RecoverySnap {
        if !facts.health {
            return RecoverySnap(health: false, freeTuner: false, tunerAnswers: false, online: facts.online, signalLost: false)
        }
        let freeTuner = lists.tuners.map { aTunerIsFree($0) } ?? false
        let tunerAnswers = lists.devices.map { aTunerAnswers($0) } ?? false
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
            signalLost: signalLost,
            watchGone: lists.playlistFound == false
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
    /// Whether the server still serves this player's playlist. Nil when unread.
    public var playlistFound: Bool?

    public init(
        tuners: [Tuner]? = nil,
        devices: [DeviceHealth]? = nil,
        signals: [ChannelSignal]? = nil,
        playlistFound: Bool? = nil
    ) {
        self.tuners = tuners
        self.devices = devices
        self.signals = signals
        self.playlistFound = playlistFound
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
    public var watchGone: Bool

    public init(health: Bool, freeTuner: Bool, tunerAnswers: Bool, online: Bool, signalLost: Bool, watchGone: Bool = false) {
        self.health = health
        self.freeTuner = freeTuner
        self.tunerAnswers = tunerAnswers
        self.online = online
        self.signalLost = signalLost
        self.watchGone = watchGone
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
    /// Probes that found a stall with nothing named. One rides out a sync hold.
    private var fineStalls = 0

    public init() {}

    public mutating func notePlaying() {
        if case .surfaced = phase {
            return
        }
        fineStalls = 0
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
    /// `afterPicture` is a stall that follows a moving picture. The second one
    /// is named: a sync hold ends before that, and the picture has stopped.
    public mutating func resolve(at now: Date, snap: RecoverySnap, fatal: Bool, afterPicture: Bool = false) -> OutageDecision? {
        if case .surfaced = phase {
            return nil
        }
        let decision = PlaybackOutage.classify(snap)
        if fatal || decision.recovery != nil {
            phase = .surfaced
            return decision
        }
        fineStalls += 1
        if afterPicture, fineStalls >= 2 {
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

/// A picture that should be moving and is not. AVPlayer can stop on a live
/// item with seconds buffered and never say why: it pauses itself, or stays
/// on "playing" while the playhead stands still. Eight seconds of a still
/// playhead, or one second of a pause AVPlayer made itself (that item does
/// not play again), reloads the item at the live edge. The next step starts
/// a new watch, and they alternate until the picture has played 20 s or the
/// cap. A reload can play the few seconds the server still lists, so a short
/// run of picture does not start the count over. A fresh item pauses itself
/// while it starts, so that second applies only once it has moved. An item
/// AVPlayer ended (failedToPlayToEndTime) is a step at once, moved or not.
public struct FrozenPicture: Equatable, Sendable {
    public enum Step: Equatable, Sendable {
        case reload
        case retune
    }

    public static let seconds: TimeInterval = 8
    public static let stoppedSeconds: TimeInterval = 1
    static let settledSeconds: TimeInterval = 20
    /// After this many steps with no settled picture, the outage clock has the last word.
    static let maxSteps = 6
    /// One sample is about a second apart. A larger step is a seek or a new
    /// item, not the picture playing.
    static let largestStep: Double = 3

    private var lastTime: Double?
    private var since: Date?
    private var movingSince: Date?
    private var tries = 0
    private var moved = false
    private var movedSinceStep = true
    /// True from the first step until the picture moves again.
    public private(set) var reconnecting = false

    public init() {}

    /// `time` is the item's playhead, nil with no item. `playing` is false
    /// while the viewer or the sync engine holds the picture. `stoppedItself`
    /// is a pause AVPlayer made on its own.
    public mutating func note(time: Double?, playing: Bool, stoppedItself: Bool = false, ended: Bool = false, at now: Date) -> Step? {
        if ended {
            lastTime = nil
            return nextStep(at: now)
        }
        guard let time, time.isFinite, playing || stoppedItself else {
            lastTime = nil
            since = nil
            movingSince = nil
            return nil
        }
        defer { lastTime = time }
        guard let last = lastTime else {
            since = now
            return nil
        }
        let step = time - last
        if step > 0.04, step < Self.largestStep {
            moved = true
            movedSinceStep = true
            reconnecting = false
            since = now
            let from = movingSince ?? now
            movingSince = from
            if now.timeIntervalSince(from) >= Self.settledSeconds {
                tries = 0
            }
            return nil
        }
        movingSince = nil
        if abs(step) >= Self.largestStep {
            since = now
            return nil
        }
        let wait = stoppedItself && movedSinceStep ? Self.stoppedSeconds : Self.seconds
        guard moved, let start = since, now.timeIntervalSince(start) >= wait else { return nil }
        return nextStep(at: now)
    }

    private mutating func nextStep(at now: Date) -> Step? {
        guard tries < Self.maxSteps else {
            reconnecting = false
            return nil
        }
        since = now
        tries += 1
        movedSinceStep = false
        reconnecting = true
        return tries % 2 == 1 ? .reload : .retune
    }
}
