import BroadwaveKit
import SwiftUI

public extension BroadwaveKit.Category {
    var color: Color {
        switch self {
        case .sports: Tokens.Category.sports
        case .news: Tokens.Category.news
        case .movies: Tokens.Category.movies
        case .kids: Tokens.Category.kids
        case .series: Tokens.Category.series
        case .other: Tokens.Category.other
        }
    }
}

/// The channel number is the station's identity, set like a broadcast chyron.
public struct ChannelBadge: View {
    let channel: Channel
    var large = false

    public init(_ channel: Channel, large: Bool = false) {
        self.channel = channel
        self.large = large
    }

    public var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text(channel.displayNumber)
                .font(large ? .title3.weight(.heavy) : .subheadline.weight(.heavy))
                .monospacedDigit()
                .padding(.horizontal, 7)
                .padding(.vertical, 3)
                .background(.white.opacity(0.12), in: .rect(cornerRadius: Tokens.Radius.xs))
            Text(channel.displayName)
                .font(large ? .headline : .subheadline.weight(.semibold))
                .foregroundStyle(.secondary)
                .lineLimit(1)
            if channel.isATSC3 {
                ATSC3Tag()
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("Channel \(channel.displayNumber), \(channel.displayName)\(channel.isATSC3 ? ", ATSC 3.0" : "")")
    }
}

/// Marks an ATSC 3.0 broadcast next to its number.
public struct ATSC3Tag: View {
    public init() {}

    public var body: some View {
        Text(String(localized: "atsc.short", defaultValue: "3.0"))
        #if os(tvOS)
            // Smaller sizes do not read across a room.
            .font(.body.weight(.bold))
        #else
            .font(.caption.weight(.bold))
        #endif
            .foregroundStyle(.secondary)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(.white.opacity(0.12), in: .rect(cornerRadius: Tokens.Radius.xs))
            // No identifier: a card that combines its children took this one as its own.
            .accessibilityLabel("ATSC 3.0")
    }
}

/// The tally light: red is reserved for live and recording.
public struct LiveDot: View {
    var label: String
    @State private var pulse = false

    public init(_ label: String = "Live") {
        self.label = label
    }

    public var body: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(Tokens.ColorToken.tally)
                .frame(width: 8, height: 8)
                .shadow(color: Tokens.ColorToken.tally.opacity(pulse ? 0.9 : 0.2), radius: pulse ? 6 : 2)
            Text(label.uppercased())
                .font(.caption.weight(.bold))
                .tracking(1)
                .foregroundStyle(Tokens.ColorToken.tally)
        }
        .onAppear {
            withAnimation(.easeInOut(duration: 1).repeatForever(autoreverses: true)) { pulse = true }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spoken)
    }

    /// VoiceOver reads the words, not the tally dot. An empty label is still "Live".
    private var spoken: String {
        let trimmed = label.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? "Live" : trimmed
    }
}

public struct AiringProgress: View {
    let value: Double
    let color: Color

    public init(_ value: Double, color: Color = .white) {
        self.value = value
        self.color = color
    }

    public var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .leading) {
                Capsule().fill(.white.opacity(0.15))
                Capsule().fill(color).frame(width: geo.size.width * value)
            }
        }
        .frame(height: 4)
        .accessibilityLabel("Progress")
        .accessibilityValue("\(Int((value * 100).rounded())) percent aired")
    }
}

/// A live channel card: what's on, how far along, tinted by category.
public struct NowCard: View {
    let channel: Channel
    let airing: Airing?
    let now: Date
    let art: URL?
    /// A preview from a mux already on. Shown only when it loads, and only when the listing has no art.
    let frame: URL?

    public init(channel: Channel, airing: Airing?, now: Date, art: URL? = nil, frame: URL? = nil) {
        self.channel = channel
        self.airing = airing
        self.now = now
        self.art = art
        self.frame = frame
    }

