import BroadwaveKit
import BroadwaveUI
import SwiftUI

struct HomeView: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @Environment(LibraryFilter.self) private var library
    #if os(iOS)
        @Environment(\.horizontalSizeClass) private var width
        @Environment(SettingsRoute.self) private var settingsRoute
    #endif
    @State private var saved = SavedMultiview.load()
    @State private var teams: [TeamFollow] = []
    /// iPad and Apple TV cover the picture. A push would keep the sidebar on it.
    @State private var playingRecording: Recording?
    /// A later program's details, from the Tonight shelf.
    @State private var details: GuideView.Selection?
    #if os(tvOS)
        @Environment(\.tvSelectedTab) private var tvSelectedTab
        @FocusState private var watch: Bool
        @FocusState private var emptyHome: Bool
        @State private var loadedOnce = false
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
                #if os(tvOS)
                    TVPageTitle(String(localized: "Home"))
                        .padding(.horizontal)
                #endif
                #if os(tvOS)
                    if let (channel, airing) = store.featured() {
                        Hero(channel: channel, airing: airing, watchFocused: $watch)
                    }
                #else
                    if let (channel, airing) = touchHero {
                        TouchHero(channel: channel, airing: airing)
                    }
                #endif
                let live = store.channels.compactMap { c -> (Channel, Airing)? in
                    guard let a = store.index.on(c.id, at: store.now) else { return nil }
                    return (c, a)
                }
                .sorted { $0.0.favorite && !$1.0.favorite }
                if !live.isEmpty {
                    Shelf("On now") {
                        ForEach(live, id: \.0.id) { channel, airing in
                            Button { nowPlaying.play(channel) } label: {
                                let art = store.artURL(airing, width: 640)
                                NowCard(
                                    channel: channel,
                                    airing: airing,
                                    now: store.now,
                                    art: art,
                                    frame: art == nil ? store.api?.frameURL(channelID: channel.id, width: 480, listed: store.frameIDs) : nil
                                )
                                .playerZoomSource(channel.id)
                            }
                            .cardButton()
                            .contextMenu { ChannelActions(channel: channel, airing: airing) }
                        }
                    }
                }
                #if os(iOS)
                    tonightShelf
                #endif
                let games = homeGames
                let yours = games.filter { WidgetFeed.followed($0.1, teams) }
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
                                #if os(tvOS)
                                    Button("Watch together", systemImage: "rectangle.split.2x2") {
                                        nowPlaying.watchTogether(liveGames.prefix(4).map(\.0))
                                    }
                                    .buttonStyle(TVChipStyle())
                                #else
                                    Button("Watch together") {
                                        nowPlaying.watchTogether(liveGames.prefix(4).map(\.0))
                                    }
                                    .buttonStyle(.bordered)
                                #endif
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
                                let ids = set.channels
                                let layout = set.layout
                                Task {
                                    await openSavedMultiview(ids: ids, layout: layout, store: store, nowPlaying: nowPlaying)
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
                let recent = store.recordings.filter { !$0.isRecording && !$0.isMissing }.prefix(12)
                if !recent.isEmpty {
                    Shelf("Recently recorded") {
                        ForEach(Array(recent)) { rec in
                            recentRecording(rec)
                        }
                    }
                }
                if store.featured() == nil, !store.loading, saved.isEmpty, store.recordings.isEmpty {
                    let empty = ContentUnavailableView(
                        "No channels yet",
                        systemImage: "tv",
                        description: Text("Add a tuner or scan for channels.")
                    )
                    .padding(.top, 60)
                    #if os(tvOS)
                        // A focusable message is not enough: the sidebar stays open until a button in the page has focus.
                        Button(action: {}, label: {
                            empty.frame(maxWidth: .infinity)
                        })
                        .buttonStyle(FocusPlainStyle())
                        .focused($emptyHome)
                        .accessibilityRemoveTraits(.isButton)
                        .accessibilityIdentifier("home-empty")
                    #else
                        empty
                    #endif
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
        .fullScreenCover(item: $playingRecording) { recording in
            RecordingCover {
                RecordingPlayerScreen(recording: recording).environment(store)
            }
        }
        .onChange(of: playingRecording != nil) { _, up in library.playing = up }
        .sheet(item: $details) { sel in
            ProgramSheet(channel: sel.channel, airing: sel.airing)
            #if os(iOS)
                .presentationDetents([.medium, .large])
            #endif
        }
        .onChange(of: library.closeToken) { _, _ in playingRecording = nil }
        .navigationTitle("Home")
        #if os(tvOS)
            .tvPageChrome()
        #endif
        #if os(iOS)
        .toolbarTitleDisplayMode(.inlineLarge)
        .toolbar {
            if usesPhoneTabs(width) {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Settings", systemImage: "gearshape") {
                        settingsRoute.show(on: .home)
                    }
                    .accessibilityIdentifier("home-settings")
                }
            }
        }
        #endif
        .overlay {
            if store.channels.isEmpty, store.loading {
                ProgressView()
                    .accessibilityLabel("Loading")
            }
        }
        #if os(tvOS)
        .modifier(HomeDefaultFocus(hasHero: store.featured() != nil, watch: $watch, empty: $emptyHome))
        .onAppear { claimHomeFocus() }
        .onChange(of: tvSelectedTab) { _, _ in claimHomeFocus() }
        .onChange(of: store.channels.isEmpty) { _, _ in claimHomeFocus() }
        .task(id: store.loading) {
            // Only the first load: every server event reloads, and focus must
            // not jump back to Watch while someone is on another shelf. The
            // sidebar takes focus on the same turn the page appears and
            // clears a focus set then.
            guard !store.loading, !loadedOnce else { return }
            loadedOnce = true
            try? await Task.sleep(for: .milliseconds(400))
            claimHomeFocus()
        }
        #endif
        .onAppear { saved = SavedMultiview.load() }
    }

    /// Games only, each once, as the Sports pages list them. A sitcom called The Game is not one.
    private var homeGames: [(Channel, Airing)] {
        SportsBoard.games(store.sports()).map { ($0.channel, $0.airing) }
    }

    #if os(iOS)
        /// Something worth watching now: not a show in its last minutes, not paid programming.
        private var touchHero: (Channel, Airing?)? {
            HomeBoard.hero(store.channels.map { ($0, store.index.on($0.id, at: store.now)) }, now: store.now)
        }

        /// Prime time tonight on the channels with art, favorites first.
        @ViewBuilder
        private var tonightShelf: some View {
            let picks = tonight
            if picks.count >= 3 {
                Shelf("Tonight") {
                    ForEach(picks, id: \.1.id) { channel, airing in
                        Button { details = GuideView.Selection(channel: channel, airing: airing) } label: {
                            LaterCard(channel: channel, airing: airing)
                        }
                        .cardButton()
                        .accessibilityIdentifier("home-tonight")
                    }
                }
            }
        }

        private var tonight: [(Channel, Airing)] {
            let cal = Calendar.current
            var parts = cal.dateComponents([.year, .month, .day], from: store.now)
            parts.hour = 20
            parts.minute = 5
            guard let prime = cal.date(from: parts), prime > store.now else { return [] }
            let list = store.channels.compactMap { c -> (Channel, Airing)? in
                guard let a = store.index.on(c.id, at: prime), a.imageUrl?.isEmpty == false else { return nil }
                return (c, a)
            }
            var seen = Set<String>()
            return list
                .sorted { ($0.0.favorite ? 0 : 1, $0.1.start) < ($1.0.favorite ? 0 : 1, $1.1.start) }
                .filter { seen.insert($0.1.title.lowercased()).inserted }
                .prefix(12)
                .map(\.self)
        }
    #endif

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

    /// iPad and Apple TV play a recording over the whole screen. iPhone pushes it.
    @ViewBuilder
    private func recentRecording(_ rec: Recording) -> some View {
        if recordingCovers {
            Button {
                playingRecording = rec
            } label: {
                RecordingCard(recording: rec)
            }
            .cardButton()
        } else {
            NavigationLink(value: rec) {
                RecordingCard(recording: rec)
            }
            .cardButton()
        }
    }

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
        let layout = airing.map { ArtLayout.choose(width: $0.imageWidth ?? 0, height: $0.imageHeight ?? 0, slot: 1400) } ?? "composed"
        let frame = store.api?.frameURL(channelID: channel.id, width: 1280, listed: store.frameIDs)
        ZStack(alignment: .bottomLeading) {
            if tvLiveFirst, layout != "bleed" {
                // A small picture (often the station's logo) would be blown up past its pixels.
                // The channel's live frame (or its number on a tint) fills the hero instead,
                // with the picture beside it at no more than its own size.
                HeroBackdrop(frame: frame, number: channel.displayNumber, tint: kind.color)
                    .overlay(alignment: .topTrailing) {
                        if let airing, let art {
                            ProgramPicture(url: art, width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0)
                                .frame(maxWidth: 420, maxHeight: 236)
                                .clipShape(.rect(cornerRadius: Tokens.Radius.lg))
                                .shadow(color: .black.opacity(0.5), radius: 20, y: 10)
                                .padding(28)
                                .accessibilityHidden(true)
                        }
                    }
            } else if let airing, let art {
                // The art fills the hero's space; its own size must never widen the card.
                Color.clear.overlay {
                    HeroArt(url: art, layout: layout)
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
                HeroBackdrop(frame: frame, number: channel.displayNumber, tint: kind.color)
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
                    #if os(tvOS)
                        // The one chip style of the Apple TV pages: white when focused, glass when not.
                        Button("Watch", systemImage: "play.fill") { nowPlaying.play(channel) }
                            .buttonStyle(TVChipStyle())
                            .focused(watchFocused)
                        Button(store.activeRecording(on: channel) == nil ? "Record" : "Recording", systemImage: "record.circle") {
                            Task { await store.toggleRecord(channel) }
                        }
                        .buttonStyle(TVChipStyle(selected: store.activeRecording(on: channel) != nil))
                    #else
                        Button("Watch", systemImage: "play.fill") { nowPlaying.play(channel) }
                            .buttonStyle(.glassProminent)
                            .controlSize(.large)
                        Button(store.activeRecording(on: channel) == nil ? "Record" : "Recording", systemImage: "record.circle") {
                            Task { await store.toggleRecord(channel) }
                        }
                        .buttonStyle(.glass)
                        .controlSize(.large)
                    #endif
                }
            }
            .padding(28)
        }
        .frame(minHeight: 380)
        .playerZoomSource(channel.id)
        .padding(.horizontal)
    }

    private var tvLiveFirst: Bool {
        #if os(tvOS)
            true
        #else
            false
        #endif
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
                    Text("AT").font(.caption2.weight(.bold)).foregroundStyle(.secondary)
                    Text(b).font(.headline.weight(.heavy)).lineLimit(1)
                } else {
                    Text(airing.subtitle ?? airing.title).font(.headline.weight(.heavy)).lineLimit(3)
                }
                Spacer(minLength: 0)
                if let score, !score.isEmpty {
                    // Each new point rolls in, as a scoreboard's digits do.
                    Text(score).font(.subheadline.weight(.semibold)).monospacedDigit()
                        .contentTransition(.numericText())
                        .animation(.snappy, value: score)
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
        .accessibilityElement(children: .combine)
        .accessibilityLabel(spoken)
    }

    private var spoken: String {
        if let (a, b) = airing.matchup {
            return "\(a) at \(b), \(channel.displayNumber) \(channel.displayName)"
        }
        let title = airing.subtitle ?? airing.title
        return "\(title), \(channel.displayNumber) \(channel.displayName)"
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
        .accessibilityElement(children: .combine)
        .accessibilityLabel(spoken)
    }

    private var spoken: String {
        let when = recording.subtitle ?? recording.startedAt.formatted(date: .abbreviated, time: .omitted)
        if when == recording.title {
            return recording.title
        }
        return "\(recording.title), \(when)"
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
                Task { try? await store.recordSeries(airing) }
            }
        }
        Button(channel.favorite ? "Remove favorite" : "Add favorite", systemImage: channel.favorite ? "star.slash" : "star") {
            Task { await store.toggleFavorite(channel) }
        }
        Button("Hide channel", systemImage: "eye.slash") {
            Task {
                do {
                    _ = try await store.editChannel(channel.id, ChannelPatch(hidden: true))
                } catch {
                    store.error = error.localizedDescription
                }
            }
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

#if os(tvOS)
    /// The system button style draws a card behind its label. This one only takes focus.
    private struct FocusPlainStyle: ButtonStyle {
        func makeBody(configuration: Configuration) -> some View {
            configuration.label
        }
    }

    /// One default: Watch when a channel is on screen, otherwise the empty message.
    private struct HomeDefaultFocus: ViewModifier {
        var hasHero: Bool
        var watch: FocusState<Bool>.Binding
        var empty: FocusState<Bool>.Binding

        func body(content: Content) -> some View {
            if hasHero {
                content.defaultFocus(watch, true)
            } else {
                content.defaultFocus(empty, true)
            }
        }
    }
#endif

struct PressCardStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .frame(width: cardWidth)
            .scaleEffect(configuration.isPressed ? 0.97 : 1)
            .animation(Tokens.Motion.spring, value: configuration.isPressed)
    }
}

