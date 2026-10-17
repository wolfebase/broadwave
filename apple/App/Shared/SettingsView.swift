import BroadwaveKit
import SwiftUI
#if os(iOS)
    import UIKit
    import UniformTypeIdentifiers
#endif

/// What Settings loaded from the server and what the viewer has typed. A class, so a page
/// pushed on iPhone sees a load that finishes after it opened.
@MainActor
@Observable
final class SettingsModel {
    var seat: ClientSeat?
    var storage: APIClient.StorageInfo?
    var showRows: [StorageShow]?
    var backupRows: [CatalogBackup]?
    var restoreName: String?
    var showAbout = false
    var showDiagnostics = false
    var showChannels = false
    var showSources = false
    var showPasses = false
    var checkUpdates = true
    var updatesKnown = false
    var liveScores = true
    var scoresKnown = false
    var sportsDB = false
    var sportsKey = ""
    var supportBusy = false
    var hideScores = false
    var hideScoresKnown = false
    var gameAlerts = "all"
    var alertsKnown = false
    var autoplay = true
    var autoplayKnown = false
    var shareTuner = false
    var shareKnown = false
    var pictureMode = "broadcast"
    var liveDelay = LiveDelay.saved
    var pictureKnown = false
    var sdUser = ""
    var sdLineup = ""
    var guideURL = ""
    var reserve = "10"
    var bufferMinutes = "60"
    var loadedBuffer = "60"
    var bufferKnown = false
    var makeRoom = "0"
    var loadedMakeRoom = "0"
    var roomKnown = false
    var deleteWatchedDays = "0"
    var loadedDays = "0"
    var daysKnown = false
    var folderLayout = "shows"
    var loadedFolder = "shows"
    var folderKnown = false
    var sdPassword = ""
    var tmdbKey = ""
    var passwordSaved = false
    var artSaved = false
    var guideKnown = false
    var loadedUser = ""
    var loadedLineup = ""
    var loadedGuide = ""
    var loadedReserve = "10"
    var showError = ""
    var backupError = ""
    var backupBusy = ""
    var importBackup = false
    var saveError = ""
    var savedNote = ""
    var screenLabel = ""
    var screenLabelSaved = ""
    var screenNameReady = false
    var confirmImport = false
}

struct SettingsView: View {
    @State private var m = SettingsModel()
    @Environment(AppStore.self) private var store
    @Environment(LibraryFilter.self) private var library
    /// How the server knows this device: open at home, or a paired device.
    @AppStorage(BreakSkip.key) private var breakSkip = BreakSkip.auto
    #if os(iOS)
        @AppStorage(LiveActivities.recordingsKey) private var activityRecordings = true
        @AppStorage(LiveActivities.gamesKey) private var activityGames = true
    #endif
    /// The page on the right on iPad.
    @State private var padPage: SettingsPage? = .playback
    /// A page a debug launch opens on iPhone.
    @State private var phonePage: SettingsPage?

    private var demo: Bool {
        store.server?.id == "demo"
    }

    var body: some View {
        @Bindable var store = store
        ScrollViewReader { proxy in
            settingsForm(store)
                .onAppear { openDebug(proxy) }
        }
    }

    /// iPhone pushes a page from the list; iPad keeps the list on the left and the page on the right.
    @ViewBuilder
    private func settingsForm(_: AppStore) -> some View {
        #if os(iOS)
            if UIDevice.current.userInterfaceIdiom == .pad {
                NavigationSplitView {
                    List(selection: $padPage) { rootSections }
                        .navigationTitle("Settings")
                } detail: {
                    NavigationStack {
                        page(padPage ?? .playback)
                    }
                }
                .modifier(settingsWork)
            } else {
                List { rootSections }
                    .navigationTitle("Settings")
                    .navigationDestination(item: $phonePage) { page($0) }
                    .modifier(settingsWork)
            }
        #else
            Form {
                ForEach(SettingsPage.allCases) { item in
                    pageSections(item)
                }
            }
            .navigationTitle("Settings")
            .modifier(settingsWork)
        #endif
    }

    /// Loading, saving on the way out, and the restore question, for every layout.
    private var settingsWork: SettingsWork {
        SettingsWork(load: load, leave: {
            flushServerText()
            commitScreenName()
        }, restoreName: $m.restoreName, restore: { name in Task { await restore(name) } })
    }

    // MARK: The list

    /// This device first, then the server every screen shares, so a change says where it lands.
    @ViewBuilder
    private var rootSections: some View {
        Section {
            row(.screen, "Name", symbol: "character.cursor.ibeam", value: m.screenLabel.isEmpty ? ScreenIdentity.name : m.screenLabel)
            row(.playback, "Playback", symbol: "play.rectangle", value: qualityName)
            row(.sync, "Whole-Home Sync", symbol: "dot.radiowaves.left.and.right", value: store.syncEnabled ? String(localized: "On") : String(localized: "Off"))
            #if os(iOS)
                row(.activities, "Live Activities", symbol: "platter.filled.bottom.iphone", value: "")
            #endif
        } header: {
            Text(deviceName)
        } footer: {
            Text(deviceFooter)
        }
        Section {
            row(.home, "Your home", symbol: "house", value: "")
        } footer: {
            Text("Tuners, screens, and servers on this network.")
        }
        Section {
            row(.server, demo ? "Demo" : "Server", symbol: "server.rack", value: store.info?.name ?? store.server?.name ?? "")
            row(.recording, "Recording and storage", symbol: "record.circle", value: "")
            row(.picture, "Picture", symbol: "sparkles.tv", value: m.pictureMode.capitalized)
            row(.sports, "Sports and scores", symbol: "sportscourt", value: "")
            if !demo {
                row(.guide, "Guide data", symbol: "list.bullet.rectangle", value: "")
            }
            row(.sources, "Tuners and channels", symbol: "antenna.radiowaves.left.and.right", value: "")
            if !demo {
                row(.backups, "Backups", symbol: "externaldrive", value: "")
            }
            row(.support, "Updates and support", symbol: "lifepreserver", value: "")
        } header: {
            Text("Server")
        } footer: {
            Text(demo ? "The demo changes nothing on your server." : "These apply to every screen in the house.")
        }
        Section {
            row(.about, "About", symbol: "info.circle", value: "")
        } footer: {
            if !demo {
                Text(accessLine)
                    .accessibilityIdentifier("settings-access")
            }
        }
        if !m.saveError.isEmpty {
            Section {
                Text(m.saveError).foregroundStyle(.red)
            }
        }
    }

