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
            // With a panel open, a swipe down puts the panel away, not the player. Filled,
            // a pinch in fits the picture first; from fit it closes the player.
            .interactiveDismissDisabled(panel != .none || fill)
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
                .background {
                    PinchToFill { scale in
                        if (scale > 1) != fill {
                            toggleFill()
                        }
                    }
                }
                .accessibilityElement()
                .accessibilityLabel("Picture")
                .accessibilityValue(fill ? "Fill" : "Fit")
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
    }

    /// Pinch out fills the screen, pinch in fits it. The zoom transition's own pinch
    /// takes a SwiftUI pinch over the picture, so this one listens on the window beside it.
    private struct PinchToFill: UIViewRepresentable {
        var onPinch: (CGFloat) -> Void

        func makeCoordinator() -> Coordinator {
            Coordinator()
        }

        func makeUIView(context: Context) -> WindowWatcher {
            let view = WindowWatcher()
            view.coordinator = context.coordinator
            view.isUserInteractionEnabled = false
            return view
        }

        func updateUIView(_: WindowWatcher, context: Context) {
            context.coordinator.onPinch = onPinch
        }

        static func dismantleUIView(_: WindowWatcher, coordinator: Coordinator) {
            coordinator.detach()
        }

        @MainActor
        final class Coordinator: NSObject, UIGestureRecognizerDelegate {
            var onPinch: (CGFloat) -> Void = { _ in }
            private var acted = false
            private weak var host: UIView?
            private lazy var pinch = UIPinchGestureRecognizer(target: self, action: #selector(pinched(_:)))

            func attach(to view: UIView) {
                guard host !== view else { return }
                detach()
                pinch.delegate = self
                pinch.cancelsTouchesInView = false
                view.addGestureRecognizer(pinch)
                host = view
            }

            func detach() {
                host?.removeGestureRecognizer(pinch)
                host = nil
            }

            @objc private func pinched(_ gesture: UIPinchGestureRecognizer) {
                switch gesture.state {
                case .began:
                    acted = false
                case .changed:
                    if !acted, abs(gesture.scale - 1) > 0.12 {
                        acted = true
                        onPinch(gesture.scale)
                    }
                default:
                    break
                }
            }

            nonisolated func gestureRecognizer(_: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith _: UIGestureRecognizer) -> Bool {
                true
            }
        }

        final class WindowWatcher: UIView {
            weak var coordinator: Coordinator?

            override func didMoveToWindow() {
                super.didMoveToWindow()
                if let window {
                    coordinator?.attach(to: window)
                } else {
                    coordinator?.detach()
                }
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
                        AirPlayRoute(compact: true, onPresent: {
                            nowPlaying.live.playLogNote("airplay open")
                        })
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
                GlassCircleLabel(title: String(localized: "Audio"), symbol: "speaker.wave.2")
            }
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
                GlassCircleLabel(title: String(localized: "More"), symbol: "ellipsis")
            }
            .accessibilityLabel("More")
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

    /// A menu's face in the same glass circle. The menu's button takes its name from
    /// the text inside it, so the title is there, drawn clear.
    struct GlassCircleLabel: View {
        let title: String
        let symbol: String

        var body: some View {
            ZStack {
                Text(title)
                    .font(.system(size: 1))
                    .foregroundStyle(.clear)
                Image(systemName: symbol)
                    .font(.body.weight(.semibold))
                    .foregroundStyle(.white)
                    .accessibilityHidden(true)
            }
            .frame(width: 44, height: 44)
            .contentShape(Circle())
            .glassEffect(.regular.interactive(), in: .circle)
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

        private func syncScreens(_ sync: SyncEngine, words: Bool) -> some View {
            Button("Sync screens", systemImage: "arrow.triangle.2.circlepath") { sync.syncScreens() }
                .labelStyle(SyncLabel(words: words))
                .lineLimit(1)
                .fixedSize()
                .buttonStyle(.glass)
                .accessibilityLabel("Sync \(sync.members) screens")
        }

        private struct SyncLabel: LabelStyle {
            let words: Bool

            func makeBody(configuration: Configuration) -> some View {
                HStack(spacing: 6) {
                    configuration.icon
                    if words {
                        configuration.title
                    }
                }
            }
        }

        private var phase: String {
            guard let sync = nowPlaying.live.sync else { return "none" }
            return "\(sync.offByChoice) \(sync.detached) \(sync.state) \(sync.members)"
        }

        @ViewBuilder private var shape: some View {
            if let sync = nowPlaying.live.sync, sync.offByChoice {
                // Playing on its own; offer to line up only when someone else is on this channel.
                if sync.members > 1 {
                    // The words when they fit on one line; the symbol alone at large text sizes.
                    ViewThatFits(in: .horizontal) {
                        syncScreens(sync, words: true)
                        syncScreens(sync, words: false)
                    }
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

    // MARK: Mini player

    /// Live TV keeps a place at the bottom while you browse: the picture itself, what is
    /// on, play and pause, and Picture in Picture. A tap opens the player.
    struct MiniPlayerBar: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        @State private var window = MiniWindow()

        var body: some View {
            if let channel = nowPlaying.channel {
                HStack(spacing: 12) {
                    MiniPicture(channel: channel, window: window)
                    title(channel)
                    Spacer(minLength: 4)
                    controls
                        .foregroundStyle(.primary)
                        .buttonStyle(MiniControl())
                }
                .labelStyle(.iconOnly)
                .padding(.leading, 10)
                .padding(.trailing, 14)
                .contentShape(.rect)
                .playerZoomSource(id: PlayerZoom.mini)
                .onTapGesture {
                    nowPlaying.zoomFrom = PlayerZoom.mini
                    nowPlaying.expanded = true
                }
            }
        }

        private func title(_ channel: Channel) -> some View {
            let live = nowPlaying.live
            return VStack(alignment: .leading, spacing: 0) {
                Text(store.index.on(channel.id, at: store.now)?.title ?? channel.displayName)
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                Text(live.error ?? "\(channel.displayNumber) \(channel.displayName)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            .accessibilityElement(children: .combine)
            .accessibilityValue(live.paused ? "Paused" : live.moving ? "Playing" : live.error == nil ? "Tuning" : "Stopped")
            .accessibilityAddTraits(.isButton)
            .accessibilityHint("Opens the player")
            .accessibilityIdentifier("miniPlayer")
            .accessibilityAction { nowPlaying.expanded = true }
        }

        @ViewBuilder private var controls: some View {
            let live = nowPlaying.live
            if nowPlaying.externalDisplay {
                // The picture is on the display; this bar is its remote.
                Button("Previous channel", systemImage: "chevron.up") {
                    nowPlaying.step(ChannelStep.offset(up: true), in: store.channels)
                }
                Button("Next channel", systemImage: "chevron.down") {
                    nowPlaying.step(ChannelStep.offset(up: false), in: store.channels)
                }
            } else if window.possible {
                Button("Picture in Picture", systemImage: "pip.enter") { window.start() }
                    .accessibilityIdentifier("mini-pip")
            }
            Button(live.paused ? "Play" : "Pause", systemImage: live.paused ? "play.fill" : "pause.fill") {
                live.togglePause()
            }
            .contentTransition(.symbolEffect(.replace))
            .accessibilityIdentifier("mini-play")
            Button("Close", systemImage: "xmark") { nowPlaying.stop() }
                .accessibilityLabel("Close")
        }
    }

    /// A 44 pt target around a small symbol.
    private struct MiniControl: ButtonStyle {
        func makeBody(configuration: Configuration) -> some View {
            configuration.label
                .font(.body.weight(.semibold))
                .frame(minWidth: 40, minHeight: 44)
                .contentShape(Rectangle())
                .opacity(configuration.isPressed ? 0.5 : 1)
        }
    }

    /// The mini player's own Picture in Picture, for a picture docked under the tabs.
    /// It also starts by itself when the app goes to the Home Screen.
    @MainActor
    @Observable
    final class MiniWindow: NSObject, AVPictureInPictureControllerDelegate {
        private(set) var possible = false
        @ObservationIgnored private var controller: AVPictureInPictureController?
        @ObservationIgnored private var watch: NSKeyValueObservation?
        @ObservationIgnored var onRestore: () -> Void = {}
        @ObservationIgnored var onChange: (Bool) -> Void = { _ in }

        func attach(_ layer: AVPlayerLayer) {
            guard controller?.playerLayer !== layer, AVPictureInPictureController.isPictureInPictureSupported() else { return }
            let pip = AVPictureInPictureController(playerLayer: layer)
            pip?.canStartPictureInPictureAutomaticallyFromInline = true
            pip?.delegate = self
            controller = pip
            watch = pip?.observe(\.isPictureInPicturePossible, options: [.initial, .new]) { [weak self] pip, _ in
                let can = pip.isPictureInPicturePossible
                Task { @MainActor in self?.possible = can }
            }
        }

        func start() {
            controller?.startPictureInPicture()
        }

        nonisolated func pictureInPictureControllerDidStartPictureInPicture(_: AVPictureInPictureController) {
            MainActor.assumeIsolated { onChange(true) }
        }

        nonisolated func pictureInPictureControllerDidStopPictureInPicture(_: AVPictureInPictureController) {
            MainActor.assumeIsolated { onChange(false) }
        }

        nonisolated func pictureInPictureController(
            _: AVPictureInPictureController,
            restoreUserInterfaceForPictureInPictureStopWithCompletionHandler completion: @escaping (Bool) -> Void
        ) {
            MainActor.assumeIsolated { onRestore() }
            completion(true)
        }
    }

    /// The live picture, small, over the show's art until it moves.
    private struct MiniPicture: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        let channel: Channel
        let window: MiniWindow

        var body: some View {
            let live = nowPlaying.live
            ZStack {
                TuneBackdrop(
                    frame: store.api?.frameURL(channelID: channel.id, width: 480, listed: store.frameIDs),
                    art: store.index.on(channel.id, at: store.now).flatMap { store.artURL($0, width: 480) }
                )
                MiniLayer(player: live.player, window: window)
                    .opacity(live.moving ? 1 : 0)
                if !live.moving, live.error == nil {
                    ProgressView()
                        .controlSize(.small)
                        .accessibilityLabel("Tuning")
                }
            }
            .frame(width: 64, height: 36)
            .clipShape(.rect(cornerRadius: 6))
            .accessibilityHidden(true)
            .onAppear {
                window.onRestore = { nowPlaying.expanded = true }
                window.onChange = { live.notePictureInPicture($0) }
            }
        }
    }

    private struct MiniLayer: UIViewRepresentable {
        let player: AVPlayer
        let window: MiniWindow

        func makeUIView(context _: Context) -> PlayerHost {
            let view = PlayerHost()
            view.playerLayer?.player = player
            view.playerLayer?.videoGravity = .resizeAspectFill
            if let layer = view.playerLayer {
                window.attach(layer)
            }
            return view
        }

        func updateUIView(_ view: PlayerHost, context _: Context) {
            if view.playerLayer?.player !== player {
                view.playerLayer?.player = player
            }
        }
    }
#endif
