import BroadwaveKit
import SwiftUI
#if os(iOS)
    import UIKit
    import UniformTypeIdentifiers
#endif

struct SettingsView: View {
    @Environment(AppStore.self) private var store
    @State private var showAbout = false
    @State private var showDiagnostics = false
    @State private var showChannels = false
    @State private var showSources = false
    @State private var showPasses = false
    @State private var checkUpdates = true
    @State private var updatesKnown = false
    @State private var liveScores = true
    @State private var scoresKnown = false
    @State private var sportsDB = false
    @State private var sportsKey = ""
    @State private var supportBusy = false
    @State private var hideScores = false
    @State private var hideScoresKnown = false
    @State private var autoplay = true
    @State private var autoplayKnown = false
    @State private var shareTuner = false
    @State private var shareKnown = false
    @State private var pictureMode = "broadcast"
    @State private var pictureKnown = false
    @State private var sdUser = ""
    @State private var sdLineup = ""
    @State private var guideURL = ""
    @AppStorage(BreakSkip.key) private var breakSkip = BreakSkip.auto
    @State private var reserve = "10"
    @State private var sdPassword = ""
    @State private var tmdbKey = ""
    @State private var passwordSaved = false
    @State private var artSaved = false
    @State private var guideKnown = false
    @State private var loadedUser = ""
    @State private var loadedLineup = ""
    @State private var loadedGuide = ""
    @State private var loadedReserve = "10"
    @State private var storage: APIClient.StorageInfo?
    @State private var backupRows: [CatalogBackup]?
    @State private var backupError = ""
    @State private var backupBusy = ""
    @State private var restoreName: String?
    @State private var importBackup = false
    @State private var saveError = ""
    @State private var savedNote = ""

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

    @ViewBuilder
    private func settingsForm(_ store: AppStore) -> some View {
        @Bindable var store = store
        Form {
            if !saveError.isEmpty {
                Section {
                    Text(saveError)
                        .foregroundStyle(.red)
                }
            }
            Section {
                HomeListView(grabsFocus: true)
            }
            serverSection
            playbackSection(store)
            pictureSection
            if demo {
                Section {
                    Text("The demo does not change a guide account, a folder, or a backup.")
                        .foregroundStyle(.secondary)
                }
            } else {
                guideSection.id("guide")
            }
            sportsSection
            dvrSection
            sourcesSection
            syncSection(store)
            if !demo {
                storageSection.id("storage")
                backupSection.id("backups")
            }
            updatesSection
            supportSection
            if !demo {
                Section {
                    Button("Diagnostics") { showDiagnostics = true }
                } footer: {
                    Text("Tuner health, the guide, and the last antenna reading.")
                }
            }
            Section {
                Button("About") { showAbout = true }
                    .accessibilityLabel("About Broadwave")
            }
            Section {
                Text("This server is for the home network. A password is required before remote access.")
                    .foregroundStyle(.secondary)
            }
        }
        .navigationTitle("Settings")
        .task { await load() }
        .onDisappear { flushServerText() }
        .navigationDestination(isPresented: $showAbout) { AboutView() }
        .navigationDestination(isPresented: $showDiagnostics) { DiagnosticsView() }
        .navigationDestination(isPresented: $showChannels) { ChannelsView() }
        .navigationDestination(isPresented: $showSources) { SourcesView() }
        .navigationDestination(isPresented: $showPasses) { PassesView() }
        .confirmationDialog(
            "Restore this backup? The catalog goes back to that copy. Recordings stay.",
            isPresented: Binding(
                get: { restoreName != nil },
                set: {
                    if !$0 {
                        restoreName = nil
                    }
                }
            ),
            titleVisibility: .visible
        ) {
            Button("Restore", role: .destructive) {
                if let name = restoreName {
                    Task { await restore(name) }
                }
            }
            Button("Cancel", role: .cancel) {}
        }
        #if os(iOS)
        .fileImporter(isPresented: $importBackup, allowedContentTypes: [.data]) { result in
            Task { await importCatalog(result) }
        }
        #endif
    }

