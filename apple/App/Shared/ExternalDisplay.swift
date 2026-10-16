#if os(iOS)
    import AVFoundation
    import BroadwaveKit
    import BroadwaveUI
    import SwiftUI

    /// A TV or monitor on a cable (iOS 27): it shows the channel full screen, and the
    /// iPhone or iPad stays the guide and the remote. AirPlay is unchanged.
    struct ExternalDisplayAccessory: ViewModifier {
        let store: AppStore
        let nowPlaying: NowPlaying

        func body(content: Content) -> some View {
            #if compiler(>=6.4)
                if #available(iOS 27, *) {
                    content.sceneAccessory {
                        ExternalNonInteractiveAccessory {
                            ExternalPicture()
                                .environment(store)
                                .environment(nowPlaying)
                        }
                        .onAvailabilityChange { available in
                            nowPlaying.externalDisplay = available
                            // The picture moves to the display; the phone keeps the guide under the mini player.
                            if available {
                                nowPlaying.expanded = false
                            }
                        }
                    }
                } else {
                    content
                }
            #else
                content
            #endif
        }
    }

    /// What the external display shows: the live picture, and for a few seconds after
    /// each channel change the channel and the program, as a TV's own banner does.
    private struct ExternalPicture: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        @State private var banner = false

        var body: some View {
            ZStack(alignment: .bottomLeading) {
                Color.black
                if let channel = nowPlaying.channel, nowPlaying.together.isEmpty {
                    PlayerLayerBox(player: nowPlaying.live.player, gravity: .resizeAspect)
                    if banner {
                        ExternalBanner(channel: channel, airing: store.index.on(channel.id, at: store.now))
                            .padding(48)
                            .transition(.move(edge: .bottom).combined(with: .opacity))
                    }
                } else {
                    VStack(spacing: 12) {
                        Image(systemName: "dot.radiowaves.left.and.right")
                            .font(.system(size: 64, weight: .semibold))
                            .foregroundStyle(Tokens.ColorToken.accent)
                        Text("Broadwave")
                            .font(.largeTitle.weight(.heavy))
                        Text("Pick a channel on your iPhone.")
                            .font(.title3)
                            .foregroundStyle(.secondary)
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                }
            }
            .ignoresSafeArea()
            .preferredColorScheme(.dark)
            .task(id: nowPlaying.channel?.id) {
                guard nowPlaying.channel != nil else { return }
                withAnimation(.smooth) { banner = true }
                try? await Task.sleep(for: .seconds(5))
                withAnimation(.smooth) { banner = false }
            }
        }
    }

    private struct ExternalBanner: View {
        let channel: Channel
        let airing: Airing?

        var body: some View {
            HStack(spacing: 20) {
                Text(channel.displayNumber)
                    .font(.system(size: 44, weight: .heavy).monospacedDigit())
                VStack(alignment: .leading, spacing: 4) {
                    Text(airing?.title ?? channel.displayName)
                        .font(.title.weight(.bold))
                        .lineLimit(1)
                    Text(airing.map { "\(channel.displayName) · \($0.minutesLeft(at: .now))" } ?? channel.displayName)
                        .font(.title3)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            .padding(.horizontal, 28)
            .padding(.vertical, 18)
            .glassEffect(in: .rect(cornerRadius: 28))
        }
    }
#endif

#if os(iOS)
    import AVKit

    /// The system's subtitles and caption style menu for a player that is not an
    /// AVPlayerViewController, such as multiview's sound tile (iOS 26.4).
    @available(iOS 26.4, *)
    struct SubtitlesMenu: UIViewRepresentable {
        let player: AVPlayer

        func makeCoordinator() -> AVLegibleMediaOptionsMenuController {
            AVLegibleMediaOptionsMenuController(player: player)
        }

        func makeUIView(context: Context) -> UIButton {
            var config = UIButton.Configuration.glass()
            config.image = UIImage(systemName: "captions.bubble")
            config.baseForegroundColor = .white
            let button = UIButton(configuration: config)
            button.showsMenuAsPrimaryAction = true
            button.accessibilityLabel = String(localized: "Subtitles")
            // Built when it opens, so it lists the tracks the item has by then.
            button.menu = UIMenu(children: [UIDeferredMenuElement.uncached { [weak menus = context.coordinator] done in
                done(menus?.menu(contents: .all)?.children ?? [])
            }])
            return button
        }

        func updateUIView(_: UIButton, context: Context) {
            if context.coordinator.player !== player {
                context.coordinator.player = player
            }
        }
    }
#endif
