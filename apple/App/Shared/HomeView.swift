import BroadwaveKit
import BroadwaveUI
import SwiftUI

struct HomeView: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @State private var saved = SavedMultiview.load()
    @State private var teams: [TeamFollow] = []
    #if os(tvOS)
        @Environment(\.tvSelectedTab) private var tvSelectedTab
        @FocusState private var watch: Bool
        @FocusState private var emptyHome: Bool
    #endif

    var body: some View {
        ScrollViewReader { proxy in
            homeStack
                .task(id: teams.count) { await scrollForScreenshot(proxy) }
        }
    }

    private var homeStack: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 36) {
                if let (channel, airing) = store.featured() {
                    #if os(tvOS)
                        Hero(channel: channel, airing: airing, watchFocused: $watch)
                    #else
                        Hero(channel: channel, airing: airing)
                    #endif
                }
                let live = store.channels.compactMap { c -> (Channel, Airing)? in
                    guard let a = store.index.on(c.id, at: store.now) else { return nil }
                    return (c, a)
                }
                .sorted { $0.0.favorite && !$1.0.favorite }
                Shelf("On now") {
                    ForEach(live, id: \.0.id) { channel, airing in
                        Button { nowPlaying.play(channel) } label: {
                            let art = store.artURL(airing, width: 640)
                            NowCard(
                                channel: channel,
                                airing: airing,
                                now: store.now,
                                art: art,
                                frame: art == nil ? store.api?.frameURL(channelID: channel.id, width: 480) : nil
                            )
                        }
                        .cardButton()
                        .contextMenu { ChannelActions(channel: channel, airing: airing) }
                    }
                }
                let games = store.sports()
                let yours = games.filter { pair in
                    teams.contains { team in
                        let name = team.short.flatMap { $0.isEmpty ? nil : $0 } ?? team.name
                        return name.count >= 4 && (pair.1.title.localizedCaseInsensitiveContains(name) || (pair.1.subtitle ?? "").localizedCaseInsensitiveContains(name))
                    }
                }
                if !yours.isEmpty {
                    Shelf("Your teams") {
                        ForEach(yours.prefix(12), id: \.1.id) { channel, airing in
                            Button {
                                if airing.isOn(at: store.now) {
                                    nowPlaying.play(channel)
                                }
                            } label: {
                                GameCard(channel: channel, airing: airing, now: store.now)
                            }
                            .cardButton()
                        }
                    }
                    .id("teams")
                }
                if !games.isEmpty {
                    let liveGames = games.filter { $0.1.isOn(at: store.now) }
                    VStack(alignment: .leading, spacing: 12) {
                        HStack {
                            Text("Sports").font(.title2.weight(.bold))
                            Spacer()
                            if liveGames.count >= 2 {
                                Button("Watch together") {
                                    nowPlaying.watchTogether(liveGames.prefix(4).map(\.0))
                                }
                                .buttonStyle(.bordered)
                            }
                        }
                        .padding(.horizontal)
                        ScrollView(.horizontal) {
                            LazyHStack(alignment: .top, spacing: 14) {
                                ForEach(games.prefix(16), id: \.1.id) { channel, airing in
                                    Button {
                                        if airing.isOn(at: store.now) {
                                            nowPlaying.play(channel)
                                        }
                                    } label: {
                                        GameCard(channel: channel, airing: airing, now: store.now)
                                    }
                                    .cardButton()
                                }
                            }
                            .padding(.horizontal)
                        }
                        .scrollIndicators(.hidden)
                        .scrollClipDisabled()
                    }
                }
                if !saved.isEmpty {
                    Shelf("Saved sets") {
                        ForEach(saved) { set in
                            Button {
                                let channels = set.channels.compactMap { id in store.channels.first { $0.id == id } }
                                if !channels.isEmpty {
                                    nowPlaying.watchTogether(channels, layout: set.layout)
                                }
                            } label: {
                                Text(set.name)
                                    .font(.headline.weight(.semibold))
                                    .frame(width: cardWidth, height: 88)
                            }
                            .cardButton()
                        }
                    }
                }
                let recent = store.recordings.filter { !$0.isRecording }.prefix(12)
                if !recent.isEmpty {
                    Shelf("Recently recorded") {
                        ForEach(Array(recent)) { rec in
                            NavigationLink(value: rec) {
                                RecordingCard(recording: rec)
                            }
                            .cardButton()
                        }
                    }
                }
            }
            .padding(.vertical, 20)
        }
        .task {
            if let api = store.api {
                teams = await (try? api.teams()) ?? []
            }
        }
        .navigationDestination(for: Recording.self) { RecordingPlayerScreen(recording: $0) }
        .navigationTitle("Home")
        #if os(iOS)
            .toolbarTitleDisplayMode(.inlineLarge)
        #endif
            .overlay {
                if store.channels.isEmpty, store.loading {
                    ProgressView()
                }
            }
        #if os(tvOS)
            .background {
                if store.featured() == nil {
                    Color.clear
                        .frame(width: 20, height: 20)
                        .focusable()
                        .focused($emptyHome)
                        .accessibilityHidden(true)
                }
            }
            .onAppear { claimHomeFocus() }
            .onChange(of: tvSelectedTab) { _, _ in claimHomeFocus() }
            .onChange(of: store.channels.isEmpty) { _, _ in claimHomeFocus() }
        #endif
            .onAppear { saved = SavedMultiview.load() }
    }

    #if os(tvOS)
        /// Guide and Settings put focus in the page, which collapses the sidebar. Home does not, unless asked.
        private func claimHomeFocus() {
            guard tvSelectedTab == .home else { return }
            if store.featured() == nil {
                emptyHome = true
            } else {
                watch = true
            }
        }
    #endif

    /// `-BroadwaveScroll teams` on a debug launch. Off-screen shelves cannot be reached with a click while another simulator window is in front.
    private func scrollForScreenshot(_ proxy: ScrollViewProxy) async {
        #if DEBUG
            guard UserDefaults.standard.string(forKey: "BroadwaveScroll") == "teams", !teams.isEmpty else { return }
            try? await Task.sleep(for: .milliseconds(500))
            proxy.scrollTo("teams", anchor: .top)
        #endif
    }
}