    private func openDebug(_ proxy: ScrollViewProxy) {
        #if DEBUG
            if UserDefaults.standard.bool(forKey: "BroadwaveAbout") {
                showAbout = true
            }
            if UserDefaults.standard.bool(forKey: "BroadwaveDiagnostics") {
                showDiagnostics = true
            }
            if UserDefaults.standard.bool(forKey: "BroadwaveSources") {
                showSources = true
            }
            if UserDefaults.standard.bool(forKey: "BroadwaveChannels") {
                showChannels = true
            }
            if UserDefaults.standard.bool(forKey: "BroadwavePasses") {
                showPasses = true
            }
            if let anchor = UserDefaults.standard.string(forKey: "BroadwaveSettingsAnchor") {
                Task {
                    try? await Task.sleep(for: .milliseconds(700))
                    proxy.scrollTo(anchor, anchor: .top)
                }
            }
        #else
            _ = proxy
        #endif
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

    private var pictureSection: some View {
        Section {
            Picker("Picture", selection: Binding(
                get: { pictureMode },
                set: { mode in
                    guard pictureKnown else { return }
                    pictureMode = mode
                    save(["pictureMode": mode])
                }
            )) {
                Text("Broadcast").tag("broadcast")
                Text("Smooth").tag("smooth")
                Text("Film").tag("film")
            }
            .disabled(!pictureKnown)
        } footer: {
            Text("Broadcast rebuilds interlaced channels at 60 frames a second. Smooth adds motion compensation when this server can hold it. Film is for movies.")
        }
    }

    private var guideSection: some View {
        Section {
            TextField("Schedules Direct", text: $sdUser)
                .fieldTyping()
                .onSubmit { flushServerText() }
            SecureField("Password", text: $sdPassword, prompt: passwordSaved ? Text("Saved") : nil)
                .onSubmit { saveSecret("sdPassword", sdPassword) { passwordSaved = $0 } }
            TextField("Lineup", text: $sdLineup)
                .fieldTyping()
                .onSubmit { flushServerText() }
            TextField("Guide address", text: $guideURL)
                .fieldTyping()
                .onSubmit { flushServerText() }
            SecureField("Movie artwork", text: $tmdbKey, prompt: artSaved ? Text("Saved") : nil)
                .onSubmit { saveSecret("tmdbKey", tmdbKey) { artSaved = $0 } }
        } header: {
            Text("Guide")
        } footer: {
            Text("Schedules Direct fills channels the tuner guide skips. Fourteen days when the account allows it. An XMLTV link does the same. Movie artwork fills posters the guide does not include.")
        }
    }

    private var sportsSection: some View {
        Section {
            Toggle("Live scores", isOn: Binding(
                get: { liveScores },
                set: { on in
                    guard scoresKnown else { return }
                    liveScores = on
                    Task {
                        try? await store.api?.saveSettings(["liveScores": on ? "1" : "0"])
                        await store.refresh(lineup: false)
                    }
                }
            ))
            .disabled(!scoresKnown)
            Toggle("Hide scores", isOn: Binding(
                get: { hideScores },
                set: { on in
                    guard hideScoresKnown else { return }
                    hideScores = on
                    save(["hideScores": on ? "1" : "0"])
                }
            ))
            .disabled(!hideScoresKnown)
            Text("A recorded game stays hidden until you watch it.")
                .font(.caption)
                .foregroundStyle(.secondary)
            SecureField("TheSportsDB key", text: $sportsKey)
                .onSubmit {
                    let key = sportsKey.trimmingCharacters(in: .whitespacesAndNewlines)
                    sportsKey = ""
                    guard !key.isEmpty else { return }
                    Task {
                        try? await store.api?.saveSettings(["sportsdbKey": key])
                        sportsDB = true
                    }
                }
        } header: {
            Text(sportsDB ? "Scores from TheSportsDB" : "Scores from ESPN's public scoreboard")
        } footer: {
            Text("On asks your server for the scoreboard. Off sends nothing. A key is optional and never included.")
        }
    }

    private var dvrSection: some View {
        Section {
            Toggle("Play the next episode", isOn: Binding(
                get: { autoplay },
                set: { on in
                    guard autoplayKnown else { return }
                    autoplay = on
                    save(["autoplay": on ? "1" : "0"])
                }
            ))
            .disabled(!autoplayKnown)
            Picker("Commercial breaks", selection: $breakSkip) {
                ForEach(BreakSkip.allCases, id: \.self) { Text($0.label).tag($0) }
            }
            Button("Series passes") { showPasses = true }
        } header: {
            Text("DVR")
        } footer: {
            Text("For recordings with marked breaks. Kept on this device.")
        }
    }

    private var sourcesSection: some View {
        Section {
            if !demo {
                Button("Tuners and playlists") { showSources = true }
            }
            Button("Channels") { showChannels = true }
            if demo {
                Text("The demo does not share a tuner.")
                    .foregroundStyle(.secondary)
            } else if let message = Compatibility.gateFeature(store.info, "hdhrEmulation") {
                Text(message)
                    .foregroundStyle(.secondary)
            } else {
                Toggle("Offer this server as an HDHomeRun on port 8478", isOn: Binding(
                    get: { shareTuner },
                    set: { on in
                        guard shareKnown else { return }
                        shareTuner = on
                        save(["hdhrEmulate": on ? "1" : "0"])
                    }
                ))
                .disabled(!shareKnown)
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
            }
        } footer: {
            if Compatibility.gateFeature(store.info, "wholeHomeSync") == nil {
                Text("Every screen on the same channel shows the same moment, so nobody hears the next room cheer first.")
            }
        }
    }

    private var storageSection: some View {
        Section {
            if let storage {
                Text(storageSummary(storage))
                    .foregroundStyle(.secondary)
            }
            if let path = storage?.path, !path.isEmpty {
                LabeledContent("Recordings folder", value: path)
            }
            LabeledContent("Keep this much free") {
                TextField("GB", text: $reserve)
                    .multilineTextAlignment(.trailing)
                    .fieldTyping()
                    .accessibilityLabel("Gigabytes to keep free")
                #if os(iOS)
                    .keyboardType(.numberPad)
                #endif
                    .onSubmit { flushServerText() }
            }
        } header: {
            Text("Storage")
        } footer: {
            Text("The container writes the original broadcast files on the recordings share. Use 0 to turn the reserve off. A show already recording is left alone.")
        }
    }

    private var backupSection: some View {
        Section {
            if let backupError = backupError.nilIfEmpty {
                Text(backupError)
                    .foregroundStyle(.red)
            }
            if let rows = backupRows, rows.isEmpty {
                Text("No backups yet.")
                    .foregroundStyle(.secondary)
            }
            ForEach(backupRows ?? [], id: \.name) { row in
                VStack(alignment: .leading, spacing: 6) {
                    Text("\(backupKindLabel(row.kind)) · \(row.takenAt.formatted(date: .abbreviated, time: .shortened))")
                    Text(ByteCountFormatter.string(fromByteCount: row.bytes, countStyle: .file))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    HStack {
                        Button("Download") { Task { await downloadBackup(row) } }
                            .disabled(backupBusy != "")
                            .accessibilityLabel("Download \(backupKindLabel(row.kind)) \(row.takenAt.formatted(date: .abbreviated, time: .shortened))")
                        Button("Restore") { restoreName = row.name }
                            .disabled(backupBusy != "")
                            .accessibilityLabel("Restore \(backupKindLabel(row.kind)) \(row.takenAt.formatted(date: .abbreviated, time: .shortened))")
                    }
                }
            }
            Button("Download backup") { Task { await downloadCatalog() } }
                .disabled(backupBusy != "" || store.api == nil)
            #if os(iOS)
                Button("Restore a catalog backup") { importBackup = true }
                    .disabled(backupBusy != "" || store.api == nil)
            #endif
            #if os(tvOS)
                if savedNote == "backup" {
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
                get: { checkUpdates },
                set: { on in
                    guard updatesKnown else { return }
                    checkUpdates = on
                    Task {
                        try? await store.api?.saveSettings(["checkUpdates": on ? "1" : "0"])
                        await store.refresh(lineup: false)
                    }
                }
            ))
            .disabled(!updatesKnown)
        } footer: {
            Text("Once a day. Nothing else is sent.")
        }
    }

