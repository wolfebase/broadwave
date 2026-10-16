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
    /// A cover, like live TV: a pushed player would keep the sidebar's handle on the picture.
    @State private var playing: Recording?
    @State private var playingChannel: VirtualChannel?
    #if os(iOS)
        /// A recording a link named (Spotlight), pushed like a tapped row.
        @State private var linked: Recording?
    #endif

    #if os(tvOS)
        @Environment(\.tvSelectedTab) private var tvSelectedTab
        @FocusState private var focusedRec: Int64?
        @FocusState private var emptyRecordings: Bool
        @Environment(\.tvGo) private var tvGo
        @State private var loaded = false
        @State private var explaining = false
    #endif

    var body: some View {
        page
            .navigationTitle(show ?? "Recordings")
        #if os(iOS)
            .toolbar { libraryToolbar }
        #endif
            .fullScreenCover(item: $playing) { recording in
                RecordingCover {
                    RecordingPlayerScreen(recording: recording).environment(store)
                }
            }
            .fullScreenCover(item: $playingChannel) { channel in
                RecordingCover {
                    RecordingPlayerScreen(channel: channel).environment(store)
                }
            }
        #if os(iOS)
            .navigationDestination(for: Recording.self) { RecordingPlayerScreen(recording: $0) }
            .navigationDestination(for: VirtualChannel.self) { RecordingPlayerScreen(channel: $0) }
            .navigationDestination(item: $linked) { RecordingPlayerScreen(recording: $0) }
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
                "Delete \(chosen.count) files?",
                isPresented: $confirmingMany,
                titleVisibility: .visible
            ) {
                Button("Delete \(chosen.count) files", role: .destructive) { runMany(.delete) }
                Button("Keep them", role: .cancel) {}
            } message: {
                Text("The recordings and their commercial markers are removed from the server.")
            }
        #if os(tvOS)
            .onAppear { claimRecordingFocus() }
            .onChange(of: tvSelectedTab) { _, _ in claimRecordingFocus() }
            .onChange(of: store.recordings.isEmpty) { _, _ in claimRecordingFocus() }
            .onChange(of: loaded) { _, _ in claimRecordingFocus() }
        #endif
            .onChange(of: listed.isEmpty) { _, empty in
                if empty {
                    stopSelecting()
                }
            }
            .onChange(of: library.recording, initial: true) { _, _ in openLinked() }
            .onChange(of: library.closeToken) { _, _ in
                playing = nil
                playingChannel = nil
            }
            .onChange(of: playing != nil || playingChannel != nil) { _, up in library.playing = up }
            .task {
                async let recordings: Void = store.refreshRecordings()
                async let channels: Void = store.refreshVirtuals()
                _ = await (recordings, channels)
                #if os(tvOS)
                    loaded = true
                #endif
                #if DEBUG
                    // Simulator testing: -BroadwaveSampleRecording shows one row when the
                    // demo server has none. -BroadwaveRecording <id> opens it as Select
                    // would; simctl cannot press Select. iPhone does not open a cover.
                    if UserDefaults.standard.bool(forKey: "BroadwaveSampleRecording"), store.recordings.isEmpty {
                        store.previewRecording(Recording(
                            id: 1,
                            channelId: 1,
                            guideNumber: "4.1",
                            title: "Big Buck Bunny",
                            subtitle: "Open movie",
                            status: "complete",
                            startedAt: Date(timeIntervalSince1970: 1_790_000_000),
                            bytes: 12_000_000,
                            durationSec: 596
                        ))
                    }
                    let id = Int64(UserDefaults.standard.integer(forKey: "BroadwaveRecording"))
                    if id > 0, playing == nil, recordingCovers {
                        playing = store.recordings.first { $0.id == id }
                    }
                #endif
            }
    }

    /// Apple TV's library page is shelves of posters; a show's own page, and Select, stay a list.
    @ViewBuilder
    private var page: some View {
        #if os(tvOS)
            if show == nil, !selecting {
                shelves
            } else {
                list.tvPageChrome()
            }
        #else
            list
        #endif
    }

    private var list: some View {
        let built = Library.build(shown, sort: order)
        let resume = show == nil && !selecting ? Library.continueWatching(listed, limit: 5) : []
        return List {
            #if os(tvOS)
                TVPageTitle(show ?? String(localized: "Recordings"), subtitle: Library.count(listed))
            #endif
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
            #if os(tvOS)
                // A TV lists a show's episodes, oldest first. Filters and sorting by size are for the web.
                if selecting || !choosable.isEmpty {
                    Button(selecting ? "Done" : "Select") { selecting ? stopSelecting() : (selecting = true) }
                        .disabled(busy)
                        .accessibilityIdentifier("library-select")
                }
            #else
                // The filters live in the bar's menu; the page keeps one line of what they show.
                if !listed.isEmpty {
                    Text(filterLine)
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                        .listRowBackground(Color.clear)
                        .accessibilityIdentifier("library-summary")
                }
            #endif
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
                #if os(tvOS)
                    ContentUnavailableView(
                        listed.isEmpty ? "No recordings yet" : unwatchedOnly ? "All watched" : "Nothing here",
                        systemImage: "record.circle",
                        description: Text(listed.isEmpty ? "Record from the guide, or set a series to record every episode." : unwatchedOnly ? "Everything here has been watched." : "Nothing here matches.")
                    )
                    .focusable()
                    .focused($emptyRecordings)
                #else
                    emptyState
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
                            if season.id == item.seasons.last?.id, !isTV {
                                Text(item.line)
                            }
                        }
                    }
                }
            } else {
                #if os(iOS)
                    // A show with one recording is a row under Recent, not a section whose
                    // header repeats the row's own title.
                    let singles = built.shows.filter { $0.items.count == 1 }.flatMap(\.items)
                    if !singles.isEmpty {
                        Section("Recent") {
                            ForEach(singles) { rec in
                                item(rec, titled: true)
                            }
                        }
                    }
                    let groups = built.shows.filter { $0.items.count > 1 }
                #else
                    let groups = built.shows
                #endif
                ForEach(groups) { group in
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
    }

    /// Plays the recording a link named (the Top Shelf's Continue watching, Spotlight).
    /// The link sets it only once the recording is in a fresh list.
    private func openLinked() {
        guard show == nil, let id = library.recording else { return }
        library.recording = nil
        if recordingCovers {
            playing = store.recordings.first { $0.id == id }
        } else {
            #if os(iOS)
                linked = store.recordings.first { $0.id == id }
            #endif
        }
    }

    /// The row in one sentence, with the same facts it shows: status, date, size, length, Watched, Kept forever.
    private func recordingSpoken(_ rec: Recording) -> String {
        var parts: [String] = []
        if rec.isRecording {
            parts.append("Recording")
        }
        // The show's title first: an episode name alone does not say what it is.
        parts.append(rec.title)
        if let sub = rec.subtitle, !sub.isEmpty, sub != rec.title {
            parts.append(sub)
        }
        parts.append(details(rec).replacingOccurrences(of: " · ", with: ", "))
        if let line = rec.signalLine {
            parts.append(line)
        }
        return parts.joined(separator: ", ")
    }

    private func libraryRow(_ channel: VirtualChannel) -> some View {
        let label = VStack(alignment: .leading, spacing: 4) {
            Text("\(channel.number) \(channel.name)").font(.headline).lineLimit(1)
            Text(libraryLine(channel.recordings.count))
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .accessibilityElement(children: .combine)
        return Group {
            if recordingCovers {
                Button { playingChannel = channel } label: { label }.rowInk()
            } else {
                NavigationLink(value: channel) { label }
            }
        }
        .accessibilityIdentifier("library-\(channel.id)")
    }

    private func libraryLine(_ count: Int) -> String {
        switch count {
        case 0: String(localized: "No recordings on it yet. Uses no tuner.")
        case 1: String(localized: "Plays 1 recording. Uses no tuner.")
        default: String(localized: "Plays \(count) recordings in turn. Uses no tuner.")
        }
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

    private func row(_ rec: Recording, focus: Int64, titled: Bool = false) -> some View {
        Group {
            if rec.isMissing {
                // Nothing to play. Select offers to take it off the list.
                Button { deleting = rec } label: { rowLabel(rec, titled: titled) }.rowInk()
            } else if recordingCovers {
                Button { playing = rec } label: { rowLabel(rec, titled: titled) }.rowInk()
            } else {
                NavigationLink(value: rec) { rowLabel(rec, titled: titled) }
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

    /// `titled` leads with the show's name: a row outside its show's section has no header to say it.
    private func rowLabel(_ rec: Recording, titled: Bool = false) -> some View {
        HStack(spacing: 14) {
            RecordingPoster(recording: rec)
                .frame(width: 120, height: 68)
                .clipShape(.rect(cornerRadius: Tokens.Radius.sm))
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    if rec.isRecording {
                        LiveDot("Recording")
                    }
                    Text(titled ? rec.title : rec.subtitle ?? rec.title).font(.headline).lineLimit(1)
                }
                if titled, let sub = rec.subtitle, !sub.isEmpty, sub != rec.title {
                    Text(sub).font(.subheadline).foregroundStyle(.secondary).lineLimit(1)
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
            // The file is gone. Record it again still asks the guide for the next airing.
            if RecordAgain.offered(rec), !scheduled.contains(rec.id) {
                Button(RecordAgain.label, systemImage: "arrow.clockwise.circle") {
                    recordAgain(rec)
                }
                .disabled(scheduling.contains(rec.id))
                .accessibilityLabel(RecordAgain.label)
                .accessibilityIdentifier("record-again")
            }
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
        #if os(iOS)
            // A television is no place to manage disk space.
            if let bytes = rec.bytes, bytes > 0 {
                parts.append(ByteCountFormatter.string(fromByteCount: bytes, countStyle: .file))
            }
        #endif
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

    #if os(iOS)
        @Environment(\.openURL) private var openURL

        /// Unwatched, the kind, and the order in one menu, and Select beside it.
        @ToolbarContentBuilder
        private var libraryToolbar: some ToolbarContent {
            if !listed.isEmpty {
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        Picker("Show", selection: $unwatchedOnly) {
                            Text("All").tag(false)
                            Text("Unwatched").tag(true)
                        }
                        if show == nil {
                            Picker("Show only", selection: $kind) {
                                ForEach(Library.Kind.allCases, id: \.self) { Text($0.label).tag($0) }
                            }
                            .pickerStyle(.menu)
                        }
                        Picker("Sort", selection: Binding(get: { order }, set: { sort = $0 })) {
                            ForEach(Library.Sort.allCases, id: \.self) { Text($0.label).tag($0) }
                        }
                        .pickerStyle(.menu)
                    } label: {
                        Label("Filter", systemImage: filtered ? "line.3.horizontal.decrease.circle.fill" : "line.3.horizontal.decrease.circle")
                    }
                    .accessibilityIdentifier("library-filter")
                }
            }
            if selecting || !choosable.isEmpty {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(selecting ? "Done" : "Select") { selecting ? stopSelecting() : (selecting = true) }
                        .disabled(busy)
                        .accessibilityIdentifier("library-select")
                }
            }
        }

        private var filtered: Bool {
            unwatchedOnly || kind != .all
        }

        /// "10 recordings · 9.65 GB", with what the menu narrowed it to.
        private var filterLine: String {
            var parts: [String] = []
            if unwatchedOnly {
                parts.append(String(localized: "Unwatched"))
            }
            if show == nil, kind != .all {
                parts.append(kind.label)
            }
            parts.append(Library.summary(listed))
            return parts.joined(separator: " · ")
        }

        private var emptyState: some View {
            ContentUnavailableView {
                Label(listed.isEmpty ? "No recordings yet" : unwatchedOnly ? "All watched" : "Nothing here", systemImage: "record.circle")
            } description: {
                Text(listed.isEmpty ? "Record from the guide, or set a series to record every episode." : unwatchedOnly ? "Everything here has been watched." : "Nothing here matches.")
            } actions: {
                if listed.isEmpty, show == nil {
                    Button("Open the guide") {
                        if let url = URL(string: "broadwave://guide") {
                            openURL(url)
                        }
                    }
                    .buttonStyle(.glassProminent)
                    .accessibilityIdentifier("recordings-open-guide")
                }
            }
            .listRowBackground(Color.clear)
        }
    #endif

    private var isTV: Bool {
        #if os(tvOS)
            true
        #else
            false
        #endif
    }

    /// A show on the main list shows this many; its own page shows them all.
    private static let preview = 4

    private var order: Library.Sort {
        sort ?? (show == nil ? .newest : .oldest)
    }

    /// The show filter, then the kind and Unwatched. An empty show lists every recording.
    private var listed: [Recording] {
        #if DEBUG
            // The offline page test (-BroadwaveFocus recordings) is an empty library,
            // whatever server the simulator used last.
            if UserDefaults.standard.string(forKey: "BroadwaveFocus") == "recordings" {
                return []
            }
        #endif
        return store.recordings.filter { sameShowTitle($0.title, show ?? library.show) }
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
            Text("\(chosen.count) selected")
                .accessibilityIdentifier("selected-count")
        }
    }

    /// A recording in Continue watching is also in its show, so its row there
    /// takes focus under the negated id.
    @ViewBuilder
    private func item(_ rec: Recording, resume: Bool = false, titled: Bool = false) -> some View {
        if selecting {
            pickRow(rec)
        } else {
            row(rec, focus: resume ? -rec.id : rec.id, titled: titled || resume)
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
            if show == nil, !selecting {
                // The shelves: Continue watching, then the newest recording, then the empty page's button.
                if let first = Library.continueWatching(listed, limit: 1).first {
                    focusedRec = -first.id
                } else if let newest = listed.filter({ !$0.isRecording && !$0.isMissing }).max(by: { $0.startedAt < $1.startedAt }) {
                    focusedRec = newest.id
                } else if listed.isEmpty {
                    emptyRecordings = true
                }
                return
            }
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

#if os(tvOS)
    /// Apple TV's library: shelves of posters, like the TV app. Select plays; a long press
    /// has the same actions as a row. A show's card opens its own page with every episode.
    extension RecordingsView {
        static let cardWidth: CGFloat = 400

        var shelves: some View {
            let all = listed
            let built = Library.build(all, sort: .newest)
            let now = all.filter(\.isRecording)
            let resume = Library.continueWatching(all, limit: 12)
            let recent = Array(all.filter { !$0.isRecording && !$0.isMissing }.sorted { $0.startedAt > $1.startedAt }.prefix(12))
            return ScrollView {
                LazyVStack(alignment: .leading, spacing: 64) {
                    TVPageTitle(String(localized: "Recordings"), subtitle: all.isEmpty ? nil : Library.count(all))
                    if all.isEmpty {
                        if loaded || !store.recordings.isEmpty {
                            emptyLibrary
                        } else {
                            ProgressView().frame(maxWidth: .infinity)
                        }
                    } else {
                        topChips(choosable: !choosable.isEmpty)
                        if let notice {
                            Text(notice)
                                .font(.callout)
                                .foregroundStyle(.secondary)
                                .accessibilityIdentifier("recordings-notice")
                        }
                        if !now.isEmpty {
                            TVShelf(String(localized: "Recording now")) {
                                ForEach(now) { card($0, focus: nil) }
                            }
                        }
                        if !resume.isEmpty {
                            TVShelf(String(localized: "Continue watching")) {
                                ForEach(resume) { card($0, focus: -$0.id) }
                            }
                        }
                        if !recent.isEmpty {
                            TVShelf(String(localized: "Recently recorded")) {
                                ForEach(recent) { card($0, focus: $0.id) }
                            }
                        }
                        if !built.shows.isEmpty {
                            TVShelf(String(localized: "Shows")) {
                                ForEach(built.shows) { showCard($0) }
                            }
                        }
                        if !built.movies.isEmpty {
                            TVShelf(String(localized: "Movies")) {
                                ForEach(built.movies) { card($0, focus: nil) }
                            }
                        }
                        if !store.virtuals.isEmpty {
                            TVShelf(String(localized: "Library channels"), detail: String(localized: "Your recordings in turn, with no tuner.")) {
                                ForEach(store.virtuals) { channelCard($0) }
                            }
                        }
                    }
                }
                .padding(.bottom, 80)
            }
            .scrollClipDisabled()
            .tvPageChrome()
            .accessibilityIdentifier("recordings-page")
            .alert("How recording works", isPresented: $explaining) {
                Button("OK", role: .cancel) {}
            } message: {
                Text("In the guide, press and hold a show and choose Record. Choose Record every episode to keep a series. Games keep recording until they are over.")
            }
        }

        /// Nothing recorded yet: a way forward that takes focus, never a page with nothing focused.
        private var emptyLibrary: some View {
            TVEmptyState(
                symbol: "record.circle",
                title: String(localized: "No recordings yet"),
                message: String(localized: "Record a show from the guide, or set a series to record every episode. Recordings show up here.")
            ) {
                Button("Open the guide", systemImage: "square.grid.3x3.topleft.filled") { tvGo(.guide) }
                    .focused($emptyRecordings)
                    .accessibilityIdentifier("recordings-open-guide")
                NavigationLink {
                    ScheduleView()
                } label: {
                    Label("Upcoming", systemImage: "calendar")
                }
                .accessibilityIdentifier("recordings-upcoming")
                Button("How recording works", systemImage: "questionmark.circle") { explaining = true }
                    .accessibilityIdentifier("recordings-how")
            }
            .defaultFocus($emptyRecordings, true)
        }

        private func topChips(choosable: Bool) -> some View {
            HStack(spacing: 24) {
                NavigationLink {
                    ScheduleView()
                } label: {
                    Label("Upcoming", systemImage: "calendar")
                }
                .buttonStyle(TVChipStyle())
                .accessibilityIdentifier("recordings-upcoming")
                if choosable {
                    Button("Select", systemImage: "checkmark.circle") { selecting = true }
                        .buttonStyle(TVChipStyle())
                        .accessibilityIdentifier("library-select")
                }
                if !library.show.isEmpty {
                    Button("Showing \(library.show) · Show all", systemImage: "xmark") { library.clear() }
                        .buttonStyle(TVChipStyle(selected: true))
                        .accessibilityIdentifier("show-all")
                }
            }
            .padding(.vertical, 8)
            .focusSection()
        }

        private func card(_ rec: Recording, focus: Int64?) -> some View {
            let width = Self.cardWidth
            return Button {
                if rec.isMissing {
                    deleting = rec
                } else {
                    playing = rec
                }
            } label: {
                VStack(alignment: .leading, spacing: 16) {
                    TVCardArt(width: width) {
                        RecordingPoster(recording: rec)
                            .overlay(alignment: .topLeading) {
                                if rec.isRecording {
                                    TVBadge(String(localized: "Live"), tone: .live).padding(14)
                                } else if rec.isMissing {
                                    TVBadge(String(localized: "File gone")).padding(14)
                                } else if rec.isWatched {
                                    TVBadge(String(localized: "Watched")).padding(14)
                                }
                            }
                            .overlay(alignment: .bottom) {
                                if let fraction = progress(rec) {
                                    AiringProgress(fraction)
                                        .padding(.horizontal, 14)
                                        .padding(.bottom, 12)
                                }
                            }
                    }
                    TVCardCaption(title: rec.subtitle ?? rec.title, line: cardLine(rec), width: width)
                }
            }
            .buttonStyle(TVCardStyle())
            .contextMenu { actions(rec) }
            .focused($focusedRec, equals: focus)
            .accessibilityLabel(recordingSpoken(rec))
            .accessibilityIdentifier("recording-card")
        }

        private func showCard(_ group: Library.Show) -> some View {
            let width = Self.cardWidth
            let line = group.unwatched == 0
                ? String(localized: "\(String(group.items.count)) recorded · all watched")
                : String(localized: "\(String(group.items.count)) recorded · \(String(group.unwatched)) new")
            return NavigationLink {
                RecordingsView(show: group.title)
            } label: {
                VStack(alignment: .leading, spacing: 16) {
                    TVCardArt(width: width) {
                        RecordingPoster(recording: group.items[0])
                            .overlay(alignment: .topTrailing) {
                                if group.items.count > 1 {
                                    TVBadge(String(group.items.count)).padding(14)
                                }
                            }
                    }
                    TVCardCaption(title: group.title, line: line, width: width)
                }
            }
            .buttonStyle(TVCardStyle())
            .accessibilityLabel("\(group.title), \(line)")
            .accessibilityIdentifier("show-page")
        }

        private func channelCard(_ channel: VirtualChannel) -> some View {
            let width = Self.cardWidth
            return Button { playingChannel = channel } label: {
                VStack(alignment: .leading, spacing: 16) {
                    TVCardArt(width: width) {
                        TVNumberField(number: channel.number, tint: Tokens.ColorToken.accent)
                    }
                    TVCardCaption(
                        title: channel.name,
                        line: String(localized: "Plays \(String(channel.recordings.count)) recordings in turn"),
                        width: width
                    )
                }
            }
            .buttonStyle(TVCardStyle())
            .accessibilityIdentifier("library-\(channel.id)")
        }

        /// "S2 E5 · Sep 27" or "4.1 · Recording since 8:00 PM": what helps choose, no file sizes.
        private func cardLine(_ rec: Recording) -> String {
            if rec.isRecording {
                return "\(rec.guideNumber) · " + String(localized: "Recording since \(rec.startedAt.formatted(date: .omitted, time: .shortened))")
            }
            var parts: [String] = []
            if rec.subtitle != nil {
                parts.append(rec.title)
            }
            if let tag = rec.episodeTag {
                parts.append(tag)
            }
            parts.append(rec.startedAt.formatted(.dateTime.month(.abbreviated).day()))
            return parts.joined(separator: " · ")
        }

        private func progress(_ rec: Recording) -> Double? {
            guard let pos = rec.position, let dur = rec.durationSec, dur > 0, pos > 30, !rec.isWatched else { return nil }
            return min(1, pos / dur)
        }
    }
#endif

private extension View {
    /// A list tints a button's whole label with the accent; a row reads like a link instead.
    /// tvOS draws its own focused colors, so it keeps the system style.
    func rowInk() -> some View {
        #if os(iOS)
            tint(.primary)
        #else
            self
        #endif
    }
}