struct Hero: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    let channel: Channel
    let airing: Airing?
    #if os(tvOS)
        var watchFocused: FocusState<Bool>.Binding
    #endif

    var body: some View {
        let kind = airing?.kind ?? .other
        let art = airing.flatMap { store.artURL($0, width: 1600) }
        ZStack(alignment: .bottomLeading) {
            if let airing, let art {
                // The art fills the hero's space; its own size must never widen the card.
                Color.clear.overlay {
                    HeroArt(url: art, layout: ArtLayout.choose(width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0, slot: 1400))
                }
                .clipShape(.rect(cornerRadius: Tokens.Radius.xl))
                RoundedRectangle(cornerRadius: Tokens.Radius.xl)
                    .fill(.clear)
                    .overlay(
                        RadialGradient(colors: [kind.color.opacity(0.6), .clear], center: .topTrailing, startRadius: 0, endRadius: 520)
                            .clipShape(.rect(cornerRadius: Tokens.Radius.xl))
                    )
                    .clipShape(.rect(cornerRadius: Tokens.Radius.xl))
            } else {
                HeroBackdrop(frame: store.api?.frameURL(channelID: channel.id, width: 1280), number: channel.displayNumber, tint: kind.color)
            }
            VStack(alignment: .leading, spacing: 14) {
                HStack(spacing: 12) {
                    LiveDot("Live now")
                    if kind != .other {
                        Text(kind.label.uppercased()).font(.caption.weight(.bold)).tracking(1).foregroundStyle(.secondary)
                    }
                }
                Text(airing?.title ?? channel.displayName)
                    .font(.system(.largeTitle, weight: .heavy))
                    .lineLimit(2)
                    .minimumScaleFactor(0.7)
                if let sub = airing?.subtitle {
                    Text(sub).font(.title3).foregroundStyle(.secondary).lineLimit(1)
                }
                HStack(spacing: 12) {
                    ChannelBadge(channel)
                    if let airing {
                        AiringProgress(airing.progress(at: store.now), color: kind.color).frame(maxWidth: 220)
                        Text(airing.minutesLeft(at: store.now)).font(.footnote).foregroundStyle(.secondary)
                    }
                }
                HStack(spacing: 12) {
                    Button("Watch", systemImage: "play.fill") { nowPlaying.play(channel) }
                        .buttonStyle(.glassProminent)
                        .controlSize(.large)
                    #if os(tvOS)
                        .focused(watchFocused)
                    #endif
                    Button(store.activeRecording(on: channel) == nil ? "Record" : "Recording", systemImage: "record.circle") {
                        Task { await store.toggleRecord(channel) }
                    }
                    .buttonStyle(.glass)
                    .controlSize(.large)
                }
            }
            .padding(28)
        }
        .frame(minHeight: 380)
        .padding(.horizontal)
    }
}

