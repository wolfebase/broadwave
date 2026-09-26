import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// First run: find a tuner, then the server finishes setup on its own.
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
    @State private var held = false
    @State private var autoWatch = false
    @State private var progress: SetupFinish?
    private let titles = ["Sources", "Ready"]

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 24) {
                    Text("Let's set up your TV")
                        .font(.largeTitle.weight(.heavy))
                    HStack(spacing: 8) {
                        ForEach(0 ..< titles.count, id: \.self) { i in
                            stepChip(i)
                        }
                    }
                    Group {
                        switch step {
                        case 0: sources
                        default: finishStep
                        }
                    }
                    if !note.isEmpty {
                        Text(note).foregroundStyle(.secondary)
                    }
                    if step == 0 {
                        Button("Continue") {
                            held = false
                            step = 1
                        }
                        .buttonStyle(.glassProminent)
                        .disabled(!canContinue || busy)
                    } else {
                        Button("Watch") { finish() }
                            .buttonStyle(.glassProminent)
                            .disabled(busy || (progress?.ready ?? "").isEmpty)
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
    }

    private func stepChip(_ i: Int) -> some View {
        Button {
            if i == 0, step == 1 {
                held = true
            }
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
        !devices.isEmpty || store.channels.contains { !$0.hidden }
    }

    private var sources: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(devices.isEmpty ? "Looking for your tuner…" : "Your tuner").font(.title2.weight(.bold))
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
            HomeListView(hideAdded: true)
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

    private var finishStep: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(readyTitle)
                .font(.title2.weight(.bold))
                .accessibilityAddTraits(.isHeader)
            if let steps = progress?.steps {
                ForEach(steps) { item in
                    VStack(alignment: .leading, spacing: 4) {
                        HStack {
                            Text(item.title).font(.headline)
                            Spacer()
                            Text(stateWord(item.state))
                                .foregroundStyle(stateColor(item.state))
                        }
                        if let detail = item.detail, !detail.isEmpty {
                            Text(detail).foregroundStyle(.secondary)
                        }
                    }
                }
            }
            if progress?.ready?.isEmpty == false {
                Text("Open Broadwave on your other screens. They find this server on their own.")
                    .foregroundStyle(.secondary)
                if let url = store.api?.base.absoluteString {
                    Text(url).font(.title3.weight(.semibold))
                }
            }
        }
        .accessibilityElement(children: .contain)
    }

    private var readyTitle: String {
        if let ready = progress?.ready, !ready.isEmpty {
            return ready
        }
        return "Setting up your TV"
    }

    private func stateWord(_ state: String) -> String {
        switch state {
        case "running": "Working"
        case "done": "Done"
        case "check": "Needs a look"
        case "skipped": "Skipped"
        default: ""
        }
    }

    private func stateColor(_ state: String) -> Color {
        switch state {
        case "done": Tokens.ColorToken.success
        case "check": Tokens.ColorToken.warning
        default: .secondary
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

    private func load() async {
        let token = store.socket?.on("sources.found") { _ in
            Task {
                await loadDevices()
                await maybeAdvance()
            }
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
            await store.refresh()
            busy = false
        }
        #if DEBUG
            if let raw = UserDefaults.standard.string(forKey: "BroadwaveSetup") {
                if let n = Int(raw), n >= 0, n < titles.count {
                    step = n
                } else if raw == "walk" {
                    autoWatch = true
                    await walk()
                    return
                }
            }
        #endif
        await maybeAdvance()
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(3600))
        }
    }

    private func onStep() async {
        guard step == 1 else { return }
        await runFinish()
    }

    private func maybeAdvance() async {
        guard step == 0, !held else { return }
        guard !devices.isEmpty || store.channels.contains(where: { !$0.hidden }) else { return }
        try? await Task.sleep(for: .seconds(2))
        guard !Task.isCancelled, step == 0, !held else { return }
        guard !devices.isEmpty || store.channels.contains(where: { !$0.hidden }) else { return }
        step = 1
    }

    private func runFinish() async {
        guard let api = store.api else { return }
        do {
            var status = try await api.setupFinish()
            if !status.running, status.ready?.isEmpty != false {
                status = try await api.startSetupFinish()
            }
            progress = status
            while !Task.isCancelled, status.running {
                try await Task.sleep(for: .milliseconds(500))
                status = try await api.setupFinish()
                progress = status
            }
            if !Task.isCancelled, status.ready?.isEmpty != false {
                note = "Setup did not finish."
            } else if autoWatch, !Task.isCancelled {
                finish()
            }
        } catch {
            note = error.localizedDescription
        }
    }

    private func loadDevices() async {
        if let found = try? await store.api?.devices() {
            devices = found
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

    /// Debug only: pause on Sources, then run the finish checklist.
    private func walk() async {
        try? await Task.sleep(for: .seconds(2))
        if Task.isCancelled {
            return
        }
        step = 1
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
