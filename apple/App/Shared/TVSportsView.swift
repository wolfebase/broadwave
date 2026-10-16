#if os(tvOS)
    import BroadwaveKit
    import BroadwaveUI
    import SwiftUI

    /// Sports on Apple TV: games only, each once, as shelves of cards. Live games lead,
    /// with the score from the server's scoreboard; later games are by day.
    struct TVSportsView: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        @State private var board: [String: Game] = [:]
        @State private var asking: Airing?
        @State private var note = ""
        @FocusState private var focus: Int64?
        @FocusState private var emptyFocus: Bool

        static let cardWidth: CGFloat = 400

        var body: some View {
            let games = SportsBoard.games(store.sports(hours: 7 * 24))
            let live = games.filter { $0.airing.isOn(at: store.now) }
            let later = games.filter { !$0.airing.isOn(at: store.now) }
            let days = Dictionary(grouping: later) { Calendar.current.startOfDay(for: $0.airing.start) }
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 64) {
                    TVPageTitle(String(localized: "Sports"), subtitle: subtitle(live: live.count, all: games.count))
                    if !note.isEmpty {
                        Text(note)
                            .font(.callout)
                            .foregroundStyle(.secondary)
                            .accessibilityIdentifier("sports-note")
                    }
                    if games.isEmpty {
                        TVEmptyState(
                            symbol: "sportscourt",
                            title: String(localized: "No games in the guide"),
                            message: String(localized: "Games on your channels show up here as soon as the guide lists them.")
                        ) {
                            Button("Open the guide", systemImage: "square.grid.3x3.topleft.filled") { tvGo(.guide) }
                                .focused($emptyFocus)
                                .accessibilityIdentifier("sports-open-guide")
                        }
                        .defaultFocus($emptyFocus, true)
                    }
                    if !live.isEmpty {
                        TVShelf(String(localized: "Live now")) {
                            if live.count >= 2 {
                                together(live)
                            }
                            ForEach(live) { card($0) }
                        }
                    }
                    ForEach(days.keys.sorted(), id: \.self) { day in
                        TVShelf(dayTitle(day)) {
                            ForEach(days[day] ?? []) { card($0) }
                        }
                    }
                }
                .padding(.bottom, 80)
            }
            .scrollClipDisabled()
            .tvPageChrome()
            .defaultFocus($focus, games.first?.airing.id)
            // The guide can load after the page opens; the first game then takes focus.
            // Set too early, while the push is still running, focus is dropped.
            .task(id: games.first?.airing.id) {
                try? await Task.sleep(for: .milliseconds(350))
                if let first = games.first?.airing.id {
                    if focus == nil {
                        focus = first
                    }
                } else {
                    emptyFocus = true
                }
            }
            .accessibilityIdentifier("sports-page")
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
                Button("Record this game") { Task { await record(airing, every: false) } }
                Button("Record every airing") { Task { await record(airing, every: true) } }
                Button("Cancel", role: .cancel) {}
            } message: { airing in
                Text(dayTitle(airing.start) + ", " + airing.start.formatted(date: .omitted, time: .shortened))
            }
            .task { await follow() }
        }

        @Environment(\.tvGo) private var tvGo

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

        private func together(_ live: [SportsBoard.Entry]) -> some View {
            let count = min(4, live.count)
            return Button {
                nowPlaying.watchTogether(live.prefix(4).map(\.channel))
            } label: {
                VStack(alignment: .leading, spacing: 16) {
                    TVCardArt(width: 220) {
                        Image(systemName: "rectangle.split.2x2.fill")
                            .font(.system(size: 56))
                            .foregroundStyle(.white.opacity(0.85))
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                            .background(Tokens.ColorToken.surface2)
                    }
                    TVCardCaption(title: String(localized: "Watch together"), line: String(localized: "\(String(count)) games at once"), width: 220)
                }
            }
            .buttonStyle(TVCardStyle())
            .accessibilityIdentifier("sports-together")
        }

        private func record(_ airing: Airing, every: Bool) async {
            do {
                if every {
                    try await store.recordSeries(airing)
                    note = String(localized: "Recording every \(airing.title).")
                } else {
                    try await store.recordOnce(airing)
                    note = String(localized: "\(title(airing)) will record.")
                }
            } catch {
                note = PlaybackOutage.actionMessage(error)
            }
        }

        private func card(_ entry: SportsBoard.Entry) -> some View {
            let airing = entry.airing
            let on = airing.isOn(at: store.now)
            let game = airing.gameId.flatMap { board[$0] }
            let width = Self.cardWidth
            return Button {
                if on {
                    nowPlaying.play(entry.channel)
                } else {
                    asking = airing
                }
            } label: {
                VStack(alignment: .leading, spacing: 16) {
                    TVCardArt(width: width) {
                        TVProgramArt(channel: entry.channel, airing: airing, width: 640)
                            .overlay(alignment: .topLeading) {
                                HStack(spacing: 8) {
                                    if on {
                                        TVBadge(String(localized: "Live"), tone: .live)
                                    } else {
                                        TVBadge(airing.start.formatted(date: .omitted, time: .shortened))
                                    }
                                    if let league = leagueName(airing, game: game) {
                                        TVBadge(league, tone: .bright)
                                    }
                                }
                                .padding(14)
                            }
                            .overlay(alignment: .bottom) {
                                if let game, let teams = game.teams, teams.count == 2 {
                                    TVScoreBar(game: game, teams: teams, live: on)
                                }
                            }
                    }
                    TVCardCaption(title: title(airing), line: line(entry), width: width)
                }
            }
            .buttonStyle(TVCardStyle())
            .focused($focus, equals: airing.id)
            .contextMenu { ChannelActions(channel: entry.channel, airing: airing) }
            .accessibilityLabel(spoken(entry, game: game, on: on))
            .accessibilityIdentifier(on ? "sports-live" : "sports-later")
        }

        // MARK: Words

        private func subtitle(live: Int, all: Int) -> String? {
            guard all > 0 else { return nil }
            if live > 0 {
                return live == 1 ? String(localized: "1 game on now") : String(localized: "\(String(live)) games on now")
            }
            return all == 1 ? String(localized: "1 game this week") : String(localized: "\(String(all)) games this week")
        }

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

        /// "Memphis at Tulane" when the guide has the matchup, else the listing's title.
        private func title(_ airing: Airing) -> String {
            if let pair = airing.matchup {
                return "\(pair.0) at \(pair.1)"
            }
            return airing.title
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

        private func leagueName(_ airing: Airing, game: Game?) -> String? {
            if let game {
                return Self.leagues[game.league] ?? game.league.uppercased()
            }
            // A badge has room for a word or two.
            return SportsBoard.league(airing).map { Self.shortLeagues[$0] ?? $0 }
        }

        private static let leagues = [
            "nfl": "NFL", "ncaaf": "NCAAF", "nba": "NBA", "wnba": "WNBA",
            "ncaamb": "NCAAB", "mlb": "MLB", "nhl": "NHL", "mls": "MLS",
        ]

        private static let shortLeagues = [
            "College Football": "NCAAF", "College Basketball": "NCAAB", "High School Football": "High School",
            "Fútbol Premier League": "Premier League", "NFL Football": "NFL", "MLB Baseball": "MLB",
            "NBA Basketball": "NBA", "NHL Hockey": "NHL",
        ]

        private func spoken(_ entry: SportsBoard.Entry, game: Game?, on: Bool) -> String {
            var parts = [title(entry.airing)]
            if on {
                parts.append(String(localized: "live"))
            }
            if let teams = game?.teams, teams.count == 2, on || game?.state == "post" {
                parts.append(teams.map { "\($0.short ?? $0.name) \($0.score ?? "")" }.joined(separator: ", "))
            }
            parts.append(line(entry))
            return parts.joined(separator: ", ")
        }
    }

    /// Two teams in their colors and the score, under a live or finished game's picture.
    /// The numbers roll when they change.
    struct TVScoreBar: View {
        let game: Game
        let teams: [SportsTeam]
        let live: Bool

        var body: some View {
            let ordered = teams.sorted { ($0.home ?? false) == false && ($1.home ?? false) }
            let showScore = game.state != "pre"
            HStack(spacing: 10) {
                ForEach(ordered, id: \.name) { team in
                    HStack(spacing: 8) {
                        Text(team.abbr ?? team.short ?? team.name)
                            .font(.caption.weight(.heavy))
                            .padding(.horizontal, 8)
                            .padding(.vertical, 3)
                            .background(Color(hex: team.color) ?? .gray, in: .rect(cornerRadius: 6))
                        if showScore {
                            Text(team.score ?? "0")
                                .font(.callout.weight(.heavy))
                                .monospacedDigit()
                                .contentTransition(.numericText())
                        }
                    }
                }
                Spacer(minLength: 8)
                let status = SportsBoard.status(game)
                if !status.isEmpty, showScore {
                    Text(status)
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.white.opacity(0.85))
                        .lineLimit(1)
                }
            }
            .foregroundStyle(.white)
            .padding(.horizontal, 14)
            .padding(.vertical, 10)
            .background(.black.opacity(0.72))
            .accessibilityHidden(true)
        }
    }

    extension Color {
        /// "#006747" from the scoreboard. Names and colors only: no remote logos (ADR 0011).
        init?(hex: String?) {
            guard var text = hex?.trimmingCharacters(in: .whitespaces), !text.isEmpty else { return nil }
            if text.hasPrefix("#") {
                text.removeFirst()
            }
            guard text.count == 6, let value = UInt32(text, radix: 16) else { return nil }
            self.init(
                red: Double((value >> 16) & 0xFF) / 255,
                green: Double((value >> 8) & 0xFF) / 255,
                blue: Double(value & 0xFF) / 255
            )
        }
    }
#endif
