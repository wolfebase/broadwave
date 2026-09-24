import BroadwaveKit
import BroadwaveUI
import SwiftUI

struct GuideView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.verticalSizeClass) private var verticalSize
    @State private var filter: BroadwaveKit.Category?
    @State private var favoritesOnly = false
    @State private var selected: Selection?
    @State private var jump: Date?
    @State private var scores: [String: String] = [:]

    struct Selection: Identifiable {
        let channel: Channel
        let airing: Airing?
        var id: String {
            "\(channel.id)-\(airing?.id ?? 0)"
        }
    }

    private var rows: [Channel] {
        store.channels.filter { c in
            if favoritesOnly, !c.favorite {
                return false
            }
            guard let filter else { return true }
            let soon = store.now.addingTimeInterval(4 * 3600)
            return store.index.airings(c.id).contains { $0.end > store.now && $0.start < soon && $0.kind == filter }
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            filters
            dayJump
            #if os(tvOS)
                GuideGrid(channels: rows, highlight: filter, jump: jump, scores: scores) { selected = Selection(channel: $0, airing: $1) }
            #else
                if sizeClass == .compact, verticalSize != .compact {
                    onNowList
                } else {
                    GuideGrid(channels: rows, highlight: filter, jump: jump, scores: scores) { selected = Selection(channel: $0, airing: $1) }
                }
            #endif
        }
        .navigationTitle("Guide")
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
        #if os(iOS)
        .toolbarTitleDisplayMode(.inline)
        .modifier(GuideDetail(selected: $selected, wide: sizeClass == .regular))
        #else
        .sheet(item: $selected) { sel in
            ProgramSheet(channel: sel.channel, airing: sel.airing)
        }
        #endif
    }

    private var dayJump: some View {
        ScrollView(.horizontal) {
            HStack(spacing: 8) {
                Button("Now") { jump = store.now.addingTimeInterval(-15 * 60) }.buttonStyle(.glass)
                Button("Tonight") { jump = primeTime(on: store.now, after: store.now) }.buttonStyle(.glass)
                ForEach(comingDays, id: \.timeIntervalSince1970) { day in
                    Button(dayLabel(day)) { jump = primeTime(on: day, after: day) }.buttonStyle(.glass)
                }
            }
            .padding(.horizontal)
            .padding(.bottom, 8)
        }
        .scrollIndicators(.hidden)
    }

    private var comingDays: [Date] {
        let cal = Calendar.current
        let start = cal.startOfDay(for: store.now)
        return (1 ... 2).compactMap { cal.date(byAdding: .day, value: $0, to: start) }
    }

    private func dayLabel(_ day: Date) -> String {
        if Calendar.current.isDateInTomorrow(day) {
            return "Tomorrow"
        }
        return day.formatted(.dateTime.weekday(.abbreviated))
    }

    /// 8 pm on that day. If that moment has passed, the next day's 8 pm.
    private func primeTime(on day: Date, after now: Date) -> Date {
        let cal = Calendar.current
        var parts = cal.dateComponents([.year, .month, .day], from: day)
        parts.hour = 20
        let prime = cal.date(from: parts) ?? day
        if prime > now {
            return prime.addingTimeInterval(-15 * 60)
        }
        return primeTime(on: cal.date(byAdding: .day, value: 1, to: day) ?? day, after: now)
    }

    private var filters: some View {
        ScrollView(.horizontal) {
            HStack(spacing: 8) {
                chip("All", on: filter == nil && !favoritesOnly) { filter = nil; favoritesOnly = false }
                chip("Favorites", on: favoritesOnly) { favoritesOnly.toggle() }
                ForEach([BroadwaveKit.Category.sports, .news, .movies, .kids], id: \.self) { c in
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
                if let color {
                    Circle().fill(color).frame(width: 8, height: 8)
                }
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
    @Environment(NowPlaying.self) private var nowPlaying
    let channels: [Channel]
    let highlight: BroadwaveKit.Category?
    let jump: Date?
    var scores: [String: String] = [:]
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

    private var hours: Double {
        var latest = origin.addingTimeInterval(6 * 3600)
        for channel in channels {
            if let last = store.index.airings(channel.id).last {
                latest = max(latest, last.end)
            }
        }
        let span = latest.timeIntervalSince(origin) / 3600
        return min(14 * 24, max(6, span.rounded(.up)))
    }

    private var origin: Date {
        let cal = Calendar.current
        let now = store.now
        let minute = cal.component(.minute, from: now)
        let floored = cal.date(bySetting: .minute, value: minute < 30 ? 0 : 30, of: now) ?? now
        let clean = cal.date(bySetting: .second, value: 0, of: floored) ?? floored
        return clean.addingTimeInterval(-30 * 60)
    }

    private func x(_ date: Date) -> CGFloat {
        CGFloat(date.timeIntervalSince(origin) / 60) * perMinute
    }

    var body: some View {
        let width = CGFloat(hours * 60) * perMinute
        let end = origin.addingTimeInterval(hours * 3600)
        ScrollViewReader { proxy in
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
                    ForEach(0 ..< Int(hours), id: \.self) { hour in
                        Color.clear
                            .frame(width: 1, height: 1)
                            .id(hour)
                            .offset(x: channelW + CGFloat(hour * 60) * perMinute)
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
                                    if let api = store.api, channel.artUrl?.isEmpty == false {
                                        AsyncImage(url: api.artURL(kind: "channel", id: channel.id, width: 72)) { phase in
                                            if let image = phase.image {
                                                image.resizable().scaledToFit()
                                            }
                                        }
                                        .frame(width: 36, height: 22)
                                        .accessibilityHidden(true)
                                    }
                                    Text(channel.displayName).font(.caption.weight(.semibold)).foregroundStyle(.secondary).lineLimit(1)
                                    Spacer(minLength: 0)
                                    if channel.favorite {
                                        Image(systemName: "star.fill").font(.caption2).foregroundStyle(Tokens.ColorToken.warning)
                                    }
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
                        ForEach(0 ..< Int(hours * 2), id: \.self) { i in
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
            .onChange(of: jump) { _, date in
                guard let date else { return }
                let hour = max(0, min(Int(hours) - 1, Int(date.timeIntervalSince(origin) / 3600)))
                proxy.scrollTo(hour, anchor: .leading)
            }
        }
    }

    private func row(_ channel: Channel, end: Date) -> some View {
        let list = store.index.airings(channel.id).filter { $0.end > origin && $0.start < end }
        return ZStack(alignment: .leading) {
            ForEach(list) { airing in
                let s = max(airing.start, origin)
                let e = min(airing.end, end)
                let w = max(24, x(e) - x(s) - 4)
                Button { onSelect(channel, airing) } label: {
                    GuideCell(airing: airing, now: store.now, dim: highlight != nil && airing.kind != highlight, recording: store.activeRecording(on: channel) != nil && airing.isOn(at: store.now), score: airing.gameId.flatMap { scores[$0] })
                        .frame(width: w, height: rowH - 10)
                }
                .buttonStyle(GuideCellStyle())
                #if os(tvOS)
                    .onPlayPauseCommand { nowPlaying.play(channel) }
                #endif
                    .offset(x: x(s))
            }
            if list.isEmpty {
                Text(listingsNote(store.index.airings(channel.id), windowStart: origin)).font(.footnote).foregroundStyle(.tertiary).offset(x: offset.x + 12)
            } else if let last = store.index.airings(channel.id).last, last.end < end.addingTimeInterval(-60) {
                Text(listingsNote(store.index.airings(channel.id), windowStart: end))
                    .font(.footnote)
                    .foregroundStyle(.tertiary)
                    .offset(x: x(last.end) + 12)
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
    var score: String?

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
                    if recording {
                        Circle().fill(Tokens.ColorToken.tally).frame(width: 7, height: 7)
                    }
                    Text(airing.title).font(.footnote.weight(.semibold)).lineLimit(1)
                    if let score, !score.isEmpty {
                        Text(score).font(.caption2.weight(.semibold)).monospacedDigit().lineLimit(1)
                    }
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
private func listingsNote(_ airings: [Airing], windowStart: Date) -> String {
    guard let last = airings.last else { return "No listings" }
    if windowStart < last.end.addingTimeInterval(-60) {
        return "No listings"
    }
    let day = last.end.formatted(.dateTime.weekday(.wide))
    return "Listings through \(day)"
}

private func guideSourceLine(_ source: String?) -> String? {
    switch source {
    case "silicondust": "From the tuner guide."
    case "schedules-direct": "From Schedules Direct."
    case "xmltv": "From your guide file."
    case "playlist": "From the playlist."
    case "broadcast": "From the broadcast."
    default: nil
    }
}

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
                if let airing, let api = store.api, airing.imageUrl?.isEmpty == false {
                    ProgramArt(url: api.artURL(kind: "airing", id: airing.id, width: 640), width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0)
                        .frame(maxWidth: .infinity)
                        .frame(height: 180)
                        .clipShape(RoundedRectangle(cornerRadius: 12))
                        .accessibilityHidden(true)
                }
                HStack {
                    ChannelBadge(channel, large: true)
                    Spacer()
                    if on, airing != nil {
                        LiveDot()
                    }
                }
                if let airing {
                    Text("\(kind.label) · \(airing.start.formatted(.dateTime.weekday().hour().minute())) – \(airing.end.formatted(date: .omitted, time: .shortened))")
                        .font(.caption.weight(.bold))
                        .foregroundStyle(.secondary)
                }
                if let line = guideSourceLine(airing?.guideSource) {
                    Text(line).font(.caption).foregroundStyle(.secondary)
                }
                Text(airing?.title ?? channel.displayName).font(.largeTitle.weight(.heavy))
                if let sub = airing?.subtitle {
                    Text(sub).font(.title3).foregroundStyle(.secondary)
                }
                if let label = airing?.episodeLabel, !label.isEmpty {
                    Text(label).font(.caption.weight(.bold)).foregroundStyle(.secondary)
                }
                if let aired = airing?.originalAir, !aired.isEmpty {
                    Text("First aired \(aired)").font(.caption).foregroundStyle(.secondary)
                }
                if let airing, on {
                    HStack {
                        AiringProgress(airing.progress(at: store.now), color: kind.color)
                        Text(airing.minutesLeft(at: store.now)).font(.footnote).foregroundStyle(.secondary)
                    }
                }
                if let desc = airing?.description {
                    Text(desc).foregroundStyle(.secondary)
                }
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
                            dismiss()
                            if let current = nowPlaying.channel, current.id != channel.id {
                                nowPlaying.watchTogether([current, channel])
                            } else {
                                nowPlaying.watchTogether([channel])
                            }
                        } label: {
                            Label("Watch together", systemImage: "rectangle.split.2x1").frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.glass)
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

/// Crisp poster over a blurred copy, unless the art is wide enough to fill the slot.
private struct ProgramArt: View {
    let url: URL
    let width: Int
    let height: Int

    var body: some View {
        GeometryReader { geo in
            let bleed = ArtLayout.choose(width: width, height: height, slot: Int(geo.size.width)) == "bleed"
            let cap = CGFloat(ArtLayout.displayEdge(native: max(width, 1), slot: Int(geo.size.width)))
            AsyncImage(url: url) { phase in
                if let image = phase.image {
                    ZStack {
                        image.resizable().scaledToFill().blur(radius: bleed ? 0 : 22).opacity(bleed ? 0.85 : 0.45)
                        if !bleed {
                            image.resizable().scaledToFit()
                                .frame(maxWidth: width > 0 ? min(geo.size.width * 0.72, cap) : min(geo.size.width * 0.55, 320),
                                       maxHeight: geo.size.height * 0.82)
                        }
                    }
                }
            }
            .frame(width: geo.size.width, height: geo.size.height)
            .clipped()
        }
    }
}

/// A wide screen keeps the program beside the guide. A phone covers it with a sheet.
private struct GuideDetail: ViewModifier {
    @Binding var selected: GuideView.Selection?
    var wide: Bool

    func body(content: Content) -> some View {
        #if os(iOS)
            if wide {
                content.inspector(isPresented: Binding(get: { selected != nil }, set: {
                    if !$0 {
                        selected = nil
                    }
                })) {
                    if let selected {
                        ProgramSheet(channel: selected.channel, airing: selected.airing)
                            .inspectorColumnWidth(min: 320, ideal: 380, max: 440)
                    }
                }
            } else {
                content.sheet(item: $selected) { sel in
                    ProgramSheet(channel: sel.channel, airing: sel.airing)
                        .presentationDetents([.medium, .large])
                }
            }
        #else
            content
        #endif
    }
}
