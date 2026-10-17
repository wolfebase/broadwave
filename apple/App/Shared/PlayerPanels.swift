import AVKit
import BroadwaveKit
import SwiftUI
import UIKit

/// Channels, Stream, and Move tabs for the Apple TV playback info panel. The
/// system Info tab comes from the item's metadata, so there is no custom one.
@MainActor
enum PlayerPanels {
    static func controllers(store: AppStore, now: NowPlaying, live: LivePlayer) -> [UIViewController] {
        [
            host("Channels", ChannelListPanel(), store: store, now: now, live: live),
            host("Stream", StreamFactsPanel(), store: store, now: now, live: live),
            // The other screen took the channel, so this one stops and the player closes.
            host("Move to another screen", MoveToScreenList { now.stop() }, store: store, now: now, live: live, id: "move"),
        ]
    }

    private static func host(
        _ title: String, _ content: some View, store: AppStore, now: NowPlaying, live: LivePlayer, id: String? = nil
    ) -> UIViewController {
        let root = AnyView(
            content
                .environment(store)
                .environment(now)
                .environment(live)
        )
        let controller = UIHostingController(rootView: root)
        controller.title = title
        controller.preferredContentSize = CGSize(width: 960, height: 520)
        controller.view.backgroundColor = .clear
        controller.view.accessibilityIdentifier = "panel-\(id ?? title.lowercased())"
        return controller
    }
}

/// The lineup. Choosing a row changes the channel without leaving the player.
struct ChannelListPanel: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @State private var warming: Task<Void, Never>?
    @FocusState private var focus: Int64?

    var body: some View {
        ScrollViewReader { proxy in
            list
                // The page opened scrolled to the end, with focus on the last rows.
                .defaultFocus($focus, nowPlaying.channel?.id)
                .onAppear {
                    if let id = nowPlaying.channel?.id {
                        proxy.scrollTo(id, anchor: .center)
                    }
                }
        }
        .onChange(of: focus) { _, id in
            warming?.cancel()
            guard let id, id != nowPlaying.channel?.id, let channel = store.channels.first(where: { $0.id == id }) else { return }
            warm(channel)
        }
        .accessibilityIdentifier("panel-channels")
    }

    private var list: some View {
        ScrollView {
            VStack(spacing: 4) {
                ForEach(store.channels) { channel in
                    let current = channel.id == nowPlaying.channel?.id
                    let title = store.index.on(channel.id, at: store.now)?.title ?? channel.displayName
                    Button {
                        guard !current else { return }
                        nowPlaying.channel = channel
                    } label: {
                        HStack(spacing: 16) {
                            Text(channel.displayNumber)
                                .font(.headline.monospacedDigit())
                                .lineLimit(1)
                                .fixedSize()
                                .frame(minWidth: 72, alignment: .leading)
                            Text(title)
                                .lineLimit(1)
                            Spacer(minLength: 0)
                            if current {
                                Image(systemName: "checkmark")
                                    .accessibilityHidden(true)
                            }
                        }
                        .padding(.horizontal, 16)
                        .padding(.vertical, 10)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("\(channel.displayNumber) \(channel.displayName), \(title)")
                    .accessibilityAddTraits(current ? .isSelected : [])
                    .accessibilityIdentifier("panel-channel-\(channel.id)")
                    .focused($focus, equals: channel.id)
                }
            }
            .padding(12)
        }
        // AVKit sizes a page to its ideal height: the whole lineup, taller than the
        // screen, shown from its last rows, so the list never scrolled.
        .frame(idealHeight: 432)
    }
}

extension ChannelListPanel {
    /// A row the remote rests on starts its picture, when that costs no tuner.
    private func warm(_ channel: Channel) {
        warming?.cancel()
        warming = Task {
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled, let api = store.api else { return }
            await api.warm(channelID: channel.id, caps: Capabilities.current(), prefs: store.prefs)
        }
    }
}

/// Rendition, encoder, bitrate, dropped frames, and sync drift.
struct StreamFactsPanel: View {
    @Environment(LivePlayer.self) private var live

