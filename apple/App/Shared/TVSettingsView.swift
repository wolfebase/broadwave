#if os(tvOS)
    import BroadwaveKit
    import BroadwaveUI
    import CoreImage.CIFilterBuiltins
    import SwiftUI

    /// What this server says for the settings an Apple TV shows. Saved as soon as one changes.
    @MainActor
    @Observable
    final class TVServerSettings {
        var pictureMode = "broadcast"
        var autoplay = true
        var hideScores = false
        var gameAlerts = "all"
        var seat: ClientSeat?
        var known = false
        var problem = ""

        func load(_ api: APIClient?) async {
            guard let api else { return }
            seat = try? await api.clientMe()
            guard let values = try? await api.settings() else { return }
            if let mode = values["pictureMode"], ["broadcast", "smooth", "film"].contains(mode) {
                pictureMode = mode
            }
            autoplay = values["autoplay"] != "0"
            hideScores = values["hideScores"] == "1"
            let alerts = values["gameAlerts"] ?? "all"
            gameAlerts = alerts == "teams" || alerts == "off" ? alerts : "all"
            known = true
        }

        func save(_ api: APIClient?, _ values: [String: String]) {
            guard known else { return }
            Task {
                do {
                    try await api?.saveSettings(values)
                    problem = ""
                } catch {
                    problem = error.localizedDescription
                }
            }
        }
    }

    /// Apple TV settings in the system's list and detail pattern. Only what a viewer on the
    /// couch changes lives here. Tuners, guide accounts, storage, backups, and devices are
    /// on the web app, which this page points to with an address and a code to scan.
    struct TVSettingsView: View {
        @Environment(AppStore.self) private var store
        @State private var server = TVServerSettings()
        @AppStorage(BreakSkip.key) private var breakSkip = BreakSkip.auto
        @State private var showAbout = false
        @State private var showDiagnostics = false
        @State private var showChannels = false
        @FocusState private var first: Bool

        private var demo: Bool {
            store.server?.id == "demo"
        }

        var body: some View {
            TVSettingsPage(symbol: "gearshape.fill", title: "Settings", caption: serverLine) {
                Section {
                    link("Playback and Picture", symbol: "play.rectangle.fill", value: store.prefs.quality.title, id: "settings-playback") {
                        TVPlaybackSettings(server: server, breakSkip: $breakSkip)
                    }
                    .focused($first)
                    link("Captions and Audio", symbol: "captions.bubble.fill", value: store.prefs.audio.title, id: "settings-audio") {
                        TVAudioSettings()
                    }
                    link("Live Delay", symbol: "clock.arrow.circlepath", value: LiveDelay.saved.title, id: "settings-delay") {
                        TVDelaySettings()
                    }
                    link("Whole-Home Sync", symbol: "dot.radiowaves.left.and.right", value: syncValue, id: "settings-sync") {
                        TVSyncSettings()
                    }
                    link("Sports", symbol: "trophy.fill", value: server.hideScores ? "Scores hidden" : "Scores shown", id: "settings-sports") {
                        TVSportsSettings(server: server)
                    }
                }
                Section {
                    link("This Screen's Name", symbol: "appletv.fill", value: store.screenName.isEmpty ? ScreenIdentity.name : store.screenName, id: "settings-name") {
                        TVNameSettings()
                    }
                    link(demo ? "Demo" : "Server and Sign-In", symbol: "server.rack", value: store.info?.name ?? store.server?.name ?? "", id: "settings-server") {
                        TVServerDetails(server: server)
                    }
                    if !demo, store.server.flatMap({ TVWebSettings.address($0.url) }) != nil {
                        link("More Settings", symbol: "laptopcomputer", value: "On your computer", id: "settings-more") {
                            TVWebSettings()
                        }
                    }
                    link("About", symbol: "info.circle.fill", value: "", id: "settings-about") {
                        TVAboutSettings()
                    }
                }
                if !server.problem.isEmpty {
                    Section {
                        Text(server.problem).foregroundStyle(.red)
                    }
                }
            }
            .defaultFocus($first, true)
            .task { await server.load(store.api) }
            .navigationDestination(isPresented: $showAbout) { TVAboutSettings() }
            .navigationDestination(isPresented: $showDiagnostics) { DiagnosticsView() }
            .navigationDestination(isPresented: $showChannels) { ChannelsView() }
            .onAppear(perform: openDebug)
        }

        private var serverLine: String {
            let name = store.info?.name ?? store.server?.name ?? ""
            return name.isEmpty ? "" : "Connected to \(name)"
        }

        private var syncValue: String {
            guard store.syncEnabled else { return "Off" }
            return store.lineUpAutomatically ? "Automatic" : "On request"
        }

        private func link(_ title: LocalizedStringKey, symbol: String, value: String, id: String, @ViewBuilder destination: @escaping () -> some View) -> some View {
            NavigationLink(destination: destination) {
                HStack(spacing: 24) {
                    Image(systemName: symbol)
                        .frame(width: 44)
                        .accessibilityHidden(true)
                    Text(title)
                    Spacer(minLength: 24)
                    Text(value)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            .accessibilityIdentifier(id)
        }

        private func openDebug() {
            #if DEBUG
                let defaults = UserDefaults.standard
                showAbout = defaults.bool(forKey: "BroadwaveAbout")
                showDiagnostics = defaults.bool(forKey: "BroadwaveDiagnostics")
                // The 3.0 / 1.0 choice test edits a channel here. Viewers do that on the web app.
                showChannels = defaults.bool(forKey: "BroadwaveChannels")
            #endif
        }
    }

    /// The tvOS settings layout: a large symbol and the page's name on the left, the list on the right.
    struct TVSettingsPage<Rows: View>: View {
        let symbol: String
        let title: LocalizedStringKey
        var caption: String = ""
        @ViewBuilder var rows: Rows

        var body: some View {
            HStack(alignment: .top, spacing: 80) {
                VStack(spacing: 36) {
                    Spacer(minLength: 0)
                    Image(systemName: symbol)
                        .font(.system(size: 220, weight: .regular))
                        .foregroundStyle(.white.opacity(0.85))
                        .symbolRenderingMode(.hierarchical)
                        .frame(height: 300)
                        .accessibilityHidden(true)
                    VStack(spacing: 12) {
                        Text(title)
                            .font(.title2.weight(.bold))
                            .multilineTextAlignment(.center)
                            .accessibilityAddTraits(.isHeader)
                        if !caption.isEmpty {
                            Text(caption)
                                .font(.callout)
                                .foregroundStyle(.secondary)
                                .multilineTextAlignment(.center)
                        }
                    }
                    Spacer(minLength: 0)
                }
                .frame(width: 640)
                .frame(maxHeight: .infinity)
                List { rows }
                    .frame(maxWidth: .infinity)
            }
            .tvPageChrome()
        }
    }

    // MARK: Details

    private struct TVPlaybackSettings: View {
        @Environment(AppStore.self) private var store
        let server: TVServerSettings
        @Binding var breakSkip: BreakSkip

        var body: some View {
            @Bindable var store = store
            TVSettingsPage(symbol: "play.rectangle.fill", title: "Playback and Picture") {
                Section {
                    TVChoiceRow(
                        "Quality",
                        symbol: "play.rectangle.fill",
                        selection: $store.prefs.quality,
                        choices: [Prefs.Quality.auto, .original, .high, .medium, .saver].map { ($0, $0.title) }
                    )
                    .accessibilityIdentifier("settings-quality")
                } footer: {
                    Text("Auto plays the original broadcast whenever this Apple TV and the network can.")
                }
                Section {
                    TVChoiceRow(
                        "Picture",
                        symbol: "play.rectangle.fill",
                        selection: Binding(get: { server.pictureMode }, set: { mode in
                            server.pictureMode = mode
                            server.save(store.api, ["pictureMode": mode])
                        }),
                        choices: [("broadcast", "Broadcast"), ("smooth", "Smooth"), ("film", "Film")]
                    )
                    .disabled(!server.known)
                } footer: {
                    Text("Broadcast plays interlaced channels at 60 frames a second. Smooth adds motion smoothing when the server can keep up. Film suits movies. This applies to every screen.")
                }
                Section {
                    TVChoiceRow(
                        "Commercial Breaks",
                        symbol: "play.rectangle.fill",
                        selection: $breakSkip,
                        choices: BreakSkip.allCases.map { ($0, $0.label) }
                    )
                    Toggle("Play the Next Episode", isOn: Binding(get: { server.autoplay }, set: { on in
                        server.autoplay = on
                        server.save(store.api, ["autoplay": on ? "1" : "0"])
                    }))
                    .disabled(!server.known)
                } footer: {
                    Text("For recordings. Breaks are skipped only where the server has marked them.")
                }
            }
        }
    }

    private struct TVAudioSettings: View {
        @Environment(AppStore.self) private var store

        var body: some View {
            @Bindable var store = store
            TVSettingsPage(symbol: "captions.bubble.fill", title: "Captions and Audio") {
                Section {
                    TVChoiceRow(
                        "Sound",
                        symbol: "captions.bubble.fill",
                        selection: $store.prefs.audio,
                        choices: [Prefs.Sound.auto, .surround, .stereo].map { ($0, $0.title) }
                    )
                    TVChoiceRow(
                        "Audio Track",
                        symbol: "captions.bubble.fill",
                        selection: Binding(get: { store.prefs.track ?? "main" }, set: { track in
                            store.prefs.track = track == "main" ? nil : track
                        }),
                        choices: [("main", "Main"), ("language", "Second language"), ("described", "Described video")]
                    )
                    Toggle("Even Volume", isOn: $store.prefs.even)
                } footer: {
                    Text("Auto plays Dolby Digital when the TV or receiver takes it. Even volume keeps loud ads and quiet shows at one level.")
                }
                Section {
                    Text("Captions follow this Apple TV")
                        .focusable()
                } footer: {
                    Text("Turn them on in the Apple TV's Settings, under Accessibility, then Subtitles and Captioning. While watching, swipe down to switch them for one show.")
                }
            }
        }
    }

    private struct TVDelaySettings: View {
        @State private var delay = LiveDelay.saved

        var body: some View {
            TVChoiceList(
                title: "Live Delay",
                symbol: "clock.arrow.circlepath",
                selection: Binding(get: { delay }, set: { choice in
                    delay = choice
                    LiveDelay.saved = choice
                }),
                choices: LiveDelay.allCases.map { ($0, $0.title) },
                footer: "How far behind the broadcast a channel plays when this Apple TV starts it. Balanced is about 16 seconds and starts fastest. Stable is 20 and rides out a weak signal."
            )
        }
    }

    /// A setting's row in a list: its name and current value. Select opens the choices.
    struct TVChoiceRow<Value: Hashable>: View {
        let title: String
        let symbol: String
        @Binding var selection: Value
        let choices: [(Value, String)]

        init(_ title: String, symbol: String, selection: Binding<Value>, choices: [(Value, String)]) {
            self.title = title
            self.symbol = symbol
            _selection = selection
            self.choices = choices
        }

        var body: some View {
            NavigationLink {
                TVChoiceList(title: title, symbol: symbol, selection: $selection, choices: choices, popOnChoice: true)
            } label: {
                HStack(spacing: 24) {
                    Text(title)
                    Spacer(minLength: 24)
                    Text(choices.first { $0.0 == selection }?.1 ?? "")
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
        }
    }

    /// The choices for one setting, with a check on the current one.
    struct TVChoiceList<Value: Hashable>: View {
        @Environment(\.dismiss) private var dismiss
        let title: String
        let symbol: String
        @Binding var selection: Value
        let choices: [(Value, String)]
        var footer = ""
        var popOnChoice = false
        @FocusState private var focus: Int?

        var body: some View {
            TVSettingsPage(symbol: symbol, title: LocalizedStringKey(title)) {
                Section {
                    ForEach(Array(choices.enumerated()), id: \.offset) { index, choice in
                        Button {
                            selection = choice.0
                            if popOnChoice {
                                dismiss()
                            }
                        } label: {
                            HStack {
                                Text(choice.1)
                                Spacer()
                                if choice.0 == selection {
                                    Image(systemName: "checkmark")
                                        .accessibilityHidden(true)
                                }
                            }
                        }
                        .focused($focus, equals: index)
                        .accessibilityAddTraits(choice.0 == selection ? .isSelected : [])
                    }
                } footer: {
                    if !footer.isEmpty {
                        Text(footer)
                    }
                }
            }
            .defaultFocus($focus, choices.firstIndex { $0.0 == selection } ?? 0)
        }
    }

    private struct TVSyncSettings: View {
        @Environment(AppStore.self) private var store

        var body: some View {
            @Bindable var store = store
            TVSettingsPage(symbol: "dot.radiowaves.left.and.right", title: "Whole-Home Sync") {
                if let message = Compatibility.gateFeature(store.info, "wholeHomeSync") {
                    Section {
                        Text(message).focusable()
                    }
                } else {
                    Section {
                        Toggle("Whole-Home Sync", isOn: $store.syncEnabled)
                        if store.syncEnabled {
                            Toggle("Line Up Automatically", isOn: $store.lineUpAutomatically)
                        }
                    } footer: {
                        Text(store.syncEnabled && !store.lineUpAutomatically
                            ? "Channels start right away. Choose Sync screens in the player to put every screen on the same moment."
                            : "Every screen on the same channel shows the same moment, so nobody hears the next room cheer first. A channel can take a few seconds longer to start.")
                    }
                }
            }
        }
    }

    private struct TVSportsSettings: View {
        @Environment(AppStore.self) private var store
        let server: TVServerSettings

        var body: some View {
            TVSettingsPage(symbol: "trophy.fill", title: "Sports") {
                Section {
                    Toggle("Hide Scores", isOn: Binding(get: { server.hideScores }, set: { on in
                        server.hideScores = on
                        server.save(store.api, ["hideScores": on ? "1" : "0"])
                    }))
                    .disabled(!server.known)
                } footer: {
                    Text("A game you recorded keeps its score hidden until you watch it.")
                }
                Section {
                    TVChoiceRow(
                        "Game Alerts",
                        symbol: "trophy.fill",
                        selection: Binding(get: { server.gameAlerts }, set: { next in
                            server.gameAlerts = next
                            server.save(store.api, ["gameAlerts": next])
                        }),
                        choices: [("all", "Your teams and close games"), ("teams", "Your teams only"), ("off", "Off")]
                    )
                    .disabled(!server.known)
                } footer: {
                    Text("A note when a team you follow starts, or a game on your channels comes down to the last minutes.")
                }
            }
        }
    }

    private struct TVNameSettings: View {
        @Environment(AppStore.self) private var store
        @State private var name = ScreenIdentity.name
        @State private var saved = ScreenIdentity.name

        var body: some View {
            TVSettingsPage(symbol: "appletv.fill", title: "This Screen's Name") {
                Section {
                    TextField("Name", text: $name)
                        .onSubmit(commit)
                        .accessibilityIdentifier("screen-name")
                } footer: {
                    Text("Other screens see this name when they send a channel here, and in Whole-Home Sync. Leave it blank to use the Apple TV's own name.")
                }
            }
            .onDisappear(perform: commit)
        }

        private func commit() {
            guard name != saved else { return }
            let sent = ScreenName.save(name, device: ScreenIdentity.deviceName)
            name = sent
            saved = sent
            store.screenName = sent
            store.socket?.announce(id: ScreenIdentity.id, name: sent, kind: ScreenIdentity.kind)
        }
    }

    private struct TVServerDetails: View {
        @Environment(AppStore.self) private var store
        let server: TVServerSettings
        @State private var confirmLeave = false

        private var demo: Bool {
            store.server?.id == "demo"
        }

        var body: some View {
            TVSettingsPage(symbol: "server.rack", title: demo ? "Demo" : "Server and Sign-In", caption: access) {
                Section {
                    LabeledContent("Name", value: store.info?.name ?? store.server?.name ?? "")
                        .focusable()
                    LabeledContent("Address", value: store.server?.url.absoluteString ?? "")
                        .focusable()
                    if let info = store.info {
                        LabeledContent("Version", value: info.version)
                            .focusable()
                    }
                }
                if !demo {
                    Section {
                        NavigationLink("Diagnostics") { DiagnosticsView() }
                    } footer: {
                        Text("Tuner health, the guide, and the last antenna reading.")
                    }
                }
                Section {
                    Button(demo ? "Leave the Demo" : "Use a Different Server", role: .destructive) { confirmLeave = true }
                        .accessibilityIdentifier("settings-leave")
                }
            }
            .confirmationDialog(
                demo ? "Leave the demo?" : "Use a different server?",
                isPresented: $confirmLeave,
                titleVisibility: .visible
            ) {
                Button(demo ? "Leave the Demo" : "Choose a Server", role: .destructive) { store.forget() }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text(demo ? "Sample films stop. Your server is unchanged." : "This Apple TV stops using \(store.info?.name ?? "this server"). Its recordings and settings stay on the server.")
            }
        }

        private var access: String {
            if demo {
                return "Sample films on this device."
            }
            if server.seat?.auth == "device", let name = server.seat?.device?.name, !name.isEmpty {
                return "Signed in as \(name)."
            }
            return "Open on your home network. Away from home, this Apple TV needs an invite from the owner."
        }
    }

    /// Everything a stranger sets up once, on the web app: an address to type and a code to scan.
    struct TVWebSettings: View {
        @Environment(AppStore.self) private var store

        static func address(_ base: URL) -> URL? {
            guard base.scheme == "http" || base.scheme == "https", base.host() != nil else { return nil }
            return base.appending(path: "settings")
        }

        var body: some View {
            let url = store.server.flatMap { Self.address($0.url) }
            HStack(alignment: .center, spacing: 100) {
                VStack(spacing: 24) {
                    if let url, let code = Self.code(url.absoluteString) {
                        code
                            .resizable()
                            .interpolation(.none)
                            .scaledToFit()
                            .frame(width: 440, height: 440)
                            .padding(28)
                            .background(.white, in: .rect(cornerRadius: 28))
                            .accessibilityLabel("A code to scan with a phone camera. It opens \(url.absoluteString).")
                    }
                    Text("Scan with your phone's camera")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
                .frame(width: 560)
                VStack(alignment: .leading, spacing: 32) {
                    Text("Open Settings on your computer")
                        .font(.title2.weight(.bold))
                    if let url {
                        Text(url.absoluteString)
                            .font(.title3.weight(.semibold).monospaced())
                            .lineLimit(1)
                            .minimumScaleFactor(0.6)
                            .accessibilityIdentifier("settings-web-address")
                    }
                    VStack(alignment: .leading, spacing: 16) {
                        ForEach(Self.there, id: \.self) { line in
                            Label(line, systemImage: "checkmark.circle")
                                .font(.callout)
                                .foregroundStyle(.secondary)
                        }
                    }
                    Text("These change the whole house, so they live where typing is easy.")
                        .font(.callout)
                        .foregroundStyle(.tertiary)
                }
                .focusable()
                .frame(maxWidth: 900, alignment: .leading)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .tvPageChrome()
        }

        private static let there = [
            "Tuners, channels, and the guide",
            "Recordings folder, storage, and backups",
            "Series passes and recording rules",
            "Devices, invites, and away from home",
            "Scores, updates, and support",
        ]

        static func code(_ text: String) -> Image? {
            let filter = CIFilter.qrCodeGenerator()
            filter.message = Data(text.utf8)
            filter.correctionLevel = "M"
            guard let output = filter.outputImage?.transformed(by: CGAffineTransform(scaleX: 12, y: 12)),
                  let image = CIContext().createCGImage(output, from: output.extent) else { return nil }
            return Image(decorative: image, scale: 1)
        }
    }

    private struct TVAboutSettings: View {
        var body: some View {
            TVSettingsPage(symbol: "info.circle.fill", title: "About", caption: version) {
                Section {
                    Text("Broadwave plays your antenna's channels on every screen in the house. Nothing about what you watch leaves your network.")
                        .focusable()
                }
                Section {
                    Text("Store art and the demo use Big Buck Bunny, Sintel, Tears of Steel, and Elephants Dream. They are Blender Foundation films under CC BY.")
                        .focusable()
                } header: {
                    Text("Credits")
                }
            }
        }

        private var version: String {
            let short = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? ""
            let build = Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? ""
            return "Version \(short) (\(build))"
        }
    }

    extension Prefs.Quality {
        var title: String {
            switch self {
            case .auto: "Auto"
            case .original: "Original"
            case .high: "High"
            case .medium: "Medium"
            case .saver: "Data saver"
            case .tile, .tile360, .focus: "Auto"
            }
        }
    }

    extension Prefs.Sound {
        var title: String {
            switch self {
            case .auto: "Auto"
            case .surround: "Surround"
            case .stereo: "Stereo"
            case .none: "Off"
            }
        }
    }
#endif
