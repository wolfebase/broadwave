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

    var body: some View {
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
                                .frame(width: 72, alignment: .leading)
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
                }
            }
            .padding(12)
        }
        .accessibilityIdentifier("panel-channels")
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

    /// True while an Info, Channels, or Stream page is on screen.
    private var infoPanelVisible: Bool {
        #if os(tvOS)
            (customInfoViewControllers ?? []).contains { controller in
                guard let view = controller.viewIfLoaded, view.window != nil else { return false }
                return !view.isHidden && view.alpha > 0.01 && view.bounds.height > 1
            }
        #else
            false
        #endif
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
        #endif
    }
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
    }
#endif
