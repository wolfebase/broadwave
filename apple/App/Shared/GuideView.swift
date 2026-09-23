import OTAKit
import OTAUI
import SwiftUI

struct GuideView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.horizontalSizeClass) private var sizeClass
    @State private var filter: OTAKit.Category?
    @State private var favoritesOnly = false
    @State private var selected: Selection?

    struct Selection: Identifiable {
        let channel: Channel
        let airing: Airing?
        var id: String { "\(channel.id)-\(airing?.id ?? 0)" }
    }

    private var rows: [Channel] {
        store.channels.filter { c in
            if favoritesOnly && !c.favorite { return false }
            guard let filter else { return true }
            let soon = store.now.addingTimeInterval(4 * 3600)
            return store.index.airings(c.id).contains { $0.end > store.now && $0.start < soon && $0.kind == filter }
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            filters
            #if os(tvOS)
            GuideGrid(channels: rows, highlight: filter) { selected = Selection(channel: $0, airing: $1) }
            #else
            if sizeClass == .compact {
                onNowList
            } else {
                GuideGrid(channels: rows, highlight: filter) { selected = Selection(channel: $0, airing: $1) }
            }
            #endif
        }
        .navigationTitle("Guide")
        #if os(iOS)
        .toolbarTitleDisplayMode(.inline)
        #endif
        .sheet(item: $selected) { sel in
            ProgramSheet(channel: sel.channel, airing: sel.airing)
                .presentationDetents([.medium, .large])
        }
    }

    private var filters: some View {
        ScrollView(.horizontal) {
            HStack(spacing: 8) {
                chip("All", on: filter == nil && !favoritesOnly) { filter = nil; favoritesOnly = false }
                chip("Favorites", on: favoritesOnly) { favoritesOnly.toggle() }
                ForEach([OTAKit.Category.sports, .news, .movies, .kids], id: \.self) { c in
                    chip(c.label, color: c.color, on: filter == c) { filter = filter == c ? nil : c }
                }
            }
            .padding(.horizontal)
            .padding(.vertical, 10)
        }
        .scrollIndicators(.hidden)
    }

    private func chip(_ title: String, color: Color? = nil, on: Bool, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack(spacing: 6) {
                if let color { Circle().fill(color).frame(width: 8, height: 8) }
                Text(title).font(.subheadline.weight(.semibold))
            }
            .padding(.horizontal, 6)
        }
        .buttonStyle(.glass)
        .tint(on ? .white : nil)
        .foregroundStyle(on ? .primary : .secondary)
        .accessibilityAddTraits(on ? .isSelected : [])
    }

    #if os(iOS)
    private var onNowList: some View {
        List(rows) { channel in
            let airing = store.index.on(channel.id, at: store.now)
            Button {
                selected = Selection(channel: channel, airing: airing)
            } label: {
                OnNowRow(
                    channel: channel,
                    airing: airing,
                    next: store.index.next(channel.id, after: airing?.end ?? store.now),
                    now: store.now,
                    recording: store.activeRecording(on: channel) != nil
                )
            }
            .buttonStyle(.plain)
            .listRowBackground(Tokens.ColorToken.surface1)
            .contextMenu { ChannelActions(channel: channel, airing: airing) }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
    }
    #endif
}

/// The time grid. The channel column and time header stay pinned while the grid
/// scrolls both ways, and the tally-red line marks now.
struct GuideGrid: View {
    @Environment(AppStore.self) private var store
    let channels: [Channel]
    let highlight: OTAKit.Category?
    let onSelect: (Channel, Airing?) -> Void
    @State private var offset: CGPoint = .zero

    #if os(tvOS)
    private let rowH: CGFloat = 110
    private let perMinute: CGFloat = 10
    private let channelW: CGFloat = 300
    #else
    private let rowH: CGFloat = 72
    private let perMinute: CGFloat = 6.4
    private let channelW: CGFloat = 170
    #endif
    private let headH: CGFloat = 44
    private let hours = 24.0

    private var origin: Date {
        let cal = Calendar.current
        let now = store.now
        let minute = cal.component(.minute, from: now)
        let floored = cal.date(bySetting: .minute, value: minute < 30 ? 0 : 30, of: now) ?? now
        let clean = cal.date(bySetting: .second, value: 0, of: floored) ?? floored
        return clean.addingTimeInterval(-30 * 60)
    }

    private func x(_ date: Date) -> CGFloat { CGFloat(date.timeIntervalSince(origin) / 60) * perMinute }

