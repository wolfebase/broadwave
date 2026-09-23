import OTAKit
import SwiftUI

extension OTAKit.Category {
    public var color: Color {
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
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("Channel \(channel.displayNumber), \(channel.displayName)")
    }
}

/// The tally light: red is reserved for live and recording.
public struct LiveDot: View {
    var label: String
    @State private var pulse = false

    public init(_ label: String = "Live") { self.label = label }

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
        .accessibilityValue("\(Int(value * 100)) percent aired")
    }
}

/// A live channel card: what's on, how far along, tinted by category.
public struct NowCard: View {
    let channel: Channel
    let airing: Airing?
    let now: Date

    public init(channel: Channel, airing: Airing?, now: Date) {
        self.channel = channel
        self.airing = airing
        self.now = now
    }

    public var body: some View {
        let kind = airing?.kind ?? .other
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(channel.displayNumber).font(.title2.weight(.heavy)).monospacedDigit()
                Text(channel.displayName).font(.footnote.weight(.semibold)).foregroundStyle(.secondary).lineLimit(1)
            }
            Text(airing?.title ?? "No listing")
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
    }
}

/// One row of the "On now" list: channel, show, progress, and what's next.
public struct OnNowRow: View {
    let channel: Channel
    let airing: Airing?
    let next: Airing?
    let now: Date
    let recording: Bool

    public init(channel: Channel, airing: Airing?, next: Airing?, now: Date, recording: Bool) {
        self.channel = channel
        self.airing = airing
        self.next = next
        self.now = now
        self.recording = recording
    }

    public var body: some View {
        let kind = airing?.kind ?? .other
        HStack(spacing: 14) {
            RoundedRectangle(cornerRadius: 2).fill(kind.color).frame(width: 3)
            VStack(alignment: .leading, spacing: 2) {
                Text(channel.displayNumber).font(.headline.weight(.heavy)).monospacedDigit()
                Text(channel.displayName).font(.caption.weight(.semibold)).foregroundStyle(.secondary).lineLimit(1)
            }
            .frame(width: 84, alignment: .leading)
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    if recording { Circle().fill(Tokens.ColorToken.tally).frame(width: 8, height: 8) }
                    Text(airing?.title ?? "No listing").font(.body.weight(.semibold)).lineLimit(1)
                }
                AiringProgress(airing?.progress(at: now) ?? 0, color: kind.color)
                if let next {
                    Text("\(next.start.formatted(date: .omitted, time: .shortened))  \(next.title)")
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                        .lineLimit(1)
                }
            }
        }
        .padding(.vertical, 8)
        .accessibilityElement(children: .combine)
    }
}
