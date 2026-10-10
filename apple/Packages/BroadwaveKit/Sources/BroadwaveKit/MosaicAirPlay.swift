import Foundation

/// Whether a multiview should keep playing out to AirPlay as one mosaic.
/// The player, the clock, and the audio route are passed in, so each case
/// can be tested without a device. A simulator has no AirPlay receiver.
public enum MosaicAirPlay {
    /// How long a route that already says AirPlay may take to claim the player.
    public static let routeWait: TimeInterval = 8

    public enum Phase: Equatable, Sendable {
        case idle
        case starting
        case playing
    }

    public struct State: Equatable, Sendable {
        public var generation: Int
        public var pickerOpen: Bool
        public var itemReady: Bool
        public var routed: Bool
        public var onDevice: Bool
        public var external: Bool
        public var channelIDs: [Int64]
        public var message: String
        public var paused: Bool
        public var phase: Phase

        public init(
            generation: Int = 0,
            pickerOpen: Bool = false,
            itemReady: Bool = false,
            routed: Bool = false,
            onDevice: Bool = false,
            external: Bool = false,
            channelIDs: [Int64] = [],
            message: String = "",
            paused: Bool = false,
            phase: Phase = .idle
        ) {
            self.generation = generation
            self.pickerOpen = pickerOpen
            self.itemReady = itemReady
            self.routed = routed
            self.onDevice = onDevice
            self.external = external
            self.channelIDs = channelIDs
            self.message = message
            self.paused = paused
            self.phase = phase
        }
    }

    public enum Step: Equatable, Sendable {
        case none
        /// Ask the server for a mosaic. Sound channel first.
        case begin(channelIDs: [Int64], onDevice: Bool, generation: Int)
        /// Drop the mosaic and hand sound back to the tiles.
        case end(generation: Int)
        /// AirPlay is the route, and the player is not on it yet.
        case wait(generation: Int, seconds: TimeInterval)
        case cancelWait
        case pause
        case play
    }

    /// Opening the route picker. The same channels already on AirPlay do not start again.
    public static func openPicker(_ state: State, channelIDs: [Int64]) -> (State, Step) {
        begin(state, channelIDs: channelIDs, onDevice: false, pickerOpen: true)
    }

    /// A start that did not come from the picker (the debug player on this device).
    public static func startDirect(_ state: State, channelIDs: [Int64], onDevice: Bool) -> (State, Step) {
        begin(state, channelIDs: channelIDs, onDevice: onDevice, pickerOpen: false)
    }

    /// The picker closed. `routeIsAirPlay` is the audio session's current route,
    /// so a cancelled pick leaves the tiles at once. A route that already says
    /// AirPlay still gets `routeWait` for the player to follow.
    public static func closePicker(_ state: State, routeIsAirPlay: Bool) -> (State, Step) {
        var next = state
        next.pickerOpen = false
        guard state.phase != .idle, !state.onDevice else { return (next, .none) }
        if routeIsAirPlay, state.external || state.routed {
            next.routed = true
            return (next, .cancelWait)
        }
        if routeIsAirPlay {
            return (next, .wait(generation: next.generation, seconds: routeWait))
        }
        return end(next)
    }

    /// The player's external-playback flag, including the value at subscribe time.
    /// A route that was already on has to be remembered, or turning it off never
    /// stops the mosaic and the server keeps the tunes.
    public static func noteExternal(_ state: State, active: Bool, generation: Int) -> (State, Step) {
        guard generation == state.generation else { return (state, .none) }
        var next = state
        next.external = active
        if active {
            next.routed = true
            return (next, .cancelWait)
        }
        guard state.routed, !state.onDevice else { return (next, .none) }
        next.routed = false
        return end(next)
    }

    /// The wait after the picker closed. Finding AirPlay already active remembers
    /// it. Returning without that leaves a later "off" ignored.
    public static func idleFired(_ state: State, generation: Int) -> (State, Step) {
        guard generation == state.generation, state.itemReady, !state.pickerOpen, !state.onDevice, state.phase != .idle else {
            return (state, .none)
        }
        if state.external {
            var next = state
            next.routed = true
            return (next, .cancelWait)
        }
        return end(state)
    }

