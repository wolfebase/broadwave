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
            SourceFinder(disabled: busy) {
                await loadDevices()
                await store.refresh()
            }
            HomeListView(hideAdded: true)
            NavigationLink("Add a playlist or server") { AddSourceView() }
                .buttonStyle(.glass)
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
            onFinish()
            // The tabs replace the wizard first. A player presented over tabs that
            // are still being built leaves the Apple TV guide rows unfocusable.
            try? await Task.sleep(for: .milliseconds(300))
            if let pick {
                nowPlaying.play(pick)
            }
        }
    }
}