    public var body: some View {
        let kind = airing?.kind ?? .other
        VStack(alignment: .leading, spacing: 10) {
            if let art {
                AsyncImage(url: art) { image in
                    image.resizable().aspectRatio(16 / 9, contentMode: .fill)
                } placeholder: {
                    Rectangle().fill(Tokens.ColorToken.surface2)
                }
                .aspectRatio(16 / 9, contentMode: .fit)
                .clipShape(.rect(cornerRadius: Tokens.Radius.md))
                .accessibilityHidden(true)
            } else if let frame {
                AsyncImage(url: frame) { phase in
                    if let image = phase.image {
                        image.resizable().aspectRatio(16 / 9, contentMode: .fill)
                            .aspectRatio(16 / 9, contentMode: .fit)
                            .clipShape(.rect(cornerRadius: Tokens.Radius.md))
                    }
                }
                .accessibilityHidden(true)
            }
            VStack(alignment: .leading, spacing: 2) {
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Text(channel.displayNumber).font(.title2.weight(.heavy)).monospacedDigit()
                    if channel.isATSC3 {
                        ATSC3Tag()
                    }
                }
                Text(channel.displayName).font(.footnote.weight(.semibold)).foregroundStyle(.secondary).lineLimit(1)
            }
            Text(airing?.title ?? String(localized: "No listing"))
                .font(.headline)
                .lineLimit(2, reservesSpace: true)
            Spacer(minLength: 0)
            if let airing {
                HStack(spacing: 10) {
                    AiringProgress(airing.progress(at: now), color: kind.color)
                    Text(airing.minutesLeft(at: now)).font(.caption).foregroundStyle(.secondary).fixedSize()
                }
            }
        }
        .padding(Tokens.Space.s4)
        .frame(maxWidth: .infinity, minHeight: 150, alignment: .topLeading)
        .background {
            RoundedRectangle(cornerRadius: Tokens.Radius.lg)
                .fill(
                    RadialGradient(colors: [kind.color.opacity(0.35), .clear], center: .topLeading, startRadius: 0, endRadius: 260)
                )
                .background(Tokens.ColorToken.surface1, in: .rect(cornerRadius: Tokens.Radius.lg))
        }
        .overlay(RoundedRectangle(cornerRadius: Tokens.Radius.lg).stroke(Tokens.ColorToken.line))
        .accessibilityElement(children: .combine)
        .accessibilityLabel(Self.spoken(channel, airing))
    }

    private static func spoken(_ channel: Channel, _ airing: Airing?) -> String {
        let station = channel.isATSC3
            ? "\(channel.displayNumber) \(channel.displayName), ATSC 3.0"
            : "\(channel.displayNumber) \(channel.displayName)"
        let title = airing?.title ?? String(localized: "No listing")
        if title == channel.displayName {
            return station
        }
        return "\(station), \(title)"
    }
}

/// One row of the "On now" list: channel, show, progress, and what's next.
public struct OnNowRow: View {
    let channel: Channel
    let airing: Airing?
    let next: Airing?
    let now: Date
    let recording: Bool
    /// The row shows a later time: the program's hours instead of how far along it is.
    let later: Bool

    public init(channel: Channel, airing: Airing?, next: Airing?, now: Date, recording: Bool, later: Bool = false) {
        self.channel = channel
        self.airing = airing
        self.next = next
        self.now = now
        self.recording = recording
        self.later = later
    }

    public var body: some View {
        let kind = airing?.kind ?? .other
        HStack(spacing: 14) {
            RoundedRectangle(cornerRadius: 2).fill(kind.color).frame(width: 3)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(channel.displayNumber).font(.headline.weight(.heavy)).monospacedDigit()
                    if channel.isATSC3 {
                        ATSC3Tag()
                    }
                }
                Text(channel.displayName).font(.caption.weight(.semibold)).foregroundStyle(.secondary).lineLimit(1)
            }
            .frame(minWidth: 84, alignment: .leading)
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    if recording {
                        Image(systemName: "record.circle.fill")
                            .font(.caption)
                            .foregroundStyle(Tokens.ColorToken.tally)
                            .symbolEffect(.pulse, options: .repeat(.continuous))
                            .accessibilityLabel("Recording")
                    }
                    Text(airing?.title ?? String(localized: "No listing")).font(.body.weight(.semibold)).lineLimit(1)
                }
                if later, let airing {
                    Text("\(airing.start.formatted(date: .omitted, time: .shortened)) – \(airing.end.formatted(date: .omitted, time: .shortened))")
                        .font(.caption.weight(.semibold))
                        .monospacedDigit()
                        .foregroundStyle(kind.color)
                } else {
                    AiringProgress(airing?.progress(at: now) ?? 0, color: kind.color)
                }
                if let next {
                    // Secondary, not tertiary: the lighter gray failed contrast on the dark rows.
                    Text("Next \(next.start.formatted(date: .omitted, time: .shortened)) · \(next.title)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
        }
        .padding(.vertical, 8)
        .accessibilityElement(children: .combine)
    }
}
