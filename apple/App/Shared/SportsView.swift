import BroadwaveKit
import BroadwaveUI
import SwiftUI
#if os(iOS)
    import UIKit
#endif

struct SportsView: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @State private var scores: [String: String] = [:]

    var body: some View {
        let games = store.sports(hours: 7 * 24)
        let live = games.filter { $0.1.isOn(at: store.now) }
        let later = games.filter { !$0.1.isOn(at: store.now) }
        let days = Dictionary(grouping: later) { Calendar.current.startOfDay(for: $0.1.start) }
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 28) {
                if games.isEmpty {
                    ContentUnavailableView("No games in the guide", systemImage: "sportscourt", description: Text("Sports on your channels show up here as soon as they're listed."))
                        .padding(.top, 60)
                }
                if !live.isEmpty {
                    section("Live now", live)
                }
                ForEach(days.keys.sorted(), id: \.self) { day in
                    section(day.formatted(.dateTime.weekday(.wide).month().day()), days[day] ?? [])
                }
            }
            .padding(.vertical)
        }
        .navigationTitle("Sports")
        .task {
            guard let api = store.api else { return }
            let games = await (try? api.scoreboard()) ?? []
            var map: [String: String] = [:]
            for game in games {
                if let line = game.line {
                    map[game.id] = line
                }
            }
            scores = map
        }
        .toolbar {
            if live.count >= 2 {
                Button("Watch together") {
                    nowPlaying.watchTogether(live.prefix(4).map(\.0))
                }
            }
        }
    }

    private func section(_ title: String, _ list: [(Channel, Airing)]) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title).font(.title2.weight(.bold)).padding(.horizontal)
            LazyVGrid(columns: [GridItem(.adaptive(minimum: cardWidth), spacing: 14)], spacing: 14) {
                ForEach(list, id: \.1.id) { channel, airing in
                    Button {
                        if airing.isOn(at: store.now) {
                            nowPlaying.play(channel)
                        }
                    } label: {
                        GameCard(channel: channel, airing: airing, now: store.now, score: airing.gameId.flatMap { scores[$0] })
                    }
                    .cardButton()
                    .contextMenu { ChannelActions(channel: channel, airing: airing) }
                }
            }
            .padding(.horizontal)
        }
    }
}

struct RecordingsView: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        let groups = Dictionary(grouping: store.recordings) { $0.title }
        List {
            if store.recordings.isEmpty {
                ContentUnavailableView("No recordings yet", systemImage: "record.circle", description: Text("Record from the guide, or set a series to record every episode."))
            }
            ForEach(groups.keys.sorted(), id: \.self) { title in
                Section(title) {
                    ForEach(groups[title] ?? []) { rec in
                        NavigationLink(value: rec) {
                            HStack(spacing: 14) {
                                RecordingPoster(recording: rec)
                                    .frame(width: 120, height: 68)
                                    .clipShape(.rect(cornerRadius: Tokens.Radius.sm))
                                VStack(alignment: .leading, spacing: 4) {
                                    HStack(spacing: 6) {
                                        if rec.isRecording {
                                            LiveDot("Recording")
                                        }
                                        Text(rec.subtitle ?? rec.title).font(.headline).lineLimit(1)
                                    }
                                    Text(rec.startedAt.formatted(date: .abbreviated, time: .shortened)).font(.caption).foregroundStyle(.secondary)
                                    if let pos = rec.position, let dur = rec.durationSec, dur > 0, pos > 30 {
                                        AiringProgress(pos / dur).frame(maxWidth: 160)
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
        .navigationTitle("Recordings")
        .navigationDestination(for: Recording.self) { RecordingPlayerScreen(recording: $0) }
        .task { await store.refreshRecordings() }
    }
}

struct SettingsView: View {
    @Environment(AppStore.self) private var store
    @State private var showAbout = false
    @State private var checkUpdates = true
    @State private var updatesKnown = false
    @State private var liveScores = true
    @State private var scoresKnown = false
    @State private var sportsDB = false
    @State private var sportsKey = ""
    @State private var supportBusy = false
    #if os(tvOS)
        @State private var supportSaved = false
    #endif

    var body: some View {
        @Bindable var store = store
        Form {
            Section {
                HomeListView()
            }
            Section("Server") {
                LabeledContent("Name", value: store.info?.name ?? store.server?.name ?? "")
                LabeledContent("Address", value: store.server?.url.absoluteString ?? "")
                if let info = store.info {
                    LabeledContent("Version", value: info.version)
                    if let enc = info.encoder {
                        LabeledContent("Encoding", value: enc.replacingOccurrences(of: "h264_", with: "").uppercased())
                    }
                }
                Button("Run setup again") { store.presentSetup = true }
                Button("Use a different server", role: .destructive) { store.forget() }
            }
            Section {
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
            Section {
                if let message = Compatibility.gateFeature(store.info, "wholeHomeSync") {
                    Text(message)
                        .foregroundStyle(.secondary)
                } else {
                    Toggle("Whole-Home Sync", isOn: $store.syncEnabled)
                }
            } footer: {
                if Compatibility.gateFeature(store.info, "wholeHomeSync") == nil {
                    Text("Every screen on the same channel shows the same moment, so nobody hears the next room cheer first.")
                }
            }
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
            Section {
                Button("Download a support bundle") {
                    Task { await downloadSupport() }
                }
                .disabled(supportBusy || store.api == nil)
                #if os(tvOS)
                    if supportSaved {
                        Text("Saved on this Apple TV.")
                    }
                #endif
            } footer: {
                Text("Logs, versions, and settings. Passwords are left out.")
            }
            Section {
                Button("About") { showAbout = true }
                    .accessibilityLabel("About Broadwave")
            }
        }
        .navigationTitle("Settings")
        .task {
            if let values = try? await store.api?.settings() {
                checkUpdates = values["checkUpdates"] != "0"
                updatesKnown = true
                liveScores = values["liveScores"] != "0"
                scoresKnown = true
                sportsDB = values["sportsdbKeySet"] == "1"
            }
        }
        .navigationDestination(isPresented: $showAbout) {
            AboutView()
        }
        #if DEBUG
        .onAppear {
            if UserDefaults.standard.bool(forKey: "BroadwaveAbout") {
                showAbout = true
            }
        }
        #endif
    }

    @MainActor
    private func downloadSupport() async {
        guard !supportBusy, let api = store.api else { return }
        supportBusy = true
        defer { supportBusy = false }
        #if os(tvOS)
            supportSaved = false
        #endif
        guard let file = await supportZip(from: api.supportURL()) else { return }
        #if os(iOS)
            SupportShare.present(file)
        #elseif os(tvOS)
            supportSaved = true
        #endif
    }

    private func supportZip(from url: URL) async -> URL? {
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.cachePolicy = .reloadIgnoringLocalCacheData
        do {
            let (temp, response) = try await URLSession.shared.download(for: request)
            guard let http = response as? HTTPURLResponse, (200 ..< 300).contains(http.statusCode) else { return nil }
            let mime = http.mimeType?.lowercased() ?? ""
            guard mime == "application/zip" || mime == "application/octet-stream" else { return nil }
            let bytes = try temp.resourceValues(forKeys: [.fileSizeKey]).fileSize ?? 0
            guard bytes > 0 else { return nil }
            let dest = FileManager.default.temporaryDirectory.appendingPathComponent("broadwave-support.zip")
            try? FileManager.default.removeItem(at: dest)
            try FileManager.default.moveItem(at: temp, to: dest)
            return dest
        } catch {
            return nil
        }
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
    private enum SupportShare {
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
