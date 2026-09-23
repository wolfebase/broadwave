import OTAKit
import OTAUI
import SwiftUI

/// Which channel is playing, and whether the player is full screen or docked.
@MainActor
@Observable
final class NowPlaying {
    var channel: Channel?
    var expanded = false
    /// Channel ids playing side by side. Empty means one channel.
    var together: [Int64] = []

    func play(_ channel: Channel) {
        together = []
        self.channel = channel
        expanded = true
    }

    func watchTogether(_ channels: [Channel]) {
        var seen = Set<Int64>()
        together = channels.map(\.id).filter { seen.insert($0).inserted }
        if channel == nil {
            channel = channels.first
        }
        expanded = true
    }

    func stop() {
        channel = nil
        together = []
        expanded = false
    }
}

enum AppTab: Hashable {
    case home, guide, search, sports, recordings, settings
}

struct RootView: View {
    @Environment(AppStore.self) private var store
    @State private var nowPlaying = NowPlaying()
    @State private var tab: AppTab = .home

    var body: some View {
        Group {
            if store.connected {
                tabs
            } else {
                ConnectView()
            }
        }
        .environment(nowPlaying)
        .background(Tokens.ColorToken.canvas.ignoresSafeArea())
        .onOpenURL(perform: open)
        #if DEBUG
            .task {
                if let name = UserDefaults.standard.string(forKey: "OTATab") {
                    switch name {
                    case "guide": tab = .guide
                    case "search": tab = .search
                    case "sports": tab = .sports
                    case "recordings": tab = .recordings
                    case "settings": tab = .settings
                    default: tab = .home
                    }
                }
                // Simulator testing: -OTAWatch <channel id>, or -OTAMultiview 1,3.
                if let raw = UserDefaults.standard.string(forKey: "OTAMultiview"), !raw.isEmpty {
                    if store.channels.isEmpty {
                        await store.refresh()
                    }
                    let channels = raw.split(separator: ",").compactMap { piece -> Channel? in
                        let id = Int64(piece.trimmingCharacters(in: .whitespaces)) ?? 0
                        return store.channels.first { $0.id == id }
                    }
                    if !channels.isEmpty {
                        nowPlaying.watchTogether(channels)
                    }
                    return
                }
                let id = UserDefaults.standard.integer(forKey: "OTAWatch")
                if id > 0 {
                    open(URL(string: "waveguide://watch/\(id)")!)
                }
            }
        #endif
    }

    /// waveguide://watch/<channel id>, waveguide://guide, waveguide://sports — for widgets, Top Shelf, and Siri.
    private func open(_ url: URL) {
        guard url.scheme == "waveguide" else { return }
        switch url.host() {
        case "connect":
            let q = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems
            if let raw = q?.first(where: { $0.name == "url" })?.value, let server = URL(string: raw) {
                Task {
                    if let info = try? await APIClient(base: server).server() {
                        store.connect(FoundServer(id: info.id, name: info.name, url: server))
                    }
                }
            }
        case "watch":
            let id = Int64(url.lastPathComponent) ?? 0
            Task {
                if store.channels.isEmpty {
                    await store.refresh()
                }
                if let channel = store.channels.first(where: { $0.id == id }) {
                    nowPlaying.play(channel)
                }
            }
        case "guide": tab = .guide
        case "search": tab = .search
        case "sports": tab = .sports
        case "recordings": tab = .recordings
        default: tab = .home
        }
    }

    private var tabs: some View {
        TabView(selection: $tab) {
            Tab("Home", systemImage: "house.fill", value: AppTab.home) {
                NavigationStack { HomeView() }
            }
            Tab("Guide", systemImage: "square.grid.3x3.topleft.filled", value: AppTab.guide) {
                NavigationStack { GuideView() }
            }
            Tab("Search", systemImage: "magnifyingglass", value: AppTab.search) {
                NavigationStack { SearchView() }
            }
            Tab("Sports", systemImage: "sportscourt.fill", value: AppTab.sports) {
                NavigationStack { SportsView() }
            }
            Tab("Recordings", systemImage: "play.rectangle.on.rectangle.fill", value: AppTab.recordings) {
                NavigationStack { RecordingsView() }
            }
            Tab("Settings", systemImage: "gearshape.fill", value: AppTab.settings) {
                NavigationStack { SettingsView() }
            }
        }
        .tabViewStyle(.sidebarAdaptable)
        #if os(iOS)
            .tabBarMinimizeBehavior(.onScrollDown)
            .tabViewBottomAccessory(isEnabled: nowPlaying.channel != nil && !nowPlaying.expanded) {
                MiniPlayerBar()
            }
            .fullScreenCover(isPresented: Binding(get: { nowPlaying.expanded && nowPlaying.channel != nil }, set: { nowPlaying.expanded = $0 })) {
                playingCover
            }
        #else
            .fullScreenCover(isPresented: Binding(get: { nowPlaying.channel != nil }, set: {
                if !$0 {
                    nowPlaying.stop()
                }
            })) {
                playingCover
            }
        #endif
            .refreshable { await store.refresh() }
    }

    private var playingCover: some View {
        Group {
            if nowPlaying.together.isEmpty {
                PlayerScreen()
            } else {
                MultiviewScreen()
            }
        }
        .environment(store)
        .environment(nowPlaying)
    }
}

#if os(iOS)
    /// Live TV keeps a place at the bottom while you browse.
    struct MiniPlayerBar: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying

        var body: some View {
            if let channel = nowPlaying.channel {
                HStack(spacing: 12) {
                    LiveDot("")
                    VStack(alignment: .leading, spacing: 0) {
                        Text(store.index.on(channel.id, at: store.now)?.title ?? channel.displayName)
                            .font(.subheadline.weight(.semibold))
                            .lineLimit(1)
                        Text("\(channel.displayNumber) \(channel.displayName)")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    Button("Close", systemImage: "xmark") { nowPlaying.stop() }
                        .labelStyle(.iconOnly)
                }
                .padding(.horizontal, 16)
                .contentShape(.rect)
                .onTapGesture { nowPlaying.expanded = true }
            }
        }
    }
#endif
