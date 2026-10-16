#if os(tvOS)
    import BroadwaveKit
    import BroadwaveUI
    import SwiftUI

    /// Down in the live player: the lineup as a row of cards, each with the channel's
    /// latest picture, what is on and how far along, and what is next. Left and Right
    /// browse, Select changes channel, Up or Back closes. It closes itself when the
    /// remote rests.
    struct ChannelStrip: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        let close: () -> Void
        @FocusState private var focus: Int64?
        @State private var minute = Int(Date().timeIntervalSince1970) / 60
        @State private var warming: Task<Void, Never>?

        var body: some View {
            ScrollViewReader { proxy in
                ScrollView(.horizontal) {
                    LazyHStack(spacing: 40) {
                        ForEach(store.channels) { channel in
                            card(channel)
                                .id(channel.id)
                        }
                    }
                    .padding(.horizontal, 80)
                    .padding(.top, 36)
                    .padding(.bottom, 60)
                }
                .scrollClipDisabled()
                .scrollIndicators(.hidden)
                .fixedSize(horizontal: false, vertical: true)
                .onAppear {
                    let current = nowPlaying.channel?.id
                    proxy.scrollTo(current, anchor: .center)
                    focus = current
                }
            }
            .focusSection()
            .defaultFocus($focus, nowPlaying.channel?.id)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottom)
            .background(alignment: .bottom) {
                LinearGradient(colors: [.clear, .black.opacity(0.88)], startPoint: .top, endPoint: .bottom)
                    .frame(height: 560)
                    .ignoresSafeArea()
                    .allowsHitTesting(false)
            }
            .onExitCommand(perform: close)
            .onMoveCommand { direction in
                if direction == .up {
                    close()
                }
            }
            .onChange(of: focus) { _, id in
                warm(id)
            }
            // Ten seconds without a press, and the picture is clear again.
            .task(id: focus) {
                try? await Task.sleep(for: .seconds(10))
                if !Task.isCancelled {
                    close()
                }
            }
            .task {
                // The server grabs a new frame about once a minute.
                while !Task.isCancelled {
                    try? await Task.sleep(for: .seconds(30))
                    minute = Int(Date().timeIntervalSince1970) / 60
                }
            }
            .onDisappear { warming?.cancel() }
            .accessibilityIdentifier("channel-strip")
        }

        private func card(_ channel: Channel) -> some View {
            let current = channel.id == nowPlaying.channel?.id
            let now = store.now
            let airing = store.index.on(channel.id, at: now)
            let next = store.index.next(channel.id, after: airing?.end ?? now)
            return Button {
                if !current {
                    nowPlaying.play(channel)
                }
                close()
            } label: {
                VStack(alignment: .leading, spacing: 0) {
                    picture(channel, current: current)
                        .frame(width: 400, height: 225)
                        .clipped()
                    VStack(alignment: .leading, spacing: 6) {
                        HStack(spacing: 10) {
                            Text(channel.displayNumber)
                                .font(.headline.monospacedDigit())
                            Text(channel.displayName)
                                .font(.headline)
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                        }
                        Text(airing?.title ?? "No listing")
                            .font(.title3.weight(.semibold))
                            .lineLimit(1)
                        if let airing {
                            AiringProgress(airing.progress(at: now))
                        }
                        Text(next.map { "Next \($0.start.formatted(date: .omitted, time: .shortened))  \($0.title)" } ?? " ")
                            .font(.callout)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                    .padding(.horizontal, 20)
                    .padding(.vertical, 16)
                    .frame(width: 400, alignment: .leading)
                }
            }
            .buttonStyle(.card)
            .focused($focus, equals: channel.id)
            .accessibilityLabel(spoken(channel, airing: airing, next: next, current: current))
            .accessibilityIdentifier("strip-\(channel.id)")
        }

        private func picture(_ channel: Channel, current: Bool) -> some View {
            ZStack {
                Tokens.ColorToken.surface1
                Text(channel.displayNumber)
                    .font(.system(size: 64, weight: .black))
                    .monospacedDigit()
                    .foregroundStyle(.white.opacity(0.14))
                if let url = store.api?.frameURL(channelID: channel.id, width: 480) {
                    AsyncImage(url: url.appending(queryItems: [URLQueryItem(name: "t", value: String(minute))])) { phase in
                        if let image = phase.image {
                            image.resizable().scaledToFill()
                        }
                    }
                }
                if current {
                    LiveDot("Watching")
                        .padding(.horizontal, 12)
                        .padding(.vertical, 6)
                        .background(.black.opacity(0.6), in: .capsule)
                        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                        .padding(12)
                }
            }
            .accessibilityHidden(true)
        }

        private func spoken(_ channel: Channel, airing: Airing?, next: Airing?, current: Bool) -> String {
            var parts = ["\(channel.displayNumber) \(channel.displayName)", airing?.title ?? "No listing"]
            if let next {
                parts.append("next \(next.title)")
            }
            if current {
                parts.append("watching")
            }
            return parts.joined(separator: ", ")
        }

        /// A card the remote rests on starts its picture, when that costs no tuner.
        private func warm(_ id: Int64?) {
            warming?.cancel()
            guard let id, id != nowPlaying.channel?.id else { return }
            warming = Task {
                try? await Task.sleep(for: .milliseconds(400))
                guard !Task.isCancelled, let api = store.api else { return }
                _ = await api.warm(channelID: id, caps: Capabilities.current(), prefs: store.prefs)
            }
        }
    }
#endif

#if os(tvOS)
    /// The channel the player stepped away from, still playing beside the guide's header.
    /// Select (or Back anywhere in the guide) brings it back full screen.
    struct GuideLivePane: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        let channel: Channel

        var body: some View {
            Button { nowPlaying.expanded = true } label: {
                ZStack(alignment: .bottomLeading) {
                    PlayerLayerBox(player: nowPlaying.live.player, gravity: .resizeAspect)
                        .background(.black)
                    LinearGradient(colors: [.clear, .black.opacity(0.8)], startPoint: .center, endPoint: .bottom)
                    VStack(alignment: .leading, spacing: 2) {
                        LiveDot()
                        Text("\(channel.displayNumber)  \(store.index.on(channel.id, at: store.now)?.title ?? channel.displayName)")
                            .font(.caption.weight(.semibold))
                            .lineLimit(1)
                    }
                    .padding(10)
                }
                .frame(width: 256, height: 144)
            }
            .buttonStyle(.card)
            .accessibilityLabel("Playing \(channel.displayNumber) \(channel.displayName)")
            .accessibilityHint("Back to full screen")
            .accessibilityIdentifier("guide-live-pane")
        }
    }
#endif
