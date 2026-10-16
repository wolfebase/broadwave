import BroadwaveKit
import SwiftUI

/// Words for a game on the Sports pages of every device.
enum GameWords {
    /// "Memphis at Tulane" when the guide has the matchup, else the listing's title.
    static func title(_ airing: Airing) -> String {
        if let pair = airing.matchup {
            return "\(pair.0) at \(pair.1)"
        }
        return airing.title
    }

    static func league(_ airing: Airing, game: Game?) -> String? {
        if let game {
            return leagues[game.league] ?? game.league.uppercased()
        }
        // A badge has room for a word or two.
        return SportsBoard.league(airing).map { shortLeagues[$0] ?? $0 }
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
}

/// Two teams in their colors and the score, under a live or finished game's picture.
/// The numbers roll when they change.
struct ScoreBar: View {
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
