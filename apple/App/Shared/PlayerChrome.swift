import AVKit
import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// What stands in for a picture that has not started: the channel's last frame,
/// fitted the way the picture will be, or the show's art blurred. Small art is
/// never stretched over the screen.
struct TuneBackdrop: View {
    let frame: URL?
    let art: URL?

    var body: some View {
        ZStack {
            Color.black
            if let frame {
                AsyncImage(url: frame) { phase in
                    if let image = phase.image {
                        image.resizable().scaledToFit()
                    }
                }
            } else if let art {
                AsyncImage(url: art) { phase in
                    if let image = phase.image {
                        image.resizable().scaledToFill()
                            .blur(radius: 28)
                            .opacity(0.75)
                    }
                }
            }
            Color.black.opacity(0.35)
        }
        .clipped()
        .accessibilityHidden(true)
        .allowsHitTesting(false)
    }
}

#if os(iOS)
    /// One panel over the live picture at a time.
    enum PlayerPanel: Equatable {
        case none, channels, stream
    }

    /// What the live chrome asks of the player screen.
    struct LiveChromeActions {
        var step: (Int) -> Void
        /// Nil when neither the picture nor a recording holds the show's start.
        var startOver: (() -> Void)?
        var audio: [ChannelMenuEntry]
        var delay: [ChannelMenuEntry]
    }

    /// The iPhone and iPad live player's controls, as the TV app draws them. A portrait
    /// iPhone keeps them around the picture. iPad and a landscape iPhone float them over
    /// the picture: the title top left, play in the middle, the timeline at the bottom.
    /// They hide after 3 s; a tap brings them back.
    struct LiveChrome: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        @Environment(\.verticalSizeClass) private var vertical
        @Environment(\.horizontalSizeClass) private var horizontal
        @Environment(\.accessibilityVoiceOverEnabled) private var voiceOver
        @Environment(\.accessibilityReduceMotion) private var reduceMotion
        @Binding var panel: PlayerPanel
        @Binding var fill: Bool
        @Binding var showMove: Bool
        let actions: LiveChromeActions
        let tuning: Bool
        @State private var shown = true
        @State private var touched = Date()
        @State private var pinchScale: CGFloat = 1

        init(panel: Binding<PlayerPanel>, fill: Binding<Bool>, showMove: Binding<Bool>, actions: LiveChromeActions, tuning: Bool) {
            _panel = panel
            _fill = fill
            _showMove = showMove
            self.actions = actions
            self.tuning = tuning
        }

        private var live: LivePlayer {
            nowPlaying.live
        }

        /// A portrait iPhone. Everything else floats over the picture.
        private var stacked: Bool {
            vertical == .regular && horizontal == .compact
        }

        private var keepShown: Bool {
            #if DEBUG
                if UserDefaults.standard.bool(forKey: "BroadwaveChromeStays") {
                    return true
                }
            #endif
            return stacked || voiceOver || tuning || panel != .none || live.paused || live.error != nil
        }

        private var visible: Bool {
            shown || keepShown
        }

        var body: some View {
            ZStack {
                gesturePad
                if stacked {
                    StackedLiveControls(panel: $panel, showMove: $showMove, actions: actions, tuning: tuning, touch: touch)
                } else if visible {
                    FloatingLiveControls(panel: $panel, showMove: $showMove, actions: actions, tuning: tuning, touch: touch)
                        .transition(.opacity)
                }
            }
            .tint(.white)
            // With a panel open, a swipe down puts the panel away, not the player.
            .interactiveDismissDisabled(panel != .none)
            .animation(reduceMotion ? nil : .easeInOut(duration: 0.25), value: visible)
            .statusBarHidden(!stacked && !visible)
            .persistentSystemOverlays(!stacked && !visible ? .hidden : .automatic)
            .task(id: "\(touched.timeIntervalSince1970) \(visible) \(keepShown)") { await hideLater() }
            .onChange(of: nowPlaying.channel?.id) { _, _ in touch() }
            .overlay { probe }
        }

        /// UI tests read whether the controls are up here.
        @ViewBuilder private var probe: some View {
            #if DEBUG
                if UserDefaults.standard.bool(forKey: "BroadwaveChannelNow") {
                    Text(visible ? "shown" : "hidden")
                        .font(.system(size: 2))
                        .foregroundStyle(.clear)
                        .allowsHitTesting(false)
                        .accessibilityIdentifier("chromeProbe")
                }
            #endif
        }

        private func touch() {
            touched = Date()
            shown = true
        }

        private func hideLater() async {
            guard visible, !keepShown else { return }
            try? await Task.sleep(for: .seconds(3))
            guard !Task.isCancelled, !keepShown else { return }
            shown = false
        }

        /// Tap shows or hides the controls and closes a panel. Double tap fills the
        /// screen. Swipe up or down changes channel, unless a panel is open: then it closes it.
        private var gesturePad: some View {
            Color.clear
                .contentShape(Rectangle())
                .ignoresSafeArea()
                .gesture(
                    TapGesture(count: 2).onEnded { toggleFill() }
                        .exclusively(before: TapGesture().onEnded { tapPicture() })
                )
                .simultaneousGesture(channelDrag)
                .simultaneousGesture(fillPinch)
                .accessibilityElement()
                .accessibilityLabel("Picture")
                .accessibilityValue(fill ? "Fill" : "Fit")
                .accessibilityHint("Double-tap to show or hide the controls")
                .accessibilityAction { tapPicture() }
                .accessibilityAction(named: fill ? "Fit to screen" : "Fill screen") { toggleFill() }
                .accessibilityIdentifier("player-gesture")
        }

        private func tapPicture() {
            if panel != .none {
                panel = .none
                touch()
            } else if visible, !keepShown {
                shown = false
            } else {
                touch()
            }
        }

        private func toggleFill() {
            fill.toggle()
            live.playLogNote(fill ? "fill" : "fit")
        }

        private var channelDrag: some Gesture {
            DragGesture(minimumDistance: 24)
                .onEnded { value in
                    let dx = value.translation.width
                    let dy = value.translation.height
                    guard abs(dy) > abs(dx), abs(dy) > 80 else { return }
                    // A swipe meant to put a panel away must not also change the channel.
                    if panel != .none {
                        panel = .none
                        live.playLogNote("panel closed by swipe")
                        return
                    }
                    actions.step(dy < 0 ? ChannelStep.offset(up: true) : ChannelStep.offset(up: false))
                }
        }

        private var fillPinch: some Gesture {
            MagnificationGesture()
                .onChanged { pinchScale = $0 }
                .onEnded { scale in
                    let amount = abs(pinchScale - 1) > abs(scale - 1) ? pinchScale : scale
                    pinchScale = 1
                    guard abs(amount - 1) > 0.08 else { return }
                    let wants = amount > 1
                    guard wants != fill else { return }
                    toggleFill()
                }
        }
    }

    // MARK: Layouts

    /// iPad and a landscape iPhone: everything floats on glass over the picture.
    private struct FloatingLiveControls: View {
        @Environment(NowPlaying.self) private var nowPlaying
        @Binding var panel: PlayerPanel
        @Binding var showMove: Bool
        let actions: LiveChromeActions
        let tuning: Bool
        let touch: () -> Void

        var body: some View {
            ZStack {
                scrims
                VStack(spacing: 0) {
                    HStack(alignment: .top, spacing: 12) {
                        MinimizeButton()
                        ProgramTitle(compact: true)
                        Spacer(minLength: 8)
                        TopControls(panel: $panel, showMove: $showMove, actions: actions, touch: touch)
                    }
                    Spacer(minLength: 0)
                    if !tuning {
                        Transport(actions: actions, size: .large, touch: touch)
                    }
                    Spacer(minLength: 0)
                    if !tuning {
                        LiveTimeline(touch: touch)
                            .padding(.bottom, 10)
                    }
                    HStack(spacing: 8) {
                        BottomActions(panel: $panel, actions: actions, compact: true, touch: touch)
                        Spacer(minLength: 8)
                        ChannelStepper(step: actions.step, across: true)
                    }
                }
                .padding(.horizontal, 20)
                .padding(.vertical, 12)
                .overlay(alignment: .trailing) {
                    if panel == .channels {
                        ChannelPanel(panel: $panel)
                            .frame(width: 340)
                            .padding(.vertical, 64)
                            .transition(.move(edge: .trailing).combined(with: .opacity))
                    }
                }
                .overlay(alignment: .bottomLeading) {
                    if panel == .stream {
                        StreamPanel(stream: nowPlaying.live.session?.stream, stats: nowPlaying.live.picture, sync: nowPlaying.live.sync)
                            .padding(.horizontal, 20)
                            .padding(.bottom, 120)
                    }
                }
            }
        }

        private var scrims: some View {
            VStack {
                LinearGradient(colors: [.black.opacity(0.65), .clear], startPoint: .top, endPoint: .bottom)
                    .frame(height: 170)
                Spacer()
                LinearGradient(colors: [.clear, .black.opacity(0.7)], startPoint: .top, endPoint: .bottom)
                    .frame(height: 220)
            }
            .ignoresSafeArea()
            .allowsHitTesting(false)
        }
    }

    /// A portrait iPhone: the channel and show above the picture, play and the timeline under it.
    private struct StackedLiveControls: View {
        @Environment(NowPlaying.self) private var nowPlaying
        @Binding var panel: PlayerPanel
        @Binding var showMove: Bool
        let actions: LiveChromeActions
        let tuning: Bool
        let touch: () -> Void

        var body: some View {
            VStack(alignment: .leading, spacing: 12) {
                HStack(spacing: 10) {
                    MinimizeButton()
                    Spacer(minLength: 4)
                    TopControls(panel: $panel, showMove: $showMove, actions: actions, touch: touch)
                }
                HStack(alignment: .top, spacing: 12) {
                    ProgramTitle(compact: false)
                    ChannelStepper(step: actions.step)
                }
                Spacer(minLength: 0)
                if panel == .channels {
                    ChannelPanel(panel: $panel)
                        .frame(maxHeight: 300)
                } else if panel == .stream {
                    StreamPanel(stream: nowPlaying.live.session?.stream, stats: nowPlaying.live.picture, sync: nowPlaying.live.sync)
                }
                if !tuning {
                    LiveTimeline(touch: touch)
                    Transport(actions: actions, size: .regular, touch: touch)
                        .frame(maxWidth: .infinity)
                }
                BottomActions(panel: $panel, actions: actions, compact: false, touch: touch)
            }
            .padding(.horizontal)
            .padding(.top, 8)
            .padding(.bottom, 12)
        }
    }

    // MARK: Pieces

    private struct MinimizeButton: View {
        @Environment(NowPlaying.self) private var nowPlaying

        var body: some View {
            Button("Minimize", systemImage: "chevron.down") { nowPlaying.expanded = false }
                .labelStyle(.iconOnly)
                .buttonStyle(GlassCircle())
                .accessibilityLabel("Minimize")
        }
    }

    /// The channel, the show, and how long it has left.
    private struct ProgramTitle: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        let compact: Bool

        var body: some View {
            VStack(alignment: .leading, spacing: 2) {
                if let channel = nowPlaying.channel {
                    let airing = store.index.on(channel.id, at: store.now)
                    Text(String(localized: "guide.channelName", defaultValue: "\(channel.displayNumber)  \(channel.displayName)"))
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(.white.opacity(0.86))
                    Text(airing?.title ?? String(localized: "No listing"))
                        .font(compact ? .headline : .title2.weight(.bold))
                        .lineLimit(compact ? 1 : 2)
                        .fixedSize(horizontal: false, vertical: true)
                    if let airing {
                        let left = airing.minutesLeft(at: store.now)
                        Text(left)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                            .contentTransition(.numericText(countsDown: true))
                            .animation(.snappy, value: left)
                    }
                }
            }
            .frame(maxWidth: compact ? 420 : .infinity, alignment: .leading)
            .shadow(color: .black.opacity(compact ? 0.5 : 0), radius: 6)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("portrait-program")
        }
    }

    private struct ChannelStepper: View {
        let step: (Int) -> Void
        var across = false

        var body: some View {
            GlassEffectContainer(spacing: 6) {
                let layout = across ? AnyLayout(HStackLayout(spacing: 6)) : AnyLayout(VStackLayout(spacing: 6))
                layout {
                    Button("Previous channel", systemImage: "chevron.up") { step(-1) }
                        .accessibilityIdentifier("portrait-previous")
                    Button("Next channel", systemImage: "chevron.down") { step(1) }
                        .accessibilityIdentifier("portrait-next")
                }
                .labelStyle(.iconOnly)
                .buttonStyle(GlassCircle())
            }
        }
    }

    /// Sync, AirPlay, subtitles, and More, as one glass group.
    private struct TopControls: View {
        @Environment(NowPlaying.self) private var nowPlaying
        @Binding var panel: PlayerPanel
        @Binding var showMove: Bool
        let actions: LiveChromeActions
        let touch: () -> Void

        var body: some View {
            HStack(spacing: 8) {
                SyncPill()
                GlassEffectContainer(spacing: 8) {
                    HStack(spacing: 8) {
                        AirPlayRoute(compact: true) {
                            nowPlaying.live.playLogNote("airplay open")
                        }
                        .frame(width: 44, height: 44)
                        if #available(iOS 26.4, *) {
                            SubtitlesMenu(player: nowPlaying.live.player)
                                .frame(width: 44, height: 44)
                                .accessibilityIdentifier("player-subtitles")
                        }
                        AudioMenu(actions: actions)
                        MoreMenu(panel: $panel, showMove: $showMove, actions: actions, touch: touch)
                    }
                }
            }
        }
    }

    private struct AudioMenu: View {
        let actions: LiveChromeActions

        var body: some View {
            Menu {
                ForEach(actions.audio) { entry in
                    Button(action: entry.action) {
                        if entry.current {
                            Label(entry.title, systemImage: "checkmark")
                        } else {
                            Text(entry.title)
                        }
                    }
                }
            } label: {
                Label("Audio", systemImage: "speaker.wave.2")
            }
            .labelStyle(.iconOnly)
            .menuStyle(.button)
            .buttonStyle(GlassCircle())
            .accessibilityLabel("Audio")
            .accessibilityIdentifier("portrait-audio")
        }
    }

    /// Stream details, Move to another screen, and the live delay.
    private struct MoreMenu: View {
        @Environment(NowPlaying.self) private var nowPlaying
        @Binding var panel: PlayerPanel
        @Binding var showMove: Bool
        let actions: LiveChromeActions
        let touch: () -> Void

        var body: some View {
            Menu {
                Button("Move to another screen", systemImage: "rectangle.portrait.and.arrow.forward") { showMove = true }
                    .accessibilityIdentifier("portrait-move")
                Button(panel == .stream ? "Hide stream details" : "Stream details", systemImage: "info.circle") {
                    panel = panel == .stream ? .none : .stream
                    touch()
                }
                .accessibilityIdentifier("portrait-stream")
                if !actions.delay.isEmpty {
                    Picker(selection: delayChoice) {
                        ForEach(actions.delay) { entry in
                            Text(entry.title).tag(entry.id)
                        }
                    } label: {
                        Label("Live delay", systemImage: "clock")
                    }
                    .pickerStyle(.menu)
                }
            } label: {
                Label("More", systemImage: "ellipsis")
            }
            .labelStyle(.iconOnly)
            .menuStyle(.button)
            .buttonStyle(GlassCircle())
            .accessibilityIdentifier("player-more")
        }

        private var delayChoice: Binding<Int64> {
            Binding(
                get: { actions.delay.first(where: \.current)?.id ?? 0 },
                set: { id in actions.delay.first { $0.id == id }?.action() }
            )
        }
    }

    /// Back 10, play or pause, and forward 10 while behind live.
    private struct Transport: View {
        enum Size {
            case regular, large
        }

        @Environment(NowPlaying.self) private var nowPlaying
        let actions: LiveChromeActions
        let size: Size
        let touch: () -> Void

        private var live: LivePlayer {
            nowPlaying.live
        }

        var body: some View {
            let window = live.window
            HStack(spacing: size == .large ? 56 : 36) {
                symbolButton("Back 10 seconds", "gobackward.10", side: 52, id: "player-back") {
                    if let window {
                        live.jump(to: window.moved(by: -10))
                    }
                }
                .disabled(window == nil)
                symbolButton(live.paused ? "Play" : "Pause", live.paused ? "play.fill" : "pause.fill", side: 72, id: "player-play") {
                    live.togglePause()
                }
                .font(.system(size: size == .large ? 44 : 34))
                .contentTransition(.symbolEffect(.replace))
                symbolButton("Forward 10 seconds", "goforward.10", side: 52, id: "player-forward") {
                    if let window {
                        live.jump(to: window.moved(by: 10))
                    }
                }
                .disabled(window?.isLive ?? true)
            }
            .font(.system(size: size == .large ? 30 : 24))
            .foregroundStyle(.white)
            .shadow(color: .black.opacity(0.4), radius: 8)
        }

        private func symbolButton(_ title: String, _ symbol: String, side: CGFloat, id: String, action: @escaping () -> Void) -> some View {
            Button {
                action()
                touch()
            } label: {
                Image(systemName: symbol)
                    .frame(width: side, height: side)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(title)
            .accessibilityIdentifier(id)
        }
    }

    /// The server's buffer from its oldest frame to live, with LIVE at the right end.
    private struct LiveTimeline: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        let touch: () -> Void
        @State private var drag: Double?

        private var live: LivePlayer {
            nowPlaying.live
        }

        var body: some View {
            let window = live.window
            HStack(spacing: 12) {
                Text(leftText(window))
                    .font(.footnote.monospacedDigit().weight(.semibold))
                    .foregroundStyle(.white.opacity(0.85))
                    .frame(minWidth: 52, alignment: .leading)
                track(window)
                LivePill(atLive: window?.isLive ?? true) {
                    live.goLive()
                    touch()
                }
            }
            .accessibilityElement(children: .contain)
        }

        private func leftText(_ window: LiveWindow?) -> String {
            guard let window else { return "" }
            if let drag {
                return LiveWindow.behindText(window.edge.timeIntervalSince(window.date(at: drag)))
            }
            return window.isLive ? window.position.formatted(date: .omitted, time: .shortened) : LiveWindow.behindText(window.behind)
        }

        private func track(_ window: LiveWindow?) -> some View {
            GeometryReader { geo in
                let done = drag ?? window?.fraction ?? 1
                ZStack(alignment: .leading) {
                    Capsule().fill(.white.opacity(0.28))
                    Capsule().fill(.white).frame(width: max(6, geo.size.width * done))
                    Circle().fill(.white)
                        .frame(width: drag == nil ? 0 : 16, height: drag == nil ? 0 : 16)
                        .offset(x: geo.size.width * done - 8)
                }
                .frame(height: drag == nil ? 6 : 10)
                .frame(maxHeight: .infinity)
                .contentShape(Rectangle())
                .gesture(scrub(width: geo.size.width, window: window))
            }
            .frame(height: 32)
            .animation(.snappy(duration: 0.2), value: drag == nil)
            .accessibilityElement()
            .accessibilityLabel("Timeline")
            .accessibilityValue(spoken(window))
            .accessibilityAdjustableAction { direction in
                guard let window else { return }
                live.jump(to: window.moved(by: direction == .increment ? 30 : -30))
                touch()
            }
            .accessibilityIdentifier("player-timeline")
        }

        private func spoken(_ window: LiveWindow?) -> String {
            guard let window, !window.isLive else { return String(localized: "Live") }
            let minutes = Int((window.behind / 60).rounded())
            return minutes < 1 ? String(localized: "Less than a minute behind live") : String(localized: "\(minutes) min behind live")
        }

        private func scrub(width: CGFloat, window: LiveWindow?) -> some Gesture {
            DragGesture(minimumDistance: 0)
                .onChanged { value in
                    guard window != nil, width > 0 else { return }
                    drag = min(1, max(0, value.location.x / width))
                    touch()
                }
                .onEnded { _ in
                    defer { drag = nil }
                    guard let window, let drag else { return }
                    let to = window.date(at: drag)
                    if window.edge.timeIntervalSince(to) <= LiveWindow.liveSlack {
                        live.goLive()
                    } else {
                        live.jump(to: to)
                    }
                    touch()
                }
        }
    }

    /// Red while the picture is live. Behind live it is the way back.
    private struct LivePill: View {
        let atLive: Bool
        let action: () -> Void

        var body: some View {
            Button(action: action) {
                HStack(spacing: 5) {
                    Circle()
                        .fill(atLive ? Tokens.ColorToken.tally : .white.opacity(0.6))
                        .frame(width: 7, height: 7)
                    Text("LIVE")
                        .font(.caption.weight(.heavy))
                        .tracking(0.5)
                }
                .padding(.horizontal, 10)
                .frame(minHeight: 32)
            }
            .buttonStyle(.glass)
            .foregroundStyle(.white)
            .allowsHitTesting(!atLive)
            .accessibilityLabel(atLive ? "Live" : "Back to live")
            .accessibilityIdentifier("player-live")
        }
    }

    /// Start over, the channel list, recording, and side by side.
    private struct BottomActions: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        @Binding var panel: PlayerPanel
        let actions: LiveChromeActions
        /// Floating over the picture: short capsules instead of a full-width row.
        let compact: Bool
        let touch: () -> Void

        var body: some View {
            GlassEffectContainer(spacing: 8) {
                HStack(spacing: 8) {
                    if let startOver = actions.startOver {
                        ChromeButton(title: "Start over", symbol: "backward.end", id: "player-start-over") {
                            startOver()
                            touch()
                        }
                    }
                    ChromeButton(title: "Channels", symbol: "list.bullet", id: "portrait-channels", spoken: panel == .channels ? "Close guide" : nil) {
                        panel = panel == .channels ? .none : .channels
                        touch()
                    }
                    .accessibilityAddTraits(panel == .channels ? .isSelected : [])
                    RecordButton()
                    ChromeButton(title: "Side by side", symbol: "rectangle.split.2x1", id: "portrait-together") {
                        if let channel = nowPlaying.channel {
                            nowPlaying.watchTogether([channel])
                        }
                    }
                }
                .environment(\.chromeCompact, compact)
            }
        }
    }

    private struct ChromeCompactKey: EnvironmentKey {
        static let defaultValue = false
    }

    extension EnvironmentValues {
        /// Bottom buttons as capsules with the name beside the symbol.
        var chromeCompact: Bool {
            get { self[ChromeCompactKey.self] }
            set { self[ChromeCompactKey.self] = newValue }
        }
    }

    /// A glass button with its name under the symbol, so it reads without VoiceOver.
    private struct ChromeButton: View {
        let title: String
        let symbol: String
        let id: String
        var spoken: String?
        let action: () -> Void

        var body: some View {
            Button(action: action) {
                Label(title, systemImage: symbol)
                    .labelStyle(StackedControlLabel())
            }
            .buttonStyle(.glass)
            .accessibilityLabel(spoken ?? title)
            .accessibilityIdentifier(id)
        }
    }

    private struct RecordButton: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying

        var body: some View {
            let recording = nowPlaying.channel.flatMap { store.activeRecording(on: $0) } != nil
            Button {
                if let channel = nowPlaying.channel {
                    Task { await store.toggleRecord(channel) }
                }
            } label: {
                Label(recording ? "Recording" : "Record", systemImage: recording ? "record.circle.fill" : "record.circle")
                    .labelStyle(StackedControlLabel())
            }
            .buttonStyle(.glass)
            .foregroundStyle(Tokens.ColorToken.tally)
            .contentTransition(.symbolEffect(.replace))
            .symbolEffect(.pulse, options: .repeat(.continuous), isActive: recording)
            .accessibilityLabel(recording ? "Stop recording" : "Record")
            .accessibilityIdentifier("portrait-record")
        }
    }

    /// A 44 pt glass circle around a symbol, the size of the system player's buttons.
    struct GlassCircle: ButtonStyle {
        func makeBody(configuration: Configuration) -> some View {
            configuration.label
                .font(.body.weight(.semibold))
                .frame(width: 44, height: 44)
                .contentShape(Circle())
                .glassEffect(.regular.interactive(), in: .circle)
                .opacity(configuration.isPressed ? 0.7 : 1)
        }
    }

    /// Icon over a short name. Never wraps mid-word; a long name shrinks instead.
    /// Floating over the picture it is a capsule with the name beside the icon.
    struct StackedControlLabel: LabelStyle {
        @Environment(\.chromeCompact) private var compact

        func makeBody(configuration: Configuration) -> some View {
            if compact {
                HStack(spacing: 6) {
                    configuration.icon.accessibilityHidden(true)
                    configuration.title
                        .font(.subheadline.weight(.semibold))
                        .lineLimit(1)
                }
                .frame(minHeight: 36)
                .padding(.horizontal, 4)
            } else {
                stacked(configuration)
            }
        }

        private func stacked(_ configuration: Configuration) -> some View {
            VStack(spacing: 4) {
                // The symbol's own name ("Volume High") would be read beside the control's name.
                configuration.icon.accessibilityHidden(true)
                configuration.title
                    .font(.caption2.weight(.semibold))
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
            }
            .frame(maxWidth: .infinity, minHeight: 44)
            .padding(.vertical, 4)
        }
    }

    /// The channel list over the picture. Picking one plays it and closes the list.
    private struct ChannelPanel: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        @Binding var panel: PlayerPanel

        var body: some View {
            VStack(spacing: 0) {
                HStack {
                    Text("Channels").font(.headline)
                    Spacer()
                    Button("Close guide", systemImage: "xmark") { panel = .none }
                        .labelStyle(.iconOnly)
                        .buttonStyle(GlassCircle())
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                ScrollViewReader { proxy in
                    ScrollView {
                        LazyVStack(spacing: 0) {
                            ForEach(store.channels) { channel in
                                row(channel).id(channel.id)
                            }
                        }
                    }
                    .onAppear { proxy.scrollTo(nowPlaying.channel?.id, anchor: .center) }
                }
            }
            .glassEffect(in: .rect(cornerRadius: Tokens.Radius.lg))
            .clipShape(.rect(cornerRadius: Tokens.Radius.lg))
            .accessibilityElement(children: .contain)
            .accessibilityLabel("Channels")
            .accessibilityIdentifier("portrait-guide")
        }

        private func row(_ channel: Channel) -> some View {
            let title = store.index.on(channel.id, at: store.now)?.title ?? String(localized: "No listing")
            let current = channel.id == nowPlaying.channel?.id
            return Button {
                nowPlaying.channel = channel
                panel = .none
            } label: {
                HStack(alignment: .firstTextBaseline, spacing: 10) {
                    Text(channel.displayNumber)
                        .font(.headline.monospacedDigit())
                        .frame(minWidth: 52, alignment: .leading)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(channel.displayName).font(.subheadline.weight(.semibold)).lineLimit(1)
                        Text(title).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                    }
                    Spacer(minLength: 0)
                    if current {
                        Image(systemName: "speaker.wave.2.fill")
                            .foregroundStyle(Tokens.ColorToken.tally)
                            .accessibilityHidden(true)
                    }
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                .frame(minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("\(channel.displayNumber) \(channel.displayName), \(title)")
            .accessibilityAddTraits(current ? .isSelected : [])
        }
    }

    /// One glass shape that morphs between Sync screens, Back in sync, and the synced badge.
    private struct SyncPill: View {
        @Environment(NowPlaying.self) private var nowPlaying
        @Namespace private var glass

        var body: some View {
            GlassEffectContainer {
                shape.glassEffectID("sync", in: glass)
            }
            .animation(.bouncy, value: phase)
        }

        private var phase: String {
            guard let sync = nowPlaying.live.sync else { return "none" }
            return "\(sync.offByChoice) \(sync.detached) \(sync.state) \(sync.members)"
        }

        @ViewBuilder private var shape: some View {
            if let sync = nowPlaying.live.sync, sync.offByChoice {
                // Playing on its own; offer to line up only when someone else is on this channel.
                if sync.members > 1 {
                    Button("Sync screens", systemImage: "arrow.triangle.2.circlepath") { sync.syncScreens() }
                        .labelStyle(.titleAndIcon)
                        .buttonStyle(.glass)
                        .accessibilityLabel("Sync \(sync.members) screens")
                }
            } else if let sync = nowPlaying.live.sync, sync.detached {
                Button("Back in sync", systemImage: "arrow.triangle.2.circlepath") { sync.rejoin() }
                    .labelStyle(.iconOnly)
                    .buttonStyle(.glass)
                    .accessibilityLabel("Back in sync")
            } else if let sync = nowPlaying.live.sync, sync.state != .off, sync.members > 1 {
                HStack(spacing: 5) {
                    Image(systemName: "arrow.triangle.2.circlepath")
                    Text("\(sync.members)").monospacedDigit()
                }
                .font(.footnote.weight(.bold))
                .fixedSize()
                .padding(.horizontal, 12)
                .padding(.vertical, 9)
                .glassEffect(.regular.tint(sync.state == .locked ? Tokens.ColorToken.success.opacity(0.4) : nil))
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Synced with \(sync.members) screens")
            }
        }
    }
#endif