/// Program art behind the hero. A small or portrait picture is shown at its own size over a
/// blurred copy of itself, never stretched.
struct HeroArt: View {
    let url: URL
    let layout: String

    var body: some View {
        AsyncImage(url: url) { image in
            ZStack(alignment: .trailing) {
                image.resizable().scaledToFill()
                    .blur(radius: layout == "bleed" ? 0 : 40)
                    .opacity(layout == "bleed" ? 1 : 0.6)
                if layout != "bleed" {
                    image.resizable().scaledToFit()
                        .clipShape(.rect(cornerRadius: Tokens.Radius.lg))
                        .padding(28)
                }
            }
        } placeholder: {
            Color.clear
        }
        .overlay(
            LinearGradient(colors: [.black.opacity(0.92), .black.opacity(0.6), .black.opacity(0.1)], startPoint: .bottomLeading, endPoint: .topTrailing)
        )
        .clipShape(.rect(cornerRadius: Tokens.Radius.xl))
        .accessibilityHidden(true)
    }
}

/// The hero's background when the listing has no art. A tuned mux shows its preview.
/// A missing frame keeps the channel number on the empty card.
struct HeroBackdrop: View {
    var frame: URL?
    let number: String
    let tint: Color

    var body: some View {
        RoundedRectangle(cornerRadius: Tokens.Radius.xl)
            .fill(Tokens.ColorToken.surface1)
            .overlay {
                if let frame {
                    AsyncImage(url: frame) { phase in
                        if let image = phase.image {
                            image.resizable().scaledToFill()
                                .overlay(
                                    LinearGradient(
                                        colors: [.black.opacity(0.92), .black.opacity(0.6), .black.opacity(0.1)],
                                        startPoint: .bottomLeading,
                                        endPoint: .topTrailing
                                    )
                                )
                        } else {
                            fallback
                        }
                    }
                } else {
                    fallback
                }
            }
            .clipShape(.rect(cornerRadius: Tokens.Radius.xl))
            .accessibilityHidden(true)
    }

    private var fallback: some View {
        ZStack(alignment: .topTrailing) {
            RadialGradient(colors: [tint.opacity(0.6), .clear], center: .topTrailing, startRadius: 0, endRadius: 520)
            Text(number)
                .font(.system(size: 220, weight: .black))
                .monospacedDigit()
                .foregroundStyle(.white.opacity(0.06))
                .offset(x: 20, y: -40)
        }
        .clipped()
    }
}

struct Shelf<Content: View>: View {
    let title: String
    @ViewBuilder var content: Content

    init(_ title: String, @ViewBuilder content: () -> Content) {
        self.title = title
        self.content = content()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title).font(.title2.weight(.bold)).padding(.horizontal)
            ScrollView(.horizontal) {
                LazyHStack(alignment: .top, spacing: 14) { content }
                    .padding(.horizontal)
            }
            .scrollIndicators(.hidden)
            .scrollClipDisabled()
        }
    }
}

struct GameCard: View {
    @Environment(AppStore.self) private var store
    let channel: Channel
    let airing: Airing
    let now: Date
    var score: String?

