import AVKit
import BroadwaveKit
import SwiftUI
import UIKit

/// Channels and Stream tabs for the Apple TV playback info panel. The system
/// Info tab comes from the item's metadata, so there is no custom one.
@MainActor
enum PlayerPanels {
    static func controllers(store: AppStore, now: NowPlaying, live: LivePlayer) -> [UIViewController] {
        [
            host("Channels", ChannelListPanel(), store: store, now: now, live: live),
            host("Stream", StreamFactsPanel(), store: store, now: now, live: live),
        ]
    }

    private static func host(_ title: String, _ content: some View, store: AppStore, now: NowPlaying, live: LivePlayer) -> UIViewController {
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
        controller.view.accessibilityIdentifier = "panel-\(title.lowercased())"
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
final class LivePlayerController: AVPlayerViewController, UIGestureRecognizerDelegate {
    var onStep: ((Int) -> Void)?
    var onTransport: ((Bool) -> Void)?
    #if os(tvOS)
        var onPictureInPicture: ((Bool) -> Void)?
        var onPictureRestore: (() -> Void)?
        var onPictureClosed: (() -> Void)?
        private var pipRestoring = false
    #endif
    private(set) var transportShown = true
    private var installedGestures = false
    private var swallowedPress = false
    private var lastStep = Date.distantPast

    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        installChannelSwipes()
    }

    #if os(tvOS)
        override func pressesBegan(_ presses: Set<UIPress>, with event: UIPressesEvent?) {
            // A click while the bar is up moves through the bar. A click while it
            // is hidden changes channel, the same as a swipe on the clickpad.
            // The info panel uses the same arrows. A hidden bar with that panel up is not a channel swipe.
            if !transportShown, !infoPanelVisible, let press = presses.first(where: { $0.type == .upArrow || $0.type == .downArrow }) {
                swallowedPress = true
                channelStep(up: press.type == .upArrow)
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

    func gestureRecognizer(_: UIGestureRecognizer, shouldReceive _: UITouch) -> Bool {
        !transportShown && !infoPanelVisible
    }

    /// True while an Info, Channels, or Stream page is on screen. AVKit leaves a
    /// closed page in the window, moved below the screen at full alpha.
    private var infoPanelVisible: Bool {
        #if os(tvOS)
            (customInfoViewControllers ?? []).contains { controller in
                controller.viewIfLoaded.map(Self.onScreen) ?? false
            }
        #else
            false
        #endif
    }

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
            for direction: UISwipeGestureRecognizer.Direction in [.up, .down] {
                let swipe = UISwipeGestureRecognizer(target: self, action: #selector(swiped(_:)))
                swipe.direction = direction
                swipe.delegate = self
                swipe.cancelsTouchesInView = true
                host.addGestureRecognizer(swipe)
            }
        #endif
    }

    @objc private func swiped(_ gesture: UISwipeGestureRecognizer) {
        guard !transportShown else { return }
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
        #if DEBUG
            print("broadwave transport \(shown ? "shown" : "hidden")")
            if shown, UserDefaults.standard.bool(forKey: "BroadwaveSyncLog") {
                let names = controlNames(in: view)
                print("broadwave pip controls \(names.isEmpty ? "none" : names.joined(separator: " | "))")
                fflush(stdout)
            }
        #endif
    }

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