    var body: some View {
        let stream = live.session?.stream
        let facts = StreamFacts.make(
            rendition: stream?.rendition,
            encoder: stream?.encoder,
            bitrate: stream?.bitrate,
            dropped: live.picture.dropped,
            sync: .init(milliseconds: live.sync?.drift, state: live.sync?.state.rawValue)
        )
        VStack(alignment: .leading, spacing: 12) {
            fact("Rendition", facts.rendition)
            fact("Encoder", facts.encoder)
            fact("Bitrate", facts.bitrate)
            fact("Dropped frames", facts.dropped)
            fact("Drift", facts.drift)
            fact("Behind live", live.picture.behind.map { String(format: "%.1f s", $0) } ?? "—")
            fact("Connection", live.picture.secure ? String(localized: "Secure (HTTP/2)") : String(localized: "Home address"))
            Spacer(minLength: 0)
        }
        .padding(28)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .accessibilityIdentifier("panel-stream")
    }

    private func fact(_ name: String, _ value: String) -> some View {
        HStack(alignment: .firstTextBaseline) {
            Text(name)
                .foregroundStyle(.secondary)
            Spacer(minLength: 16)
            Text(value)
                .fontWeight(.semibold)
                .multilineTextAlignment(.trailing)
        }
        .font(.body)
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(name), \(value)")
    }
}