    private var supportSection: some View {
        Section {
            Button("Download a support bundle") {
                Task { await downloadSupport() }
            }
            .disabled(supportBusy || store.api == nil)
            #if os(tvOS)
                if savedNote == "support" {
                    Text("Saved on this Apple TV.")
                }
            #endif
        } footer: {
            Text("Logs, versions, and settings. Passwords are left out.")
        }
    }

    @MainActor
    private func load() async {
        if let values = try? await store.api?.settings() {
            checkUpdates = values["checkUpdates"] != "0"
            updatesKnown = true
            liveScores = values["liveScores"] != "0"
            scoresKnown = true
            sportsDB = values["sportsdbKeySet"] == "1"
            hideScores = values["hideScores"] == "1"
            hideScoresKnown = true
            autoplay = values["autoplay"] != "0"
            autoplayKnown = true
            shareTuner = values["hdhrEmulate"] == "1"
            shareKnown = true
            if let mode = values["pictureMode"], mode == "broadcast" || mode == "smooth" || mode == "film" {
                pictureMode = mode
            }
            pictureKnown = true
            sdUser = values["sdUser"] ?? ""
            sdLineup = values["sdLineup"] ?? ""
            guideURL = values["guideUrl"] ?? ""
            reserve = values["watermarkGB"] ?? "10"
            loadedUser = sdUser
            loadedLineup = sdLineup
            loadedGuide = guideURL
            loadedReserve = reserve
            passwordSaved = values["sdPasswordSet"] == "1"
            artSaved = values["tmdbKeySet"] == "1"
            guideKnown = true
        }
        guard !demo else { return }
        storage = try? await store.api?.storage()
        do {
            backupRows = try await store.api?.backups() ?? []
            backupError = ""
        } catch {
            backupRows = []
            backupError = "Could not load backups."
        }
    }

