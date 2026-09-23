import OTAKit
import OTAUI
import SwiftUI

struct SportsView: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying

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
    }

    private func section(_ title: String, _ list: [(Channel, Airing)]) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title).font(.title2.weight(.bold)).padding(.horizontal)
            LazyVGrid(columns: [GridItem(.adaptive(minimum: cardWidth), spacing: 14)], spacing: 14) {
                ForEach(list, id: \.1.id) { channel, airing in
                    Button {
                        if airing.isOn(at: store.now) { nowPlaying.play(channel) }
                    } label: {
                        GameCard(channel: channel, airing: airing, now: store.now)
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
                                AsyncImage(url: store.api?.posterURL(recordingID: rec.id)) { img in
                                    img.resizable().aspectRatio(16 / 9, contentMode: .fill)
                                } placeholder: {
                                    Rectangle().fill(Tokens.ColorToken.surface2)
                                }
                                .frame(width: 120, height: 68)
                                .clipShape(.rect(cornerRadius: Tokens.Radius.sm))
                                VStack(alignment: .leading, spacing: 4) {
                                    HStack(spacing: 6) {
                                        if rec.isRecording { LiveDot("Recording") }
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

    var body: some View {
        @Bindable var store = store
        Form {
            Section("Server") {
                LabeledContent("Name", value: store.info?.name ?? store.server?.name ?? "")
                LabeledContent("Address", value: store.server?.url.absoluteString ?? "")
                if let info = store.info {
                    LabeledContent("Version", value: info.version)
                    if let enc = info.encoder { LabeledContent("Encoding", value: enc.replacingOccurrences(of: "h264_", with: "").uppercased()) }
                }
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
                Toggle("Whole-Home Sync", isOn: $store.syncEnabled)
            } footer: {
                Text("Every screen on the same channel shows the same moment, so nobody hears the next room cheer first.")
            }
        }
        .navigationTitle("Settings")
    }
}
