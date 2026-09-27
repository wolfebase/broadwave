import BroadwaveKit
import SwiftUI

/// Doctor notes, tuner health, guide depth, and the last antenna readings.
struct DiagnosticsView: View {
    @Environment(AppStore.self) private var store
    @State private var notes: [APIClient.DoctorNote] = []
    @State private var health: [DeviceHealth] = []
    @State private var depth: APIClient.GuideDepth?
    @State private var signals: [ChannelSignal] = []
    @State private var checking = false
    @State private var note = ""
    @State private var loaded = false

    var body: some View {
        ScrollViewReader { proxy in
            diagnosticsForm
                .onAppear { openAntenna(proxy) }
        }
    }

    private var diagnosticsForm: some View {
        Form {
            Section {
                if !loaded {
                    Text("Checking…")
                        .foregroundStyle(.secondary)
                } else if notes.isEmpty {
                    Text("Nothing needs attention.")
                } else {
                    ForEach(notes, id: \.id) { item in
                        Text(item.message)
                    }
                }
            } header: {
                Text("Fix these")
            }
            Section {
                if let depth {
                    Text("\(depth.channelsWithListings) of \(depth.channels) channels listed · \(depth.airings) shows")
                } else if loaded {
                    Text("Guide listings are not available yet.")
                        .foregroundStyle(.secondary)
                }
            } header: {
                Text("Guide")
            }
            Section {
                if health.isEmpty, loaded {
                    Text("No tuner answered.")
                        .foregroundStyle(.secondary)
                }
                ForEach(health, id: \.deviceId) { device in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(device.model.isEmpty ? "Unknown model" : device.model)
                        Text(device.firmwareVersion.isEmpty ? "Firmware unknown" : "Firmware \(device.firmwareVersion)")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        if device.tuners.isEmpty {
                            Text("No tuner status.")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        ForEach(device.tuners, id: \.index) { tuner in
                            Text("Tuner \(tuner.index + 1) \(tuner.locked ? "locked" : "has no lock")")
                                .font(.caption)
                        }
                        if let error = device.error, !error.isEmpty {
                            Text(error)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            } header: {
                Text("Tuner health")
            } footer: {
                Text("Model, firmware, and lock. This app does not install firmware.")
            }
            Section {
                if signals.isEmpty, loaded {
                    Text("No antenna reading yet.")
                        .foregroundStyle(.secondary)
                }
                ForEach(signals, id: \.channelId) { row in
                    VStack(alignment: .leading, spacing: 2) {
                        Text("\(row.number) \(row.name)")
                        Text(signalLine(row))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
                Button(checking ? "Checking the antenna" : "Check the antenna") {
                    Task { await checkAntenna() }
                }
                .disabled(checking || store.api == nil)
            } header: {
                Text("Antenna")
            } footer: {
                Text("Reads each channel on a free tuner and stops if someone starts watching.")
            }
            .id("antenna")
            if !note.isEmpty {
                Section {
                    Text(note)
                }
            }
        }
        .navigationTitle("Diagnostics")
        .task { await load() }
    }

    private func openAntenna(_ proxy: ScrollViewProxy) {
        #if DEBUG
            guard UserDefaults.standard.string(forKey: "BroadwaveSettingsAnchor") == "antenna" else { return }
            Task {
                try? await Task.sleep(for: .milliseconds(900))
                proxy.scrollTo("antenna", anchor: .top)
            }
        #else
            _ = proxy
        #endif
    }

    private func signalLine(_ row: ChannelSignal) -> String {
        if let verdict = row.verdict, !verdict.isEmpty {
            var line = verdict
            if let strength = row.strength {
                line += " · signal \(strength)%"
            }
            if row.live == true {
                line += " · live"
            }
            return line
        }
        return "No reading yet"
    }

    @MainActor
    private func load() async {
        guard let api = store.api else {
            loaded = true
            return
        }
        notes = await (try? api.doctorNotes()) ?? []
        depth = try? await api.guideDepth()
        health = await (try? api.deviceHealth()) ?? []
        if let snap = try? await api.signals() {
            signals = snap.channels
            checking = snap.running
        }
        loaded = true
    }

    @MainActor
    private func checkAntenna() async {
        guard !checking, let api = store.api else { return }
        checking = true
        defer { checking = false }
        do {
            let message = try await api.checkSignals()
            note = message.isEmpty ? "Checking channels." : message
            for _ in 0 ..< 15 {
                try await Task.sleep(for: .seconds(2))
                if Task.isCancelled {
                    return
                }
                let snap = try await api.signals()
                signals = snap.channels
                if !snap.running {
                    return
                }
            }
        } catch {
            note = error.localizedDescription
        }
    }
}
