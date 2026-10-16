#if os(tvOS)
    import BroadwaveKit
    import BroadwaveUI
    import SwiftUI

    /// Search on Apple TV: the system keyboard on top, results under it as you type.
    /// Nothing waits for the keyboard's search key.
    struct TVSearchView: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        @Environment(LibraryFilter.self) private var library
        @State private var query = ""
        @State private var shown = ""
        @State private var groups = SearchBoard.Groups(onNow: [], later: [])
        @State private var recordings: [Recording] = []
        @State private var lineup: [Channel] = []
        @State private var searching = false
        @State private var note = ""
        @State private var recent = RecentSearches.load()
        @State private var asking: Airing?
        @State private var playing: Recording?

        static let cardWidth: CGFloat = 400
        private let columns = Array(repeating: GridItem(.fixed(TVSearchView.cardWidth), spacing: 48, alignment: .top), count: 4)

        var body: some View {
            ScrollView {
                VStack(alignment: .leading, spacing: 56) {
                    if typed.count < 2 {
                        startPage
                    } else {
                        results
                    }
                }
                .padding(.top, 20)
                .padding(.bottom, 60)
            }
            .scrollClipDisabled()
            .searchable(text: $query, prompt: Text("Shows, movies, teams, channels"))
            .searchSuggestions {
                ForEach(SearchBoard.suggestions(query, titles: guideTitles), id: \.self) { title in
                    Text(title).searchCompletion(title)
                }
            }
            .onSubmit(of: .search) { keep(query) }
            .task(id: query) { await liveSearch() }
            .task { await loadLineup() }
            .confirmationDialog(
                asking?.title ?? "",
                isPresented: Binding(get: { asking != nil }, set: {
                    if !$0 {
                        asking = nil
                    }
                }),
                titleVisibility: .visible,
                presenting: asking
            ) { airing in
                Button("Record this one") { Task { await recordOnce(airing) } }
                Button(airing.kind == .sports ? "Record every airing" : "Record every episode") { Task { await recordAll(airing) } }
                Button("Cancel", role: .cancel) {}
            } message: { airing in
                Text(when(airing))
            }
            .fullScreenCover(item: $playing) { recording in
                RecordingCover {
                    RecordingPlayerScreen(recording: recording).environment(store)
                }
            }
            .onChange(of: playing != nil) { _, up in library.playing = up }
            .onChange(of: library.closeToken) { _, _ in playing = nil }
            .background(Tokens.ColorToken.canvas.ignoresSafeArea())
            .accessibilityIdentifier("search-page")
            #if DEBUG
                .task {
                    // UI tests that cannot type start with a query, as a viewer would after typing it.
                    guard query.isEmpty, let seeded = UserDefaults.standard.string(forKey: "BroadwaveSearch"), !seeded.isEmpty else { return }
                    query = seeded
                }
            #endif
        }

        private var typed: String {
            query.trimmingCharacters(in: .whitespacesAndNewlines)
        }

        // MARK: Before typing

        @ViewBuilder
        private var startPage: some View {
            if !recent.items.isEmpty {
                VStack(alignment: .leading, spacing: 20) {
                    TVSectionTitle("Recent searches")
                    ScrollView(.horizontal) {
                        HStack(spacing: 24) {
                            ForEach(recent.items, id: \.self) { word in
                                Button(word) { query = word }
                                    .buttonStyle(TVChipStyle())
                                    .accessibilityIdentifier("search-recent")
                            }
                            Button("Clear", systemImage: "xmark") {
                                recent.clear()
                                recent.save()
                            }
                            .buttonStyle(TVChipStyle())
                        }
                        .padding(.vertical, 16)
                    }
                    .scrollClipDisabled()
                    .focusSection()
                }
            }
            let live = popular
            if !live.isEmpty {
                VStack(alignment: .leading, spacing: 20) {
                    TVSectionTitle("On now", detail: "Or type a show, a team, or a channel.")
                    ScrollView(.horizontal) {
                        LazyHStack(alignment: .top, spacing: 48) {
                            ForEach(live, id: \.1.id) { channel, airing in
                                airingCard(airing, channel: channel)
                            }
                        }
                        .padding(.vertical, 24)
                    }
                    .scrollClipDisabled()
                    .focusSection()
                }
            }
        }

        /// What's on the favorites first, then the rest, for a page that is never empty.
        private var popular: [(Channel, Airing)] {
            let list = store.channels.compactMap { c -> (Channel, Airing)? in
                guard let a = store.index.on(c.id, at: store.now) else { return nil }
                return (c, a)
            }
            return Array(list.sorted { $0.0.favorite && !$1.0.favorite }.prefix(12))
        }

        // MARK: Results

        @ViewBuilder
        private var results: some View {
            if !groups.onNow.isEmpty {
                grid("On now", count: groups.onNow.count) {
                    ForEach(groups.onNow) { airing in
                        airingCard(airing, channel: channel(for: airing))
                    }
                }
            }
            if !recordings.isEmpty {
                grid("Recordings", count: recordings.count) {
                    ForEach(recordings) { rec in
                        recordingCard(rec)
                    }
                }
            }
            if !groups.later.isEmpty {
                grid("Coming up", count: groups.later.count) {
                    ForEach(groups.later.prefix(40)) { airing in
                        airingCard(airing, channel: channel(for: airing))
                    }
                }
            }
            if groups.isEmpty, recordings.isEmpty {
                VStack(alignment: .leading, spacing: 12) {
                    if searching || shown != typed {
                        HStack(spacing: 20) {
                            ProgressView()
                            Text("Searching for “\(typed)”…")
                                .foregroundStyle(.secondary)
                        }
                    } else {
                        Text(note.isEmpty ? "Nothing on your channels matches “\(typed)”." : note)
                            .font(.title3.weight(.semibold))
                            .accessibilityIdentifier("search-nothing")
                        Text("Try a show, a team, or a channel name.")
                            .foregroundStyle(.secondary)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.top, 20)
            } else if !note.isEmpty {
                Text(note)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("search-note")
            }
        }

        private func grid(_ title: String, count: Int, @ViewBuilder cards: () -> some View) -> some View {
            VStack(alignment: .leading, spacing: 20) {
                TVSectionTitle(title, detail: count == 1 ? "1 result" : "\(count) results")
                LazyVGrid(columns: columns, alignment: .leading, spacing: 64) {
                    cards()
                }
                .padding(.vertical, 20)
                .focusSection()
            }
        }

        private func airingCard(_ airing: Airing, channel: Channel?) -> some View {
            let on = airing.isOn(at: store.now)
            let width = Self.cardWidth
            return Button {
                keep(query)
                if on {
                    watch(airing)
                } else {
                    asking = airing
                }
            } label: {
                VStack(alignment: .leading, spacing: 16) {
                    TVCardArt(width: width) {
                        TVProgramArt(channel: channel, airing: airing, width: 640)
                            .overlay(alignment: .topLeading) {
                                HStack(spacing: 8) {
                                    if on {
                                        TVBadge("Live", tone: .live)
                                    } else {
                                        TVBadge(dayAndTime(airing.start))
                                    }
                                    if channel?.isATSC3 == true {
                                        TVBadge("3.0")
                                    }
                                }
                                .padding(14)
                            }
                            .overlay(alignment: .bottom) {
                                if on {
                                    AiringProgress(airing.progress(at: store.now), color: Tokens.ColorToken.tally)
                                        .padding(.horizontal, 14)
                                        .padding(.bottom, 12)
                                }
                            }
                    }
                    TVCardCaption(title: airing.title, line: line(airing, channel: channel), width: width)
                }
            }
            .buttonStyle(TVCardStyle())
            .accessibilityLabel(spoken(airing, channel: channel, on: on))
            .accessibilityIdentifier(on ? "search-watch" : "search-later")
            .contextMenu {
                if on {
                    Button("Watch now", systemImage: "play.fill") { watch(airing) }
                    if let channel {
                        Button(store.activeRecording(on: channel) == nil ? "Record" : "Stop recording", systemImage: "record.circle") {
                            Task { await store.toggleRecord(channel) }
                        }
                    }
                } else {
                    Button("Record this one", systemImage: "record.circle") { Task { await recordOnce(airing) } }
                }
                Button(airing.kind == .sports ? "Record every airing" : "Record every episode", systemImage: "repeat") {
                    Task { await recordAll(airing) }
                }
            }
        }

        private func recordingCard(_ rec: Recording) -> some View {
            let width = Self.cardWidth
            return Button {
                keep(query)
                playing = rec
            } label: {
                VStack(alignment: .leading, spacing: 16) {
                    TVCardArt(width: width) {
                        RecordingPoster(recording: rec)
                            .overlay(alignment: .topLeading) {
                                TVBadge(rec.isRecording ? "Recording" : "Recorded", tone: rec.isRecording ? .live : .quiet)
                                    .padding(14)
                            }
                    }
                    TVCardCaption(
                        title: rec.title,
                        line: [rec.subtitle, rec.startedAt.formatted(.dateTime.month(.abbreviated).day())].compactMap(\.self).joined(separator: " · "),
                        width: width
                    )
                }
            }
            .buttonStyle(TVCardStyle())
            .disabled(rec.isMissing)
            .accessibilityLabel("\(rec.title), recorded \(rec.startedAt.formatted(date: .abbreviated, time: .omitted))")
            .accessibilityIdentifier("search-recording")
        }

        // MARK: Words

        private func channel(for airing: Airing) -> Channel? {
            store.channels.first { $0.id == airing.channelId } ?? lineup.first { $0.id == airing.channelId }
        }

        private func line(_ airing: Airing, channel: Channel?) -> String {
            let number = channel?.displayNumber ?? airing.guideNumber ?? ""
            let name = channel?.displayName ?? airing.channelName ?? ""
            let station = [number, name].filter { !$0.isEmpty }.joined(separator: " ")
            if airing.isOn(at: store.now) {
                return [station, airing.minutesLeft(at: store.now)].filter { !$0.isEmpty }.joined(separator: " · ")
            }
            return [airing.subtitle ?? "", station].filter { !$0.isEmpty }.joined(separator: " · ")
        }

        private func spoken(_ airing: Airing, channel: Channel?, on: Bool) -> String {
            var parts = [airing.title]
            if let sub = airing.subtitle, !sub.isEmpty {
                parts.append(sub)
            }
            if let channel {
                parts.append("\(channel.displayNumber) \(channel.displayName)")
                if channel.isATSC3 {
                    parts.append("ATSC 3.0")
                }
            }
            parts.append(on ? "On now" : when(airing))
            return parts.joined(separator: ", ")
        }

        private func dayAndTime(_ date: Date) -> String {
            let cal = Calendar.current
            let time = date.formatted(date: .omitted, time: .shortened)
            if cal.isDateInToday(date) {
                return time
            }
            if cal.isDateInTomorrow(date) {
                return "Tomorrow \(time)"
            }
            return "\(date.formatted(.dateTime.weekday(.abbreviated))) \(time)"
        }

        private func when(_ airing: Airing) -> String {
            let cal = Calendar.current
            let time = airing.start.formatted(date: .omitted, time: .shortened)
            if cal.isDateInToday(airing.start) {
                return "Today at \(time)"
            }
            if cal.isDateInTomorrow(airing.start) {
                return "Tomorrow at \(time)"
            }
            return "\(airing.start.formatted(.dateTime.weekday(.wide))) at \(time)"
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

        // MARK: Work

        private func keep(_ word: String) {
            recent.note(word)
            recent.save()
        }

        /// Runs a quarter second after the last letter, so typing stays smooth.
        private func liveSearch() async {
            let q = typed
            guard q.count >= 2 else {
                shown = q
                groups = SearchBoard.Groups(onNow: [], later: [])
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
                groups = SearchBoard.group(found.airings, visible: visible, now: store.now)
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

        private func watch(_ airing: Airing) {
            let all = lineup.isEmpty ? store.channels : lineup
            guard let choice = ClearBroadcast.play(id: airing.channelId, visible: store.channels, lineup: all) else {
                note = "That channel is not on the guide."
                return
            }
            nowPlaying.play(choice.channel, note: choice.note)
        }

        private func recordOnce(_ airing: Airing) async {
            do {
                try await store.recordOnce(airing)
                note = "\(airing.title) will record, \(when(airing))."
            } catch {
                note = PlaybackOutage.actionMessage(error)
            }
        }

        private func recordAll(_ airing: Airing) async {
            do {
                try await store.recordSeries(airing)
                note = "Recording every \(airing.title)."
            } catch {
                note = PlaybackOutage.actionMessage(error)
            }
        }
    }
#endif
