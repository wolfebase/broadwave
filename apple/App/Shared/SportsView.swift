#if os(iOS)
    import BroadwaveKit
    import BroadwaveUI
    import SwiftUI

    /// Sports on iPhone and iPad: games only, each once (the boards Apple TV uses), as
    /// full-width cards with the score. Live games lead; later games are by day.
    struct SportsView: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        @Environment(\.horizontalSizeClass) private var width
        @State private var board: [String: Game] = [:]
        @State private var asking: Airing?
        @State private var askingEvery: Airing?
        @State private var note = ""

        var body: some View {
            let games = SportsBoard.games(store.sports(hours: 7 * 24))
            let live = games.filter { $0.airing.isOn(at: store.now) }
            let later = games.filter { !$0.airing.isOn(at: store.now) }
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 28) {
                    if !note.isEmpty {
                        Text(note)
                            .font(.callout)
                            .foregroundStyle(.secondary)
                            .padding(.horizontal)
                            .accessibilityIdentifier("sports-note")
                    }
                    if games.isEmpty {
                        empty
                    }
                    if !live.isEmpty {
                        section(String(localized: "Live now"), live)
                    }
                    days(later)
                }
                .padding(.vertical, 12)
            }
            .scrollEdgeEffectStyle(.hard, for: .top)
            .background(Tokens.ColorToken.canvas.ignoresSafeArea())
            .navigationTitle("Sports")
            .toolbarTitleDisplayMode(.inlineLarge)
            .toolbar {
                if live.count >= 2 {
                    Button("Watch together", systemImage: "rectangle.split.2x1") {
                        nowPlaying.watchTogether(live.prefix(4).map(\.channel))
                    }
                    .accessibilityIdentifier("sports-together")
                }
            }
            .task { await follow() }
            .accessibilityIdentifier("sports-page")
            .confirmationDialog(
                asking.map(GameWords.title) ?? "",
                isPresented: Binding(get: { asking != nil }, set: {
                    if !$0 {
                        asking = nil
                    }
                }),
                titleVisibility: .visible,
                presenting: asking
            ) { airing in
                Button("Record this game") { Task { await record(airing, every: false) } }
                    .accessibilityIdentifier("sports-record-once")
                Button("Record every airing") { Task { await record(airing, every: true) } }
                Button("Cancel", role: .cancel) {}
            } message: { airing in
                Text(AiringWhen.long(airing.start))
            }
            .confirmationDialog(
                "Record every airing of \(askingEvery?.title ?? "")?",
                isPresented: Binding(get: { askingEvery != nil }, set: {
                    if !$0 {
                        askingEvery = nil
                    }
                }),
                titleVisibility: .visible,
                presenting: askingEvery
            ) { airing in
                Button("Record every airing") { Task { await record(airing, every: true) } }
                Button("Cancel", role: .cancel) {}
            } message: { _ in
                Text("New airings record as they come up. You can stop it in Recordings › Upcoming.")
            }
        }

        private var empty: some View {
            ContentUnavailableView {
                Label("No games in the guide", systemImage: "sportscourt")
            } description: {
                Text("Games on your channels show up here as soon as the guide lists them.")
            }
            .padding(.top, 60)
        }

        @ViewBuilder
        private func days(_ later: [SportsBoard.Entry]) -> some View {
            let byDay = Dictionary(grouping: later) { Calendar.current.startOfDay(for: $0.airing.start) }
            ForEach(byDay.keys.sorted(), id: \.self) { day in
                section(dayTitle(day), byDay[day] ?? [])
            }
        }

        private func section(_ title: String, _ list: [SportsBoard.Entry]) -> some View {
            VStack(alignment: .leading, spacing: 12) {
                Text(title)
                    .font(.title2.weight(.bold))
                    .padding(.horizontal)
                    .accessibilityAddTraits(.isHeader)
                LazyVGrid(columns: columns, spacing: 16) {
                    ForEach(list) { entry in
                        card(entry)
                    }
                }
                .padding(.horizontal)
            }
        }

        /// One full-width card on iPhone; two or three across on iPad.
        private var columns: [GridItem] {
            if width == .regular {
                return [GridItem(.adaptive(minimum: 320), spacing: 16)]
            }
            return [GridItem(.flexible())]
        }

        private func card(_ entry: SportsBoard.Entry) -> some View {
            let airing = entry.airing
            let on = airing.isOn(at: store.now)
            let game = airing.gameId.flatMap { board[$0] }
            return Button {
                if on {
                    nowPlaying.play(entry.channel)
                } else {
                    asking = airing
                }
            } label: {
                SportsCard(entry: entry, game: game, live: on, line: line(entry))
                    .playerZoomSource(entry.channel.id)
            }
            .buttonStyle(.plain)
            .accessibilityLabel(spoken(entry, game: game, on: on))
            .accessibilityHint(on ? "Plays the game" : "Records it")
            .accessibilityIdentifier(on ? "sports-live" : "sports-later")
            .contextMenu {
                if on {
                    Button("Watch", systemImage: "play.fill") { nowPlaying.play(entry.channel) }
                    Button(store.activeRecording(on: entry.channel) == nil ? "Record" : "Stop recording", systemImage: "record.circle") {
                        Task { await store.toggleRecord(entry.channel) }
                    }
                } else {
                    Button("Record this game", systemImage: "record.circle") { asking = airing }
                }
                Button("Record every airing", systemImage: "repeat") { askingEvery = airing }
            }
        }

        /// The scoreboard, again every 30 seconds while a listed game is on. The server caches it.
        private func follow() async {
            while !Task.isCancelled {
                if let api = store.api, let games = try? await api.scoreboardGames() {
                    var map: [String: Game] = [:]
                    for game in games {
                        map[game.id] = game
                    }
                    withAnimation(.snappy) { board = map }
                }
                let anyLive = store.sports(hours: 7 * 24).contains { $0.1.isOn(at: store.now) }
                try? await Task.sleep(for: .seconds(anyLive ? 30 : 300))
            }
        }

        private func record(_ airing: Airing, every: Bool) async {
            do {
                if every {
                    try await store.recordSeries(airing)
                    note = String(localized: "Recording every \(airing.title).")
                } else {
                    try await store.recordOnce(airing)
                    note = String(localized: "\(GameWords.title(airing)) will record.")
                }
            } catch {
                note = PlaybackOutage.actionMessage(error)
            }
        }

        // MARK: Words

        private func dayTitle(_ day: Date) -> String {
            let cal = Calendar.current
            if cal.isDateInToday(day) {
                return String(localized: "Later today")
            }
            if cal.isDateInTomorrow(day) {
                return String(localized: "Tomorrow")
            }
            return day.formatted(.dateTime.weekday(.wide).month().day())
        }

        private func line(_ entry: SportsBoard.Entry) -> String {
            let channel = entry.channel
            var parts = ["\(channel.displayNumber) \(channel.displayName)"]
            if let more = entry.also.first {
                parts.append(String(localized: "also \(more.displayNumber)"))
            }
            if entry.airing.isOn(at: store.now) {
                parts.append(entry.airing.minutesLeft(at: store.now))
            }
            return parts.filter { !$0.isEmpty }.joined(separator: " · ")
        }

        private func spoken(_ entry: SportsBoard.Entry, game: Game?, on: Bool) -> String {
            var parts = [GameWords.title(entry.airing)]
            if on {
                parts.append(String(localized: "live"))
            } else {
                parts.append(AiringWhen.long(entry.airing.start))
            }
            if let teams = game?.teams, teams.count == 2, on || game?.state == "post" {
                parts.append(teams.map { "\($0.short ?? $0.name) \($0.score ?? "")" }.joined(separator: ", "))
            }
            parts.append(line(entry))
            return parts.joined(separator: ", ")
        }
    }

    /// A game as a wide card: the picture with Live or the start time and the league,
    /// the teams and score along its foot, and the matchup and channel under it.
    private struct SportsCard: View {
        @Environment(AppStore.self) private var store
        let entry: SportsBoard.Entry
        let game: Game?
        let live: Bool
        let line: String

        var body: some View {
            let airing = entry.airing
            VStack(alignment: .leading, spacing: 0) {
                picture(airing)
                    .frame(maxWidth: .infinity)
                    .aspectRatio(16 / 9, contentMode: .fit)
                    .overlay(alignment: .topLeading) { badges(airing) }
                    .overlay(alignment: .bottom) {
                        if let game, let teams = game.teams, teams.count == 2 {
                            ScoreBar(game: game, teams: teams, live: live)
                        } else if live {
                            AiringProgress(airing.progress(at: store.now), color: Tokens.ColorToken.tally)
                                .padding(.horizontal, 14)
                                .padding(.bottom, 12)
                        }
                    }
                    .clipped()
                VStack(alignment: .leading, spacing: 4) {
                    Text(GameWords.title(airing))
                        .font(.headline.weight(.heavy))
                        .lineLimit(2)
                    Text(line)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                .padding(14)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .background(Tokens.ColorToken.surface1)
            .clipShape(.rect(cornerRadius: Tokens.Radius.lg))
            .contentShape(.rect(cornerRadius: Tokens.Radius.lg))
        }

        @ViewBuilder
        private func picture(_ airing: Airing) -> some View {
            if let url = store.artURL(airing, width: 960) {
                Color.clear.overlay {
                    ProgramPicture(url: url, width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0)
                }
            } else {
                ZStack {
                    LinearGradient(colors: [Tokens.Category.sports.opacity(0.45), Tokens.ColorToken.surface2], startPoint: .topLeading, endPoint: .bottomTrailing)
                    Text(entry.channel.displayNumber)
                        .font(.system(size: 72, weight: .black))
                        .monospacedDigit()
                        .foregroundStyle(.white.opacity(0.12))
                }
            }
        }

        private func badges(_ airing: Airing) -> some View {
            HStack(spacing: 6) {
                if live {
                    badge(String(localized: "LIVE"), fill: Tokens.ColorToken.tally)
                } else {
                    badge(AiringWhen.short(airing.start), fill: .black.opacity(0.65))
                }
                if let league = GameWords.league(airing, game: game) {
                    badge(league, fill: .white.opacity(0.9), text: .black)
                }
            }
            .padding(10)
        }

        private func badge(_ title: String, fill: Color, text: Color = .white) -> some View {
            Text(title)
                .font(.caption.weight(.heavy))
                .foregroundStyle(text)
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
                .background(fill, in: .capsule)
        }
    }
#endif
