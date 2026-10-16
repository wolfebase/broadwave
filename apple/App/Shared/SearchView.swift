#if os(iOS)
    import BroadwaveKit
    import BroadwaveUI
    import SwiftUI

    /// Search on iPhone and iPad: results as you type, one card per show. Watch is there
    /// only for what is on now; recording waits for the show's page or a long press.
    struct SearchView: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        @State private var query = ""
        @State private var shown = ""
        @State private var shows: [SearchBoard.Show] = []
        @State private var recordings: [Recording] = []
        @State private var lineup: [Channel] = []
        @State private var searching = false
        @State private var note = ""
        @State private var recent = RecentSearches.load()
        @State private var playing: Recording?

        var body: some View {
            List {
                if typed.count < 2 {
                    startPage
                } else {
                    results
                }
            }
            .listStyle(.insetGrouped)
            .scrollContentBackground(.hidden)
            .background(Tokens.ColorToken.canvas.ignoresSafeArea())
            .navigationTitle("Search")
            .searchable(text: $query, prompt: Text("Shows, movies, teams, channels"))
            .onSubmit(of: .search) { keep(query) }
            .task(id: query) { await liveSearch() }
            .task { await loadLineup() }
            .navigationDestination(for: SearchBoard.Show.ID.self) { id in
                if let show = shows.first(where: { $0.id == id }) {
                    ShowAiringsView(show: show, lineup: lineup)
                }
            }
            .navigationDestination(for: Recording.self) { RecordingPlayerScreen(recording: $0) }
            .fullScreenCover(item: $playing) { recording in
                RecordingCover {
                    RecordingPlayerScreen(recording: recording).environment(store)
                }
            }
            .accessibilityIdentifier("search-page")
            #if DEBUG
                .task {
                    guard query.isEmpty, let seeded = UserDefaults.standard.string(forKey: "BroadwaveSearch"), !seeded.isEmpty else { return }
                    query = seeded
                }
            #endif
        }

        private var typed: String {
            query.trimmingCharacters(in: .whitespacesAndNewlines)
        }

        /// Titles from the guide that complete what was typed, while there is nothing else to show.
        /// The system's suggestion list would cover the results as they arrive.
        @ViewBuilder
        private var suggestions: some View {
            let titles = SearchBoard.suggestions(query, titles: guideTitles, limit: 4)
            if !titles.isEmpty {
                Section("Suggestions") {
                    ForEach(titles, id: \.self) { title in
                        Button {
                            query = title
                            keep(title)
                        } label: {
                            Label(title, systemImage: "magnifyingglass")
                                .foregroundStyle(.primary)
                        }
                        .accessibilityIdentifier("search-suggestion")
                    }
                }
            }
        }

        // MARK: Before typing

        @ViewBuilder
        private var startPage: some View {
            if !recent.items.isEmpty {
                Section {
                    ForEach(recent.items, id: \.self) { word in
                        Button {
                            query = word
                        } label: {
                            Label(word, systemImage: "clock.arrow.circlepath")
                                .foregroundStyle(.primary)
                        }
                        .accessibilityIdentifier("search-recent")
                    }
                } header: {
                    HStack {
                        Text("Recent")
                        Spacer()
                        Button("Clear") {
                            recent.clear()
                            recent.save()
                        }
                        .font(.subheadline)
                        .textCase(nil)
                    }
                }
            }
            let ideas = startIdeas
            if !ideas.isEmpty {
                Section("Try") {
                    ForEach(ideas, id: \.self) { title in
                        Button {
                            query = title
                            keep(title)
                        } label: {
                            Label(title, systemImage: "sparkle.magnifyingglass")
                                .foregroundStyle(.primary)
                        }
                        .accessibilityIdentifier("search-idea")
                    }
                }
            }
        }

        /// Games and favorites on the guide today, so the page is never blank.
        private var startIdeas: [String] {
            var seen = Set<String>()
            var titles: [String] = []
            let soon = store.now.addingTimeInterval(12 * 3600)
            let games = SportsBoard.games(store.sports(hours: 12)).map(\.airing.title)
            let favorites = store.channels.filter(\.favorite).compactMap { store.index.on($0.id, at: store.now)?.title }
            let movies = store.channels.flatMap { c in
                store.index.airings(c.id).filter { $0.kind == .movies && $0.end > store.now && $0.start < soon }.map(\.title)
            }
            for title in games + favorites + movies where seen.insert(title.lowercased()).inserted {
                titles.append(title)
            }
            return Array(titles.prefix(6))
        }

        // MARK: Results

        @ViewBuilder
        private var results: some View {
            if !shows.isEmpty {
                Section {
                    ForEach(shows) { show in
                        NavigationLink(value: show.id) {
                            ShowCard(show: show, channel: channel(for: show.lead), now: store.now, onWatch: { watch(show.lead) })
                        }
                        .accessibilityIdentifier(show.live != nil ? "search-show-live" : "search-show")
                        .contextMenu {
                            ShowActions(show: show, channel: channel(for: show.lead), lineup: lineup)
                        }
                    }
                } header: {
                    Text(shows.count == 1 ? "1 show" : "\(shows.count) shows")
                }
            }
            if !recordings.isEmpty {
                Section("Recordings") {
                    ForEach(recordings) { rec in
                        recordingRow(rec)
                    }
                }
            }
            if shows.isEmpty, recordings.isEmpty {
                suggestions
                Section {
                    if searching || shown != typed {
                        HStack(spacing: 12) {
                            ProgressView()
                            Text("Searching for “\(typed)”…")
                                .foregroundStyle(.secondary)
                        }
                        .accessibilityElement(children: .combine)
                        .accessibilityIdentifier("search-loading")
                    } else {
                        ContentUnavailableView.search(text: typed)
                            .accessibilityIdentifier("search-nothing")
                    }
                }
                .listRowBackground(Color.clear)
            }
            if !note.isEmpty {
                Section {
                    Text(note)
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("search-note")
                }
            }
        }

        @ViewBuilder
        private func recordingRow(_ rec: Recording) -> some View {
            let row = HStack(spacing: 12) {
                RecordingPoster(recording: rec)
                    .frame(width: 96, height: 54)
                    .clipShape(.rect(cornerRadius: Tokens.Radius.sm))
                VStack(alignment: .leading, spacing: 2) {
                    Text(rec.title).font(.headline).lineLimit(2)
                    Text([rec.subtitle, rec.startedAt.formatted(.dateTime.month(.abbreviated).day())].compactMap(\.self).joined(separator: " · "))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            .accessibilityElement(children: .combine)
            if recordingCovers {
                Button {
                    keep(query)
                    playing = rec
                } label: {
                    row
                }
                .foregroundStyle(.primary)
                .disabled(rec.isMissing)
            } else {
                NavigationLink(value: rec) { row }
                    .disabled(rec.isMissing)
            }
        }

        // MARK: Work

        private func channel(for airing: Airing) -> Channel? {
            store.channels.first { $0.id == airing.channelId } ?? lineup.first { $0.id == airing.channelId }
        }

        /// Every title in the loaded guide, for suggestions while typing.
        private var guideTitles: [String] {
            var titles: [String] = []
            for channel in store.channels {
                for airing in store.index.airings(channel.id) where airing.end > store.now {
                    titles.append(airing.title)
                }
            }
            return titles
        }

        private func keep(_ word: String) {
            recent.note(word)
            recent.save()
        }

        private func watch(_ airing: Airing) {
            keep(query)
            note = SearchPlay.watch(airing, store: store, nowPlaying: nowPlaying, lineup: lineup) ?? ""
        }

        /// Runs a quarter second after the last letter, so typing stays smooth.
        private func liveSearch() async {
            let q = typed
            guard q.count >= 2 else {
                shown = q
                shows = []
                recordings = []
                note = ""
                return
            }
            try? await Task.sleep(for: .milliseconds(250))
            guard !Task.isCancelled, let api = store.api else { return }
            searching = true
            defer { searching = false }
            do {
                let found = try await api.search(q)
                guard !Task.isCancelled else { return }
                let visible = Set(store.channels.map(\.id))
                shows = SearchBoard.shows(found.airings, visible: visible, now: store.now)
                recordings = found.recordings.filter { !$0.isMissing }
                note = ""
                shown = q
            } catch {
                guard !Task.isCancelled else { return }
                note = PlaybackOutage.actionMessage(error)
                shown = q
            }
        }

        private func loadLineup() async {
            guard lineup.isEmpty, let api = store.api else { return }
            lineup = await (try? api.lineup()) ?? []
        }
    }

    /// Plays a search result's channel, or its clear twin. Returns a note when it cannot.
    @MainActor
    enum SearchPlay {
        static func watch(_ airing: Airing, store: AppStore, nowPlaying: NowPlaying, lineup: [Channel]) -> String? {
            let all = lineup.isEmpty ? store.channels : lineup
            guard let choice = ClearBroadcast.play(id: airing.channelId, visible: store.channels, lineup: all) else {
                return String(localized: "That channel is not on the guide.")
            }
            nowPlaying.play(choice.channel, note: choice.note)
            return nil
        }
    }

    /// One show: its picture, when it is on, and Watch when it is on now.
    private struct ShowCard: View {
        @Environment(AppStore.self) private var store
        let show: SearchBoard.Show
        let channel: Channel?
        let now: Date
        let onWatch: () -> Void

        var body: some View {
            let lead = show.lead
            HStack(spacing: 12) {
                art(lead)
                    .frame(width: 112, height: 63)
                    .clipShape(.rect(cornerRadius: Tokens.Radius.sm))
                VStack(alignment: .leading, spacing: 3) {
                    Text(GameWords.title(lead))
                        .font(.headline)
                        .lineLimit(2)
                    if show.live != nil {
                        LiveDot("On now")
                    }
                    Text(line(lead))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }
                Spacer(minLength: 0)
                if show.live != nil {
                    Button("Watch", systemImage: "play.fill", action: onWatch)
                        .labelStyle(.iconOnly)
                        .buttonStyle(.glassProminent)
                        .accessibilityLabel("Watch \(lead.title)")
                        .accessibilityIdentifier("search-watch")
                }
            }
            .padding(.vertical, 4)
        }

        @ViewBuilder
        private func art(_ airing: Airing) -> some View {
            if let url = store.artURL(airing, width: 320) {
                ProgramPicture(url: url, width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0, hero: false)
            } else {
                ZStack {
                    Tokens.ColorToken.surface2
                    Text(channel?.displayNumber ?? airing.guideNumber ?? "")
                        .font(.title3.weight(.heavy))
                        .monospacedDigit()
                        .foregroundStyle(.white.opacity(0.35))
                }
            }
        }

        private func line(_ lead: Airing) -> String {
            let station = [channel?.displayNumber ?? lead.guideNumber ?? "", channel?.displayName ?? lead.channelName ?? ""]
                .filter { !$0.isEmpty }.joined(separator: " ")
            var parts: [String] = []
            if show.live != nil {
                parts = [station, lead.minutesLeft(at: now)]
            } else {
                parts = [AiringWhen.short(lead.start), station]
            }
            let more = show.airings.count - 1
            if more > 0 {
                parts.append(more == 1 ? String(localized: "1 more airing") : String(localized: "\(more) more airings"))
            }
            return parts.filter { !$0.isEmpty }.joined(separator: " · ")
        }
    }

    /// A long press on a show: watch what's on, record the next airing or every one. Both ask first.
    private struct ShowActions: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        let show: SearchBoard.Show
        let channel: Channel?
        let lineup: [Channel]

        var body: some View {
            if let live = show.live {
                Button("Watch now", systemImage: "play.fill") { _ = SearchPlay.watch(live, store: store, nowPlaying: nowPlaying, lineup: lineup) }
            }
            NavigationLink(value: show.id) {
                Label("All airings", systemImage: "list.bullet")
            }
        }
    }

    /// Every airing of one show, with Watch for the one on now and the recording choices,
    /// each confirmed before anything is scheduled.
    struct ShowAiringsView: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        let show: SearchBoard.Show
        let lineup: [Channel]
        @State private var askOnce: Airing?
        @State private var askSeries = false
        @State private var note = ""

        var body: some View {
            let lead = show.lead
            List {
                Section {
                    VStack(alignment: .leading, spacing: 12) {
                        if let url = store.artURL(lead, width: 960) {
                            ProgramPicture(url: url, width: lead.imageWidth ?? 0, height: lead.imageHeight ?? 0)
                                .frame(maxWidth: .infinity)
                                .frame(height: 190)
                                .clipShape(.rect(cornerRadius: Tokens.Radius.lg))
                        }
                        Text(GameWords.title(lead)).font(.title.weight(.heavy))
                        if let desc = lead.description, !desc.isEmpty {
                            Text(desc).font(.subheadline).foregroundStyle(.secondary).lineLimit(4)
                        }
                        if let live = show.live {
                            Button {
                                note = SearchPlay.watch(live, store: store, nowPlaying: nowPlaying, lineup: lineup) ?? ""
                            } label: {
                                Label("Watch now", systemImage: "play.fill")
                                    .foregroundStyle(.white)
                                    .frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.glassProminent)
                            .controlSize(.large)
                            .accessibilityIdentifier("show-watch")
                        }
                        Button {
                            askSeries = true
                        } label: {
                            Label(lead.kind == .sports ? "Record every airing" : "Record series", systemImage: "repeat")
                                .frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.glass)
                        .controlSize(.large)
                        .disabled(Pass.series(in: store.passes, for: lead) != nil)
                        .accessibilityIdentifier("show-record-series")
                    }
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                }
                Section("Airings") {
                    ForEach(show.airings) { airing in
                        airingRow(airing)
                    }
                }
                if !note.isEmpty {
                    Section {
                        Text(note).foregroundStyle(.secondary).accessibilityIdentifier("show-note")
                    }
                }
            }
            .navigationTitle(GameWords.title(lead))
            .toolbarTitleDisplayMode(.inline)
            .task { await store.refreshPasses() }
            .confirmationDialog(
                lead.kind == .sports ? "Record every airing of \(lead.title)?" : "Record every episode of \(lead.title)?",
                isPresented: $askSeries,
                titleVisibility: .visible
            ) {
                Button(lead.kind == .sports ? "Record every airing" : "Record series") {
                    act(String(localized: "Recording every \(lead.title).")) { try await store.recordSeries(lead) }
                }
                .accessibilityIdentifier("show-record-series-confirm")
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("New airings record as they come up. You can stop it in Recordings › Upcoming.")
            }
            .confirmationDialog(
                askOnce?.title ?? "",
                isPresented: Binding(get: { askOnce != nil }, set: {
                    if !$0 {
                        askOnce = nil
                    }
                }),
                titleVisibility: .visible,
                presenting: askOnce
            ) { airing in
                Button("Record this airing") {
                    act(String(localized: "\(airing.title) will record, \(AiringWhen.long(airing.start)).")) { try await store.recordOnce(airing) }
                }
                .accessibilityIdentifier("show-record-once-confirm")
                Button("Cancel", role: .cancel) {}
            } message: { airing in
                Text(AiringWhen.long(airing.start))
            }
        }

        private func airingRow(_ airing: Airing) -> some View {
            let on = airing.isOn(at: store.now)
            let channel = store.channels.first { $0.id == airing.channelId } ?? lineup.first { $0.id == airing.channelId }
            let planned = Pass.once(in: store.passes, for: airing) != nil || Pass.series(in: store.passes, for: airing) != nil
            return HStack(spacing: 12) {
                VStack(alignment: .leading, spacing: 3) {
                    Text(on ? String(localized: "On now") : AiringWhen.long(airing.start))
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(on ? Tokens.ColorToken.tally : .primary)
                    Text([airing.subtitle, channel.map { "\($0.displayNumber) \($0.displayName)" } ?? airing.guideNumber].compactMap(\.self).joined(separator: " · "))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }
                Spacer(minLength: 8)
                if on {
                    Button("Watch") { note = SearchPlay.watch(airing, store: store, nowPlaying: nowPlaying, lineup: lineup) ?? "" }
                        .buttonStyle(.glassProminent)
                } else if planned {
                    Label("Will record", systemImage: "record.circle")
                        .labelStyle(.iconOnly)
                        .foregroundStyle(Tokens.ColorToken.tally)
                        .accessibilityLabel("Will record")
                } else {
                    Button("Record") { askOnce = airing }
                        .buttonStyle(.glass)
                        .accessibilityIdentifier("show-record-once")
                }
            }
            .accessibilityElement(children: .combine)
        }

        private func act(_ done: String, _ work: @escaping () async throws -> Void) {
            Task {
                do {
                    try await work()
                    note = done
                    await store.refreshPasses()
                } catch {
                    note = PlaybackOutage.actionMessage(error)
                }
            }
        }
    }

    /// "Today 8:00 PM", "Tomorrow 8:00 PM", "Sat 8:00 PM".
    enum AiringWhen {
        static func short(_ date: Date) -> String {
            let cal = Calendar.current
            let time = date.formatted(date: .omitted, time: .shortened)
            if cal.isDateInToday(date) {
                return time
            }
            if cal.isDateInTomorrow(date) {
                return String(localized: "Tomorrow \(time)")
            }
            return "\(date.formatted(.dateTime.weekday(.abbreviated))) \(time)"
        }

        static func long(_ date: Date) -> String {
            let cal = Calendar.current
            let time = date.formatted(date: .omitted, time: .shortened)
            if cal.isDateInToday(date) {
                return String(localized: "Today at \(time)")
            }
            if cal.isDateInTomorrow(date) {
                return String(localized: "Tomorrow at \(time)")
            }
            return String(localized: "\(date.formatted(.dateTime.weekday(.wide))) at \(time)")
        }
    }
#endif