    var body: some View {
        let width = CGFloat(hours * 60) * perMinute
        let end = origin.addingTimeInterval(hours * 3600)
        ScrollView([.horizontal, .vertical]) {
            ZStack(alignment: .topLeading) {
                LazyVStack(alignment: .leading, spacing: 0) {
                    Color.clear.frame(height: headH)
                    ForEach(channels) { channel in
                        row(channel, end: end)
                            .frame(width: width, height: rowH, alignment: .leading)
                            .padding(.leading, channelW)
                    }
                }
                // Now line
                Rectangle()
                    .fill(Tokens.ColorToken.tally)
                    .frame(width: 2, height: CGFloat(channels.count) * rowH)
                    .shadow(color: Tokens.ColorToken.tally.opacity(0.7), radius: 6)
                    .offset(x: channelW + x(store.now) - 1, y: headH)
                    .allowsHitTesting(false)
                // Pinned channel column
                VStack(spacing: 0) {
                    Color.clear.frame(height: headH)
                    ForEach(channels) { channel in
                        Button { onSelect(channel, store.index.on(channel.id, at: store.now)) } label: {
                            HStack(spacing: 10) {
                                Text(channel.displayNumber).font(.title3.weight(.heavy)).monospacedDigit()
                                Text(channel.displayName).font(.caption.weight(.semibold)).foregroundStyle(.secondary).lineLimit(1)
                                Spacer(minLength: 0)
                                if channel.favorite { Image(systemName: "star.fill").font(.caption2).foregroundStyle(Tokens.ColorToken.warning) }
                            }
                            .padding(.horizontal, 14)
                            .frame(width: channelW, height: rowH)
                            .background(Tokens.ColorToken.surface1)
                            .overlay(alignment: .bottom) { Rectangle().fill(Tokens.ColorToken.line).frame(height: 1) }
                        }
                        .buttonStyle(.plain)
                    }
                }
                .offset(x: offset.x)
                .zIndex(2)
                // Pinned time header
                ZStack(alignment: .topLeading) {
                    Rectangle().fill(.ultraThinMaterial).frame(width: width + channelW, height: headH)
                    ForEach(0..<Int(hours * 2), id: \.self) { i in
                        let t = origin.addingTimeInterval(Double(i) * 1800)
                        Text(t.formatted(date: .omitted, time: .shortened))
                            .font(.footnote.weight(.semibold))
                            .monospacedDigit()
                            .foregroundStyle(.secondary)
                            .offset(x: channelW + x(t) + 8, y: 13)
                    }
                    Text(store.now.formatted(date: .omitted, time: .shortened))
                        .font(.caption.weight(.heavy))
                        .monospacedDigit()
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(Tokens.ColorToken.tally, in: .capsule)
                        .shadow(color: Tokens.ColorToken.tally.opacity(0.6), radius: 8)
                        .offset(x: channelW + x(store.now) - 30, y: 10)
                }
                .offset(y: offset.y)
                .zIndex(3)
            }
        }
        .scrollIndicators(.hidden)
        .onScrollGeometryChange(for: CGPoint.self) { $0.contentOffset } action: { _, new in
            offset = CGPoint(x: max(0, new.x), y: max(0, new.y))
        }
        .defaultScrollAnchor(UnitPoint(x: max(0, x(store.now.addingTimeInterval(-900)) / (width + channelW)), y: 0))
        .background(Tokens.ColorToken.surface1)
    }

    private func row(_ channel: Channel, end: Date) -> some View {
        let list = store.index.airings(channel.id).filter { $0.end > origin && $0.start < end }
        return ZStack(alignment: .leading) {
            ForEach(list) { airing in
                let s = max(airing.start, origin)
                let e = min(airing.end, end)
                let w = max(24, x(e) - x(s) - 4)
                Button { onSelect(channel, airing) } label: {
                    GuideCell(airing: airing, now: store.now, dim: highlight != nil && airing.kind != highlight, recording: store.activeRecording(on: channel) != nil && airing.isOn(at: store.now))
                        .frame(width: w, height: rowH - 10)
                }
                .buttonStyle(GuideCellStyle())
                .offset(x: x(s))
            }
            if list.isEmpty {
                Text("No listings").font(.footnote).foregroundStyle(.tertiary).offset(x: offset.x + 12)
            }
        }
        .overlay(alignment: .bottom) { Rectangle().fill(Tokens.ColorToken.line).frame(height: 1) }
    }
}