#if os(iOS)
    /// Home's lead on iPhone and iPad: the program's art fills the card edge to edge, the
    /// words sit on a dark fade at its foot. A small or tall picture is shown whole over a
    /// blurred copy of itself, never stretched or left with a band of tint above it.
    struct TouchHero: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        @Environment(\.horizontalSizeClass) private var width
        let channel: Channel
        let airing: Airing?

        var body: some View {
            let kind = airing?.kind ?? .other
            ZStack(alignment: .bottomLeading) {
                Color.clear
                    .overlay { backdrop }
                    .overlay {
                        LinearGradient(
                            stops: [.init(color: .clear, location: 0.3), .init(color: .black.opacity(0.92), location: 0.95)],
                            startPoint: .top,
                            endPoint: .bottom
                        )
                    }
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 10) {
                    HStack(spacing: 10) {
                        // On a dark chip: red on bright art did not read.
                        LiveDot("Live now")
                            .padding(.horizontal, 8)
                            .padding(.vertical, 4)
                            .background(.black.opacity(0.7), in: .capsule)
                        if let label = kindLabel(kind) {
                            Text(label).font(.caption.weight(.bold)).tracking(1).foregroundStyle(.white.opacity(0.8))
                        }
                    }
                    Text(airing.map(GameWords.title) ?? channel.displayName)
                        .font(.system(.largeTitle, weight: .heavy))
                        .lineLimit(2)
                        .minimumScaleFactor(0.7)
                    if let sub = airing?.subtitle, airing?.matchup == nil {
                        Text(sub).font(.title3).foregroundStyle(.white.opacity(0.8)).lineLimit(1)
                    }
                    HStack(spacing: 12) {
                        ChannelBadge(channel)
                            .fixedSize()
                        if let airing {
                            AiringProgress(airing.progress(at: store.now), color: kind.color).frame(minWidth: 40, maxWidth: 160)
                            Text(airing.minutesLeft(at: store.now)).font(.footnote).foregroundStyle(.white.opacity(0.8)).fixedSize()
                        }
                    }
                    HStack(spacing: 12) {
                        Button("Watch", systemImage: "play.fill") { nowPlaying.play(channel) }
                            .buttonStyle(.glassProminent)
                            .controlSize(.large)
                            .accessibilityIdentifier("home-watch")
                        Button(store.activeRecording(on: channel) == nil ? "Record" : "Recording", systemImage: "record.circle") {
                            Task { await store.toggleRecord(channel) }
                        }
                        .buttonStyle(.glass)
                        .controlSize(.large)
                    }
                    .padding(.top, 4)
                }
                .foregroundStyle(.white)
                .padding(20)
            }
            .aspectRatio(width == .regular ? 16 / 7 : 4 / 5, contentMode: .fit)
            .frame(maxWidth: .infinity)
            .background(Tokens.ColorToken.surface1)
            .clipShape(.rect(cornerRadius: Tokens.Radius.xl))
            .playerZoomSource(channel.id)
            .padding(.horizontal)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("home-hero")
        }

        /// The category, except Sports on a show that only talks about it.
        private func kindLabel(_ kind: BroadwaveKit.Category) -> String? {
            guard kind != .other else { return nil }
            if kind == .sports, let airing, !SportsBoard.isGame(airing) {
                return nil
            }
            return kind.label.uppercased()
        }

        @ViewBuilder
        private var backdrop: some View {
            if let airing, let art = store.artURL(airing, width: 1200) {
                let layout = ArtLayout.choose(width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0, slot: 1200)
                AsyncImage(url: art) { phase in
                    if let image = phase.image {
                        if layout == "bleed" {
                            image.resizable().scaledToFill()
                        } else {
                            ZStack(alignment: .top) {
                                image.resizable().scaledToFill().blur(radius: 36).opacity(0.7)
                                image.resizable().scaledToFit()
                                    .clipShape(.rect(cornerRadius: Tokens.Radius.lg))
                                    .padding(.horizontal, 28)
                                    .padding(.top, 28)
                                    .frame(maxHeight: 260, alignment: .top)
                            }
                        }
                    } else {
                        tint
                    }
                }
            } else if let frame = store.api?.frameURL(channelID: channel.id, width: 960, listed: store.frameIDs) {
                AsyncImage(url: frame) { phase in
                    if let image = phase.image {
                        image.resizable().scaledToFill()
                    } else {
                        tint
                    }
                }
            } else {
                tint
            }
        }

        private var tint: some View {
            ZStack(alignment: .topTrailing) {
                RadialGradient(colors: [(airing?.kind ?? .other).color.opacity(0.5), .clear], center: .topTrailing, startRadius: 0, endRadius: 420)
                Text(channel.displayNumber)
                    .font(.system(size: 160, weight: .black))
                    .monospacedDigit()
                    .foregroundStyle(.white.opacity(0.06))
                    .padding(20)
            }
        }
    }

    /// A program later today: its art, the time, the title, and the channel.
    struct LaterCard: View {
        @Environment(AppStore.self) private var store
        let channel: Channel
        let airing: Airing

        var body: some View {
            VStack(alignment: .leading, spacing: 6) {
                Group {
                    if let art = store.artURL(airing, width: 640) {
                        ProgramPicture(url: art, width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0, hero: false)
                    } else {
                        Tokens.ColorToken.surface2
                    }
                }
                .frame(width: cardWidth, height: cardWidth * 9 / 16)
                .clipShape(.rect(cornerRadius: Tokens.Radius.md))
                Text(airing.start.formatted(date: .omitted, time: .shortened))
                    .font(.caption.weight(.bold))
                    .foregroundStyle(.secondary)
                Text(airing.title).font(.subheadline.weight(.semibold)).lineLimit(1)
                Text("\(channel.displayNumber) \(channel.displayName)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            .frame(width: cardWidth, alignment: .leading)
            .accessibilityElement(children: .combine)
            .accessibilityLabel("\(airing.title), \(airing.start.formatted(date: .omitted, time: .shortened)), \(channel.displayNumber) \(channel.displayName)")
            .accessibilityHint("Shows the details")
        }
    }
#endif
