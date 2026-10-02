import BroadwaveKit
import SwiftUI

/// Every channel the server knows, including hidden, off, and off-air ones.
struct ChannelsView: View {
    @Environment(AppStore.self) private var store
    @State private var lineup: [Channel] = []
    @State private var loaded = false
    @State private var error = ""
    @State private var openFirst = false

    var body: some View {
        List {
            if !error.isEmpty {
                Section {
                    Text(error).foregroundStyle(.red)
                }
            }
            Section {
                if !loaded {
                    Text("Loading…").foregroundStyle(.secondary)
                } else if lineup.isEmpty {
                    Text("No channels yet. Add a tuner or scan for channels.").foregroundStyle(.secondary)
                }
                ForEach(lineup) { channel in
                    NavigationLink {
                        ChannelEditView(channel: channel) { updated in
                            if let i = lineup.firstIndex(where: { $0.id == updated.id }) {
                                lineup[i] = updated
                            }
                            // A pair's choice also changes the other channel.
                            if updated.twinId != nil {
                                Task { await load() }
                            }
                        }
                    } label: {
                        row(channel)
                    }
                }
            } footer: {
                Text("A hidden channel stays off the guide and Home.")
            }
        }
        .navigationTitle("Channels")
        .task { await load() }
        .navigationDestination(isPresented: $openFirst) {
            if let channel = lineup.first(where: \.hidden) ?? lineup.first {
                ChannelEditView(channel: channel) { _ in }
            }
        }
    }

    private func row(_ channel: Channel) -> some View {
        HStack(spacing: 12) {
            Text(channel.displayNumber)
                .font(.headline.monospacedDigit())
                .frame(minWidth: 56, alignment: .leading)
            VStack(alignment: .leading, spacing: 2) {
                Text(channel.displayName).font(.headline).lineLimit(1)
                Text(Self.status(channel)).font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            if channel.favorite {
                Image(systemName: "star.fill").foregroundStyle(.yellow).accessibilityHidden(true)
            }
            if channel.hidden {
                Image(systemName: "eye.slash").foregroundStyle(.secondary).accessibilityHidden(true)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityValue(channel.favorite ? "Favorite" : "")
    }

    static func status(_ channel: Channel) -> String {
        var parts = [channel.hd ? "HD" : "SD"]
        if let codec = channel.videoCodec, !codec.isEmpty {
            parts.append(codec)
        }
        if channel.isATSC3 {
            parts.append("ATSC 3.0")
        }
        if channel.protected == true {
            parts.append("Encrypted")
        }
        if !channel.present {
            parts.append("Off air")
        }
        if !channel.enabled {
            parts.append("Off the guide")
        }
        if channel.hidden {
            parts.append("Hidden")
        }
        return parts.joined(separator: " · ")
    }

    private func load() async {
        guard let api = store.api else { return }
        do {
            lineup = try await api.lineup().sorted(by: Channel.guideOrder)
            error = ""
            #if DEBUG
                openFirst = UserDefaults.standard.bool(forKey: "BroadwaveOpenFirst") && !lineup.isEmpty
            #endif
        } catch {
            self.error = error.localizedDescription
        }
        loaded = true
    }
}

/// One channel's name, number, guide match, and where it shows.
struct ChannelEditView: View {
    @Environment(AppStore.self) private var store
    @State private var channel: Channel
    @State private var name: String
    @State private var number: String
    @State private var match: String
    @State private var error = ""
    let saved: (Channel) -> Void

    init(channel: Channel, saved: @escaping (Channel) -> Void) {
        _channel = State(initialValue: channel)
        _name = State(initialValue: channel.displayName)
        _number = State(initialValue: channel.displayNumber)
        _match = State(initialValue: channel.guideKey ?? "")
        self.saved = saved
    }

    private var demo: Bool {
        store.server?.id == "demo"
    }

    var body: some View {
        Form {
            if !error.isEmpty {
                Section {
                    Text(error).foregroundStyle(.red)
                }
            }
            if channel.protected == true {
                Section {
                    Text(channel.playsAs != nil
                        ? "Encrypted (ATSC 3.0 DRM). It plays and records the same station in ATSC 1.0."
                        : channel.isATSC3
                        ? "Encrypted (ATSC 3.0 DRM). Only the tuner maker's app can play it."
                        : "Copy protected. Only the tuner maker's app can play it.")
                } footer: {
                    Text(ChannelsView.status(channel))
                }
            } else {
                Section {
                    toggle("Favorite", \.favorite) { ChannelPatch(favorite: $0) }
                    if !demo {
                        toggle("On the guide", \.enabled) { ChannelPatch(enabled: $0) }
                    }
                    toggle("Hide", \.hidden) { ChannelPatch(hidden: $0) }
                } footer: {
                    Text(ChannelsView.status(channel))
                }
            }
            if channel.twinId != nil, !demo {
                Section {
                    Picker("Show", selection: Binding(
                        get: { channel.twinChoice ?? "both" },
                        set: { choice in Task { await save(ChannelPatch(twinChoice: choice)) } }
                    )) {
                        Text("3.0 only").tag("atsc3")
                        Text("1.0 only").tag("atsc1")
                        Text("Both").tag("both")
                    }
                } footer: {
                    Text("This station broadcasts in ATSC 1.0 and 3.0. Both share one guide.")
                }
            }
            if demo {
                Section {
                    Text("The demo does not rename a channel.").foregroundStyle(.secondary)
                }
            } else {
                Section {
                    TextField("Name on the guide", text: $name)
                        .onSubmit { commitText() }
                    TextField("Number", text: $number)
                        .onSubmit { commitText() }
                    TextField("Guide match", text: $match)
                        .onSubmit { commitText() }
                } footer: {
                    Text("Broadcast name \(channel.guideName), number \(channel.guideNumber). Leave guide match blank to use the number and call sign.")
                }
            }
        }
        #if os(iOS)
        .textInputAutocapitalization(.never)
        .autocorrectionDisabled()
        #endif
        .navigationTitle(channel.displayName)
        .onDisappear { commitText() }
    }

    private func toggle(_ title: String, _ key: WritableKeyPath<Channel, Bool>, _ patch: @escaping (Bool) -> ChannelPatch) -> some View {
        Toggle(title, isOn: Binding(
            get: { channel[keyPath: key] },
            set: { on in
                let before = channel
                channel[keyPath: key] = on
                Task { await save(patch(on), revert: before) }
            }
        ))
    }

    private func commitText() {
        let n = name.trimmingCharacters(in: .whitespaces)
        let num = number.trimmingCharacters(in: .whitespaces)
        let m = match.trimmingCharacters(in: .whitespaces)
        var patch = ChannelPatch()
        if n != channel.displayName {
            patch.customName = n
        }
        if num != channel.displayNumber {
            patch.customNumber = num
        }
        if m != (channel.guideKey ?? "") {
            patch.guideKey = m
        }
        guard patch != ChannelPatch() else { return }
        Task { await save(patch) }
    }

    private func save(_ patch: ChannelPatch, revert: Channel? = nil) async {
        do {
            let updated = try await store.editChannel(channel.id, patch)
            channel = updated
            if patch.customName != nil || patch.customNumber != nil || patch.guideKey != nil {
                name = updated.displayName
                number = updated.displayNumber
                match = updated.guideKey ?? ""
            }
            error = ""
            saved(updated)
        } catch {
            if let revert {
                channel = revert
            }
            self.error = error.localizedDescription
        }
    }
}
