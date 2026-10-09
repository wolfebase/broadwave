import BroadwaveKit
import BroadwaveUI
import SwiftUI

struct RecordingsView: View {
    /// A show's own page: every recording of that title, by season.
    var show: String?
    @Environment(AppStore.self) private var store
    @Environment(LibraryFilter.self) private var library
    @State private var unwatchedOnly = false
    @State private var kind: Library.Kind = .all
    @State private var sort: Library.Sort?
    @State private var selecting = false
    @State private var picked: Set<Int64> = []
    @State private var confirmingMany = false
    @State private var busy = false
    @State private var deleting: Recording?
    @State private var notice: String?
    @State private var detecting: Set<Int64> = []
    @State private var scheduling: Set<Int64> = []
    @State private var scheduled: Set<Int64> = []
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
        let built = Library.build(shown, sort: order)
        let resume = show == nil && !selecting ? Library.continueWatching(listed, limit: 5) : []
        List {
            if show == nil {
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
            }
            if !listed.isEmpty {
                Section {
                    Picker("Show", selection: $unwatchedOnly) {
                        Text("All").tag(false)
                        Text("Unwatched").tag(true)
                    }
                    .pickerStyle(.segmented)
                    if show == nil {
                        Picker("Show only", selection: $kind) {
                            ForEach(Library.Kind.allCases, id: \.self) { Text($0.label).tag($0) }
                        }
                        .accessibilityIdentifier("library-kind")
                    }
                    Picker("Sort", selection: Binding(get: { order }, set: { sort = $0 })) {
                        ForEach(Library.Sort.allCases, id: \.self) { Text($0.label).tag($0) }
                    }
                    .accessibilityIdentifier("library-sort")
                    if selecting || !choosable.isEmpty {
                        Button(selecting ? "Done" : "Select") { selecting ? stopSelecting() : (selecting = true) }
                            .disabled(busy)
                            .accessibilityIdentifier("library-select")
                    }
                } footer: {
                    Text(Library.summary(listed))
                }
            }
            if selecting {
                selectionBar
            }
            if show == nil, !library.show.isEmpty {
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
                    .accessibilityIdentifier("recordings-notice")
            }
            if shown.isEmpty {
                ContentUnavailableView(
                    listed.isEmpty ? "No recordings yet" : unwatchedOnly ? "All watched" : "Nothing here",
                    systemImage: "record.circle",
                    description: Text(listed.isEmpty ? "Record from the guide, or set a series to record every episode." : unwatchedOnly ? "Everything here has been watched." : "Nothing here matches.")
                )
                #if os(tvOS)
                .focusable()
                .focused($emptyRecordings)
                #endif
            }
            if !resume.isEmpty {
                Section("Continue watching") {
                    ForEach(resume) { rec in
                        item(rec, resume: true)
                    }
                }
            }
            if show != nil {
                let titled = built.shows.flatMap(\.seasons).contains { $0.season > 0 }
                ForEach(built.shows) { item in
                    ForEach(item.seasons) { season in
                        Section {
                            ForEach(season.items) { rec in
                                self.item(rec)
                            }
                        } header: {
                            if titled {
                                Text(season.title)
                            }
                        } footer: {
                            if season.id == item.seasons.last?.id {
                                Text(item.line)
                            }
                        }
                    }
                }
            } else {
                ForEach(built.shows) { group in
                    Section {
                        ForEach(group.items.prefix(Self.preview)) { rec in
                            item(rec)
                        }
                        if group.items.count > Self.preview {
                            NavigationLink {
                                RecordingsView(show: group.title)
                            } label: {
                                Text("All \(group.items.count) of \(group.title)")
                            }
                            .accessibilityIdentifier("show-page")
                        }
                    } header: {
                        Text(group.title)
                    } footer: {
                        if group.items.count > 1 {
                            Text(group.line)
                        }
                    }
                }
            }
            if !built.movies.isEmpty {
                Section("Movies") {
                    ForEach(built.movies) { rec in
                        item(rec)
                    }
                }
            }
        }
        .navigationTitle(show ?? "Recordings")
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
                Button(rec.isMissing ? "Remove from the list" : "Delete this file", role: .destructive) {
                    act { try await store.deleteRecording(rec) }
                }
                Button("Cancel", role: .cancel) {}
            } message: { rec in
                Text(rec.isMissing ? "Its file is already gone." : "The recording and its commercial markers are removed from the server.")
            }
            .confirmationDialog(
                chosen.count == 1 ? "Delete 1 file?" : "Delete \(chosen.count) files?",
                isPresented: $confirmingMany,
                titleVisibility: .visible
            ) {
                Button(chosen.count == 1 ? "Delete 1 file" : "Delete \(chosen.count) files", role: .destructive) { runMany(.delete) }
                Button("Keep them", role: .cancel) {}
            } message: {
                Text("The recordings and their commercial markers are removed from the server.")
            }
        #if os(tvOS)
            .onAppear { claimRecordingFocus() }
            .onChange(of: tvSelectedTab) { _, _ in claimRecordingFocus() }
            .onChange(of: store.recordings.isEmpty) { _, _ in claimRecordingFocus() }
        #endif
            .onChange(of: listed.isEmpty) { _, empty in
                if empty {
                    stopSelecting()
                }
            }
        #if os(tvOS)
            .onChange(of: library.recording, initial: true) { _, _ in openLinked() }
            .onChange(of: library.closeToken) { _, _ in
                playing = nil
                playingChannel = nil
            }
            .onChange(of: playing != nil || playingChannel != nil) { _, up in library.playing = up }
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

    #if os(tvOS)
        /// Plays the recording a link named (the Top Shelf's Continue watching).
        /// The link sets it only once the recording is in a fresh list.
        private func openLinked() {
            guard show == nil, let id = library.recording else { return }
            library.recording = nil
            playing = store.recordings.first { $0.id == id }
        }
    #endif

    /// The row in one sentence, with the same facts it shows: status, date, size, length, Watched, Kept forever.
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

    private func row(_ rec: Recording, focus: Int64) -> some View {
        Group {
            if rec.isMissing {
                // Nothing to play. Select offers to take it off the list.
                Button { deleting = rec } label: { rowLabel(rec) }
            } else {
                #if os(tvOS)
                    Button { playing = rec } label: { rowLabel(rec) }
                #else
                    NavigationLink(value: rec) { rowLabel(rec) }
                #endif
            }
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
                if !rec.isRecording, !rec.isMissing {
                    Button(rec.isWatched ? "Unwatched" : "Watched", systemImage: rec.isWatched ? "eye.slash" : "eye") {
                        act { try await store.setWatched(rec, !rec.isWatched) }
                    }
                    .tint(Tokens.ColorToken.accent)
                }
            }
        #endif
        #if os(tvOS)
        .focused($focusedRec, equals: focus)
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
        } else if rec.isMissing {
            Button("Delete", systemImage: "trash", role: .destructive) { deleting = rec }
        } else {
            Button(rec.isWatched ? "Mark unwatched" : "Mark watched", systemImage: rec.isWatched ? "eye.slash" : "eye") {
                act { try await store.setWatched(rec, !rec.isWatched) }
            }
            // A toggle, so the menu shows a check on a kept recording instead of a second tap un-keeping it unseen.
            Toggle("Keep forever", systemImage: "pin", isOn: Binding(
                get: { rec.keep == true },
                set: { on in act { try await store.setKeep(rec, on) } }
            ))
            .accessibilityIdentifier("keep-forever")
            Button("Make a channel", systemImage: "tv") {
                makeChannel(rec)
            }
            Button(detecting.contains(rec.id) ? CommercialScan.finding : "Find commercials", systemImage: "forward.end") {
                findBreaks(rec)
            }
            .disabled(detecting.contains(rec.id))
            .accessibilityIdentifier("find-commercials")
            if RecordAgain.offered(rec), !scheduled.contains(rec.id) {
                Button(RecordAgain.label, systemImage: "arrow.clockwise.circle") {
                    recordAgain(rec)
                }
                .disabled(scheduling.contains(rec.id))
                .accessibilityLabel(RecordAgain.label)
                .accessibilityIdentifier("record-again")
            }
            Button("Delete", systemImage: "trash", role: .destructive) { deleting = rec }
        }
    }

    /// "S2 E5 · 4.1 · Stopped early · Sep 27, 8:00 PM · 2.1 GB · 1:02:00 · Watched", as the web library reads.
    private func details(_ rec: Recording) -> String {
        if rec.isMissing {
            return "\(rec.guideNumber) · The file is gone. It was moved or deleted outside Broadwave."
        }
        var parts = [rec.guideNumber]
        if let tag = rec.episodeTag {
            parts.insert(tag, at: 0)
        }
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
        if !rec.isRecording, rec.keep == true {
            parts.append("Kept forever")
        }
        return parts.joined(separator: " · ")
    }

    /// A show on the main list shows this many; its own page shows them all.
    private static let preview = 4

    private var order: Library.Sort {
        sort ?? (show == nil ? .newest : .oldest)
    }

    /// The show filter, then the kind and Unwatched. An empty show lists every recording.
    private var listed: [Recording] {
        store.recordings.filter { sameShowTitle($0.title, show ?? library.show) }
    }

    private var shown: [Recording] {
        Library.filter(listed, kind: show == nil ? kind : .all, unwatchedOnly: unwatchedOnly)
    }

    /// Only what the filters show, so a delete never reaches a recording nobody saw.
    private var choosable: [Recording] {
        shown.filter { !$0.isRecording }
    }

    private var chosen: [Recording] {
        choosable.filter { picked.contains($0.id) }
    }

    private var selectionBar: some View {
        Section {
            Button("Select all") { picked.formUnion(choosable.map(\.id)) }
                .disabled(chosen.count == choosable.count)
            Button("Mark watched", systemImage: "eye") { runMany(.watched) }
                .disabled(chosen.isEmpty || busy)
            Button("Mark unwatched", systemImage: "eye.slash") { runMany(.unwatched) }
                .disabled(chosen.isEmpty || busy)
            Button("Delete", systemImage: "trash", role: .destructive) { confirmingMany = true }
                .disabled(chosen.isEmpty || busy)
        } header: {
            Text(chosen.count == 1 ? "1 selected" : "\(chosen.count) selected")
                .accessibilityIdentifier("selected-count")
        }
    }

    /// A recording in Continue watching is also in its show, so its row there
    /// takes focus under the negated id.
    @ViewBuilder
    private func item(_ rec: Recording, resume: Bool = false) -> some View {
        if selecting {
            pickRow(rec)
        } else {
            row(rec, focus: resume ? -rec.id : rec.id)
        }
    }

    /// In Select, a row turns its mark on and off instead of playing.
    private func pickRow(_ rec: Recording) -> some View {
        let on = picked.contains(rec.id)
        return Button {
            if on {
                picked.remove(rec.id)
            } else {
                picked.insert(rec.id)
            }
        } label: {
            HStack(spacing: 12) {
                Image(systemName: on ? "checkmark.circle.fill" : "circle")
                    .foregroundStyle(on ? Tokens.ColorToken.accent : .secondary)
                    .accessibilityHidden(true)
                rowLabel(rec)
            }
        }
        .disabled(rec.isRecording)
        .accessibilityLabel(recordingSpoken(rec))
        .accessibilityAddTraits(on ? .isSelected : [])
        .accessibilityIdentifier("recording-pick")
        #if os(tvOS)
            .focused($focusedRec, equals: rec.id)
        #endif
    }

    private func stopSelecting() {
        selecting = false
        picked = []
    }

    private func runMany(_ action: AppStore.BulkAction) {
        let recs = chosen
        guard !recs.isEmpty, !busy else { return }
        busy = true
        Task {
            let result = await store.apply(action, to: recs)
            busy = false
            let done = recs.count - result.failed
            let what = done == 1 ? "1 recording" : "\(done) recordings"
            let said = switch action {
            case .watched: "Marked \(what) watched."
            case .unwatched: "Marked \(what) unwatched."
            case .delete: "Deleted \(what)."
            }
            if let error = result.error {
                notice = (done > 0 ? said + " " : "") + "\(result.failed) could not be changed: " + PlaybackOutage.actionMessage(error)
            } else {
                notice = said
            }
            stopSelecting()
            #if os(tvOS)
                // The focused action is gone with the bar.
                claimRecordingFocus()
            #endif
        }
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
        guard CommercialScan.start(rec.id, running: &detecting) else { return }
        notice = CommercialScan.finding
        Task {
            defer { detecting.remove(rec.id) }
            #if DEBUG
                // A UI test holds the scan open so it can read the working line. Release builds do not.
                let hold = UserDefaults.standard.double(forKey: "BroadwaveDetectHold")
                if hold > 0 {
                    try? await Task.sleep(nanoseconds: UInt64(hold * 1_000_000_000))
                }
            #endif
            do {
                let found = try await api.detectBreaks(recordingID: rec.id)
                notice = CommercialScan.found(found.count)
            } catch {
                notice = CommercialScan.failed
            }
        }
    }

    private func recordAgain(_ rec: Recording) {
        guard !scheduling.contains(rec.id) else { return }
        scheduling.insert(rec.id)
        Task {
            defer { scheduling.remove(rec.id) }
            do {
                if let airing = try await store.recordAgain(rec) {
                    scheduled.insert(rec.id)
                    notice = RecordAgain.scheduled(airing.start)
                } else {
                    notice = RecordAgain.noAiring
                }
            } catch {
                notice = RecordAgain.failed
            }
        }
    }

    #if os(tvOS)
        /// A list does not take focus from the sidebar the way Settings' form does.
        private func claimRecordingFocus() {
            guard tvSelectedTab == .recordings else { return }
            let built = Library.build(shown, sort: order)
            let first = show == nil && !selecting ? Library.continueWatching(listed, limit: 5).first : nil
            // The order rows are drawn in: a show page by season, the list by show.
            let top = show == nil ? built.shows.first?.items.first : built.shows.first?.seasons.first?.items.first
            if let first {
                focusedRec = -first.id
            } else if let id = (top ?? built.movies.first)?.id {
                focusedRec = id
            } else {
                emptyRecordings = true
            }
        }
    #endif
}