    private func flushServerText() {
        guard !demo, guideKnown else { return }
        var values: [String: String] = [:]
        if sdUser != loadedUser {
            values["sdUser"] = sdUser
            loadedUser = sdUser
        }
        if sdLineup != loadedLineup {
            values["sdLineup"] = sdLineup
            loadedLineup = sdLineup
        }
        if reserve != loadedReserve {
            if let n = Int(reserve), (0 ... 1_000_000).contains(n), String(n) == reserve {
                values["watermarkGB"] = reserve
                loadedReserve = reserve
                saveError = ""
            } else {
                saveError = "Keep this much free needs a whole number of gigabytes from 0 to 1000000."
            }
        }
        let guide = guideURL.trimmingCharacters(in: .whitespacesAndNewlines)
        if guide != loadedGuide {
            if guide.isEmpty || guide.hasPrefix("http://") || guide.hasPrefix("https://") {
                values["guideUrl"] = guide
                guideURL = guide
                loadedGuide = guide
                if saveError.hasPrefix("The guide address") {
                    saveError = ""
                }
            } else {
                saveError = "The guide address needs to start with http."
            }
        }
        guard !values.isEmpty else { return }
        save(values)
    }

    private func saveSecret(_ key: String, _ raw: String, mark: (Bool) -> Void) {
        let value = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !value.isEmpty else { return }
        if key == "sdPassword" {
            sdPassword = ""
        } else {
            tmdbKey = ""
        }
        mark(true)
        save([key: value])
    }

    private func save(_ values: [String: String]) {
        Task {
            do {
                try await store.api?.saveSettings(values)
            } catch {
                saveError = error.localizedDescription
            }
        }
    }

    @MainActor
    private func restore(_ name: String) async {
        restoreName = nil
        backupBusy = name
        defer { backupBusy = "" }
        do {
            try await store.api?.restoreBackup(name: name)
            await store.refresh()
            backupRows = try await store.api?.backups() ?? backupRows
            backupError = ""
        } catch {
            backupError = "Could not restore that backup."
        }
    }

    @MainActor
    private func downloadBackup(_ row: CatalogBackup) async {
        guard let api = store.api else { return }
        backupBusy = row.name
        defer { backupBusy = "" }
        guard let file = await downloadFile(from: api.backupURL(name: row.name), name: row.name) else {
            backupError = "Could not download that backup."
            return
        }
        present(file, kind: "backup")
    }

    @MainActor
    private func downloadCatalog() async {
        guard let api = store.api else { return }
        backupBusy = "catalog"
        defer { backupBusy = "" }
        guard let file = await downloadFile(from: api.catalogBackupURL(), name: "broadwave-backup.db") else {
            backupError = "Could not download that backup."
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
                    backupError = "Could not restore that backup."
                    return
                }
                try await store.api?.restoreCatalog(data)
                await store.refresh()
                backupError = ""
            } catch {
                backupError = "Could not restore that backup."
            }
        }
    #endif

    @MainActor
    private func downloadSupport() async {
        guard !supportBusy, let api = store.api else { return }
        supportBusy = true
        defer { supportBusy = false }
        savedNote = ""
        guard let file = await downloadFile(from: api.supportURL(), name: "broadwave-support.zip") else { return }
        present(file, kind: "support")
    }

    private func present(_ file: URL, kind: String) {
        #if os(iOS)
            _ = kind
            FileShare.present(file)
        #elseif os(tvOS)
            savedNote = kind
        #endif
    }

    private func downloadFile(from url: URL, name: String) async -> URL? {
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.cachePolicy = .reloadIgnoringLocalCacheData
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
