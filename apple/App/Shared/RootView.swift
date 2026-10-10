import BroadwaveKit
import BroadwaveUI
import os
import SwiftUI
#if os(iOS)
    import UIKit
#endif

/// Which channel is playing, and whether the player is full screen or docked.
@MainActor
@Observable
final class NowPlaying {
    var channel: Channel?
    var expanded = false
    /// Channel ids playing side by side. Empty means one channel.
    var together: [Int64] = [] {
        // A tile that left must not bring its note back if the channel is added again.
        didSet { standIns.formIntersection(together) }
    }

    /// Layout Watch together or a saved set asked for. The multiview screen applies it once.
    var openedLayout: String?
    /// The channel the player showed when multiview opened from it. Menu goes back to it.
    var openedFrom: Channel?
    /// A short note over the picture, such as why another channel is playing.
    var note: String?
    /// Tile channel ids that stand in for an encrypted 3.0 station.
    var standIns: Set<Int64> = []
    /// The one-channel player. It outlives the full-screen player, so the mini
    /// player keeps the sound and its place in the room.
    let live = LivePlayer()
    /// SharePlay in a FaceTime call: which channel everyone watches.
    let sharePlay = SharePlay()
    /// Counts track changes that need a new watch. A master switches in place.
    var trackRestarts = 0
    /// Bumped to close the player's Start over recording before another channel plays.
    var closeStartOver = 0
    /// Recording players on screen. A docked live picture is not what is playing then.
    var recordingScreens: Set<UUID> = []

    /// Changes whenever the one-channel player needs a new watch.
    func watchKey(even: Bool) -> String {
        guard let channel, together.isEmpty else { return "none" }
        return "\(channel.id) \(trackRestarts) \(even) \(live.attempt)"
    }

    /// Starts the watch the key names, or stops the player when no single channel is on.
    func playLive(store: AppStore) async {
        guard let channel, together.isEmpty else {
            await live.stop()
            return
        }
        #if DEBUG
            // Layout checks must not take a tuner. -BroadwaveChrome YES skips the session.
            if UserDefaults.standard.bool(forKey: "BroadwaveChrome") {
                return
            }
        #endif
        await live.start(channel, store: store)
    }

    func play(_ channel: Channel, note: String? = nil) {
        together = []
        standIns = []
        openedLayout = nil
        openedFrom = nil
        self.channel = channel
        self.note = note
        expanded = true
    }

    func watchTogether(_ channels: [Channel], layout: String? = nil, standIns: Set<Int64> = []) {
        if together.isEmpty {
            openedFrom = channel
        }
        var seen = Set<Int64>()
        together = channels.map(\.id).filter { seen.insert($0).inserted }
        self.standIns = standIns.intersection(seen)
        if let layout, TileLayout(rawValue: layout) != nil {
            openedLayout = layout
        } else {
            openedLayout = TileLayout.fitting(together.count).rawValue
        }
        if channel == nil {
            channel = channels.first
        }
        expanded = true
    }

    func stop() {
        channel = nil
        note = nil
        together = []
        standIns = []
        openedLayout = nil
        openedFrom = nil
        expanded = false
    }
}

/// A multiview named by channel ids (a deep link, a saved set, or a launch
/// argument). A hidden half plays as the half on the guide. An encrypted 3.0
/// id plays its clear broadcast.
@MainActor
func openSavedMultiview(ids: [Int64], layout: String?, store: AppStore, nowPlaying: NowPlaying) async {
    if store.channels.isEmpty {
        await store.refresh()
    }
    var lineup = store.channels
    let missing = ids.contains { id in !lineup.contains(where: { $0.id == id }) }
    if missing, let all = try? await store.api?.allChannels() {
        lineup = all
    }
    let grid = ClearBroadcast.grid(ids: ids, visible: store.channels, lineup: lineup)
    guard !grid.channels.isEmpty else { return }
    nowPlaying.watchTogether(grid.channels, layout: layout, standIns: grid.standIns)
}

enum AppTab: Hashable {
    case home, guide, search, sports, recordings, settings
}

/// Which show Recordings is listing. Empty means every recording.
@MainActor
@Observable
final class LibraryFilter {
    var show = ""
    /// Bumped when a show row asks for the Recordings tab.
    var openToken = 0

