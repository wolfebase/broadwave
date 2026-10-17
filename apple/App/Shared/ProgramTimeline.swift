#if os(tvOS)
    import BroadwaveKit
    import BroadwaveUI
    import SwiftUI

    /// The program's slot, over the live transport bar: when it started and ends, how much
    /// has aired, where the picture is in it, and the live edge. AVKit's live scrubber
    /// shows only its buffer and a clock, which says nothing about the show.
    struct ProgramTimeline: View {
        /// "4.1 WDAF". The bar's own line beside LIVE is left empty so the Info tab
        /// leads with the program (see `LivePlayer.metadata`), so the channel is named here.
        let channel: String
        /// The listing on the air at a broadcast time.
        let program: (Date) -> Airing?
        /// The broadcast time of the frame on screen. Nil while nothing plays.
        let playhead: () -> Date?

        /// AVKit's bar, measured on tvOS 26 and 27: its title block (the LIVE badge over
        /// the title) starts 757 pt down a 1080 pt screen, with the title at 84 pt.
        /// The row sits one line above it, on the same left edge.
        static let leading: CGFloat = 86
        static let bottom: CGFloat = 1080 - 757 + 20

        var body: some View {
            TimelineView(.periodic(from: .now, by: 1)) { context in
                let at = min(playhead() ?? context.date, context.date)
                if let airing = program(at) {
                    row(airing, playhead: at, live: context.date)
                }
            }
        }

        private func row(_ airing: Airing, playhead: Date, live: Date) -> some View {
            let behind = live.timeIntervalSince(playhead)
            let note = behind > 15 ? Self.behindLine(behind) : airing.minutesLeft(at: live)
            return HStack(spacing: 22) {
                Text(channel)
                    .padding(.trailing, 12)
                Text(airing.start.formatted(date: .omitted, time: .shortened))
                ProgramTrack(watched: airing.progress(at: playhead), aired: airing.progress(at: live))
                    .frame(width: 520, height: 26)
                Text(airing.end.formatted(date: .omitted, time: .shortened))
                Text(note)
                    .foregroundStyle(.white.opacity(0.75))
                    .padding(.leading, 6)
            }
            .font(.callout.weight(.semibold))
            .monospacedDigit()
            .foregroundStyle(.white)
            .lineLimit(1)
            .shadow(color: .black.opacity(0.6), radius: 8)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(spoken(airing, note: note))
            .accessibilityIdentifier("program-timeline")
        }

        private static func behindLine(_ seconds: TimeInterval) -> String {
            let minutes = Int(seconds / 60)
            let rest = Int(seconds) % 60
            return String(localized: "\(minutes):\(String(format: "%02d", rest)) behind live")
        }

        private func spoken(_ airing: Airing, note: String) -> String {
            let start = airing.start.formatted(date: .omitted, time: .shortened)
            let end = airing.end.formatted(date: .omitted, time: .shortened)
            return String(localized: "\(airing.title) on \(channel), \(start) to \(end), \(note)")
        }
    }

    /// Aired so far in a lighter fill, watched in white, and a red mark at the live edge.
    private struct ProgramTrack: View {
        let watched: Double
        let aired: Double

        var body: some View {
            GeometryReader { geo in
                let width = geo.size.width
                ZStack(alignment: .leading) {
                    Capsule().fill(.white.opacity(0.25)).frame(height: 10)
                    Capsule().fill(.white.opacity(0.45)).frame(width: max(10, width * aired), height: 10)
                    Capsule().fill(.white).frame(width: max(10, width * watched), height: 10)
                    Capsule()
                        .fill(Tokens.ColorToken.tally)
                        .frame(width: 5, height: 26)
                        .offset(x: min(width - 5, max(0, width * aired - 2.5)))
                }
                .frame(height: geo.size.height)
            }
        }
    }
#endif
