import BroadwaveKit
import BroadwaveUI
import SwiftUI

struct SportsView: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @State private var scores: [String: String] = [:]
    #if os(tvOS)
        @Environment(\.tvSelectedTab) private var tvSelectedTab
        @FocusState private var focusedGame: Int64?
        @FocusState private var emptySports: Bool
    #endif

    var body: some View {
        let games = store.sports(hours: 7 * 24)
        let live = games.filter { $0.1.isOn(at: store.now) }
        let later = games.filter { !$0.1.isOn(at: store.now) }
        let days = Dictionary(grouping: later) { Calendar.current.startOfDay(for: $0.1.start) }
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 28) {
                if games.isEmpty {
                    ContentUnavailableView("No games in the guide", systemImage: "sportscourt", description: Text("Sports on your channels show up here as soon as they're listed."))
                        .padding(.top, 60)
                    #if os(tvOS)
                        .focusable()
                        .focused($emptySports)
                    #endif
                }
                if !live.isEmpty {
                    section("Live now", live)
                }
                ForEach(days.keys.sorted(), id: \.self) { day in
                    section(day.formatted(.dateTime.weekday(.wide).month().day()), days[day] ?? [])
                }
            }
            .padding(.vertical)
        }
        .navigationTitle("Sports")
        #if os(tvOS)
            .onAppear { claimSportsFocus() }
            .onChange(of: tvSelectedTab) { _, _ in claimSportsFocus() }
            .onChange(of: games.isEmpty) { _, _ in claimSportsFocus() }
        #endif
            .task {
                guard let api = store.api else { return }
                let games = await (try? api.scoreboard()) ?? []
                var map: [String: String] = [:]
                for game in games {
                    if let line = game.line {
                        map[game.id] = line
                    }
                }
                scores = map
            }
            .toolbar {
                if live.count >= 2 {
                    Button("Watch together") {
                        nowPlaying.watchTogether(live.prefix(4).map(\.0))
                    }
                }
            }
    }

    #if os(tvOS)
        /// Same as Home: focus in the page collapses the sidebar. An empty board has no button, so the message takes focus.
        private func claimSportsFocus() {
            guard tvSelectedTab == .sports else { return }
            if let id = store.sports(hours: 7 * 24).first?.1.id {
                focusedGame = id
            } else {
                emptySports = true
            }
        }
    #endif

    private func section(_ title: String, _ list: [(Channel, Airing)]) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title).font(.title2.weight(.bold)).padding(.horizontal)
            LazyVGrid(columns: [GridItem(.adaptive(minimum: cardWidth), spacing: 14)], spacing: 14) {
                ForEach(list, id: \.1.id) { channel, airing in
                    Button {
                        if airing.isOn(at: store.now) {
                            nowPlaying.play(channel)
                        }
                    } label: {
                        GameCard(channel: channel, airing: airing, now: store.now, score: airing.gameId.flatMap { scores[$0] })
                    }
                    .cardButton()
                    #if os(tvOS)
                        .focused($focusedGame, equals: airing.id)
                    #endif
                        .contextMenu { ChannelActions(channel: channel, airing: airing) }
                }
            }
            .padding(.horizontal)
        }
    }
}