    func open(_ title: String) {
        show = title.trimmingCharacters(in: .whitespacesAndNewlines)
        openToken += 1
    }

    /// A recording a link asked to play. The Recordings tab plays it and clears it.
    var recording: Int64?
    /// Whether a recording player covers the screen (Apple TV).
    var playing = false
    /// Bumped to close the recording player before a link opens another.
    var closeToken = 0

    func clear() {
        show = ""
    }
}

#if os(iOS)
    /// Six tabs leave Recordings under More, and More's bar stacks on the page.
    /// A compact iPhone keeps five tabs. Settings is pushed on the tab that is open.
    @MainActor
    func usesPhoneTabs(_ width: UserInterfaceSizeClass?) -> Bool {
        UIDevice.current.userInterfaceIdiom == .phone && width == .compact
    }

    @MainActor
    @Observable
    final class SettingsRoute {
        /// The tab whose stack should show Settings. Nil when it is not pushed.
        var on: AppTab?

        func show(on tab: AppTab) {
            on = tab
        }
    }

    private struct SettingsOnStack: ViewModifier {
        var tab: AppTab
        @Environment(SettingsRoute.self) private var route
        @Environment(\.horizontalSizeClass) private var width

        func body(content: Content) -> some View {
            if UIDevice.current.userInterfaceIdiom == .phone {
                content.navigationDestination(isPresented: presented) {
                    SettingsView()
                }
            } else {
                content
            }
        }

        private var presented: Binding<Bool> {
            Binding(
                get: { usesPhoneTabs(width) && route.on == tab },
                set: { show in
                    if !show, route.on == tab {
                        route.on = nil
                    }
                }
            )
        }
    }
#endif

#if os(tvOS)
    private struct TVSelectedTabKey: EnvironmentKey {
        static let defaultValue: AppTab = .home
    }

    extension EnvironmentValues {
        /// Which tab is selected. The tvOS sidebar stays open over a tab until that tab's content is focused.
        var tvSelectedTab: AppTab {
            get { self[TVSelectedTabKey.self] }
            set { self[TVSelectedTabKey.self] = newValue }
        }
    }
#endif

#if os(iOS)
    /// The iPhone tab bar floats over the page. Pull the page up so a row is not sliced behind it.
    private extension View {
        func phoneTabClearance() -> some View {
            modifier(PhoneTabClearance())
        }
    }

    private struct PhoneTabClearance: ViewModifier {
        @Environment(\.verticalSizeClass) private var height

        /// In landscape the safe area already ends above the tab bar, and 88 more points
        /// left the guide no room for a channel row.
        func body(content: Content) -> some View {
            content.padding(.bottom, UIDevice.current.userInterfaceIdiom == .phone && height != .compact ? 88 : 0)
        }
    }
#else
    private extension View {
        func phoneTabClearance() -> some View {
            self
        }
    }
#endif