struct GuideCell: View {
    let airing: Airing
    let now: Date
    let dim: Bool
    let recording: Bool

    var body: some View {
        let kind = airing.kind
        let on = airing.isOn(at: now)
        ZStack(alignment: .leading) {
            RoundedRectangle(cornerRadius: Tokens.Radius.sm).fill(on ? kind.color.opacity(0.16) : Tokens.ColorToken.surface2)
            if on {
                GeometryReader { geo in
                    RoundedRectangle(cornerRadius: Tokens.Radius.sm)
                        .fill(kind.color.opacity(0.18))
                        .frame(width: geo.size.width * airing.progress(at: now))
                }
            }
            RoundedRectangle(cornerRadius: 2).fill(kind.color).frame(width: 3).padding(.vertical, 10)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 5) {
                    if recording { Circle().fill(Tokens.ColorToken.tally).frame(width: 7, height: 7) }
                    Text(airing.title).font(.footnote.weight(.semibold)).lineLimit(1)
                    if airing.new == true {
                        Text("NEW").font(.caption2.weight(.heavy)).foregroundStyle(Tokens.ColorToken.accent)
                    }
                }
                Text(airing.subtitle ?? airing.start.formatted(date: .omitted, time: .shortened))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            .padding(.leading, 12)
            .padding(.trailing, 8)
        }
        .opacity(airing.end <= now ? 0.45 : dim ? 0.3 : 1)
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(airing.title), \(airing.start.formatted(date: .omitted, time: .shortened))")
    }
}

struct GuideCellStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        CellBody(configuration: configuration)
    }

    private struct CellBody: View {
        let configuration: ButtonStyleConfiguration
        @Environment(\.isFocused) private var focused

        var body: some View {
            configuration.label
                .overlay(RoundedRectangle(cornerRadius: Tokens.Radius.sm).stroke(.white, lineWidth: focused ? 3 : 0))
                .scaleEffect(focused ? 1.04 : configuration.isPressed ? 0.98 : 1)
                .shadow(color: .black.opacity(focused ? 0.5 : 0), radius: 14, y: 8)
                .zIndex(focused ? 1 : 0)
                .animation(Tokens.Motion.spring, value: focused)
        }
    }
}

/// Details for one airing, with the actions that matter.
struct ProgramSheet: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @Environment(\.dismiss) private var dismiss
    let channel: Channel
    let airing: Airing?

    var body: some View {
        let kind = airing?.kind ?? .other
        let on = airing?.isOn(at: store.now) ?? true
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                HStack {
                    ChannelBadge(channel, large: true)
                    Spacer()
                    if on && airing != nil { LiveDot() }
                }
                if let airing {
                    Text("\(kind.label) · \(airing.start.formatted(.dateTime.weekday().hour().minute())) – \(airing.end.formatted(date: .omitted, time: .shortened))")
                        .font(.caption.weight(.bold))
                        .foregroundStyle(.secondary)
                }
                Text(airing?.title ?? channel.displayName).font(.largeTitle.weight(.heavy))
                if let sub = airing?.subtitle { Text(sub).font(.title3).foregroundStyle(.secondary) }
                if let airing, on {
                    HStack {
                        AiringProgress(airing.progress(at: store.now), color: kind.color)
                        Text(airing.minutesLeft(at: store.now)).font(.footnote).foregroundStyle(.secondary)
                    }
                }
                if let desc = airing?.description { Text(desc).foregroundStyle(.secondary) }
                VStack(spacing: 10) {
                    if on {
                        Button {
                            dismiss()
                            nowPlaying.play(channel)
                        } label: {
                            Label("Watch", systemImage: "play.fill").frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.glassProminent)
                        .controlSize(.large)
                        Button {
                            Task { await store.toggleRecord(channel) }
                        } label: {
                            Label(store.activeRecording(on: channel) == nil ? "Record" : "Stop recording", systemImage: "record.circle")
                                .frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.glass)
                        .controlSize(.large)
                    }
                    if let airing {
                        Button {
                            Task { await store.recordSeries(airing) }
                        } label: {
                            Label(kind == .sports ? "Record every airing" : "Record series", systemImage: "repeat").frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.glass)
                        .controlSize(.large)
                    }
                }
                .padding(.top, 8)
            }
            .padding(24)
        }
        .background {
            RadialGradient(colors: [kind.color.opacity(0.35), .clear], center: .topLeading, startRadius: 0, endRadius: 400)
                .ignoresSafeArea()
        }
    }
}
