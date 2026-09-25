import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// First-run steps, the same five as the web wizard.
struct SetupWizard: View {
    var onFinish: () -> Void = {}
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @State private var step = 0
    @State private var note = ""
    @State private var busy = false
    @State private var devices: [Device] = []
    @State private var hits: [APIClient.FoundHit] = []
    @State private var feeds: [APIClient.FreeFeed] = []
    @State private var freeGuide = ""
    @State private var address = ""
    @State private var playlistURL = ""
    @State private var xtreamUser = ""
    @State private var xtreamPass = ""
    @State private var doctor: [APIClient.DoctorNote] = []
    @State private var space: APIClient.StorageInfo?
    @State private var watermark = "10"
    @State private var watermarkReady = false
    @State private var didScan = false
    @State private var didStar = false
    @State private var didPullGuide = false
    private let titles = ["Sources", "Channels", "Guide", "Recordings", "Apps"]

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 24) {
                    Text("Let's set up your TV")
                        .font(.largeTitle.weight(.heavy))
                    VStack(alignment: .leading, spacing: 8) {
                        HStack(spacing: 8) {
                            ForEach(0 ..< 3, id: \.self) { i in
                                stepChip(i)
                            }
                        }
                        HStack(spacing: 8) {
                            ForEach(3 ..< titles.count, id: \.self) { i in
                                stepChip(i)
                            }
                        }
                    }
                    Group {
                        switch step {
                        case 0: sources
                        case 1: channels
                        case 2: guide
                        case 3: recordings
                        default: apps
                        }
                    }
                    if !note.isEmpty {
                        Text(note).foregroundStyle(.secondary)
                    }
                    if step < titles.count - 1 {
                        Button("Continue") { step += 1 }
                            .buttonStyle(.glassProminent)
                            .disabled(!canContinue || busy)
                    } else {
                        Button("Start watching") { finish() }
                            .buttonStyle(.glassProminent)
                            .disabled(busy)
                    }
                }
                .padding(28)
                .frame(maxWidth: 720, alignment: .leading)
                .frame(maxWidth: .infinity, alignment: .topLeading)
            }
            .background(Tokens.ColorToken.canvas.ignoresSafeArea())
        }
        .task { await load() }
        .task(id: step) { await onStep() }
        .onChange(of: watermark) { _, value in
            guard watermarkReady else { return }
            Task { try? await store.api?.saveSettings(["watermarkGB": value]) }
        }
    }

    private func stepChip(_ i: Int) -> some View {
        Button {
            step = i
        } label: {
            Text(titles[i])
                .font(.caption.weight(.semibold))
                .lineLimit(1)
                .padding(.horizontal, 12)
                .padding(.vertical, 6)
                .background(i == step ? Tokens.ColorToken.text : Tokens.ColorToken.surface1, in: Capsule())
                .foregroundStyle(i == step ? Tokens.ColorToken.canvas : Tokens.ColorToken.textTertiary)
        }
        .buttonStyle(.plain)
    }

    private var canContinue: Bool {
        switch step {
        case 0: !devices.isEmpty || !store.channels.isEmpty
        case 1: store.channels.contains { !$0.hidden }
        default: true
        }
    }

    private var sources: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Looking for your tuner…").font(.title2.weight(.bold))
            Text("An HDHomeRun on this network is added for you. Everything else waits for a tap.")
                .foregroundStyle(.secondary)
            if devices.isEmpty {
                Text(busy ? "Searching…" : "No tuner answered yet.").foregroundStyle(.secondary)
            } else {
                ForEach(devices, id: \.deviceId) { device in
                    HStack {
                        VStack(alignment: .leading) {
                            Text(device.friendlyName.isEmpty ? (device.modelNumber ?? "Tuner") : device.friendlyName)
                                .font(.headline)
                            Text(device.tunerCount > 0 ? "\(device.tunerCount) tuners" : "Streamed")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        Spacer()
                        Text("Found").foregroundStyle(Tokens.ColorToken.success)
                    }
                }
            }
            HStack {
                Button("Look harder") { Task { await look() } }
                    .buttonStyle(.glass)
                    .disabled(busy)
                Button("Add free channels") { Task { await findFree() } }
                    .buttonStyle(.glass)
                    .disabled(busy)
            }
            ForEach(hits) { hit in
                HStack {
                    VStack(alignment: .leading) {
                        Text(hit.name).font(.headline)
                        Text(hit.addr).font(.caption).foregroundStyle(.secondary)
                    }
                    Spacer()
                    Button("Add") { Task { await add(hit) } }
                        .buttonStyle(.glass)
                }
            }
            ForEach(feeds) { feed in
                HStack {
                    Text(feed.name)
                    Spacer()
                    Button("Add") { Task { await add(feed) } }
                        .buttonStyle(.glass)
                }
            }
            if !freeGuide.isEmpty {
                Text(freeGuide).font(.caption).foregroundStyle(.secondary)
            }
            field("Playlist or Xtream server", text: $playlistURL)
            field("Username", text: $xtreamUser)
            field("Password", text: $xtreamPass, secure: true)
            Button("Add playlist") { Task { await addPlaylist() } }
                .buttonStyle(.glass)
                .disabled(busy || playlistURL.trimmingCharacters(in: .whitespaces).isEmpty)
            HStack {
                field("Tuner address", text: $address)
                Button("Add by address") { Task { await addAddress() } }
                    .buttonStyle(.glassProminent)
                    .disabled(busy || address.trimmingCharacters(in: .whitespaces).isEmpty)
            }
        }
    }

    private var channels: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Choose your channels").font(.title2.weight(.bold))
            Text(busy ? "Scanning for channels." : "ABC, CBS, FOX, and NBC are already favorites when we can tell.")
                .foregroundStyle(.secondary)
            ForEach(store.channels.filter { !$0.hidden }) { channel in
                Button {
                    Task { await toggleFavorite(channel) }
                } label: {
                    HStack {
                        Text(channel.displayNumber).font(.headline.monospacedDigit())
                        Text(channel.displayName).lineLimit(1)
                        if let network = channel.network, !network.isEmpty {
                            Text(network).foregroundStyle(.secondary)
                        }
                        Spacer()
                        Text(channel.favorite ? "Favorite" : "Add favorite")
                            .foregroundStyle(channel.favorite ? Tokens.ColorToken.success : .secondary)
                    }
                }
                .buttonStyle(.glass)
            }
            if store.channels.isEmpty {
                Text("No channels yet.").foregroundStyle(.secondary)
            }
            Button("Hide duplicates and shopping") { Task { await hideExtras() } }
                .buttonStyle(.glass)
        }
    }

    private var guide: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Guide coverage").font(.title2.weight(.bold))
            let rows = coverage()
            if rows.isEmpty {
                Text("No sources yet.").foregroundStyle(.secondary)
            }
            ForEach(rows, id: \.id) { row in
                HStack {
                    Text(row.name).font(.headline)
                    Spacer()
                    Text("\(row.channels) channels · \(row.withListings) with listings")
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    private var recordings: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Where recordings go").font(.title2.weight(.bold))
            HStack(spacing: 24) {
                stat(space.map { formatFree($0.freeBytes) } ?? "—", "free")
            }
            Text("Recordings save in the folder mapped for this server.")
                .foregroundStyle(.secondary)
            ForEach(doctor, id: \.id) { item in
                Text(item.message).foregroundStyle(Tokens.ColorToken.warning)
            }
            Picker("Keep this much space free", selection: $watermark) {
                Text("No reserve").tag("0")
                Text("10 GB").tag("10")
                Text("25 GB").tag("25")
                Text("50 GB").tag("50")
                Text("100 GB").tag("100")
            }
        }
    }

    private var apps: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Watch on iPhone and Apple TV").font(.title2.weight(.bold))
            Text("Open Broadwave on your Apple TV. It finds this server on its own.")
                .foregroundStyle(.secondary)
            if let url = store.api?.base.absoluteString {
                Text(url).font(.title3.weight(.semibold))
            }
        }
    }

    private func field(_ title: String, text: Binding<String>, secure: Bool = false) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title).font(.caption).foregroundStyle(.secondary)
            Group {
                if secure {
                    SecureField(title, text: text)
                } else {
                    TextField(title, text: text)
                        .textContentType(.URL)
                }
            }
            #if os(iOS)
            .keyboardType(secure ? .default : .URL)
            .textInputAutocapitalization(.never)
            #endif
            .autocorrectionDisabled()
            .padding(12)
            .background(Tokens.ColorToken.surface2, in: .rect(cornerRadius: Tokens.Radius.sm))
        }
    }

    private func stat(_ value: String, _ label: String) -> some View {
        VStack(alignment: .leading) {
            Text(value).font(.title.weight(.bold))
            Text(label).foregroundStyle(.secondary)
        }
    }

    private func load() async {
        let token = store.socket?.on("sources.found") { _ in
            Task { await loadDevices() }
        }
        defer {
            if let token {
                store.socket?.off("sources.found", token)
            }
        }
        await loadDevices()
        if devices.isEmpty {
            busy = true
            _ = try? await store.api?.discover(ip: "")
            await loadDevices()
            busy = false
        }
        if let notes = try? await store.api?.doctorNotes() {
            doctor = notes
        }
        if let storage = try? await store.api?.storage() {
            space = storage
        }
        if let values = try? await store.api?.settings(), let mark = values["watermarkGB"], !mark.isEmpty {
            watermark = mark
        }
        watermarkReady = true
        #if DEBUG
            if let raw = UserDefaults.standard.string(forKey: "BroadwaveSetup") {
                if let n = Int(raw), n >= 0, n < titles.count {
                    step = n
                } else if raw == "walk" {
                    await walk()
                    return
                }
            }
        #endif
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(3600))
        }
    }

    private func onStep() async {
        if step == 1 {
            await prepareChannels()
        } else if step == 2 {
            await prepareGuide()
        }
    }

    private func loadDevices() async {
        if let found = try? await store.api?.devices() {
            devices = found
        }
    }

    private func prepareChannels() async {
        if !didScan, let tuner = devices.first(where: { $0.tunerCount > 0 }), store.channels.isEmpty {
            didScan = true
            await pollScan(tuner.deviceId)
        }
        guard !didStar, !store.channels.isEmpty else { return }
        didStar = true
        _ = try? await store.api?.starNetworks()
        await store.refresh()
    }

    private func pollScan(_ id: String) async {
        guard let api = store.api else { return }
        busy = true
        note = "Scanning for channels."
        defer { busy = false }
        do {
            try await api.startScan(deviceID: id)
            for _ in 0 ..< 40 {
                if Task.isCancelled {
                    return
                }
                let prog = try await api.scanStatus(deviceID: id)
                note = prog.scanning ? "Scanning for channels. \(prog.found) found." : ""
                if !prog.scanning {
                    break
                }
                try await Task.sleep(for: .seconds(1))
            }
            await store.refresh()
            await loadDevices()
        } catch {
            note = error.localizedDescription
        }
    }

    private func prepareGuide() async {
        guard !didPullGuide, let api = store.api, !devices.isEmpty else { return }
        didPullGuide = true
        let count = await (try? api.guideAirings()) ?? 0
        if count > 0 {
            return
        }
        busy = true
        defer { busy = false }
        do {
            let loaded = try await api.refreshGuide()
            note = "Loaded \(loaded) listings."
            _ = try? await api.starNetworks()
            await store.refresh()
        } catch {
            note = error.localizedDescription
        }
    }

    private func look() async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        do {
            hits = try await api.lookHarder()
            if hits.isEmpty {
                note = "Nothing else answered."
            }
        } catch {
            hits = []
            note = error.localizedDescription
        }
    }

    private func findFree() async {
        guard let api = store.api else { return }
        busy = true
        freeGuide = ""
        defer { busy = false }
        do {
            let res = try await api.freeSources()
            feeds = res.found
            freeGuide = res.found.isEmpty ? res.guide : ""
        } catch {
            note = "No free-channel server answered."
        }
    }

    private func add(_ hit: APIClient.FoundHit) async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        do {
            if hit.kind == "fastchannels" || hit.kind == "pluto" || hit.kind == "samsung" {
                let addr = hit.addr.contains("://") ? hit.addr : "http://\(hit.addr)"
                let message = try await api.addFree(kind: hit.kind, addr: addr, playlist: "", guide: "", name: hit.name)
                note = message.isEmpty ? "Source added." : message
            } else {
                _ = try await api.discover(ip: hit.addr)
                note = "Source added."
            }
            await loadDevices()
            await store.refresh()
        } catch {
            note = error.localizedDescription
        }
    }

    private func add(_ feed: APIClient.FreeFeed) async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        do {
            let message = try await api.addFree(kind: feed.kind, addr: feed.addr, playlist: feed.playlist, guide: feed.guide, name: feed.name)
            note = message.isEmpty ? "Source added. It shows up with the lineup." : message
            await loadDevices()
            await store.refresh()
        } catch {
            note = error.localizedDescription
        }
    }

    private func addPlaylist() async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        let kind = xtreamUser.isEmpty ? "m3u" : "xtream"
        do {
            let added = try await api.addPlaylist(kind: kind, url: playlistURL, username: xtreamUser, password: xtreamPass)
            if added.pick == true {
                note = added.message ?? "This playlist is too long to add all at once."
                return
            }
            note = "Playlist added."
            await loadDevices()
            await store.refresh()
        } catch {
            note = error.localizedDescription
        }
    }

    private func addAddress() async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        do {
            _ = try await api.discover(ip: address.trimmingCharacters(in: .whitespaces))
            await loadDevices()
            await store.refresh()
        } catch {
            note = error.localizedDescription
        }
    }

    private func toggleFavorite(_ channel: Channel) async {
        _ = try? await store.api?.setFavorite(channel, !channel.favorite)
        await store.refresh()
    }

    private func hideExtras() async {
        guard let api = store.api else { return }
        var seen = Set<String>()
        var hidden = 0
        for channel in store.channels where !channel.hidden {
            let name = "\(channel.guideName) \(channel.displayName)"
            if name.range(of: "shop|qvc|hsn|jewelry", options: [.regularExpression, .caseInsensitive]) != nil {
                _ = try? await api.setHidden(channel, true)
                hidden += 1
                continue
            }
            let key = "\(channel.guideNumber)|\(channel.guideName)".lowercased()
            if seen.contains(key) {
                _ = try? await api.setHidden(channel, true)
                hidden += 1
            } else {
                seen.insert(key)
            }
        }
        note = hidden > 0 ? "Hid \(hidden) channels." : "Nothing to hide."
        await store.refresh()
    }

    private struct Coverage {
        var id: String
        var name: String
        var channels: Int
        var withListings: Int
    }

    private func coverage() -> [Coverage] {
        devices.map { device in
            let mine = store.channels.filter { $0.deviceId == device.deviceId && !$0.hidden }
            let listed = mine.filter { !store.index.airings($0.id).isEmpty }.count
            let name = device.friendlyName.isEmpty ? (device.modelNumber ?? "Source") : device.friendlyName
            return Coverage(id: device.deviceId, name: name, channels: mine.count, withListings: listed)
        }
    }

    private func formatFree(_ bytes: Int64) -> String {
        if bytes >= 1_000_000_000_000 {
            return String(format: "%.1f TB", Double(bytes) / 1_000_000_000_000)
        }
        return "\(bytes / 1_000_000_000) GB"
    }

    /// Debug only: the same Continue taps, paced so a screenshot can land on each step.
    private func walk() async {
        for next in 1 ..< titles.count {
            try? await Task.sleep(for: .seconds(2))
            if Task.isCancelled {
                return
            }
            step = next
        }
        try? await Task.sleep(for: .seconds(2))
        if Task.isCancelled {
            return
        }
        finish()
    }

    private func finish() {
        Task {
            try? await store.api?.saveSettings(["setupComplete": "1"])
            UserDefaults.standard.removeObject(forKey: "BroadwaveSetup")
            await store.refresh()
            let pick = store.channels.first { $0.favorite && !$0.hidden } ?? store.channels.first { !$0.hidden }
            if let pick {
                nowPlaying.play(pick)
            }
            onFinish()
        }
    }
}
