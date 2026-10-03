import BroadwaveKit
import BroadwaveUI
import SwiftUI

struct RecordingsView: View {
    @Environment(AppStore.self) private var store
    @Environment(LibraryFilter.self) private var library
    @State private var unwatchedOnly = false
    @State private var deleting: Recording?
    @State private var notice: String?
    @State private var detecting: Set<Int64> = []
    #if os(tvOS)
        /// A cover, like live TV: a pushed player would keep the sidebar's handle on the picture.
        @State private var playing: Recording?
        @State private var playingChannel: VirtualChannel?
    #endif
    #if os(tvOS)
        @Environment(\.tvSelectedTab) private var tvSelectedTab
        @FocusState private var focusedRec: Int64?
        @FocusState private var emptyRecordings: Bool
    #endif

    var body: some View {
        let groups = grouped
        List {
            NavigationLink {
                ScheduleView()
            } label: {
                Label("Upcoming", systemImage: "calendar")
                    .accessibilityLabel("Upcoming")
            }
            if !store.virtuals.isEmpty {
                Section("Library channels") {
                    ForEach(store.virtuals) { channel in
                        libraryRow(channel)
                    }
                }
            }
            if !store.recordings.isEmpty {
                Picker("Show", selection: $unwatchedOnly) {
                    Text("All").tag(false)
                    Text("Unwatched").tag(true)
                }
                .pickerStyle(.segmented)
            }
            if !library.show.isEmpty {
                Text("Showing \(library.show)")
                    .accessibilityIdentifier("recordings-filtered")
                Button("Show all") { library.clear() }
                    .accessibilityIdentifier("show-all")
                    .accessibilityLabel("Show all")
            }
            if let notice {
                Text(notice)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            if shown.isEmpty {
                ContentUnavailableView(
                    listed.isEmpty ? "No recordings yet" : "All watched",
                    systemImage: "record.circle",
                    description: Text(listed.isEmpty ? "Record from the guide, or set a series to record every episode." : "Everything in the library has been watched.")
                )
                #if os(tvOS)
                .focusable()
                .focused($emptyRecordings)
                #endif
            }
            ForEach(groups) { group in
                Section(group.title) {
                    ForEach(group.items) { rec in
                        row(rec)
                    }
                }
            }
        }
        .navigationTitle("Recordings")
        #if os(tvOS)
            .fullScreenCover(item: $playing) { RecordingPlayerScreen(recording: $0).environment(store) }
            .fullScreenCover(item: $playingChannel) { RecordingPlayerScreen(channel: $0).environment(store) }
        #else
            .navigationDestination(for: Recording.self) { RecordingPlayerScreen(recording: $0) }
            .navigationDestination(for: VirtualChannel.self) { RecordingPlayerScreen(channel: $0) }
        #endif
            .confirmationDialog(
                deleting.map { "Delete \($0.subtitle ?? $0.title)?" } ?? "",
                isPresented: Binding(get: { deleting != nil }, set: {
                    if !$0 {
                        deleting = nil
                    }
                }),
                titleVisibility: .visible,
                presenting: deleting
            ) { rec in
                Button("Delete this file", role: .destructive) {
                    act { try await store.deleteRecording(rec) }
                }
                Button("Cancel", role: .cancel) {}
            } message: { _ in
                Text("The recording and its commercial markers are removed from the server.")
            }
        #if os(tvOS)
            .onAppear { claimRecordingFocus() }
            .onChange(of: tvSelectedTab) { _, _ in claimRecordingFocus() }
            .onChange(of: store.recordings.isEmpty) { _, _ in claimRecordingFocus() }
        #endif
            .task {
                async let recordings: Void = store.refreshRecordings()
                async let channels: Void = store.refreshVirtuals()
                _ = await (recordings, channels)
                #if DEBUG && os(tvOS)
                    // Simulator testing: -BroadwaveTab recordings -BroadwaveRecording <id>
                    // opens it as Select would; simctl cannot press Select.
                    let id = Int64(UserDefaults.standard.integer(forKey: "BroadwaveRecording"))
                    if id > 0, playing == nil {
                        playing = store.recordings.first { $0.id == id }
                    }
                #endif
            }
    }

    /// The row in one sentence, with the same facts it shows: status, date, size, length, Watched.
    private func recordingSpoken(_ rec: Recording) -> String {
        var parts: [String] = []
        if rec.isRecording {
            parts.append("Recording")
        }
        parts.append(rec.subtitle ?? rec.title)
        parts.append(details(rec).replacingOccurrences(of: " · ", with: ", "))
        if let line = rec.signalLine {
            parts.append(line)
        }
        return parts.joined(separator: ", ")
    }

    private func libraryRow(_ channel: VirtualChannel) -> some View {
        let label = VStack(alignment: .leading, spacing: 4) {
            Text("\(channel.number) \(channel.name)").font(.headline).lineLimit(1)
            Text(channel.recordings.count == 1 ? "Plays 1 recording. Uses no tuner." : "Plays \(channel.recordings.count) recordings in turn. Uses no tuner.")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .accessibilityElement(children: .combine)
        return Group {
            #if os(tvOS)
                Button { playingChannel = channel } label: { label }
            #else
                NavigationLink(value: channel) { label }
            #endif
        }
        .accessibilityIdentifier("library-\(channel.id)")
    }

    private func makeChannel(_ rec: Recording) {
        Task {
            do {
                let made = try await store.makeChannel(from: rec)
                notice = "Channel \(made.number) now plays \(rec.title) around the clock, without a tuner."
            } catch {
                notice = PlaybackOutage.actionMessage(error)
            }
        }
    }

    private func row(_ rec: Recording) -> some View {
        Group {
            #if os(tvOS)
                Button { playing = rec } label: { rowLabel(rec) }
            #else
                NavigationLink(value: rec) { rowLabel(rec) }
            #endif
        }
        .accessibilityLabel(recordingSpoken(rec))
        .accessibilityIdentifier("recording-row")
        .contextMenu { actions(rec) }
        #if os(iOS)
            // No destructive role: it would slide the row away before the viewer confirms.
            .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                if !rec.isRecording {
                    Button("Delete", systemImage: "trash") { deleting = rec }
                        .tint(.red)
                }
            }
            .swipeActions(edge: .leading) {
                if !rec.isRecording {
                    Button(rec.isWatched ? "Unwatched" : "Watched", systemImage: rec.isWatched ? "eye.slash" : "eye") {
                        act { try await store.setWatched(rec, !rec.isWatched) }
                    }
                    .tint(Tokens.ColorToken.accent)
                }
            }
        #endif
        #if os(tvOS)
        .focused($focusedRec, equals: rec.id)
        #endif
    }

    private func rowLabel(_ rec: Recording) -> some View {
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
                Text(details(rec)).font(.caption).foregroundStyle(.secondary).lineLimit(2)
                if let error = rec.error, !error.isEmpty {
                    Text(error).font(.caption).foregroundStyle(.secondary).lineLimit(2)
                }
                if let line = rec.signalLine {
                    Text(line).font(.caption).foregroundStyle(.secondary).lineLimit(2)
                }
                if let pos = rec.position, let dur = rec.durationSec, dur > 0, pos > 30, !rec.isWatched {
                    AiringProgress(pos / dur).frame(maxWidth: 160)
                }
            }
        }
    }

    @ViewBuilder
    private func actions(_ rec: Recording) -> some View {
        if rec.isRecording {
            Button("Stop recording", systemImage: "stop.circle") {
                act { try await store.stopRecording(rec) }
            }
        } else {
            Button(rec.isWatched ? "Mark unwatched" : "Mark watched", systemImage: rec.isWatched ? "eye.slash" : "eye") {
                act { try await store.setWatched(rec, !rec.isWatched) }
            }
            Button("Make a channel", systemImage: "tv") {
                makeChannel(rec)
            }
            Button("Find commercials", systemImage: "forward.end") {
                findBreaks(rec)
            }
            .disabled(detecting.contains(rec.id))
            Button("Delete", systemImage: "trash", role: .destructive) { deleting = rec }
        }
    }

    /// "4.1 · Stopped early · Sep 27, 8:00 PM · 2.1 GB · 1:02:00 · Watched", as the web library reads.
    private func details(_ rec: Recording) -> String {
        var parts = [rec.guideNumber]
        // A recording in progress already shows the live dot.
        if !rec.isRecording, let status = rec.statusLabel {
            parts.append(status)
        }
        parts.append(rec.startedAt.formatted(date: .abbreviated, time: .shortened))
        if let bytes = rec.bytes, bytes > 0 {
            parts.append(ByteCountFormatter.string(fromByteCount: bytes, countStyle: .file))
        }
        if let dur = rec.durationSec, dur > 0 {
            parts.append(Duration.seconds(dur).formatted(.time(pattern: dur >= 3600 ? .hourMinuteSecond : .minuteSecond)))
        }
        if !rec.isRecording, rec.isWatched {
            parts.append("Watched")
        }
        return parts.joined(separator: " · ")
    }

    /// The show filter, then Unwatched. An empty show lists every recording.
    private var listed: [Recording] {
        store.recordings.filter { sameShowTitle($0.title, library.show) }
    }

    private var shown: [Recording] {
        unwatchedOnly ? listed.filter { !$0.isWatched } : listed
    }

    private struct Shelf: Identifiable {
        let id: String
        let title: String
        let items: [Recording]
    }

    /// One section per show, then Movies, like the web library.
    private var grouped: [Shelf] {
        let movies = shown.filter(\.isMovie)
        let shows = Dictionary(grouping: shown.filter { !$0.isMovie }) { $0.title }
        var out = shows.keys.sorted().map { Shelf(id: "show:\($0)", title: $0, items: shows[$0] ?? []) }
        if !movies.isEmpty {
            out.append(Shelf(id: "movies", title: "Movies", items: movies))
        }
        return out
    }

    private func act(_ work: @escaping () async throws -> Void) {
        Task {
            do {
                try await work()
                notice = nil
            } catch {
                notice = PlaybackOutage.actionMessage(error)
            }
        }
    }

    private func findBreaks(_ rec: Recording) {
        guard let api = store.api else { return }
        let name = rec.subtitle ?? rec.title
        detecting.insert(rec.id)
        notice = "Looking for commercials in \(name)…"
        Task {
            defer { detecting.remove(rec.id) }
            do {
                let found = try await api.detectBreaks(recordingID: rec.id)
                notice = switch found.count {
                case 0: "No commercials found in \(name)."
                case 1: "Found 1 commercial break in \(name)."
                default: "Found \(found.count) commercial breaks in \(name)."
                }
            } catch {
                notice = PlaybackOutage.actionMessage(error)
            }
        }
    }

    #if os(tvOS)
        /// A list does not take focus from the sidebar the way Settings' form does.
        private func claimRecordingFocus() {
            guard tvSelectedTab == .recordings else { return }
            if let id = grouped.first?.items.first?.id {
                focusedRec = id
            } else {
                emptyRecordings = true
            }
        }
    #endif
}
