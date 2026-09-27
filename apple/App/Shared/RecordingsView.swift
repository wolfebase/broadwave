import BroadwaveKit
import BroadwaveUI
import SwiftUI

struct RecordingsView: View {
    @Environment(AppStore.self) private var store
    #if os(tvOS)
        @Environment(\.tvSelectedTab) private var tvSelectedTab
        @FocusState private var focusedRec: Int64?
        @FocusState private var emptyRecordings: Bool
    #endif

    var body: some View {
        let groups = Dictionary(grouping: store.recordings) { $0.title }
        List {
            if store.recordings.isEmpty {
                ContentUnavailableView("No recordings yet", systemImage: "record.circle", description: Text("Record from the guide, or set a series to record every episode."))
                #if os(tvOS)
                    .focusable()
                    .focused($emptyRecordings)
                #endif
            }
            ForEach(groups.keys.sorted(), id: \.self) { title in
                Section(title) {
                    ForEach(groups[title] ?? []) { rec in
                        NavigationLink(value: rec) {
                            HStack(spacing: 14) {
                                RecordingPoster(recording: rec)
                                    .frame(width: 120, height: 68)
                                    .clipShape(.rect(cornerRadius: Tokens.Radius.sm))
                                VStack(alignment: .leading, spacing: 4) {
                                    HStack(spacing: 6) {
                                        if rec.isRecording {
                                            LiveDot("Recording")
                                        }
                                        Text(rec.subtitle ?? rec.title).font(.headline).lineLimit(1)
                                    }
                                    Text(rec.startedAt.formatted(date: .abbreviated, time: .shortened)).font(.caption).foregroundStyle(.secondary)
                                    if let pos = rec.position, let dur = rec.durationSec, dur > 0, pos > 30 {
                                        AiringProgress(pos / dur).frame(maxWidth: 160)
                                    }
                                }
                            }
                        }
                        #if os(tvOS)
                        .focused($focusedRec, equals: rec.id)
                        #endif
                    }
                }
            }
        }
        .navigationTitle("Recordings")
        .navigationDestination(for: Recording.self) { RecordingPlayerScreen(recording: $0) }
        #if os(tvOS)
            .onAppear { claimRecordingFocus() }
            .onChange(of: tvSelectedTab) { _, _ in claimRecordingFocus() }
            .onChange(of: store.recordings.isEmpty) { _, _ in claimRecordingFocus() }
        #endif
            .task { await store.refreshRecordings() }
    }

    #if os(tvOS)
        private var firstRecordingID: Int64? {
            let groups = Dictionary(grouping: store.recordings) { $0.title }
            guard let title = groups.keys.sorted().first else { return nil }
            return (groups[title] ?? []).first?.id
        }

        /// A list does not take focus from the sidebar the way Settings' form does.
        private func claimRecordingFocus() {
            guard tvSelectedTab == .recordings else { return }
            if let id = firstRecordingID {
                focusedRec = id
            } else {
                emptyRecordings = true
            }
        }
    #endif
}
