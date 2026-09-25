import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// Titles, descriptions, and recordings that match what you typed.
struct SearchView: View {
    @Environment(AppStore.self) private var store
    @State private var query = ""
    @State private var result = SearchResult(airings: [], recordings: [])
    @State private var note = ""

    var body: some View {
        List {
            Section {
                TextField("Shows, people, recordings", text: $query)
                #if os(iOS)
                    .textInputAutocapitalization(.never)
                #endif
                    .submitLabel(.search)
                    .onSubmit { Task { await run() } }
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
                                Text(meta(airing))
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                                Button("Record every airing") {
                                    Task { await record(airing) }
                                }
                                .buttonStyle(.glass)
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
        #endif
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
            result = try await api.search(q)
            note = result.airings.isEmpty && result.recordings.isEmpty ? "Nothing matches." : ""
        } catch {
            note = error.localizedDescription
        }
    }

    private func record(_ airing: Airing) async {
        do {
            try await store.api?.addPass(title: airing.title, channelID: airing.channelId)
            note = "Recording every \(airing.title)."
        } catch {
            note = error.localizedDescription
        }
    }
}