struct RootView: View {
    @Environment(AppStore.self) private var store
    #if os(iOS)
        @Environment(\.horizontalSizeClass) private var width
        @State private var settingsRoute = SettingsRoute()
    #endif
    @State private var nowPlaying = NowPlaying()
    @State private var libraryFilter = LibraryFilter()
    @State private var tab: AppTab = .home
    @State private var showSetup = false
    @State private var dismissedUpdate = ""
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        Group {
            if !store.connected {
                ConnectView()
            } else if let message = Compatibility.gateApp(store.info, app: installedVersion) {
                IncompatibleServer(message: message)
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
        .environment(libraryFilter)
        #if os(iOS)
            .environment(settingsRoute)
        #endif
            .task(id: libraryFilter.openToken) {
                guard libraryFilter.openToken > 0 else { return }
                tab = .recordings
                #if os(iOS)
                    if phoneTabs {
                        settingsRoute.on = nil
                    }
                #endif
            }
            .background(Tokens.ColorToken.canvas.ignoresSafeArea())
            .task(id: store.channels.first?.id) {
                #if DEBUG
                    // -BroadwaveFakeGameAlert YES shows one alert for the first channel.
                    guard fakeGameAlert, let channel = store.channels.first else { return }
                    let number = channel.displayNumber.isEmpty ? channel.guideNumber : channel.displayNumber
                    print("broadwave fake game alert \(number) \(channel.id)")
                    fflush(stdout)
                    store.noteGameAlert(GameAlert(
                        id: "lane:start",
                        kind: "start",
                        gameId: "lane",
                        channelId: channel.id,
                        channel: number,
                        text: "Starting now: CHI at LV"
                    ))
                #endif
            }
            .safeAreaInset(edge: .top, spacing: 0) {
                if !bannersInTabs {
                    banners
                }
            }
            .task(id: store.connected) {
                guard store.connected else { return }
                store.screenName = ScreenIdentity.name
                store.socket?.announce(id: ScreenIdentity.id, name: ScreenIdentity.name, kind: ScreenIdentity.kind)
                #if DEBUG
                    // Simulator testing: -BroadwaveHomeNotice "New Apple TV found: Den." shows an arrival line.
                    if let notice = UserDefaults.standard.string(forKey: "BroadwaveHomeNotice") {
                        store.showHomeNotice(notice)
                    }
                    // Setup replaces the tabs, and the player with them, so a launch
                    // straight into a channel skips it.
                    let watching = UserDefaults.standard.integer(forKey: "BroadwaveWatch") > 0
                        || !(UserDefaults.standard.string(forKey: "BroadwaveMultiview") ?? "").isEmpty
                    if UserDefaults.standard.bool(forKey: "BroadwaveMultiviewTest") || sidebarPageTest != nil || watching {
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
            .spotlightLinks(open)
            .watchHandoff(handoffChannel, server: store.server) { watch($0) }
            .onChange(of: store.screenWatch) {
                // Another screen sent this one a channel. It plays live and says who sent it.
                // Setup holds no player, so the channel is dropped and the sender keeps playing.
                guard let sent = store.takeScreenWatch(), !showSetup, !store.presentSetup else { return }
                watch(sent.channelId, note: sent.note)
            }
            .task {
                await nowPlaying.sharePlay.run(store: store) { id, force in
                    guard !showSetup, !store.presentSetup else { return }
                    if force || nowPlaying.channel?.id != id || !nowPlaying.together.isEmpty {
                        watch(id, note: "From SharePlay")
                    }
                }
            }
            .onChange(of: nowPlaying.sharePlay.note) {
                // The player covers the root's alert, so the note goes on the picture.
                if playerCovers, let note = nowPlaying.sharePlay.note {
                    nowPlaying.note = note
                    nowPlaying.sharePlay.note = nil
                }
            }
            .onChange(of: handoffChannel?.id, initial: true) {
                store.socket?.watching(handoffChannel?.id ?? 0)
                nowPlaying.sharePlay.playing(WatchTogether.invite(server: store.server, channel: handoffChannel))
            }
            .onChange(of: store.socket.map(ObjectIdentifier.init)) {
                store.socket?.watching(handoffChannel?.id ?? 0)
            }
            .alert("SharePlay", isPresented: sharePlayNote) {
                Button("OK", role: .cancel) {}
            } message: {
                Text(nowPlaying.sharePlay.note ?? "")
            }
        #if DEBUG
            .task {
                if UserDefaults.standard.bool(forKey: "BroadwaveDemo"), store.server?.id != "demo" {
                    await store.startDemo()
                }
                if UserDefaults.standard.bool(forKey: "BroadwaveMultiviewTest") {
                    UserDefaults.standard.set("2up", forKey: "BroadwaveMultiviewLayout")
                    store.previewLineup([
                        previewChannel(id: 1, number: "4.1", name: "One"),
                        previewChannel(id: 3, number: "9.1", name: "Two"),
                    ])
                    nowPlaying.watchTogether(store.channels)
                    return
                }
                if let page = sidebarPageTest {
                    store.previewLineup([
                        previewChannel(id: 1, number: "4.1", name: "One"),
                        previewChannel(id: 3, number: "5.1", name: "Two"),
                    ])
                    switch page {
                    case "settings": openSettings()
                    case "search": tab = .search
                    default: tab = .guide
                    }
                    return
                }
                if let name = UserDefaults.standard.string(forKey: "BroadwaveTab") {
                    switch name {
                    case "guide": tab = .guide
                    case "search": tab = .search
                    case "sports": tab = .sports
                    case "recordings": tab = .recordings
                    case "settings": openSettings()
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
                    let ids = raw.split(separator: ",").compactMap { Int64($0.trimmingCharacters(in: .whitespaces)) }
                    // -BroadwaveMultiviewLayout picks pip or 1+3. fitting() never does.
                    let layout = UserDefaults.standard.string(forKey: "BroadwaveMultiviewLayout")
                    await openSavedMultiview(ids: ids, layout: layout, store: store, nowPlaying: nowPlaying)
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
        /// Offline Guide, Settings, or Search for the sidebar focus tests. -BroadwaveFocus guide.
        private var sidebarPageTest: String? {
            let page = UserDefaults.standard.string(forKey: "BroadwaveFocus")
            switch page {
            case "guide", "settings", "search":
                return page
            default:
                return nil
            }
        }

        private func previewChannel(id: Int64, number: String, name: String) -> Channel {
            Channel(
                id: id, deviceId: "preview", guideNumber: number, guideName: name,
                displayNumber: number, displayName: name,
                hd: true, favorite: false, enabled: true, hidden: false, present: true
            )
        }
    #endif

    /// broadwave://connect?url=, broadwave://watch/<channel id>,
    /// broadwave://multiview?ch=<id>,<id>, broadwave://recording/<id>, broadwave://guide, broadwave://sports.
    private var installedVersion: String {
        if let raw = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String, !raw.isEmpty {
            return raw
        }
        return Compatibility.appVersion
    }

    private func open(_ url: URL) {
        guard url.scheme == "broadwave" else { return }
        switch url.host() {
        case "connect":
            if let server = ConnectLink.serverURL(from: url) {
                Task {
                    if let info = try? await APIClient(base: server).server() {
                        store.connect(FoundServer(id: info.id, name: info.name, url: server))
                    }
                }
            }
        case "watch":
            watch(Int64(url.lastPathComponent) ?? 0)
        case "multiview":
            let parts = URLComponents(url: url, resolvingAgainstBaseURL: false)
            let query = parts?.queryItems ?? []
            let raw = query.first { $0.name == "ch" }?.value
                ?? url.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            let ids = raw.split(separator: ",").compactMap { Int64($0.trimmingCharacters(in: .whitespaces)) }
            let layout = query.first { $0.name == "layout" }?.value
            Task {
                await openSavedMultiview(ids: ids, layout: layout, store: store, nowPlaying: nowPlaying)
            }
        case "guide": show(.guide)
        case "search": show(.search)
        case "sports": show(.sports)
        case "recordings": show(.recordings)
        case "recording":
            #if os(tvOS)
                guard let id = Int64(url.lastPathComponent) else { return }
                Task {
                    // The cached list can be older than the Top Shelf that sent the link.
                    await store.refreshRecordings()
                    let busy = libraryFilter.playing || nowPlaying.channel != nil
                    libraryFilter.closeToken += 1
                    show(.recordings)
                    guard store.recordings.contains(where: { $0.id == id }) else { return }
                    if busy {
                        await coverGone()
                    }
                    libraryFilter.recording = id
                }
            #else
                show(.recordings)
            #endif
        default: show(.home)
        }
    }

    /// Plays a channel live, closing a recording first. `note` says why, such as
    /// which screen sent it.
    private func watch(_ id: Int64, note: String? = nil) {
        nowPlaying.closeStartOver += 1
        Task {
            if libraryFilter.playing {
                libraryFilter.closeToken += 1
                await coverGone()
            }
            if store.channels.isEmpty {
                await store.refresh()
            }
            var lineup = store.channels
            if !lineup.contains(where: { $0.id == id }), let all = try? await store.api?.allChannels() {
                lineup = all
            }
            if let choice = ClearBroadcast.play(id: id, visible: store.channels, lineup: lineup) {
                let line = [note, choice.note].compactMap(\.self).joined(separator: ". ")
                nowPlaying.play(choice.channel, note: line.isEmpty ? nil : line)
            }
        }
    }

    private var sharePlayNote: Binding<Bool> {
        Binding { nowPlaying.sharePlay.note != nil && !playerCovers } set: { shown in
            if !shown {
                nowPlaying.sharePlay.note = nil
            }
        }
    }

    /// The live channel playing alone, which another device can pick up. Not while
    /// a recording plays over it.
    private var handoffChannel: Channel? {
        guard nowPlaying.together.isEmpty, nowPlaying.recordingScreens.isEmpty, !libraryFilter.playing else { return nil }
        return nowPlaying.channel
    }

    /// A full-screen cover asked for while another is still closing is dropped.
    private func coverGone() async {
        try? await Task.sleep(for: .milliseconds(700))
    }

    /// A page link closes the player, which would otherwise cover the page.
    private func show(_ page: AppTab) {
        tab = page
        #if os(tvOS)
            if nowPlaying.channel != nil {
                nowPlaying.stop()
            }
        #else
            nowPlaying.expanded = false
        #endif
    }

    /// The arrival banner and a settings link push Settings on the open tab.
    /// iPad and Apple TV keep the Settings tab.
    private func openSettings() {
        #if os(iOS)
            if phoneTabs {
                let page: AppTab = tab == .settings ? .home : tab
                if tab == .settings {
                    tab = .home
                }
                settingsRoute.show(on: page)
                return
            }
        #endif
        tab = .settings
    }

    private var phoneTabs: Bool {
        #if os(iOS)
            usesPhoneTabs(width)
        #else
            false
        #endif
    }

    private func stack(_ page: AppTab, @ViewBuilder content: () -> some View) -> some View {
        NavigationStack {
            #if os(iOS)
                content().modifier(SettingsOnStack(tab: page)).safeAreaInset(edge: .top, spacing: 0) {
                    if bannersInTabs {
                        banners
                    }
                }
            #else
                content()
            #endif
        }
        .phoneTabClearance()
    }

    private var tabs: some View {
        TabView(selection: $tab) {
            Tab("Home", systemImage: "house.fill", value: AppTab.home) {
                stack(.home) { HomeView() }
            }
            Tab("Guide", systemImage: "square.grid.3x3.topleft.filled", value: AppTab.guide) {
                stack(.guide) { GuideView() }
            }
            Tab("Search", systemImage: "magnifyingglass", value: AppTab.search) {
                stack(.search) { SearchView() }
            }
            Tab("Sports", systemImage: "sportscourt.fill", value: AppTab.sports) {
                stack(.sports) { SportsView() }
            }
            Tab("Recordings", systemImage: "play.rectangle.on.rectangle.fill", value: AppTab.recordings) {
                stack(.recordings) { RecordingsView() }
            }
            if !phoneTabs {
                Tab("Settings", systemImage: "gearshape.fill", value: AppTab.settings) {
                    NavigationStack {
                        SettingsView().safeAreaInset(edge: .top, spacing: 0) {
                            if bannersInTabs {
                                banners
                            }
                        }
                    }
                    .phoneTabClearance()
                }
            }
        }
        .tabViewStyle(.sidebarAdaptable)
        #if os(tvOS)
            .environment(\.tvSelectedTab, tab)
        #endif
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
        // Here rather than on the player, so minimizing keeps the picture going.
        .task(id: nowPlaying.watchKey(even: store.prefs.even)) {
            await nowPlaying.playLive(store: store)
        }
        .onDisappear {
            Task { await nowPlaying.live.stop() }
        }
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
            offlineBanner
        }
    }

    /// True while a channel is on screen, so the offline note is not buried under the player.
    private var playerCoversTheScreen: Bool {
        #if os(iOS)
            nowPlaying.expanded && nowPlaying.channel != nil
        #else
            nowPlaying.channel != nil
        #endif
    }

    /// Text only, so it never takes focus from the remote.
    @ViewBuilder
    private var offlineBanner: some View {
        if store.connected, store.offline {
            Text(offlineLine)
                .font(.subheadline)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 16)
                .padding(.vertical, 10)
                .background(.regularMaterial)
                .accessibilityAddTraits(.updatesFrequently)
                .accessibilityIdentifier("offline-banner")
                .onAppear(perform: noteOfflineBanner)
        }
    }

    /// `-BroadwaveSyncLog 1` only, so a soak can see the moment the note is on screen.
    private func noteOfflineBanner() {
        guard UserDefaults.standard.bool(forKey: "BroadwaveSyncLog") else { return }
        let line = "broadwave offline banner \(Int(Date().timeIntervalSince1970 * 1000))"
        print(line)
        fflush(stdout)
        Logger(subsystem: "com.wolfeup.broadwave", category: "socket").notice("\(line, privacy: .public)")
    }

    private var offlineLine: String {
        guard let at = store.freshAt else { return "Can't reach the server. Trying again." }
        let style: Date.FormatStyle = Calendar.current.isDateInToday(at) ? .dateTime.hour().minute() : .dateTime.month().day().hour().minute()
        return "Can't reach the server. Showing what was saved at \(at.formatted(style))."
    }

    @ViewBuilder
    private var updateBanner: some View {
        if store.connected, let update = store.info?.update, !update.message.isEmpty, dismissedUpdate != update.message {
            HStack(alignment: .center, spacing: 12) {
                Text(update.message)
                    .font(.subheadline)
                    .frame(maxWidth: .infinity, alignment: .leading)
                if let url = releasePage(update.notesUrl) {
                    Link("Release notes", destination: url)
                        .buttonStyle(.glass)
                }
                Button("Not now") { dismissedUpdate = update.message }
                    .buttonStyle(.glass)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 10)
            .background(.regularMaterial)
            .accessibilityElement(children: .contain)
        }
    }

    private func releasePage(_ raw: String) -> URL? {
        guard let url = URL(string: raw) else { return nil }
        let host = url.host?.lowercased()
        guard url.scheme == "https", host == "github.com", url.user == nil else { return nil }
        guard (url.query ?? "").isEmpty, (url.fragment ?? "").isEmpty else { return nil }
        guard url.path.hasPrefix("/wolfebase/broadwave/") else { return nil }
        guard !url.path.contains("..") else { return nil }
        return url
    }

    private var banners: some View {
        VStack(spacing: 0) {
            // The full-screen player covers this inset, so the note lives on the player then.
            if !playerCoversTheScreen {
                offlineBanner
            }
            updateBanner
            arrivalBanner
            gameBanner
        }
    }

    /// The iPad's tab bar floats at the top over the tabs, so a line above them covers it.
    /// There the lines go inside each tab, under the bar. The mini player holds the bottom.
    private var bannersInTabs: Bool {
        #if os(iOS)
            UIDevice.current.userInterfaceIdiom == .pad && store.connected && !showSetup && !store.presentSetup
                && Compatibility.gateApp(store.info, app: installedVersion) == nil
        #else
            false
        #endif
    }

    private var playerCovers: Bool {
        #if os(tvOS)
            nowPlaying.channel != nil
        #else
            nowPlaying.channel != nil && nowPlaying.expanded
        #endif
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
                openSettings()
            } onDismiss: {
                store.dismissHome()
            }
            // The player covers the root, so the notice waits until it is seen.
            .task(id: "\(notice) \(playerCovers)") {
                guard !playerCovers else { return }
                try? await Task.sleep(for: .seconds(12))
                if !Task.isCancelled, store.homeNotice == notice {
                    store.dismissHome()
                }
            }
        }
    }

    /// The full player, a recording playing, a home notice, and the background hold the alert.
    /// It cannot take the remote from the player, because it is not on screen then.
    private var gameHeld: Bool {
        playerCoversTheScreen || libraryFilter.playing || showSetup || store.presentSetup || !(store.homeNotice ?? "").isEmpty || scenePhase != .active
    }

    @ViewBuilder
    private var gameBanner: some View {
        if store.connected, let alert = store.gameAlert, let at = store.gameAlertAt, !gameHeld, Date().timeIntervalSince(at) < GameAlerts.freshFor {
            GameAlertBanner(alert: alert, onWatch: { watchGame(alert) }, onDismiss: { store.dismissGameAlert() })
                .onAppear { store.refreshGameAlerts() }
                .task(id: alert.id) {
                    let age = Date().timeIntervalSince(at)
                    if age >= GameAlerts.freshFor {
                        store.refreshGameAlerts()
                        return
                    }
                    let wait = min(GameAlerts.visibleFor, GameAlerts.freshFor - age)
                    try? await Task.sleep(for: .seconds(wait))
                    if !Task.isCancelled, store.gameAlert?.id == alert.id {
                        store.dismissGameAlert()
                    }
                }
        }
    }

    private func watchGame(_ alert: GameAlert) {
        store.dismissGameAlert()
        if let channel = store.channels.first(where: { $0.id == alert.channelId }) {
            nowPlaying.play(channel)
            return
        }
        if let url = URL(string: "broadwave://watch/\(alert.channelId)") {
            open(url)
        }
    }

    #if DEBUG
        /// `-BroadwaveFakeGameAlert YES` injects one alert once the lineup is in.
        private var fakeGameAlert: Bool {
            if UserDefaults.standard.bool(forKey: "BroadwaveFakeGameAlert") {
                return true
            }
            let raw = UserDefaults.standard.string(forKey: "BroadwaveFakeGameAlert") ?? ""
            return !raw.isEmpty && raw != "0" && raw.lowercased() != "no"
        }
    #endif
}

/// One line when a tuner, screen, or server shows up after the house is already known.
/// This app is older than the server's minAppVersion.
struct IncompatibleServer: View {
    var message: String

    var body: some View {
        VStack(spacing: 16) {
            Circle().fill(Tokens.ColorToken.tally).frame(width: 12, height: 12)
            Text("Broadwave").font(.title2.weight(.heavy))
            Text(message)
                .font(.title3.weight(.semibold))
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding(32)
        .accessibilityElement(children: .combine)
    }
}

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

/// A followed team's game starting, or a close finish, with Watch.
/// Left out while the player is up, so it never takes that focus.
struct GameAlertBanner: View {
    var alert: GameAlert
    var onWatch: () -> Void
    var onDismiss: () -> Void

    private var line: String {
        if let detail = alert.detail, !detail.isEmpty {
            return "\(alert.text) · \(detail)"
        }
        return alert.text
    }

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            Text(line)
                .font(.subheadline)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityAddTraits(.updatesFrequently)
                .accessibilityIdentifier("game-alert-text")
            Button("Watch \(alert.channel)", action: onWatch)
                .buttonStyle(.glass)
                .accessibilityIdentifier("game-alert-watch")
            Button("Not now", action: onDismiss)
                .buttonStyle(.glass)
                .accessibilityIdentifier("game-alert-dismiss")
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(.regularMaterial)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("game-alert")
        #if os(tvOS)
            // Its own section, so Up from the page reaches Watch without taking the player's focus.
            .focusSection()
        #endif
    }
}

#if os(iOS)
    /// Live TV keeps a place at the bottom while you browse.
    struct MiniPlayerBar: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying

        var body: some View {
            if let channel = nowPlaying.channel {
                let live = nowPlaying.live
                HStack(spacing: 12) {
                    // The dot means sound is playing. The player keeps going while minimized.
                    if live.moving {
                        LiveDot("")
                    } else if live.error == nil {
                        ProgressView()
                            .accessibilityLabel("Tuning")
                    }
                    VStack(alignment: .leading, spacing: 0) {
                        Text(store.index.on(channel.id, at: store.now)?.title ?? channel.displayName)
                            .font(.subheadline.weight(.semibold))
                            .lineLimit(1)
                        Text(live.error ?? "\(channel.displayNumber) \(channel.displayName)")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                    .accessibilityElement(children: .combine)
                    .accessibilityValue(live.moving ? "Playing" : live.error == nil ? "Tuning" : "Stopped")
                    .accessibilityAddTraits(.isButton)
                    .accessibilityHint("Opens the player")
                    .accessibilityIdentifier("miniPlayer")
                    .accessibilityAction { nowPlaying.expanded = true }
                    Spacer()
                    Button("Close", systemImage: "xmark") { nowPlaying.stop() }
                        .labelStyle(.iconOnly)
                        .accessibilityLabel("Close")
                }
                .padding(.horizontal, 16)
                .contentShape(.rect)
                .onTapGesture { nowPlaying.expanded = true }
            }
        }
    }
#endif