/// The system player, plus channel swipes while the transport bar is hidden.
final class LivePlayerController: AVPlayerViewController {
    var onStep: ((Int) -> Void)?
    var onTransport: ((Bool) -> Void)?
    #if os(tvOS)
        /// Down with nothing on screen: the channel strip. Nil for a recording.
        var onDown: (() -> Void)?
        /// Something of the app's own is over the picture, such as the channel strip.
        /// The remote is its, not a channel change.
        var overlayOpen = false
        /// Takes Down before AVKit's own Info panel does.
        private let downGate = DownGate()
        var onPictureInPicture: ((Bool) -> Void)?
        var onPictureRestore: (() -> Void)?
        var onPictureClosed: (() -> Void)?
        private var pipRestoring = false
        /// An Info, Channels, or Stream page opened or closed over the picture.
        var onPanel: ((Bool) -> Void)?
        private var panelShown = false
    #endif
    private(set) var transportShown = true
    private var installedGestures = false
    #if os(tvOS)
        /// Lets the channel swipes in while the bar is hidden. A delegate of its own: AVKit
        /// is the delegate of its own gestures, and a `gestureRecognizer(_:shouldReceive:)`
        /// on this class replaced AVKit's, so a tap on iPhone and iPad never showed the controls.
        private let swipeGate = SwipeGate()
    #endif
    private var swallowedPress = false
    private weak var infoTabsView: UIView?
    private var lastStep = Date.distantPast

    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        installChannelSwipes()
        #if os(tvOS)
            claimFocus()
            #if DEBUG
                installPanelProbe()
            #endif
        #endif
        #if os(iOS)
            if keysOn {
                becomeFirstResponder()
            }
        #endif
    }

    var onEscape: (() -> Void)?
    #if os(iOS)
        /// The live player's keys. Off for a file, and while a sheet or the Start
        /// over recording covers the live player.
        var keysOn = false

        override var canBecomeFirstResponder: Bool {
            true
        }

        /// A hardware keyboard. Arrows and Escape need priority, or a scroll view or
        /// the focus system takes them first. AVKit keeps its own keys.
        override var keyCommands: [UIKeyCommand]? {
            guard keysOn else { return super.keyCommands }
            let own = [
                (UIKeyCommand.inputUpArrow, "Previous channel"),
                (UIKeyCommand.inputDownArrow, "Next channel"),
                (UIKeyCommand.inputEscape, "Minimize"),
            ].map { input, title in
                let command = UIKeyCommand(title: title, action: #selector(pressedKey(_:)), input: input)
                command.wantsPriorityOverSystemBehavior = true
                return command
            }
            // Command-period is Escape on a keyboard without an Escape key.
            let cancel = UIKeyCommand(title: "Minimize", action: #selector(pressedKey(_:)), input: ".", modifierFlags: .command)
            cancel.wantsPriorityOverSystemBehavior = true
            return (super.keyCommands ?? []) + own + [cancel]
        }

        @objc private func pressedKey(_ command: UIKeyCommand) {
            switch command.input {
            case UIKeyCommand.inputUpArrow: channelStep(up: true)
            case UIKeyCommand.inputDownArrow: channelStep(up: false)
            default: onEscape?()
            }
        }
    #endif

    #if os(tvOS)
        /// A player that comes up while focus is still on the guide under it never sees
        /// the remote, so the first Back went to the guide. Focus moves in here.
        private func claimFocus() {
            if let focused = UIFocusSystem.focusSystem(for: view)?.focusedItem as? UIView, focused.isDescendant(of: view) {
                return
            }
            guard let root = view.window?.rootViewController else { return }
            root.setNeedsFocusUpdate()
            root.updateFocusIfNeeded()
        }

        /// Nothing else on screen listens to the remote: no bar, panel, or strip.
        fileprivate var pictureOnly: Bool {
            !transportShown && !infoPanelVisible && !overlayOpen
        }

        override func pressesBegan(_ presses: Set<UIPress>, with event: UIPressesEvent?) {
            // Down has one meaning with nothing on screen: the channel strip (the gate
            // recognizer takes it). Up, and a remote's channel buttons, change channel.
            // The bar, a panel, or the strip take the arrows for themselves.
            if pictureOnly, presses.contains(where: { $0.type == .downArrow }), onDown != nil {
                swallowedPress = true
                return
            }
            if pictureOnly, let press = presses.first(where: { $0.type == .upArrow || $0.type == .downArrow }) {
                swallowedPress = true
                channelStep(up: press.type == .upArrow)
                return
            }
            if !infoPanelVisible, !overlayOpen, let press = presses.first(where: { $0.type == .pageUp || $0.type == .pageDown }) {
                swallowedPress = true
                channelStep(up: press.type == .pageUp)
                return
            }
            // AVKit spends Back on hiding its bar, so leaving took two presses and the
            // first seemed to do nothing. With no page open, Back leaves the player.
            if transportShown, !infoPanelVisible, presses.contains(where: { $0.type == .menu }), let onEscape {
                swallowedPress = true
                onEscape()
                return
            }
            swallowedPress = false
            super.pressesBegan(presses, with: event)
        }

        override func pressesEnded(_ presses: Set<UIPress>, with event: UIPressesEvent?) {
            if swallowedPress {
                swallowedPress = false
                return
            }
            super.pressesEnded(presses, with: event)
        }

        override func pressesCancelled(_ presses: Set<UIPress>, with event: UIPressesEvent?) {
            if swallowedPress {
                swallowedPress = false
                return
            }
            super.pressesCancelled(presses, with: event)
        }
    #endif

    /// True while an Info, Channels, or Stream page is on screen. AVKit leaves a
    /// closed page in the window, moved below the screen at full alpha.
    private var infoPanelVisible: Bool {
        #if os(tvOS)
            // The system Info tab is not one of ours. It has nothing to focus, so while it
            // is up focus rests on its tab. The tab row stays in the window once closed.
            (customInfoViewControllers ?? []).contains { controller in
                controller.viewIfLoaded.map(Self.onScreen) ?? false
            } || focusInTabs
        #else
            false
        #endif
    }

    #if os(tvOS)
        /// Focus moving into the tab row is the first sign of a page; the coordinator's
        /// sampling catches one that closes without a focus change.
        override func didUpdateFocus(in context: UIFocusUpdateContext, with coordinator: UIFocusAnimationCoordinator) {
            super.didUpdateFocus(in: context, with: coordinator)
            notePanel()
        }

        func notePanel() {
            let shown = infoPanelVisible
            guard shown != panelShown else { return }
            panelShown = shown
            onPanel?(shown)
        }

        private var focusInTabs: Bool {
            guard let tabs = infoTabs, let focused = UIFocusSystem.focusSystem(for: view)?.focusedItem as? UIView else { return false }
            return focused.isDescendant(of: tabs)
        }

        /// AVKit's row of panel tabs (Info, Channels, …). Found once, then kept.
        private var infoTabs: UIView? {
            if let tabs = infoTabsView, tabs.window != nil {
                return tabs
            }
            var stack: [UIView] = [view]
            while let next = stack.popLast() {
                if next.accessibilityIdentifier == "AVInfoMenuCollection" {
                    infoTabsView = next
                    return next
                }
                stack.append(contentsOf: next.subviews)
            }
            return nil
        }
    #endif

    private static func onScreen(_ view: UIView) -> Bool {
        guard let window = view.window else { return false }
        var next: UIView? = view
        while let current = next {
            if current.isHidden || current.alpha < 0.01 {
                return false
            }
            next = current.superview
        }
        let frame = view.convert(view.bounds, to: window)
        return frame.height > 1 && frame.intersection(window.bounds).height > frame.height / 2
    }

    private func installChannelSwipes() {
        #if os(tvOS)
            guard !installedGestures, let host = contentOverlayView else { return }
            installedGestures = true
            swipeGate.open = { [weak self] in
                guard let self else { return false }
                return !transportShown && !infoPanelVisible
            }
            for direction: UISwipeGestureRecognizer.Direction in [.up, .down] {
                let swipe = UISwipeGestureRecognizer(target: self, action: #selector(swiped(_:)))
                swipe.direction = direction
                swipe.delegate = swipeGate
                swipe.cancelsTouchesInView = true
                host.addGestureRecognizer(swipe)
            }
            downGate.open = { [weak self] in
                guard let self else { return false }
                return pictureOnly && onDown != nil
            }
            let down = UITapGestureRecognizer(target: self, action: #selector(pressedDown))
            down.allowedPressTypes = [NSNumber(value: UIPress.PressType.downArrow.rawValue)]
            down.delegate = downGate
            view.addGestureRecognizer(down)
            downGate.recognizer = down
        #endif
        holdAVKitBehindDown()
    }

    /// AVKit's own Down opens its Info panel. With nothing on screen it now waits for
    /// the gate, which takes Down for the channel strip; otherwise AVKit keeps it.
    /// AVKit adds recognizers as its bar and panels load, so this runs again when the bar moves.
    private func holdAVKitBehindDown() {
        #if os(tvOS)
            guard let gate = downGate.recognizer else { return }
            let down = NSNumber(value: UIPress.PressType.downArrow.rawValue)
            var stack: [UIView] = [view]
            while let next = stack.popLast() {
                for recognizer in next.gestureRecognizers ?? [] where recognizer !== gate && !downGate.held.contains(ObjectIdentifier(recognizer)) {
                    if recognizer.allowedPressTypes.contains(down) {
                        recognizer.require(toFail: gate)
                        downGate.held.insert(ObjectIdentifier(recognizer))
                    }
                }
                stack.append(contentsOf: next.subviews)
            }
            #if DEBUG
                print("broadwave down gate holds \(downGate.held.count)")
            #endif
        #endif
    }

    @objc private func pressedDown() {
        #if os(tvOS)
            guard pictureOnly else { return }
            onDown?()
        #endif
    }

    @objc private func swiped(_ gesture: UISwipeGestureRecognizer) {
        guard !transportShown else { return }
        #if os(tvOS)
            if gesture.direction == .down, let onDown {
                guard pictureOnly else { return }
                onDown()
                return
            }
        #endif
        channelStep(up: gesture.direction == .up)
    }

    private func channelStep(up: Bool) {
        let now = Date()
        // A clickpad swipe can arrive as both a swipe and an arrow press.
        guard now.timeIntervalSince(lastStep) > 0.35 else { return }
        lastStep = now
        onStep?(ChannelStep.offset(up: up))
    }

    fileprivate func noteTransport(_ shown: Bool) {
        guard shown != transportShown else { return }
        transportShown = shown
        onTransport?(shown)
        holdAVKitBehindDown()
        #if DEBUG
            print("broadwave transport \(shown ? "shown" : "hidden")")
            if shown, UserDefaults.standard.bool(forKey: "BroadwaveSyncLog") {
                let names = controlNames(in: view)
                print("broadwave pip controls \(names.isEmpty ? "none" : names.joined(separator: " | "))")
                fflush(stdout)
            }
        #endif
    }

    #if DEBUG && os(tvOS)
        /// UI tests read whether an Info, Channels, or Stream page is on screen here.
        private func installPanelProbe() {
            guard UserDefaults.standard.bool(forKey: "BroadwaveChannelNow"), view.viewWithTag(0xB0B) == nil else { return }
            let probe = UILabel(frame: CGRect(x: 0, y: 0, width: 4, height: 4))
            probe.tag = 0xB0B
            probe.alpha = 0.02
            probe.font = .systemFont(ofSize: 2)
            probe.isAccessibilityElement = true
            probe.accessibilityIdentifier = "panelProbe"
            view.addSubview(probe)
            Task { @MainActor [weak self, weak probe] in
                while let self, let probe {
                    probe.text = infoPanelVisible ? "panel" : "none"
                    probe.accessibilityLabel = probe.text
                    try? await Task.sleep(for: .milliseconds(250))
                }
            }
        }
    #endif

    #if DEBUG
        private func controlNames(in view: UIView) -> [String] {
            var names: [String] = []
            if let button = view as? UIButton {
                let label = button.accessibilityLabel ?? ""
                let title = button.currentTitle ?? ""
                let text = label.isEmpty ? title : label
                if !text.isEmpty {
                    names.append(text)
                }
            }
            for child in view.subviews {
                names.append(contentsOf: controlNames(in: child))
            }
            return names
        }
    #endif
}

#if os(tvOS)
    private final class SwipeGate: NSObject, UIGestureRecognizerDelegate {
        var open: () -> Bool = { false }

        func gestureRecognizer(_: UIGestureRecognizer, shouldReceive _: UITouch) -> Bool {
            open()
        }
    }

    /// Lets Down in only while nothing but the picture is on screen.
    private final class DownGate: NSObject, UIGestureRecognizerDelegate {
        var open: () -> Bool = { false }
        weak var recognizer: UIGestureRecognizer?
        /// AVKit recognizers already waiting on this one.
        var held: Set<ObjectIdentifier> = []

        func gestureRecognizer(_: UIGestureRecognizer, shouldReceive _: UIPress) -> Bool {
            open()
        }

        func gestureRecognizerShouldBegin(_: UIGestureRecognizer) -> Bool {
            open()
        }
    }

    extension LivePlayerController: AVPlayerViewControllerDelegate {
        /// UIKit calls this on the main thread, off the actor.
        nonisolated func playerViewController(
            _: AVPlayerViewController,
            willTransitionToVisibilityOfTransportBar visible: Bool,
            with _: any AVPlayerViewControllerAnimationCoordinator
        ) {
            MainActor.assumeIsolated {
                self.noteTransport(visible)
            }
        }

        /// The player is embedded, not presented. Dismissing it when the small
        /// window starts would release the picture, so Menu would have nothing
        /// to come back to. Same decision as the iPhone player.
        nonisolated func playerViewControllerShouldAutomaticallyDismissAtPictureInPictureStart(_: AVPlayerViewController) -> Bool {
            PictureHandoff.dismissWhenPictureInPictureStarts
        }

        nonisolated func playerViewControllerWillStartPictureInPicture(_: AVPlayerViewController) {
            MainActor.assumeIsolated {
                self.onPictureInPicture?(true)
            }
        }

        nonisolated func playerViewController(_: AVPlayerViewController, failedToStartPictureInPictureWithError error: Error) {
            let text = error.localizedDescription
            MainActor.assumeIsolated {
                self.onPictureInPicture?(false)
                print("broadwave pip failed \(text)")
                fflush(stdout)
            }
        }

        nonisolated func playerViewController(
            _: AVPlayerViewController,
            restoreUserInterfaceForPictureInPictureStopWithCompletionHandler completionHandler: @escaping (Bool) -> Void
        ) {
            MainActor.assumeIsolated {
                self.pipRestoring = true
                self.onPictureRestore?()
            }
            completionHandler(true)
        }

        nonisolated func playerViewControllerDidStopPictureInPicture(_: AVPlayerViewController) {
            MainActor.assumeIsolated {
                let away = UIApplication.shared.applicationState != .active
                let stop = PictureHandoff.stopWhenClosed(restored: self.pipRestoring, away: away)
                self.pipRestoring = false
                self.onPictureInPicture?(false)
                if stop {
                    self.onPictureClosed?()
                }
            }
        }
    }
#endif
