import OTAKit
import OTAUI
import SwiftUI

/// First-run steps, the same five as the web wizard.
struct SetupWizard: View {
    var onFinish: () -> Void = {}
    @Environment(AppStore.self) private var store
    @State private var step = 0
    @State private var note = ""
    @State private var doctor: [APIClient.DoctorNote] = []
    private let titles = ["Sources", "Channels", "Guide", "Recordings", "Apps"]

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 24) {
                Text("Let's set up your TV")
                    .font(.largeTitle.weight(.heavy))
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(titles.indices, id: \.self) { i in
                            Text(titles[i])
                                .font(.caption.weight(.semibold))
                                .lineLimit(1)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 6)
                                .background(i == step ? Tokens.ColorToken.text : Tokens.ColorToken.surface1, in: Capsule())
                                .foregroundStyle(i == step ? Tokens.ColorToken.canvas : Tokens.ColorToken.textTertiary)
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
                Spacer(minLength: 0)
                if step < titles.count - 1 {
                    Button("Continue") { step += 1 }
                        .buttonStyle(.borderedProminent)
                        .disabled(step == 0 && store.channels.isEmpty)
                } else {
                    Button("Start watching") { finish() }
                        .buttonStyle(.borderedProminent)
                }
            }
            .padding(28)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
        .task {
            if store.channels.isEmpty {
                await store.refresh()
            }
            #if DEBUG
                if let raw = UserDefaults.standard.string(forKey: "OTASetup"), let n = Int(raw), n >= 0, n < titles.count {
                    step = n
                }
            #endif
            await starBigFour()
            if let notes = try? await store.api?.doctorNotes() {
                doctor = notes
            }
        }
    }

    private var sources: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Looking for your tuner…").font(.title2.weight(.bold))
            Text("An HDHomeRun on this network is added for you.")
                .foregroundStyle(.secondary)
            if store.channels.isEmpty {
                Text("No tuner answered yet.").foregroundStyle(.secondary)
            } else {
                Text("\(store.channels.count) channels are ready.")
            }
        }
    }

    private var channels: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Choose your channels").font(.title2.weight(.bold))
            Text("ABC, CBS, FOX, and NBC are already favorites when we can tell.")
                .foregroundStyle(.secondary)
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 8) {
                    ForEach(store.channels.filter { !$0.hidden }.prefix(24)) { channel in
                        HStack {
                            Text(channel.displayNumber).font(.headline.monospacedDigit())
                            Text(channel.displayName).lineLimit(1)
                            Spacer()
                            if channel.favorite {
                                Text("Favorite").foregroundStyle(Tokens.ColorToken.success)
                            }
                        }
                    }
                }
            }
            .frame(maxHeight: 360)
            if !note.isEmpty {
                Text(note).foregroundStyle(.secondary)
            }
            Button("Hide duplicates and shopping") { Task { await hideExtras() } }
        }
    }

    private var guide: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Guide coverage").font(.title2.weight(.bold))
            let withListings = store.channels.filter { !store.index.airings($0.id).isEmpty }.count
            Text("\(store.channels.count) channels · \(withListings) with listings")
        }
    }

    private var recordings: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Where recordings go").font(.title2.weight(.bold))
            Text("Recordings save in the folder mapped for this server.")
                .foregroundStyle(.secondary)
            ForEach(doctor, id: \.id) { item in
                Text(item.message).foregroundStyle(Tokens.ColorToken.warning)
            }
        }
    }

    private var apps: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Watch on iPhone and Apple TV").font(.title2.weight(.bold))
            Text("Open Waveguide on your Apple TV. It finds this server on its own.")
                .foregroundStyle(.secondary)
            if let url = store.api?.base.absoluteString {
                Text(url).font(.title3.weight(.semibold))
            }
        }
    }

    private func starBigFour() async {
        guard let api = store.api else { return }
        for channel in store.channels where !channel.favorite && !channel.hidden && bigFour(channel) {
            _ = try? await api.setFavorite(channel, true)
        }
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

    private func finish() {
        Task {
            try? await store.api?.saveSettings(["setupComplete": "1"])
            UserDefaults.standard.removeObject(forKey: "OTASetup")
            onFinish()
        }
    }

    private func bigFour(_ channel: Channel) -> Bool {
        let name = "\(channel.guideName) \(channel.displayName)".uppercased()
        return name.range(of: #"\b(ABC|CBS|FOX|NBC)\b"#, options: .regularExpression) != nil
    }
}
