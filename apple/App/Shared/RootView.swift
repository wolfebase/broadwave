import BroadwaveKit
import BroadwaveUI
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
    @State private var showSetup = false

    var body: some View {
        Group {
            if !store.connected {
                ConnectView()
            } else if showSetup || store.presentSetup {
                SetupWizard {
                    showSetup = false
                    store.presentSetup = false
                }
            } else {
                tabs
            }
        }
        .environment(nowPlaying)
        .background(Tokens.ColorToken.canvas.ignoresSafeArea())
        .safeAreaInset(edge: .top, spacing: 0) {
            arrivalBanner
        }
        .task(id: store.connected) {
            guard store.connected else { return }
            store.socket?.announce(name: ScreenIdentity.name, kind: ScreenIdentity.kind)
            #if DEBUG
                if UserDefaults.standard.bool(forKey: "BroadwaveMultiviewTest") {
                    return
                }
                if UserDefaults.standard.string(forKey: "BroadwaveSetup") != nil {
                    showSetup = true
                    return
                }
            #endif
            if let values = try? await store.api?.settings(), values["needsSetup"] == "1" {
                showSetup = true
            }
        }
        .onOpenURL(perform: open)
        #if DEBUG
            .task {
                if UserDefaults.standard.bool(forKey: "BroadwaveMultiviewTest") {
                    UserDefaults.standard.set("2up", forKey: "BroadwaveMultiviewLayout")
                    store.previewLineup([
                        previewChannel(id: 1, number: "4.1", name: "One"),
                        previewChannel(id: 3, number: "9.1", name: "Two"),
                    ])
                    nowPlaying.watchTogether(store.channels)
                    return
                }
                if let name = UserDefaults.standard.string(forKey: "BroadwaveTab") {
                    switch name {
                    case "guide": tab = .guide
                    case "search": tab = .search
                    case "sports": tab = .sports
                    case "recordings": tab = .recordings
                    case "settings": tab = .settings
                    default: tab = .home
                    }
                }
                // Simulator testing: -BroadwaveWatch <channel id>, or -BroadwaveMultiview 1,3.
                #if os(iOS)
                    if UserDefaults.standard.bool(forKey: "BroadwaveLandscape") {
                        try? await Task.sleep(for: .milliseconds(600))
                        if let scene = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene }).first {
                            scene.requestGeometryUpdate(.iOS(interfaceOrientations: .landscapeRight)) { _ in }
                        }
                    }
                #endif
                if let raw = UserDefaults.standard.string(forKey: "BroadwaveMultiview"), !raw.isEmpty {
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
                let id = UserDefaults.standard.integer(forKey: "BroadwaveWatch")
                if id > 0 {
                    open(URL(string: "broadwave://watch/\(id)")!)
                }
            }
        #endif
    }

    #if DEBUG
        private func previewChannel(id: Int64, number: String, name: String) -> Channel {
            Channel(
                id: id, deviceId: "preview", guideNumber: number, guideName: name,
                displayNumber: number, displayName: name,
                hd: true, favorite: false, enabled: true, hidden: false, present: true
            )
        }
    #endif

    /// broadwave://watch/<channel id>, broadwave://guide, broadwave://sports — for widgets, Top Shelf, and Siri.
    private func open(_ url: URL) {
        guard url.scheme == "broadwave" else { return }
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
        .safeAreaInset(edge: .top, spacing: 0) {
            arrivalBanner
        }
    }

    @ViewBuilder
    private var arrivalBanner: some View {
        if store.connected, let notice = store.homeNotice, !notice.isEmpty {
            HomeArrivalBanner(notice: notice) {
                #if os(tvOS)
                    nowPlaying.stop()
                #else
                    nowPlaying.expanded = false
                #endif
                tab = .settings
            } onDismiss: {
                store.dismissHome()
            }
        }
    }
}

/// One line when a tuner, screen, or server shows up after the house is already known.
struct HomeArrivalBanner: View {
    var notice: String
    var onOpen: () -> Void
    var onDismiss: () -> Void

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            Text(notice)
                .font(.subheadline)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityAddTraits(.updatesFrequently)
            Button("Your home", action: onOpen)
                .buttonStyle(.glass)
            Button("Not now", action: onDismiss)
                .buttonStyle(.glass)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(.regularMaterial)
        .accessibilityElement(children: .contain)
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