    /// iPad selects the row for the page beside the list. iPhone pushes it: Settings is itself
    /// pushed on the tab's stack, and a value link there did not reach a destination.
    @ViewBuilder
    private func row(_ item: SettingsPage, _ title: LocalizedStringKey, symbol: String, value: String) -> some View {
        let label = LabeledContent {
            Text(value).lineLimit(1)
        } label: {
            Label {
                Text(title).foregroundStyle(.primary)
            } icon: {
                Image(systemName: symbol).foregroundStyle(.tint)
            }
        }
        #if os(iOS)
            if UIDevice.current.userInterfaceIdiom == .phone {
                Button { phonePage = item } label: {
                    HStack {
                        label
                        Image(systemName: "chevron.right")
                            .font(.footnote.weight(.semibold))
                            .foregroundStyle(.tertiary)
                            .accessibilityHidden(true)
                    }
                    .contentShape(.rect)
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("settings-\(item.rawValue)")
            } else {
                NavigationLink(value: item) { label }
                    .accessibilityIdentifier("settings-\(item.rawValue)")
            }
        #else
            NavigationLink(value: item) { label }
                .accessibilityIdentifier("settings-\(item.rawValue)")
        #endif
    }

    private var deviceName: String {
        #if os(iOS)
            if UIDevice.current.userInterfaceIdiom == .pad {
                return String(localized: "This iPad")
            }
            return String(localized: "This iPhone")
        #else
            return String(localized: "This Apple TV")
        #endif
    }

    private var deviceFooter: String {
        #if os(iOS)
            if UIDevice.current.userInterfaceIdiom == .pad {
                return String(localized: "Kept on this iPad.")
            }
            return String(localized: "Kept on this iPhone.")
        #else
            return String(localized: "Kept on this Apple TV.")
        #endif
    }

    private var qualityName: String {
        switch store.prefs.quality {
        case .auto: String(localized: "Auto")
        case .original: String(localized: "Original")
        case .high: String(localized: "High")
        case .medium: String(localized: "Medium")
        case .saver: String(localized: "Data saver")
        case .tile, .focus, .tile360: String(localized: "Auto")
        }
    }

    // MARK: Pages

    private func page(_ item: SettingsPage) -> some View {
        Form {
            if item == .about {
                AboutView.credit
            } else {
                pageSections(item)
            }
        }
        .navigationTitle(item.title)
        #if os(iOS)
            .toolbarTitleDisplayMode(.inline)
        #endif
            .navigationDestination(isPresented: $m.showAbout) { AboutView() }
            .navigationDestination(isPresented: $m.showDiagnostics) { DiagnosticsView() }
            .navigationDestination(isPresented: $m.showChannels) { ChannelsView() }
            .navigationDestination(isPresented: $m.showSources) { SourcesView() }
            .navigationDestination(isPresented: $m.showPasses) { PassesView() }
        #if os(iOS)
            .fileImporter(isPresented: $m.importBackup, allowedContentTypes: [.data]) { result in
                Task { await importCatalog(result) }
            }
        #endif
    }

    @ViewBuilder
    private func pageSections(_ item: SettingsPage) -> some View {
        switch item {
        case .screen:
            screenNameSection
        case .playback:
            playbackSection(store)
            CaptionStyleSection()
            delaySection
            breaksSection
        case .sync:
            syncSection(store)
        case .activities:
            #if os(iOS)
                activitiesSection.id("activities")
            #else
                EmptyView()
            #endif
        case .home:
            Section {
                HomeListView(grabsFocus: true)
            }
        case .server:
            serverSection
        case .recording:
            dvrSection
            if !demo {
                storageFolderSection.id("storage")
                storageOptionsSection
                showSpaceSection
            }
        case .picture:
            pictureSection
        case .sports:
            sportsSection
        case .guide:
            if demo {
                Section {
                    Text("The demo does not change a guide account, a folder, or a backup.")
                        .foregroundStyle(.secondary)
                }
            } else {
                guideSection.id("guide")
            }
        case .sources:
            sourcesSection
        case .backups:
            if !demo {
                backupSection.id("backups")
            }
        case .support:
            updatesSection
            supportSection
            if !demo {
                Section {
                    Button("Diagnostics") { m.showDiagnostics = true }
                } footer: {
                    Text("Tuner health, the guide, and the last antenna reading.")
                }
            }
        case .about:
            Section {
                Button("About") { m.showAbout = true }
                    .accessibilityLabel("About Broadwave")
            }
        }
        if !m.saveError.isEmpty {
            Section {
                Text(m.saveError).foregroundStyle(.red)
            }
        }
    }

    private func openDebug(_ proxy: ScrollViewProxy) {
        #if DEBUG
            // A flag opens the page that holds what it names, then the screen or row on it.
            let defaults = UserDefaults.standard
            var opened: SettingsPage?
            if defaults.bool(forKey: "BroadwaveAbout") {
                opened = .about
                m.showAbout = true
            }
            if defaults.bool(forKey: "BroadwaveDiagnostics") {
                opened = .support
                m.showDiagnostics = true
            }
            if defaults.bool(forKey: "BroadwaveSources") {
                opened = .sources
                m.showSources = true
            }
            if defaults.bool(forKey: "BroadwaveChannels") {
                opened = .sources
                m.showChannels = true
            }
            if defaults.bool(forKey: "BroadwavePasses") {
                opened = .recording
                m.showPasses = true
            }
            let anchor = defaults.string(forKey: "BroadwaveSettingsAnchor")
            if let anchor {
                opened = SettingsPage.holding(anchor) ?? opened
            }
            if let opened {
                padPage = opened
                phonePage = opened
            }
            if let anchor {
                Task {
                    try? await Task.sleep(for: .milliseconds(900))
                    proxy.scrollTo(anchor, anchor: .top)
                }
            }
        #else
            _ = proxy
        #endif
    }

    private var screenNameSection: some View {
        Section {
            TextField("Name this screen", text: $m.screenLabel)
                .accessibilityIdentifier("screen-name")
                .onSubmit { commitScreenName() }
        } header: {
            Text("Name this screen")
        } footer: {
            Text("Other screens see this name.")
        }
        .onAppear {
            guard !m.screenNameReady else { return }
            m.screenNameReady = true
            let name = ScreenIdentity.name
            m.screenLabel = name
            m.screenLabelSaved = name
        }
    }

    /// Sends the new name on the open socket. Blank goes back to the device name.
    private func commitScreenName() {
        guard m.screenNameReady, m.screenLabel != m.screenLabelSaved else { return }
        let sent = ScreenName.save(m.screenLabel, device: ScreenIdentity.deviceName)
        m.screenLabel = sent
        m.screenLabelSaved = sent
        store.screenName = sent
        store.socket?.announce(id: ScreenIdentity.id, name: sent, kind: ScreenIdentity.kind)
    }

    private var serverSection: some View {
        Section("Server") {
            if demo {
                Text("Sample films on this device. Your server is unchanged.")
                    .foregroundStyle(.secondary)
                Button("Leave the demo", role: .destructive) { store.forget() }
            }
            LabeledContent("Name", value: store.info?.name ?? store.server?.name ?? "")
            LabeledContent("Address", value: store.server?.url.absoluteString ?? "")
            if let info = store.info {
                LabeledContent("Version", value: info.version)
                if let enc = info.encoder {
                    LabeledContent("Encoding", value: enc.replacingOccurrences(of: "h264_", with: "").uppercased())
                }
            }
            if !demo {
                Button("Run setup again") { store.presentSetup = true }
                Button("Use a different server", role: .destructive) { store.forget() }
            }
        }
    }

    private func playbackSection(_ store: AppStore) -> some View {
        Section {
            @Bindable var store = store
            Picker("Quality", selection: $store.prefs.quality) {
                Text("Auto").tag(Prefs.Quality.auto)
                Text("Original").tag(Prefs.Quality.original)
                Text("High").tag(Prefs.Quality.high)
                Text("Medium").tag(Prefs.Quality.medium)
                Text("Data saver").tag(Prefs.Quality.saver)
            }
            Picker("Sound", selection: $store.prefs.audio) {
                Text("Auto").tag(Prefs.Sound.auto)
                Text("Surround").tag(Prefs.Sound.surround)
                Text("Stereo").tag(Prefs.Sound.stereo)
            }
        } header: {
            Text("Playback")
        } footer: {
            Text("Auto plays the original broadcast with Dolby Digital whenever this device can.")
        }
    }

    private var delaySection: some View {
        Section {
            Picker("Live delay", selection: Binding(
                get: { m.liveDelay },
                set: { delay in
                    m.liveDelay = delay
                    LiveDelay.saved = delay
                }
            )) {
                ForEach(LiveDelay.allCases) { delay in
                    Text(delay.title).tag(delay)
                }
            }
        } footer: {
            Text("A channel starts as close to live as it plays smoothly, about 5 seconds, whichever you pick, and screens you line up meet it there. Picking a delay in the player moves every lined-up screen on that channel: Balanced to about 16 seconds, Stable to 20, which rides out a weak signal.")
        }
    }

    private var pictureSection: some View {
        Section {
            Picker("Picture", selection: Binding(
                get: { m.pictureMode },
                set: { mode in
                    guard m.pictureKnown else { return }
                    m.pictureMode = mode
                    save(["pictureMode": mode])
                }
            )) {
                Text("Broadcast").tag("broadcast")
                Text("Smooth").tag("smooth")
                Text("Film").tag("film")
            }
            .disabled(!m.pictureKnown)
        } footer: {
            Text("Broadcast rebuilds interlaced channels at 60 frames a second. Smooth adds motion compensation when this server can hold it. Film is for movies.")
        }
    }

    private var guideSection: some View {
        Section {
            TextField("Schedules Direct", text: $m.sdUser)
                .fieldTyping()
                .onSubmit { flushServerText() }
            SecureField("Password", text: $m.sdPassword, prompt: m.passwordSaved ? Text("Saved") : nil)
                .onSubmit { saveSecret("sdPassword", m.sdPassword) { m.passwordSaved = $0 } }
            TextField("Lineup", text: $m.sdLineup)
                .fieldTyping()
                .onSubmit { flushServerText() }
            TextField("Guide address", text: $m.guideURL)
                .fieldTyping()
                .onSubmit { flushServerText() }
            SecureField("Movie artwork", text: $m.tmdbKey, prompt: m.artSaved ? Text("Saved") : nil)
                .onSubmit { saveSecret("tmdbKey", m.tmdbKey) { m.artSaved = $0 } }
        } header: {
            Text("Guide")
        } footer: {
            Text("Schedules Direct fills channels the tuner guide skips. Fourteen days when the account allows it. An XMLTV link does the same. Movie artwork fills posters the guide does not include.")
        }
    }

    private var sportsSection: some View {
        Section {
            Toggle("Live scores", isOn: Binding(
                get: { m.liveScores },
                set: { on in
                    guard m.scoresKnown else { return }
                    m.liveScores = on
                    Task {
                        try? await store.api?.saveSettings(["liveScores": on ? "1" : "0"])
                        await store.refresh(lineup: false)
                    }
                }
            ))
            .disabled(!m.scoresKnown)
            Toggle("Hide scores", isOn: Binding(
                get: { m.hideScores },
                set: { on in
                    guard m.hideScoresKnown else { return }
                    m.hideScores = on
                    save(["hideScores": on ? "1" : "0"])
                }
            ))
            .disabled(!m.hideScoresKnown)
            Text("A recorded game stays hidden until you watch it.")
                .font(.caption)
                .foregroundStyle(.secondary)
            Picker("Game alerts", selection: Binding(
                get: { m.gameAlerts },
                set: { next in
                    guard m.alertsKnown, next != m.gameAlerts, next == "all" || next == "teams" || next == "off" else { return }
                    m.gameAlerts = next
                    save(["gameAlerts": next])
                }
            )) {
                Text("Your teams and close games").tag("all")
                Text("Your teams only").tag("teams")
                Text("Off").tag("off")
            }
            .disabled(!m.alertsKnown)
            .accessibilityIdentifier("game-alerts")
            Text("A note when a team you follow starts playing, or a game on your channels comes down to the last minutes. A game you're recording and haven't watched gets no score and no close-game note.")
                .font(.caption)
                .foregroundStyle(.secondary)
            SecureField("TheSportsDB key", text: $m.sportsKey)
                .onSubmit {
                    let key = m.sportsKey.trimmingCharacters(in: .whitespacesAndNewlines)
                    m.sportsKey = ""
                    guard !key.isEmpty else { return }
                    Task {
                        try? await store.api?.saveSettings(["sportsdbKey": key])
                        m.sportsDB = true
                    }
                }
        } header: {
            Text(m.sportsDB ? "Scores from TheSportsDB" : "Scores from ESPN's public scoreboard")
        } footer: {
            Text("On asks your server for the scoreboard. Off sends nothing. A key is optional and never included.")
        }
    }

    #if os(iOS)
        private var activitiesSection: some View {
            Section {
                Toggle("Recordings in progress", isOn: $activityRecordings)
                    .accessibilityIdentifier("activity-recordings")
                Toggle("Your teams' games", isOn: $activityGames)
                    .accessibilityIdentifier("activity-games")
            } header: {
                Text("Live Activities")
            } footer: {
                Text("On the Lock Screen and in the Dynamic Island, updated while Broadwave is open or playing. A game you're recording shows no score.")
            }
        }
    #endif

    private var dvrSection: some View {
        Section {
            Toggle("Play the next episode", isOn: Binding(
                get: { m.autoplay },
                set: { on in
                    guard m.autoplayKnown else { return }
                    m.autoplay = on
                    save(["autoplay": on ? "1" : "0"])
                }
            ))
            .disabled(!m.autoplayKnown)
            Button("Passes") { m.showPasses = true }
        } header: {
            Text("DVR")
        } footer: {
            Text("Passes record a show or a team every time it airs.")
        }
    }

    private var breaksSection: some View {
        Section {
            Picker("Commercial breaks", selection: $breakSkip) {
                ForEach(BreakSkip.allCases, id: \.self) { Text($0.label).tag($0) }
            }
        } footer: {
            Text("For recordings with marked breaks.")
        }
    }

    private var sourcesSection: some View {
        Section {
            if !demo {
                Button("Tuners and playlists") { m.showSources = true }
            }
            Button("Channels") { m.showChannels = true }
            if demo {
                Text("The demo does not share a tuner.")
                    .foregroundStyle(.secondary)
            } else if let message = Compatibility.gateFeature(store.info, "hdhrEmulation") {
                Text(message)
                    .foregroundStyle(.secondary)
            } else {
                Toggle("Offer this server as an HDHomeRun on port 8478", isOn: Binding(
                    get: { m.shareTuner },
                    set: { on in
                        guard m.shareKnown else { return }
                        m.shareTuner = on
                        save(["hdhrEmulate": on ? "1" : "0"])
                    }
                ))
                .disabled(!m.shareKnown)
            }
        } header: {
            Text("Sources")
        } footer: {
            if !demo, Compatibility.gateFeature(store.info, "hdhrEmulation") == nil {
                Text("Other apps can add this machine on port 8478. Discovery stays quiet so the real tuner is unchanged. The change applies within a minute.")
            }
        }
    }

    private func syncSection(_ store: AppStore) -> some View {
        Section {
            if let message = Compatibility.gateFeature(store.info, "wholeHomeSync") {
                Text(message)
                    .foregroundStyle(.secondary)
            } else {
                @Bindable var store = store
                Toggle("Whole-Home Sync", isOn: $store.syncEnabled)
                if store.syncEnabled {
                    Toggle("Line up automatically", isOn: $store.lineUpAutomatically)
                }
            }
        } footer: {
            if Compatibility.gateFeature(store.info, "wholeHomeSync") == nil {
                Text(store.syncEnabled && !store.lineUpAutomatically
                    ? "Channels start right away. Choose Sync screens in the player to put every screen on the same channel on the same moment."
                    : "Every screen on the same channel shows the same moment, so nobody hears the next room cheer first. A channel can take a few seconds longer to start.")
            }
        }
    }

    private var storageFolderSection: some View {
        Section {
            if let storage = m.storage {
                Text(storageSummary(storage))
                    .foregroundStyle(.secondary)
            }
            if let path = m.storage?.path, !path.isEmpty {
                LabeledContent("Recordings folder", value: path)
            }
        } header: {
            Text("Storage")
        }
    }

    private var storageOptionsSection: some View {
        Section {
            Picker("Recording folders", selection: Binding(
                get: { m.folderLayout },
                set: { next in
                    guard m.folderKnown, next != m.folderLayout else { return }
                    m.folderLayout = next
                    flushServerText()
                }
            )) {
                Text("By show, for Plex and Jellyfin").tag("shows")
                Text("All in one folder").tag("flat")
            }
            .disabled(!m.folderKnown)
            .accessibilityIdentifier("recording-folders")
            Text("By show puts new recordings in TV/Show/Season 01 and Movies/Title (Year). Point a TV library at TV and a movie library at Movies.")
                .font(.caption)
                .foregroundStyle(.secondary)
            LabeledContent("Keep this much free") {
                HStack(spacing: 4) {
                    TextField("10", text: $m.reserve)
                        .multilineTextAlignment(.trailing)
                        .fieldTyping()
                        .accessibilityLabel("Gigabytes to keep free")
                        .accessibilityIdentifier("keep-free")
                    #if os(iOS)
                        .keyboardType(.numberPad)
                    #endif
                        .onSubmit { flushServerText() }
                    Text("GB")
                        .foregroundStyle(.secondary)
                        .accessibilityHidden(true)
                }
            }
            Picker("When space runs low", selection: Binding(
                get: { m.makeRoom },
                set: { next in
                    guard m.roomKnown, next != m.makeRoom else { return }
                    m.makeRoom = next
                    flushServerText()
                }
            )) {
                Text("Skip new recordings").tag("0")
                Text("Delete the oldest watched").tag("1")
            }
            .disabled(!m.roomKnown)
            .accessibilityIdentifier("make-room")
            Text("Delete makes room before a new recording is skipped, oldest watched first. Unwatched and kept recordings stay.")
                .font(.caption)
                .foregroundStyle(.secondary)
            Picker("Delete watched recordings", selection: Binding(
                get: { m.deleteWatchedDays },
                set: { days in
                    guard m.daysKnown, days != m.deleteWatchedDays else { return }
                    m.deleteWatchedDays = days
                    flushServerText()
                }
            )) {
                ForEach(Self.dayChoices(m.deleteWatchedDays), id: \.self) { days in
                    Text(Self.dayLabel(days)).tag(days)
                }
            }
            .disabled(!m.daysKnown)
            .accessibilityIdentifier("delete-watched")
            Text("Counts from when a recording was played to its end or marked watched. Recordings you keep forever stay.")
                .font(.caption)
                .foregroundStyle(.secondary)
            Picker("Keep for recording from the start", selection: Binding(
                get: { m.bufferMinutes },
                set: { minutes in
                    guard m.bufferKnown, minutes != m.bufferMinutes else { return }
                    m.bufferMinutes = minutes
                    flushServerText()
                }
            )) {
                Text("Off").tag("0")
                Text("30 minutes").tag("30")
                Text("1 hour").tag("60")
                Text("2 hours").tag("120")
                Text("4 hours").tag("240")
            }
            .disabled(!m.bufferKnown)
            .accessibilityIdentifier("recording-buffer")
            Text("While a channel is on, the server keeps up to this much of it, so recording a show already on starts from its beginning. It uses at most half the free space.")
                .font(.caption)
                .foregroundStyle(.secondary)
        } footer: {
            Text("The container writes the original broadcast files on the recordings share. Use 0 to turn the reserve off. A show already recording is left alone.")
        }
    }

    private var showSpaceSection: some View {
        Section {
            if let message = m.showError.nilIfEmpty {
                Text(message)
                    .foregroundStyle(.red)
            } else if let rows = m.showRows {
                if rows.isEmpty {
                    Text("No finished recordings yet.")
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(rows, id: \.title) { row in
                        showRow(row)
                    }
                }
            }
        } header: {
            Text("Space by show")
        }
    }

    private func showRow(_ row: StorageShow) -> some View {
        let count = row.count == 1 ? "1 recording" : "\(row.count) recordings"
        let size = ByteCountFormatter.string(fromByteCount: row.bytes, countStyle: .file)
        return Button {
            library.open(row.title)
        } label: {
            VStack(alignment: .leading, spacing: 2) {
                Text(row.title)
                Text(count)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text(size)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .accessibilityLabel("\(row.title), \(count), \(size)")
        .accessibilityIdentifier("show-space-row")
    }

    private var backupSection: some View {
        Section {
            if let backupError = m.backupError.nilIfEmpty {
                Text(backupError)
                    .foregroundStyle(.red)
            }
            if let rows = m.backupRows, rows.isEmpty {
                Text("No backups yet.")
                    .foregroundStyle(.secondary)
            }
            ForEach(m.backupRows ?? [], id: \.name) { row in
                VStack(alignment: .leading, spacing: 6) {
                    Text("\(backupKindLabel(row.kind)) · \(row.takenAt.formatted(date: .abbreviated, time: .shortened))")
                    Text(ByteCountFormatter.string(fromByteCount: row.bytes, countStyle: .file))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    // Borderless: two buttons in one form row otherwise both fire on a tap.
                    HStack(spacing: 20) {
                        Button("Download") { Task { await downloadBackup(row) } }
                            .buttonStyle(.borderless)
                            .disabled(m.backupBusy != "")
                            .accessibilityLabel("Download \(backupKindLabel(row.kind)) \(row.takenAt.formatted(date: .abbreviated, time: .shortened))")
                        Button("Restore", role: .destructive) { m.restoreName = row.name }
                            .buttonStyle(.borderless)
                            .disabled(m.backupBusy != "")
                            .accessibilityLabel("Restore \(backupKindLabel(row.kind)) \(row.takenAt.formatted(date: .abbreviated, time: .shortened))")
                    }
                }
            }
            Button("Download backup") { Task { await downloadCatalog() } }
                .disabled(m.backupBusy != "" || store.api == nil)
            #if os(iOS)
                Button("Restore a catalog backup", role: .destructive) { m.confirmImport = true }
                    .disabled(m.backupBusy != "" || store.api == nil)
                    .confirmationDialog(
                        "Restore a backup from a file? The catalog goes back to that copy. Recordings stay.",
                        isPresented: $m.confirmImport,
                        titleVisibility: .visible
                    ) {
                        Button("Choose a file", role: .destructive) { m.importBackup = true }
                        Button("Cancel", role: .cancel) {}
                    }
            #endif
            #if os(tvOS)
                if m.savedNote == "backup" {
                    Text("Saved on this Apple TV.")
                }
            #endif
        } header: {
            Text("Backups")
        } footer: {
            #if os(tvOS)
                Text("The backup is the catalog. Recordings stay put. Seven nightly copies and four weekly copies are kept. A copy is saved before an update. Apple TV restores a copy this server already kept.")
            #else
                Text("The backup is the catalog. Recordings stay put. Seven nightly copies and four weekly copies are kept. A copy is saved before an update.")
            #endif
        }
    }

    private var updatesSection: some View {
        Section {
            Toggle("Check for updates", isOn: Binding(
                get: { m.checkUpdates },
                set: { on in
                    guard m.updatesKnown else { return }
                    m.checkUpdates = on
                    Task {
                        try? await store.api?.saveSettings(["checkUpdates": on ? "1" : "0"])
                        await store.refresh(lineup: false)
                    }
                }
            ))
            .disabled(!m.updatesKnown)
        } footer: {
            Text("Once a day. Nothing else is sent.")
        }
    }

    private var supportSection: some View {
        Section {
            Button("Download a support bundle") {
                Task { await downloadSupport() }
            }
            .disabled(m.supportBusy || store.api == nil)
            #if os(tvOS)
                if m.savedNote == "support" {
                    Text("Saved on this Apple TV.")
                }
            #endif
        } footer: {
            Text("Logs, versions, and settings. Passwords are left out.")
        }
    }

    /// What the server said about this device, in one plain sentence.
    @MainActor
    private var accessLine: String {
        if m.seat?.auth == "device", let name = m.seat?.device?.name, !name.isEmpty {
            return String(localized: "Signed in as \(name).")
        }
        return String(localized: "Open on your home network. Away from home, this device needs an invite from the owner.")
    }

    private func load() async {
        if let me = try? await store.api?.clientMe() {
            m.seat = me
        }
        if let values = try? await store.api?.settings() {
            m.checkUpdates = values["checkUpdates"] != "0"
            m.updatesKnown = true
            m.liveScores = values["liveScores"] != "0"
            m.scoresKnown = true
            m.sportsDB = values["sportsdbKeySet"] == "1"
            m.hideScores = values["hideScores"] == "1"
            m.hideScoresKnown = true
            m.gameAlerts = Self.alertChoice(values["gameAlerts"])
            m.alertsKnown = true
            m.autoplay = values["autoplay"] != "0"
            m.autoplayKnown = true
            m.shareTuner = values["hdhrEmulate"] == "1"
            m.shareKnown = true
            if let mode = values["pictureMode"], mode == "broadcast" || mode == "smooth" || mode == "film" {
                m.pictureMode = mode
            }
            m.pictureKnown = true
            m.sdUser = values["sdUser"] ?? ""
            m.sdLineup = values["sdLineup"] ?? ""
            m.guideURL = values["guideUrl"] ?? ""
            m.reserve = values["watermarkGB"] ?? "10"
            let minutes = values["bufferMinutes"] ?? "60"
            m.bufferMinutes = Self.bufferChoices.contains(minutes) ? minutes : "60"
            m.loadedBuffer = m.bufferMinutes
            m.makeRoom = values["makeRoom"] == "1" ? "1" : "0"
            m.loadedMakeRoom = m.makeRoom
            m.deleteWatchedDays = Self.watchedDays(values["deleteWatchedDays"])
            m.loadedDays = m.deleteWatchedDays
            m.folderLayout = values["folderLayout"] == "flat" ? "flat" : "shows"
            m.loadedFolder = m.folderLayout
            m.loadedUser = m.sdUser
            m.loadedLineup = m.sdLineup
            m.loadedGuide = m.guideURL
            m.loadedReserve = m.reserve
            m.passwordSaved = values["sdPasswordSet"] == "1"
            m.artSaved = values["tmdbKeySet"] == "1"
            m.guideKnown = true
            m.bufferKnown = true
            m.roomKnown = true
            m.daysKnown = true
            m.folderKnown = true
        }
        guard !demo else { return }
        m.storage = try? await store.api?.storage()
        if let api = store.api {
            do {
                m.showRows = try await api.storageShows().shows
                m.showError = ""
            } catch {
                m.showRows = nil
                m.showError = "Could not load space by show."
            }
        }
        do {
            m.backupRows = try await store.api?.backups() ?? []
            m.backupError = ""
        } catch {
            m.backupRows = []
            m.backupError = "Could not load backups."
        }
    }

    private func flushServerText() {
        guard !demo, m.guideKnown else { return }
        var values: [String: String] = [:]
        if m.sdUser != m.loadedUser {
            values["sdUser"] = m.sdUser
            m.loadedUser = m.sdUser
        }
        if m.sdLineup != m.loadedLineup {
            values["sdLineup"] = m.sdLineup
            m.loadedLineup = m.sdLineup
        }
        if m.reserve != m.loadedReserve {
            if let n = Int(m.reserve), (0 ... 1_000_000).contains(n), String(n) == m.reserve {
                values["watermarkGB"] = m.reserve
                m.loadedReserve = m.reserve
                m.saveError = ""
            } else {
                m.saveError = "Keep this much free needs a whole number of gigabytes from 0 to 1000000."
            }
        }
        if m.bufferMinutes != m.loadedBuffer, Self.bufferChoices.contains(m.bufferMinutes) {
            values["bufferMinutes"] = m.bufferMinutes
            m.loadedBuffer = m.bufferMinutes
        }
        if m.roomKnown, m.makeRoom != m.loadedMakeRoom, m.makeRoom == "0" || m.makeRoom == "1" {
            values["makeRoom"] = m.makeRoom
            m.loadedMakeRoom = m.makeRoom
        }
        if m.daysKnown, m.deleteWatchedDays != m.loadedDays, Self.watchedDays(m.deleteWatchedDays) == m.deleteWatchedDays {
            values["deleteWatchedDays"] = m.deleteWatchedDays
            m.loadedDays = m.deleteWatchedDays
        }
        if m.folderKnown, m.folderLayout != m.loadedFolder, m.folderLayout == "shows" || m.folderLayout == "flat" {
            values["folderLayout"] = m.folderLayout
            m.loadedFolder = m.folderLayout
        }
        let guide = m.guideURL.trimmingCharacters(in: .whitespacesAndNewlines)
        if guide != m.loadedGuide {
            if guide.isEmpty || guide.hasPrefix("http://") || guide.hasPrefix("https://") {
                values["guideUrl"] = guide
                m.guideURL = guide
                m.loadedGuide = guide
                if m.saveError.hasPrefix("The guide address") {
                    m.saveError = ""
                }
            } else {
                m.saveError = "The guide address needs to start with http."
            }
        }
        guard !values.isEmpty else { return }
        save(values)
    }

    private func saveSecret(_ key: String, _ raw: String, mark: (Bool) -> Void) {
        let value = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !value.isEmpty else { return }
        if key == "sdPassword" {
            m.sdPassword = ""
        } else {
            m.tmdbKey = ""
        }
        mark(true)
        save([key: value])
    }

    private func save(_ values: [String: String]) {
        Task {
            do {
                try await store.api?.saveSettings(values)
            } catch {
                m.saveError = error.localizedDescription
            }
        }
    }

    @MainActor
    private func restore(_ name: String) async {
        m.restoreName = nil
        m.backupBusy = name
        defer { m.backupBusy = "" }
        do {
            try await store.api?.restoreBackup(name: name)
            await store.refresh()
            m.backupRows = try await store.api?.backups() ?? m.backupRows
            m.backupError = ""
        } catch {
            m.backupError = "Could not restore that backup."
        }
    }

    @MainActor
    private func downloadBackup(_ row: CatalogBackup) async {
        guard let api = store.api else { return }
        m.backupBusy = row.name
        defer { m.backupBusy = "" }
        guard let file = await downloadFile(from: api.backupURL(name: row.name), name: row.name) else {
            m.backupError = "Could not download that backup."
            return
        }
        present(file, kind: "backup")
    }

    @MainActor
    private func downloadCatalog() async {
        guard let api = store.api else { return }
        m.backupBusy = "catalog"
        defer { m.backupBusy = "" }
        guard let file = await downloadFile(from: api.catalogBackupURL(), name: "broadwave-backup.db") else {
            m.backupError = "Could not download that backup."
            return
        }
        present(file, kind: "backup")
    }

    #if os(iOS)
        @MainActor
        private func importCatalog(_ result: Result<URL, Error>) async {
            do {
                let url = try result.get()
                let scoped = url.startAccessingSecurityScopedResource()
                defer {
                    if scoped {
                        url.stopAccessingSecurityScopedResource()
                    }
                }
                let data = try Data(contentsOf: url)
                guard !data.isEmpty else {
                    m.backupError = "Could not restore that backup."
                    return
                }
                try await store.api?.restoreCatalog(data)
                await store.refresh()
                m.backupError = ""
            } catch {
                m.backupError = "Could not restore that backup."
            }
        }
    #endif

    @MainActor
    private func downloadSupport() async {
        guard !m.supportBusy, let api = store.api else { return }
        m.supportBusy = true
        defer { m.supportBusy = false }
        m.savedNote = ""
        guard let file = await downloadFile(from: api.supportURL(), name: "broadwave-support.zip") else { return }
        present(file, kind: "support")
    }

    private func present(_ file: URL, kind: String) {
        #if os(iOS)
            _ = kind
            FileShare.present(file)
        #elseif os(tvOS)
            m.savedNote = kind
        #endif
    }

    private func downloadFile(from url: URL, name: String) async -> URL? {
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.cachePolicy = .reloadIgnoringLocalCacheData
        if let token = store.api?.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        do {
            let (temp, response) = try await URLSession.shared.download(for: request)
            guard let http = response as? HTTPURLResponse, (200 ..< 300).contains(http.statusCode) else { return nil }
            let mime = http.mimeType?.lowercased() ?? ""
            let allowed = ["application/zip", "application/octet-stream", "application/x-sqlite3", "application/vnd.sqlite3"]
            guard mime.isEmpty || allowed.contains(mime) else { return nil }
            let bytes = try temp.resourceValues(forKeys: [.fileSizeKey]).fileSize ?? 0
            guard bytes > 0 else { return nil }
            let leaf = URL(fileURLWithPath: name).lastPathComponent
            let dest = FileManager.default.temporaryDirectory.appendingPathComponent(leaf.isEmpty ? "backup.db" : leaf)
            try? FileManager.default.removeItem(at: dest)
            try FileManager.default.moveItem(at: temp, to: dest)
            return dest
        } catch {
            return nil
        }
    }
}

private extension SettingsView {
    static let bufferChoices = ["0", "30", "60", "120", "240"]

    /// Anything other than the two narrower choices is every alert.
    static func alertChoice(_ raw: String?) -> String {
        if let raw, raw == "teams" || raw == "off" {
            return raw
        }
        return "all"
    }

    /// A saved day count, or "0" when the value is missing or not a whole number of days.
    static func watchedDays(_ raw: String?) -> String {
        guard let raw, let n = Int(raw), (0 ... 3650).contains(n), String(n) == raw else { return "0" }
        return raw
    }

    /// The usual choices, plus a count saved from somewhere else, oldest first.
    static func dayChoices(_ raw: String?) -> [String] {
        let usual = ["0", "1", "3", "7", "14", "30", "60", "90"]
        let now = watchedDays(raw)
        if usual.contains(now) {
            return usual
        }
        return (usual + [now]).sorted { (Int($0) ?? 0) < (Int($1) ?? 0) }
    }

    static func dayLabel(_ days: String) -> String {
        switch days {
        case "0": "Never"
        case "1": "After 1 day"
        default: "After \(days) days"
        }
    }
}

func storageSummary(_ info: APIClient.StorageInfo) -> String {
    let free = ByteCountFormatter.string(fromByteCount: info.freeBytes, countStyle: .file)
    let total = ByteCountFormatter.string(fromByteCount: info.totalBytes, countStyle: .file)
    let reserve = info.watermarkGB == 0 ? "The free-space reserve is off." : "New recordings stop under \(info.watermarkGB) GB."
    return "\(free) free of \(total). \(reserve)"
}

func backupKindLabel(_ kind: String) -> String {
    switch kind {
    case "weekly": "Weekly"
    case "version": "Update"
    default: "Nightly"
    }
}

private extension String {
    var nilIfEmpty: String? {
        isEmpty ? nil : self
    }
}

private extension View {
    @ViewBuilder
    func fieldTyping() -> some View {
        #if os(iOS)
            autocorrectionDisabled().textInputAutocapitalization(.never)
        #else
            autocorrectionDisabled()
        #endif
    }
}

/// Keep this sentence in step with `blenderCredit` in web/src/legal.ts.
struct AboutView: View {
    @FocusState private var creditFocused: Bool

    /// The credit on its own, for the About page in the iPhone and iPad list.
    static var credit: some View {
        Section {
            Text("Store art and the demo use Big Buck Bunny, Sintel, Tears of Steel, and Elephants Dream. They are Blender Foundation films under CC BY.")
        }
    }

    var body: some View {
        Form {
            Section {
                Text("Store art and the demo use Big Buck Bunny, Sintel, Tears of Steel, and Elephants Dream. They are Blender Foundation films under CC BY.")
                    .focusable()
                    .focused($creditFocused)
            }
        }
        .navigationTitle("About")
        #if os(tvOS)
            .onAppear { creditFocused = true }
        #endif
    }
}

#if os(iOS)
    /// tvOS marks UIActivityViewController prohibited, and ShareLink is unavailable there.
    private enum FileShare {
        @MainActor static func present(_ file: URL) {
            let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
            var scene = scenes.first
            for candidate in scenes where candidate.activationState == .foregroundActive {
                scene = candidate
                break
            }
            guard let root = scene?.keyWindow?.rootViewController else { return }
            var host = root
            while let next = host.presentedViewController, !next.isBeingDismissed {
                host = next
            }
            let controller = UIActivityViewController(activityItems: [file], applicationActivities: nil)
            if let pop = controller.popoverPresentationController {
                pop.sourceView = host.view
                let bounds = host.view.bounds
                pop.sourceRect = CGRect(x: bounds.midX, y: bounds.midY, width: 1, height: 1)
                pop.permittedArrowDirections = []
            }
            host.present(controller, animated: true)
        }
    }
#endif

/// The pages of Settings: what this device keeps, then what the server keeps for every screen.
enum SettingsPage: String, CaseIterable, Identifiable, Hashable {
    case screen, playback, sync, activities, home
    case server, recording, picture, sports, guide, sources, backups, support, about

    var id: String {
        rawValue
    }

    var title: LocalizedStringKey {
        switch self {
        case .screen: "Name"
        case .playback: "Playback"
        case .sync: "Whole-Home Sync"
        case .activities: "Live Activities"
        case .home: "Your Home"
        case .server: "Server"
        case .recording: "Recording and Storage"
        case .picture: "Picture"
        case .sports: "Sports and Scores"
        case .guide: "Guide Data"
        case .sources: "Tuners and Channels"
        case .backups: "Backups"
        case .support: "Updates and Support"
        case .about: "About"
        }
    }

    /// The page with a debug anchor's section on it.
    static func holding(_ anchor: String) -> SettingsPage? {
        switch anchor {
        case "storage": .recording
        case "guide": .guide
        case "backups": .backups
        case "activities": .activities
        default: nil
        }
    }
}

/// Loads the settings once, saves typed text on the way out, and asks before a restore.
private struct SettingsWork: ViewModifier {
    var load: () async -> Void
    var leave: () -> Void
    @Binding var restoreName: String?
    var restore: (String) -> Void
    @State private var loaded = false

    func body(content: Content) -> some View {
        content
            // Not .task: pushing a page hides the list, and that cancelled the load
            // mid-request, which left the server's settings greyed out.
            .onAppear {
                guard !loaded else { return }
                loaded = true
                Task { await load() }
            }
            .onDisappear(perform: leave)
            .confirmationDialog(
                "Restore this backup? The catalog goes back to that copy. Recordings stay.",
                isPresented: Binding(get: { restoreName != nil }, set: {
                    if !$0 {
                        restoreName = nil
                    }
                }),
                titleVisibility: .visible
            ) {
                Button("Restore", role: .destructive) {
                    if let name = restoreName {
                        restore(name)
                    }
                }
                Button("Cancel", role: .cancel) {}
            }
    }
}
