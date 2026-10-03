import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// Titles, descriptions, and recordings that match what you typed.
struct SearchView: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @State private var query = ""
    @State private var result = SearchResult(airings: [], recordings: [])
    @State private var lineup: [Channel] = []
    @State private var note = ""
    #if os(tvOS)
        @Environment(\.tvSelectedTab) private var tvSelectedTab
        @FocusState private var fieldFocused: Bool
    #endif

    var body: some View {
        List {
            Section {
                TextField("Shows, people, recordings", text: $query)
                #if os(iOS)
                    .textInputAutocapitalization(.never)
                #endif
                    .submitLabel(.search)
                    .onSubmit { Task { await run() } }
                    .accessibilityIdentifier("search-field")
                #if os(tvOS)
                    .focused($fieldFocused)
                #endif
            }
            if !result.airings.isEmpty {
                Section("Guide") {
                    ForEach(result.airings) { airing in
                        HStack(alignment: .top, spacing: 12) {
                            if let art = store.artURL(airing, width: 160) {
                                ProgramPicture(url: art, width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0, hero: false)
                                    .frame(width: 84, height: 56)
                                    .clipShape(.rect(cornerRadius: Tokens.Radius.sm))
                            }
                            VStack(alignment: .leading, spacing: 4) {
                                Text(airing.title).font(.headline)
                                HStack(spacing: 6) {
                                    if channel(for: airing)?.isATSC3 == true {
                                        ATSC3Tag()
                                    }
                                    Text(meta(airing))
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                                HStack(spacing: 8) {
                                    Button("Watch") {
                                        watch(airing)
                                    }
                                    .buttonStyle(.glass)
                                    .accessibilityIdentifier("search-watch")
                                    Button("Record every airing") {
                                        Task { await record(airing) }
                                    }
                                    .buttonStyle(.glass)
                                }
                            }
                        }
                        .padding(.vertical, 4)
                    }
                }
            }
            if !result.recordings.isEmpty {
                Section("Recordings") {
                    ForEach(result.recordings) { rec in
                        HStack(spacing: 12) {
                            RecordingPoster(recording: rec)
                                .frame(width: 84, height: 48)
                                .clipShape(.rect(cornerRadius: Tokens.Radius.sm))
                            VStack(alignment: .leading, spacing: 2) {
                                Text(rec.title).font(.headline)
                                Text(rec.guideNumber).font(.caption).foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            if !note.isEmpty {
                Text(note).foregroundStyle(.secondary)
            }
        }
        .navigationTitle("Search")
        #if os(iOS)
            .toolbarTitleDisplayMode(.inline)
        #else
            .onAppear { claimSearchFocus() }
            .onChange(of: tvSelectedTab) { _, _ in claimSearchFocus() }
        #endif
        #if DEBUG
        .task {
            // UI tests cannot open the tvOS keyboard. The query is the viewer's.
            guard query.isEmpty, let seeded = UserDefaults.standard.string(forKey: "BroadwaveSearch"), !seeded.isEmpty else { return }
            query = seeded
            await run()
        }
        #endif
    }

    #if os(tvOS)
        /// Same as the guide: focus in the page closes the sidebar. The field is what this tab is for.
        private func claimSearchFocus() {
            guard tvSelectedTab == .search else { return }
            fieldFocused = true
        }
    #endif

    private func channel(for airing: Airing) -> Channel? {
        lineup.first { $0.id == airing.channelId } ?? store.channels.first { $0.id == airing.channelId }
    }

    private func watch(_ airing: Airing) {
        guard let choice = ClearBroadcast.play(id: airing.channelId, visible: store.channels, lineup: lineup) else {
            note = "That channel is not on the guide."
            return
        }
        nowPlaying.play(choice.channel, note: choice.note)
    }

    private func meta(_ airing: Airing) -> String {
        let when = airing.start.formatted(.dateTime.weekday(.abbreviated).hour().minute())
        let channel = [airing.guideNumber, airing.channelName].compactMap(\.self).filter { !$0.isEmpty }.joined(separator: " ")
        if channel.isEmpty {
            return when
        }
        return "\(channel) · \(when)"
    }

    private func run() async {
        let q = query.trimmingCharacters(in: .whitespaces)
        guard q.count >= 2, let api = store.api else {
            result = SearchResult(airings: [], recordings: [])
            return
        }
        do {
            async let found = api.search(q)
            async let every = api.lineup()
            result = try await found
            lineup = await (try? every) ?? store.channels
            note = result.airings.isEmpty && result.recordings.isEmpty ? "Nothing matches." : ""
        } catch {
            note = error.localizedDescription
        }
    }

    private func record(_ airing: Airing) async {
        do {
            try await store.recordSeries(airing)
            note = "Recording every \(airing.title)."
        } catch {
            note = error.localizedDescription
        }
    }
}