    var body: some View {
        let art = store.artURL(airing, width: 640)
        VStack(alignment: .leading, spacing: 0) {
            if let art {
                ProgramPicture(url: art, width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0)
                    .frame(maxWidth: .infinity)
                    .frame(height: 112)
                    .clipped()
            }
            VStack(alignment: .leading, spacing: 8) {
                if airing.isOn(at: now) {
                    LiveDot()
                } else {
                    Text(airing.start.formatted(.dateTime.weekday(.abbreviated).hour().minute()))
                        .font(.caption.weight(.bold))
                        .foregroundStyle(.secondary)
                }
                if let (a, b) = airing.matchup {
                    Text(a).font(.headline.weight(.heavy)).lineLimit(1)
                    Text("AT").font(.caption2.weight(.bold)).foregroundStyle(.tertiary)
                    Text(b).font(.headline.weight(.heavy)).lineLimit(1)
                } else {
                    Text(airing.subtitle ?? airing.title).font(.headline.weight(.heavy)).lineLimit(3)
                }
                Spacer(minLength: 0)
                if let score, !score.isEmpty {
                    Text(score).font(.subheadline.weight(.semibold)).monospacedDigit()
                }
                ChannelBadge(channel)
            }
            .padding(16)
            .frame(maxWidth: .infinity, minHeight: art == nil ? 190 : 0, alignment: .topLeading)
        }
        .frame(width: cardWidth, alignment: .topLeading)
        .background(
            LinearGradient(colors: [Tokens.Category.sports.opacity(0.3), .clear], startPoint: .topLeading, endPoint: .bottomTrailing),
            in: .rect(cornerRadius: Tokens.Radius.lg)
        )
        .background(Tokens.ColorToken.surface1, in: .rect(cornerRadius: Tokens.Radius.lg))
        .clipShape(.rect(cornerRadius: Tokens.Radius.lg))
    }
}

struct RecordingCard: View {
    let recording: Recording

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            RecordingPoster(recording: recording)
                .frame(width: cardWidth, height: cardWidth * 9 / 16)
                .background(Tokens.ColorToken.surface2)
                .clipShape(.rect(cornerRadius: Tokens.Radius.md))
            Text(recording.title).font(.subheadline.weight(.semibold)).lineLimit(1)
            Text(recording.subtitle ?? recording.startedAt.formatted(date: .abbreviated, time: .omitted))
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
        }
        .frame(width: cardWidth)
    }
}

struct ChannelActions: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    let channel: Channel
    let airing: Airing?

    var body: some View {
        Button("Watch", systemImage: "play.fill") { nowPlaying.play(channel) }
        Button("Watch together", systemImage: "rectangle.split.2x1") {
            if let current = nowPlaying.channel, current.id != channel.id {
                nowPlaying.watchTogether([current, channel])
            } else {
                nowPlaying.watchTogether([channel])
            }
        }
        Button(store.activeRecording(on: channel) == nil ? "Record" : "Stop recording", systemImage: "record.circle") {
            Task { await store.toggleRecord(channel) }
        }
        if let airing {
            Button(airing.kind == .sports ? "Record every airing" : "Record series", systemImage: "repeat") {
                Task { await store.recordSeries(airing) }
            }
        }
        Button(channel.favorite ? "Remove favorite" : "Add favorite", systemImage: channel.favorite ? "star.slash" : "star") {
            Task { await store.toggleFavorite(channel) }
        }
    }
}

#if os(tvOS)
    let cardWidth: CGFloat = 380
#else
    let cardWidth: CGFloat = 250
#endif

extension View {
    /// Native focus lift and parallax on tvOS; a press-in on touch screens.
    @ViewBuilder
    func cardButton() -> some View {
        #if os(tvOS)
            frame(width: cardWidth).buttonStyle(.card)
        #else
            buttonStyle(PressCardStyle())
        #endif
    }
}

struct PressCardStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .frame(width: cardWidth)
            .scaleEffect(configuration.isPressed ? 0.97 : 1)
            .animation(Tokens.Motion.spring, value: configuration.isPressed)
    }
}