    /// Tiles changed while a mosaic exists. A removed tile has to leave the
    /// mosaic or the server keeps its tune. The sound tile is first, so moving
    /// it changes what the television plays. A mosaic still starting, or one
    /// restarting, follows too: `begin` clears `routed`, and a change it missed
    /// would never come again.
    public static func channelsChanged(_ state: State, channelIDs: [Int64]) -> (State, Step) {
        guard state.phase != .idle else { return (state, .none) }
        if channelIDs.count < 2 {
            return end(state)
        }
        guard channelIDs != state.channelIDs else { return (state, .none) }
        return begin(state, channelIDs: channelIDs, onDevice: state.onDevice, pickerOpen: state.pickerOpen)
    }

    /// The mosaic item failed, or the server ended it (a restart, or a tune it was reading stopped).
    public static func playbackEnded(_ state: State, generation: Int) -> (State, Step) {
        guard generation == state.generation, state.phase != .idle else { return (state, .none) }
        return end(state)
    }

    /// The playlist is in the player. Wait for the route only once the picker is gone.
    public static func noteReady(_ state: State, generation: Int) -> (State, Step) {
        guard generation == state.generation else { return (state, .none) }
        var next = state
        next.itemReady = true
        next.phase = .playing
        guard !next.onDevice, !next.pickerOpen else { return (next, .none) }
        if next.external {
            next.routed = true
            return (next, .cancelWait)
        }
        return (next, .wait(generation: next.generation, seconds: routeWait))
    }

    public static func setPaused(_ state: State, paused: Bool) -> (State, Step) {
        var next = state
        next.paused = paused
        guard state.phase != .idle else { return (next, .none) }
        return (next, paused ? .pause : .play)
    }

    /// Stop began. The error text is cleared here.
    public static func beginStop(_ state: State) -> (State, Int) {
        var next = state
        next.generation += 1
        let generation = next.generation
        next.pickerOpen = false
        next.itemReady = false
        next.routed = false
        next.external = false
        next.onDevice = false
        next.channelIDs = []
        next.message = ""
        next.phase = .idle
        return (next, generation)
    }

    /// An `.end` decided for an older start does not stop a newer one.
    public static func isCurrent(_ state: State, generation: Int) -> Bool {
        state.generation == generation
    }

    /// A start that never got a playlist. The message stays until the next stop.
    public static func failStart(_ state: State, generation: Int, message: String) -> State {
        guard state.generation == generation else { return state }
        var next = state
        next.phase = .idle
        next.itemReady = false
        next.onDevice = false
        next.message = message
        return next
    }

    /// Unmute now only when this tile's current picture was started with sound.
    /// PiP and the small tiles of 1+3 start without it, on iPhone and Apple TV.
    /// Unmuting that item plays nothing, and the next watch is what carries sound.
    public static func unmuteNow(wantsSound: Bool, hasPicture: Bool, audio: Prefs.Sound) -> Bool {
        wantsSound && hasPicture && audio != .none
    }

    private static func begin(_ state: State, channelIDs: [Int64], onDevice: Bool, pickerOpen: Bool) -> (State, Step) {
        if state.routed, state.phase == .playing, state.channelIDs == channelIDs, pickerOpen {
            var next = state
            next.pickerOpen = true
            return (next, .none)
        }
        guard channelIDs.count >= 2 else { return (state, .none) }
        var next = state
        next.generation += 1
        next.pickerOpen = pickerOpen
        next.message = ""
        next.itemReady = false
        next.routed = false
        next.external = false
        next.onDevice = onDevice
        next.channelIDs = channelIDs
        next.phase = .starting
        return (next, .begin(channelIDs: channelIDs, onDevice: onDevice, generation: next.generation))
    }

    private static func end(_ state: State) -> (State, Step) {
        (state, .end(generation: state.generation))
    }
}
