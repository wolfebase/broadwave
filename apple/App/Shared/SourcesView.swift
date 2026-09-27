import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// Tuners with a channel scan, and every playlist, link, and folder source with its health.
struct SourcesView: View {
    @Environment(AppStore.self) private var store
    @State private var devices: [Device] = []
    @State private var sources: [Source] = []
    @State private var scanning: String?
    @State private var scanTask: Task<Void, Never>?
    @State private var scanNotes: [String: String] = [:]
    @State private var loaded = false
    @State private var error = ""
    @State private var openAdd = false

    var body: some View {
        List {
            if !error.isEmpty {
                Section {
                    Text(error).foregroundStyle(.red)
                }
            }
            tunerSection
            sourceSection
            Section {
                SourceFinder { await load() }
                NavigationLink("Add a playlist or server") {
                    AddSourceView { Task { await load() } }
                }
            } header: {
                Text("Add")
            } footer: {
                Text("Adding a source reads its lineup only. It does not open a tuner.")
            }
        }
        .navigationTitle("Sources")
        .onDisappear { scanTask?.cancel() }
        .task { await load() }
        .navigationDestination(isPresented: $openAdd) {
            AddSourceView { Task { await load() } }
        }
        .refreshable { await load() }
    }

    private var tunerSection: some View {
        Section {
            if loaded, devices.isEmpty {
                Text("No tuner answered.").foregroundStyle(.secondary)
            }
            ForEach(devices, id: \.deviceId) { device in
                VStack(alignment: .leading, spacing: 6) {
                    Text(device.friendlyName.isEmpty ? (device.modelNumber ?? "Tuner") : device.friendlyName)
                        .font(.headline)
                    Text(Self.detail(device)).font(.caption).foregroundStyle(.secondary)
                    if let note = scanNotes[device.deviceId] {
                        Text(note).font(.caption)
                    }
                    Button(scanning == device.deviceId ? "Scanning…" : "Scan channels") {
                        watch(device, start: true)
                    }
                    .disabled(scanning != nil)
                    #if os(iOS)
                        .buttonStyle(.borderless)
                    #endif
                }
            }
        } header: {
            Text("Tuners")
        } footer: {
            Text("A scan uses one tuner until it ends. New channels join the lineup then.")
        }
    }

    private var sourceSection: some View {
        Section {
            if loaded, sources.isEmpty {
                Text("No playlists, links, or folders.").foregroundStyle(.secondary)
            }
            ForEach(sources) { source in
                VStack(alignment: .leading, spacing: 4) {
                    HStack {
                        Text(source.name.isEmpty ? Self.kindName(source.kind) : source.name).font(.headline)
                        Spacer()
                        Text(Self.healthWord(source))
                            .foregroundStyle(source.health?.isEmpty == false ? Tokens.ColorToken.warning : Tokens.ColorToken.success)
                    }
                    Text(Self.sourceDetail(source)).font(.caption).foregroundStyle(.secondary)
                    if let health = source.health, !health.isEmpty {
                        Text(health).font(.caption)
                    }
                }
                .accessibilityElement(children: .combine)
            }
        } header: {
            Text("Playlists and links")
        }
    }

    static func detail(_ device: Device) -> String {
        var parts: [String] = []
        if let model = device.modelNumber, !model.isEmpty {
            parts.append(model)
        }
        parts.append(device.tunerCount == 1 ? "1 tuner" : "\(device.tunerCount) tuners")
        if let firmware = device.firmwareVersion, !firmware.isEmpty {
            parts.append("Firmware \(firmware)")
        }
        return parts.joined(separator: " · ")
    }

    static func kindName(_ kind: String) -> String {
        switch kind {
        case "m3u": "M3U playlist"
        case "xtream": "Xtream server"
        case "link": "Stream link"
        case "folder": "Folder"
        case "tvheadend": "Tvheadend"
        case "channels": "Channels DVR"
        case "threadfin": "Threadfin"
        case "xteve": "xTeVe"
        case "ersatztv": "ErsatzTV"
        case "dispatcharr": "Dispatcharr"
        case "free": "Free channels"
        case "hdhr-compatible": "HDHomeRun-compatible"
        default: "Source"
        }
    }

    static func healthWord(_ source: Source) -> String {
        if !source.enabled {
            return "Off"
        }
        return source.health?.isEmpty == false ? "Needs attention" : "Online"
    }

    static func sourceDetail(_ source: Source) -> String {
        var parts = [kindName(source.kind)]
        if let limit = source.streamLimit, limit > 0 {
            parts.append("\(source.streamsInUse ?? 0) of \(limit) streams")
        }
        if let last = source.lastRefresh, let date = ISO8601DateFormatter().date(from: last) {
            parts.append("Updated \(date.formatted(.relative(presentation: .named)))")
        }
        return parts.joined(separator: " · ")
    }

    private func load() async {
        guard let api = store.api else { return }
        do {
            async let foundDevices = api.devices()
            async let foundSources = api.sources()
            let list = try await foundSources
            let owned = Set(list.compactMap(\.deviceId))
            devices = try await foundDevices.filter { !owned.contains($0.deviceId) }
            sources = list
            error = ""
            if scanning == nil {
                for device in devices where device.tunerCount > 0 {
                    if let progress = try? await api.scanStatus(deviceID: device.deviceId), progress.scanning {
                        scanNotes[device.deviceId] = progress.line
                        watch(device, start: false)
                        break
                    }
                }
            }
            #if DEBUG
                openAdd = UserDefaults.standard.bool(forKey: "BroadwaveOpenFirst")
            #endif
        } catch {
            self.error = error.localizedDescription
        }
        loaded = true
    }

    private func watch(_ device: Device, start: Bool) {
        scanTask?.cancel()
        scanTask = Task { await scan(device, start: start) }
    }

    /// Polls for up to 15 minutes. Leaving the screen stops the polling, not the tuner's scan.
    private func scan(_ device: Device, start: Bool) async {
        guard let api = store.api else { return }
        scanning = device.deviceId
        defer { scanning = nil }
        do {
            if start {
                scanNotes[device.deviceId] = "Starting."
                try await api.startScan(deviceID: device.deviceId)
            }
            for _ in 0 ..< 450 {
                try await Task.sleep(for: .seconds(2))
                let progress = try await api.scanStatus(deviceID: device.deviceId)
                scanNotes[device.deviceId] = progress.line
                if !progress.scanning {
                    await store.refresh()
                    return
                }
            }
            scanNotes[device.deviceId] = "Still scanning. Check back later."
        } catch {
            if Task.isCancelled || (error as? URLError)?.code == .cancelled {
                return
            }
            scanNotes[device.deviceId] = error.localizedDescription
        }
    }
}
