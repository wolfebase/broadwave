import BroadwaveKit
import BroadwaveUI
import SwiftUI

struct GuideView: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.verticalSizeClass) private var verticalSize
    @State private var filter: BroadwaveKit.Category?
    @State private var favoritesOnly = false
    @State private var selected: Selection?
    @State private var jump: GuideJump?
    @State private var scores: [String: String] = [:]
    #if os(tvOS)
        @Environment(\.tvSelectedTab) private var tvSelectedTab
        @Environment(\.tvGo) private var tvGo
        @FocusState private var headerFocus: GuideHeader?
        @FocusState private var channelFocus: Int64?

        /// The day row and the filters. Arrow keys move through this list; Down from a day
        /// lands on the first channel. Geometric search cannot, because the collapsed sidebar
        /// still occupies the left edge between Now and that channel.
        private enum GuideHeader: Hashable {
            case now
            case tonight
            case later(TimeInterval)
            case chip(String)
            /// The live pane at the end of the day row.
            case pane
        }
    #endif

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
            #if os(tvOS)
                filters
                // The channel the player stepped away from plays on at the end of the day row.
                HStack(alignment: .center, spacing: 0) {
                    dayJump
                    if nowPlaying.docked, let channel = nowPlaying.channel {
                        GuideLivePane(channel: channel)
                            .focused($headerFocus, equals: .pane)
                            .onMoveCommand { moveHeader($0, from: .pane) }
                            .padding(.trailing, 60)
                            .padding(.bottom, 8)
                            .transition(.opacity)
                    }
                }
            #else
                filters
                dayJump
            #endif
            #if os(tvOS)
                // Select plays what is on now, as on any TV guide; a later program opens its
                // details. A long press opens the details of anything.
                GuideGrid(
                    channels: rows, highlight: filter, jump: jump, scores: scores, channelFocus: $channelFocus,
                    onDetails: { selected = Selection(channel: $0, airing: $1) },
                    onSelect: { channel, airing in
                        if let airing, airing.start > store.now {
                            selected = Selection(channel: channel, airing: airing)
                        } else {
                            nowPlaying.play(channel)
                        }
                    }
                )
            #else
                if sizeClass == .compact, verticalSize != .compact {
                    onNowList
                } else {
                    if sizeClass == .regular, verticalSize == .regular {
                        GuidePreview(picked: selected?.channel)
                    }
                    GuideGrid(channels: rows, highlight: filter, jump: jump, scores: scores) { selected = Selection(channel: $0, airing: $1) }
                }
            #endif
        }
        #if os(iOS)
        // A scroll view that touches the top safe area grows under the navigation bar, and
        // the bar's edge effect then hid the filter row (with the mini player up).
        .padding(.top, 1)
        #endif
        // On Apple TV the tab pill already says Guide; a large title only pushes the grid down.
        #if !os(tvOS)
        .navigationTitle("Guide")
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
        #if DEBUG
        // Simulator testing: -BroadwaveProgram <airing id> opens that program's sheet.
        .task {
            let id = Int64(UserDefaults.standard.integer(forKey: "BroadwaveProgram"))
            guard id > 0 else { return }
            for _ in 0 ..< 40 {
                for channel in store.channels {
                    if let airing = store.index.airings(channel.id).first(where: { $0.id == id }) {
                        selected = Selection(channel: channel, airing: airing)
                        return
                    }
                }
                try? await Task.sleep(for: .milliseconds(250))
            }
        }
        #endif
        // The Top Shelf's More Info: the guide on that channel, with what's on now open.
        .task(id: nowPlaying.guideChannel) {
            guard let id = nowPlaying.guideChannel else { return }
            for _ in 0 ..< 40 where store.channels.isEmpty {
                try? await Task.sleep(for: .milliseconds(250))
            }
            nowPlaying.guideChannel = nil
            guard let channel = store.channels.first(where: { $0.id == id }) else { return }
            #if os(tvOS)
                channelFocus = id
            #endif
            if let airing = store.index.on(id, at: store.now) {
                selected = Selection(channel: channel, airing: airing)
            }
        }
        #if os(iOS)
        .toolbarTitleDisplayMode(.inline)
        .modifier(GuideDetail(selected: $selected, wide: sizeClass == .regular))
        #else
        .sheet(item: $selected) { sel in
            ProgramSheet(channel: sel.channel, airing: sel.airing)
        }
        .defaultFocus($channelFocus, startChannel)
        .onAppear { claimGuideFocus() }
        .onChange(of: tvSelectedTab) { _, _ in claimGuideFocus() }
        // Closing the player lands back on the channel that played.
        .onChange(of: nowPlaying.channel == nil || !nowPlaying.expanded) { _, closed in
            if closed {
                claimGuideFocus()
            }
        }
        // Back from the guide goes back to the channel you were watching, like a
        // cable box's Last; with nothing watched yet it goes to the sidebar as usual.
        .onExitCommand(perform: backAction)
        .task(id: tvSelectedTab) {
            // The sidebar takes focus on the same turn as onAppear and clears a focus set then.
            guard tvSelectedTab == .guide else { return }
            try? await Task.sleep(for: .milliseconds(200))
            claimGuideFocus()
        }
        #endif
    }

    #if os(tvOS)
        /// The sidebar stays open over the grid until something in the page has focus, and it covers the
        /// channel column. Focus goes into the grid, on the last channel watched or the first, so a click
        /// plays it and the sidebar menu stays out of the way.
        private func claimGuideFocus() {
            guard tvSelectedTab == .guide else { return }
            if let id = startChannel {
                channelFocus = id
            } else {
                headerFocus = .now
            }
        }

        /// The row the guide opens on: the last channel watched if it's listed, else the first.
        private var startChannel: Int64? {
            let last = nowPlaying.lastChannel?.id ?? NowPlaying.lastChannelID
            return rows.first(where: { $0.id == last })?.id ?? rows.first?.id
        }

        /// Back on the guide. With the player up, a Back that reaches the guide is one the
        /// player never saw: it came up without focus (as it can right after Back resumed
        /// a channel), so the press went to the row under it. That Back leaves the player,
        /// so one press always does. Otherwise Back plays the last channel again.
        private var backAction: (() -> Void)? {
            if nowPlaying.channel != nil, nowPlaying.expanded, nowPlaying.together.isEmpty, nowPlaying.recordingScreens.isEmpty, !nowPlaying.live.pictureInPicture {
                return { nowPlaying.dock() }
            }
            // The channel plays on in the preview: Back brings it back full screen at once.
            if nowPlaying.docked {
                return {
                    guard Date().timeIntervalSince(nowPlaying.stoppedAt ?? .distantPast) > 1.5 else { return }
                    nowPlaying.expanded = true
                }
            }
            guard let last = resumable else { return nil }
            return {
                // A double press to leave the player must not bounce back into the channel.
                guard Date().timeIntervalSince(nowPlaying.stoppedAt ?? .distantPast) > 1.5 else { return }
                // Opened inside the press, the player came up without focus and the next
                // Back went to the guide under it. Open it after the press.
                Task {
                    try? await Task.sleep(for: .milliseconds(200))
                    nowPlaying.play(last)
                }
            }
        }

        /// The channel Back goes to: one that played this session and isn't on screen now.
        private var resumable: Channel? {
            guard nowPlaying.channel == nil, let last = nowPlaying.lastChannel else { return nil }
            return last
        }
    #endif

    private var dayJump: some View {
        guideRow {
            HStack(spacing: 8) {
                Button { go(store.now.addingTimeInterval(-15 * 60)) } label: { chipLabel("Now") }
                    .buttonStyle(.glass)
                    .foregroundStyle(.secondary)
                    .accessibilityLabel("Now")
                    .accessibilityIdentifier("guide-now")
                #if os(tvOS)
                    .focused($headerFocus, equals: .now)
                    .onMoveCommand { moveHeader($0, from: .now) }
                #endif
                Button { go(primeTime(on: store.now, after: store.now)) } label: { chipLabel("Tonight") }
                    .buttonStyle(.glass)
                    .foregroundStyle(.secondary)
                    .accessibilityLabel("Tonight")
                #if os(tvOS)
                    .focused($headerFocus, equals: .tonight)
                    .onMoveCommand { moveHeader($0, from: .tonight) }
                #endif
                ForEach(comingDays, id: \.timeIntervalSince1970) { day in
                    Button { go(primeTime(on: day, after: day)) } label: { chipLabel(dayLabel(day)) }
                        .buttonStyle(.glass)
                        .foregroundStyle(.secondary)
                        .accessibilityLabel(dayLabel(day))
                    #if os(tvOS)
                        .focused($headerFocus, equals: .later(day.timeIntervalSince1970))
                        .onMoveCommand { moveHeader($0, from: .later(day.timeIntervalSince1970)) }
                    #endif
                }
            }
            .padding(.horizontal)
            .padding(.bottom, 8)
        }
    }

    /// Every press scrolls, the same day twice included.
    private func go(_ date: Date) {
        jump = GuideJump(to: date, serial: (jump?.serial ?? 0) + 1)
    }

    /// Tomorrow and the day after, while the listings reach that evening. A day with
    /// nothing listed would scroll to an empty grid.
    private var comingDays: [Date] {
        let cal = Calendar.current
        let start = cal.startOfDay(for: store.now)
        let listed = store.channels.compactMap { store.index.airings($0.id).last?.end }.max() ?? store.now
        return (1 ... 2).compactMap { cal.date(byAdding: .day, value: $0, to: start) }
            .filter { primeTime(on: $0, after: $0) < listed }
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
        guideRow {
            HStack(spacing: 8) {
                chip("All", spot: "all", on: filter == nil && !favoritesOnly) { filter = nil; favoritesOnly = false }
                chip("Favorites", spot: "favorites", on: favoritesOnly) { favoritesOnly.toggle() }
                ForEach([BroadwaveKit.Category.sports, .news, .movies, .kids], id: \.self) { c in
                    chip(c.label, spot: c.rawValue, color: c.color, on: filter == c) { filter = filter == c ? nil : c }
                }
                #if os(tvOS)
                    Spacer(minLength: 24)
                    pageIcons
                #endif
            }
            .padding(.horizontal)
            .padding(.vertical, 10)
        }
    }

    #if os(tvOS)
        /// The other pages. Apple TV has no tab sidebar; Back on a page returns here.
        private var pageIcons: some View {
            HStack(spacing: 8) {
                ForEach(Self.pages, id: \.id) { page in
                    Button { tvGo(page.tab) } label: {
                        Image(systemName: page.icon)
                            .font(.subheadline.weight(.semibold))
                            .padding(.horizontal, 2)
                    }
                    .buttonStyle(.glass)
                    .foregroundStyle(.secondary)
                    .accessibilityLabel(page.title)
                    .accessibilityIdentifier("guide-go-\(page.id)")
                }
            }
        }

        private struct PageLink {
            let id: String
            let title: String
            let icon: String
            let tab: AppTab
        }

        private static let pages = [
            PageLink(id: "search", title: String(localized: "Search"), icon: "magnifyingglass", tab: .search),
            PageLink(id: "sports", title: String(localized: "Sports"), icon: "sportscourt.fill", tab: .sports),
            PageLink(id: "recordings", title: String(localized: "Recordings"), icon: "play.rectangle.on.rectangle.fill", tab: .recordings),
            PageLink(id: "home", title: String(localized: "Home"), icon: "house.fill", tab: .home),
            PageLink(id: "settings", title: String(localized: "Settings"), icon: "gearshape.fill", tab: .settings),
        ]
    #endif

    /// A horizontal scroller on Apple TV keeps Up and Down inside the row. A short row
    /// centered on the screen sits beside the channel column, so Down from Now has nowhere to land.
    @ViewBuilder
    private func guideRow(@ViewBuilder content: () -> some View) -> some View {
        #if os(tvOS)
            // A full-width section: Down from the page icons at the right end lands on
            // the row below even where nothing sits under them.
            content()
                .frame(maxWidth: .infinity, alignment: .leading)
                .focusSection()
        #else
            ScrollView(.horizontal) { content() }
                .scrollIndicators(.hidden)
        #endif
    }

    /// One label for every chip in the header: the filters and the days.
    private func chipLabel(_ title: String) -> some View {
        Text(title)
            .font(.subheadline.weight(.semibold))
            .lineLimit(1)
            .fixedSize()
    }

    private func chip(_ title: String, spot: String, color: Color? = nil, on: Bool, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack(spacing: 6) {
                if let color {
                    Circle().fill(color).frame(width: 8, height: 8)
                }
                chipLabel(title)
            }
            .padding(.horizontal, 6)
        }
        .buttonStyle(.glass)
        .tint(on ? .white : nil)
        .foregroundStyle(on ? .primary : .secondary)
        .accessibilityAddTraits(on ? .isSelected : [])
        .accessibilityIdentifier("guide-filter-\(spot)")
        #if os(tvOS)
            .focused($headerFocus, equals: .chip(spot))
            .onMoveCommand { moveHeader($0, from: .chip(spot)) }
        #endif
    }

    #if os(tvOS)
        private var headerOrder: [GuideHeader] {
            var spots: [GuideHeader] = [.chip("all"), .chip("favorites")]
            spots += [BroadwaveKit.Category.sports, .news, .movies, .kids].map { .chip($0.rawValue) }
            spots += [.now, .tonight]
            spots += comingDays.map { .later($0.timeIntervalSince1970) }
            if nowPlaying.docked {
                spots.append(.pane)
            }
            return spots
        }

        /// Left and right stay on the same row. Down from a filter returns to Now.
        /// Down from a day opens the first channel, which geometric search cannot reach.
        private func moveHeader(_ direction: MoveCommandDirection, from spot: GuideHeader) {
            let spots = headerOrder
            guard let index = spots.firstIndex(of: spot) else { return }
            switch direction {
            case .down:
                if case .chip = spot {
                    headerFocus = .now
                } else if let id = rows.first?.id {
                    channelFocus = id
                }
            case .up:
                if case .chip = spot {
                    return
                }
                headerFocus = .chip("all")
            case .left:
                guard index > 0, headerSameRow(spots[index - 1], spot) else { return }
                headerFocus = spots[index - 1]
            case .right:
                guard index + 1 < spots.count, headerSameRow(spots[index + 1], spot) else { return }
                headerFocus = spots[index + 1]
            @unknown default:
                break
            }
        }

        private func headerSameRow(_ a: GuideHeader, _ b: GuideHeader) -> Bool {
            func isChip(_ spot: GuideHeader) -> Bool {
                if case .chip = spot {
                    return true
                }
                return false
            }
            return isChip(a) == isChip(b)
        }
    #endif

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
                    .playerZoomSource(channel.id)
                }
                .buttonStyle(.plain)
                .listRowBackground(Tokens.ColorToken.surface1)
                .contextMenu { ChannelActions(channel: channel, airing: airing) }
                .modifier(ChannelSwipeActions(channel: channel))
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
        }
    #endif
}

/// The time grid. The channel column and time header stay pinned while the grid
/// scrolls both ways, and the tally-red line marks now.
struct GuideCellKey: Hashable {
    var channel: Int64
    var airing: Int64
}

/// A place in the guide to scroll to. The serial makes a second press of the same day count.
struct GuideJump: Equatable {
    var to: Date
    var serial: Int
}

struct GuideGrid: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    let channels: [Channel]
    let highlight: BroadwaveKit.Category?
    let jump: GuideJump?
    var scores: [String: String] = [:]
    #if os(tvOS)
        var channelFocus: FocusState<Int64?>.Binding
        var onDetails: (Channel, Airing?) -> Void = { _, _ in }
    #endif
    let onSelect: (Channel, Airing?) -> Void
    @State private var offset: CGPoint = .zero
    @State private var scroll = ScrollPosition()
    @State private var hasScrollSample = false
    #if os(tvOS)
        /// The program with focus, by row, so its row and channel can light up.
        @FocusState private var cellFocus: GuideCellKey?
    #endif

    /// The row the remote is on: a program's or the channel column's.
    private var focusedRow: Int64? {
        #if os(tvOS)
            cellFocus?.channel ?? channelFocus.wrappedValue
        #else
            nil
        #endif
    }

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
    #if os(tvOS)
        private let nowPillW: CGFloat = 130
    #else
        private let nowPillW: CGFloat = 80
    #endif

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
        guideOrigin(for: store.now)
    }

    #if os(tvOS)
        private let dayW: CGFloat = 200
    #else
        private let dayW: CGFloat = 110
    #endif

    /// Today, Tomorrow, or the date, for the left edge of the grid.
    private func dayTitle(at scrollX: CGFloat) -> String {
        let date = origin.addingTimeInterval(Double(scrollX / perMinute) * 60 + 60)
        let cal = Calendar.current
        if cal.isDateInToday(date) {
            return "Today"
        }
        if cal.isDateInTomorrow(date) {
            return "Tomorrow"
        }
        return date.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day())
    }

    /// Where the grid sits when it shows now: the half hour before, so the left edge
    /// starts programs whole instead of showing the last minutes of earlier ones.
    private var nowScrollX: CGFloat {
        let back = store.now.addingTimeInterval(-900).timeIntervalSince(origin)
        return max(0, x(origin.addingTimeInterval((back / 1800).rounded(.down) * 1800)))
    }

    /// The grid shows another part of the day than now.
    private var awayFromNow: Bool {
        abs(offset.x - nowScrollX) > 600
    }

    #if os(tvOS)
        private func backToNow(_ channel: Channel) {
            withAnimation(.smooth(duration: 0.35)) {
                scroll.scrollTo(point: CGPoint(x: nowScrollX, y: offset.y))
            }
            channelFocus.wrappedValue = channel.id
        }
    #endif

    private func x(_ date: Date) -> CGFloat {
        CGFloat(date.timeIntervalSince(origin) / 60) * perMinute
    }

    var body: some View {
        GeometryReader { geo in
            let row = fittedRowHeight(geo.size.height)
            scrollGrid(rowHeight: row, viewport: geo.size)
        }
    }

    /// A short lineup grows its rows so the guide fills the screen. A long one keeps the base height and scrolls.
    private func fittedRowHeight(_ viewport: CGFloat) -> CGFloat {
        let count = CGFloat(max(channels.count, 1))
        let room = viewport - headH
        guard room > count * rowH else { return rowH }
        return min(room / count, 280)
    }

    private func scrollGrid(rowHeight: CGFloat, viewport: CGSize) -> some View {
        let width = CGFloat(hours * 60) * perMinute
        let end = origin.addingTimeInterval(hours * 3600)
        let gridW = max(0, viewport.width - channelW)
        let lead = nowScrollX
        let fraction = max(0, min(1, width > 0 ? lead / width : 0))
        let scrollX = hasScrollSample ? offset.x : max(0, fraction * (width - gridW))
        return ScrollViewReader { proxy in
            HStack(alignment: .top, spacing: 0) {
                // A tvOS ScrollView scrolls its overlay with the content, so the column
                // slid off the left when the grid opened on now. Keep it beside the scroller.
                channelRail(rowHeight: rowHeight, viewport: viewport.height)
                    .frame(width: channelW, height: viewport.height, alignment: .top)
                    .background(Tokens.ColorToken.surface1)
                    .clipped()
                #if os(tvOS)
                    // Down from a channel stays in this column, ahead of a program cell to the right.
                    .focusSection()
                #endif
                ScrollView([.horizontal, .vertical]) {
                    ZStack(alignment: .topLeading) {
                        // Layout position, not offset: scrollTo ignores offset.
                        Color.clear
                            .frame(width: max(lead, 1), height: 1)
                            .overlay(alignment: .trailing) {
                                Color.clear.frame(width: 1, height: 1).id("guide-now")
                            }
                        LazyVStack(alignment: .leading, spacing: 0) {
                            Color.clear.frame(height: headH)
                            ForEach(channels) { channel in
                                row(channel, end: end, rowHeight: rowHeight)
                                    .frame(width: width, height: rowHeight, alignment: .leading)
                            }
                        }
                        .frame(height: headH + CGFloat(channels.count) * rowHeight, alignment: .top)
                        // Now line
                        Rectangle()
                            .fill(Tokens.ColorToken.tally)
                            .frame(width: 2, height: CGFloat(channels.count) * rowHeight)
                            .shadow(color: Tokens.ColorToken.tally.opacity(0.7), radius: 6)
                            .offset(x: x(store.now) - 1, y: headH)
                            .allowsHitTesting(false)
                        // Omit a mark that has crossed this edge; the column would slice it.
                        ZStack(alignment: .topLeading) {
                            // Opaque: a row scrolled up under it must not show through.
                            Rectangle().fill(Tokens.ColorToken.surface1).frame(width: width, height: headH)
                                .overlay(alignment: .bottom) { Rectangle().fill(Tokens.ColorToken.line).frame(height: 1) }
                            ForEach(0 ..< Int(hours * 2), id: \.self) { i in
                                let t = origin.addingTimeInterval(Double(i) * 1800)
                                let markX = x(t) + 8
                                // The now pill sits over this row; a mark under it would show through.
                                let underPill = markX > x(store.now) - 40 && markX < x(store.now) + nowPillW
                                // The day sits at the left edge of the header.
                                if markX >= scrollX + dayW, !underPill {
                                    Text(t.formatted(date: .omitted, time: .shortened))
                                        .font(.footnote.weight(.semibold))
                                        .monospacedDigit()
                                        .foregroundStyle(.secondary)
                                        .offset(x: markX, y: 13)
                                }
                            }
                            let nowX = x(store.now) - 30
                            if nowX >= scrollX {
                                Text(store.now.formatted(date: .omitted, time: .shortened))
                                    .font(.caption.weight(.heavy))
                                    .monospacedDigit()
                                    .padding(.horizontal, 8)
                                    .padding(.vertical, 3)
                                    .background(Tokens.ColorToken.tally, in: .capsule)
                                    .shadow(color: Tokens.ColorToken.tally.opacity(0.6), radius: 8)
                                    .offset(x: nowX, y: 10)
                            }
                            Text(dayTitle(at: scrollX))
                                .font(.footnote.weight(.bold))
                                .lineLimit(1)
                                .padding(.horizontal, 10)
                                .padding(.vertical, 3)
                                .background(.white.opacity(0.14), in: .capsule)
                                .offset(x: scrollX + 8, y: 10)
                                .accessibilityIdentifier("guide-day")
                        }
                        .offset(y: offset.y)
                        .zIndex(3)
                    }
                }
                .frame(width: gridW, height: viewport.height)
                // A tvOS scroll view draws past its bounds: rows showed below the channel column.
                .clipped()
                .scrollIndicators(.hidden)
                .scrollPosition($scroll)
                .onScrollGeometryChange(for: CGPoint.self) { $0.contentOffset } action: { _, new in
                    offset = CGPoint(x: max(0, new.x), y: max(0, new.y))
                    hasScrollSample = true
                }
                .defaultScrollAnchor(UnitPoint(x: fraction, y: 0))
                .background(Tokens.ColorToken.surface1)
                .task {
                    proxy.scrollTo("guide-now", anchor: .leading)
                }
                .onChange(of: jump) { _, jump in
                    guard let jump else { return }
                    // The left edge lands on a half hour, so no program shows as a sliver there.
                    let half = (jump.to.timeIntervalSince(origin) / 1800).rounded(.down) * 1800
                    let target = max(0, min(width - gridW, x(origin.addingTimeInterval(half))))
                    withAnimation(.smooth(duration: 0.35)) {
                        scroll.scrollTo(point: CGPoint(x: target, y: offset.y))
                    }
                }
            }
        }
    }

    private func channelRail(rowHeight: CGFloat, viewport: CGFloat) -> some View {
        ZStack(alignment: .top) {
            VStack(spacing: 0) {
                ForEach(channels) { channel in
                    Button { onSelect(channel, store.index.on(channel.id, at: store.now)) } label: {
                        // Number on top, logo and name below, so the name gets the column's
                        // full width instead of what the number and logo leave over.
                        VStack(alignment: .leading, spacing: 3) {
                            HStack(spacing: 6) {
                                Text(channel.displayNumber)
                                    .font(.title3.weight(.heavy))
                                    .monospacedDigit()
                                    .lineLimit(1)
                                    .minimumScaleFactor(0.7)
                                if channel.isATSC3 {
                                    ATSC3Tag()
                                }
                                Spacer(minLength: 0)
                                if channel.favorite {
                                    Image(systemName: "star.fill")
                                        .font(.caption2)
                                        .foregroundStyle(Tokens.ColorToken.warning)
                                        .accessibilityHidden(true)
                                }
                            }
                            HStack(spacing: 6) {
                                if let api = store.api, channel.artUrl?.isEmpty == false {
                                    AsyncImage(url: api.artURL(kind: "channel", id: channel.id, width: 72)) { phase in
                                        if let image = phase.image {
                                            image.resizable().scaledToFit()
                                        }
                                    }
                                    .frame(width: 30, height: 18)
                                    .accessibilityHidden(true)
                                }
                                Text(channel.displayName)
                                    .font(.caption.weight(.semibold))
                                    .foregroundStyle(.secondary)
                                    .lineLimit(1)
                                    .truncationMode(.tail)
                            }
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.horizontal, 14)
                        .frame(width: channelW, height: rowHeight)
                        .background(focusedRow == channel.id ? Tokens.ColorToken.surface2 : Tokens.ColorToken.surface1)
                        .overlay(alignment: .bottom) { Rectangle().fill(Tokens.ColorToken.line).frame(height: 1) }
                    }
                    .buttonStyle(GuideChannelStyle())
                    #if os(tvOS)
                        .focused(channelFocus, equals: channel.id)
                        .onMoveCommand { moveRail($0, from: channel.id, rowHeight: rowHeight, viewport: viewport) }
                        .contextMenu {
                            Button("Details", systemImage: "info.circle") { onDetails(channel, store.index.on(channel.id, at: store.now)) }
                        }
                    #endif
                        .accessibilityLabel("\(channel.displayNumber) \(channel.displayName)\(channel.isATSC3 ? ", ATSC 3.0" : "")")
                }
            }
            .padding(.top, headH)
            .offset(y: -offset.y)
            // The corner over the column stays empty: rows scroll under it.
            Rectangle()
                .fill(Tokens.ColorToken.surface1)
                .frame(height: headH)
                .overlay(alignment: .bottom) { Rectangle().fill(Tokens.ColorToken.line).frame(height: 1) }
        }
    }

    #if os(tvOS)
        /// Up and Down step through the column. With no programs to its right, the
        /// geometric search left focus on the first channel. The column is not a scroll
        /// view, so the search cannot reach a row below the screen: scroll the grid to it.
        private func moveRail(_ direction: MoveCommandDirection, from id: Int64, rowHeight: CGFloat, viewport: CGFloat) {
            guard let index = channels.firstIndex(where: { $0.id == id }) else { return }
            let next: Int
            switch direction {
            case .down:
                next = index + 1
            case .up:
                next = index - 1
            default:
                return
            }
            guard channels.indices.contains(next) else { return }
            // One more row stays in view past the focused one, so you see what is coming.
            let ahead = direction == .down ? min(next + 1, channels.count - 1) : max(next - 1, 0)
            let top = headH + CGFloat(ahead) * rowHeight - offset.y
            if top < headH {
                scroll.scrollTo(y: CGFloat(ahead) * rowHeight)
            } else if top + rowHeight > viewport + 1 {
                scroll.scrollTo(y: headH + CGFloat(ahead + 1) * rowHeight - viewport)
            }
            channelFocus.wrappedValue = channels[next].id
        }
    #endif

    private func row(_ channel: Channel, end: Date, rowHeight: CGFloat) -> some View {
        let list = store.index.airings(channel.id).filter { $0.end > origin && $0.start < end }
        return ZStack(alignment: .leading) {
            ForEach(list) { airing in
                let s = max(airing.start, origin)
                let e = min(airing.end, end)
                let w = max(24, x(e) - x(s) - 4)
                Button { onSelect(channel, airing) } label: {
                    GuideCell(
                        airing: airing,
                        now: store.now,
                        dim: highlight != nil && airing.kind != highlight,
                        recording: store.activeRecording(on: channel) != nil && airing.isOn(at: store.now),
                        score: airing.gameId.flatMap { scores[$0] },
                        art: ArtLayout.showsCellArt(slot: Int(w.rounded())) ? store.artURL(airing, width: 160) : nil,
                        showsText: w >= 44,
                        // A program that started off screen keeps its title at the visible edge.
                        inset: max(0, min(offset.x - x(s), w - 120))
                    )
                    .frame(width: w, height: rowHeight - 10)
                }
                .buttonStyle(GuideCellStyle())
                #if os(tvOS)
                    .onPlayPauseCommand { nowPlaying.play(channel) }
                    .contextMenu {
                        Button("Details", systemImage: "info.circle") { onDetails(channel, airing) }
                    }
                    // The grid opens up to an hour before now, and Right from a channel
                    // landed on the leftmost cell: a program that had ended, whose panel
                    // offered only Record series.
                    .disabled(airing.end <= store.now)
                    .focused($cellFocus, equals: GuideCellKey(channel: channel.id, airing: airing.id))
                    // Back from later in the day comes back to now, on this channel.
                    .onExitCommand(perform: awayFromNow ? { backToNow(channel) } : nil)
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
        .background(focusedRow == channel.id ? Color.white.opacity(0.05) : .clear)
        .overlay(alignment: .bottom) { Rectangle().fill(Tokens.ColorToken.line).frame(height: 1) }
    }
}

struct GuideCell: View {
    let airing: Airing
    let now: Date
    let dim: Bool
    let recording: Bool
    var score: String?
    var art: URL?
    /// A sliver of a program (its last few minutes) keeps its color bar but not a lone ellipsis.
    var showsText = true
    var inset: CGFloat = 0

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
            if showsText {
                HStack(spacing: 8) {
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
                                    .fixedSize()
                                    .layoutPriority(1)
                            }
                        }
                        Text(airing.subtitle ?? airing.start.formatted(date: .omitted, time: .shortened))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    if let art {
                        ProgramPicture(url: art, width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0, hero: false)
                            .frame(width: 52)
                            .padding(.vertical, 6)
                            .clipShape(.rect(cornerRadius: 4))
                    }
                }
                .padding(.leading, 12 + inset)
                .padding(.trailing, 8)
            }
        }
        .opacity(airing.end <= now ? 0.45 : dim ? 0.3 : 1)
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(airing.title), \(airing.start.formatted(date: .omitted, time: .shortened))")
    }
}

/// Unfocused this is the same row as the system plain style. Focused, it draws the same stroke as a program cell.
struct GuideChannelStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        ChannelBody(configuration: configuration)
    }

    private struct ChannelBody: View {
        let configuration: ButtonStyleConfiguration
        @Environment(\.isFocused) private var focused

        var body: some View {
            configuration.label
                .overlay(
                    RoundedRectangle(cornerRadius: Tokens.Radius.sm)
                        .stroke(.white, lineWidth: focused ? 3 : 0)
                )
                .opacity(configuration.isPressed && !focused ? 0.75 : 1)
        }
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
                // Focused, the cell lifts and fills, so it reads from across the room.
                .overlay(RoundedRectangle(cornerRadius: Tokens.Radius.sm).fill(.white.opacity(focused ? 0.16 : 0)))
                .overlay(RoundedRectangle(cornerRadius: Tokens.Radius.sm).stroke(.white, lineWidth: focused ? 4 : 0))
                .scaleEffect(focused ? 1.06 : configuration.isPressed ? 0.98 : 1)
                .shadow(color: .black.opacity(focused ? 0.6 : 0), radius: 18, y: 10)
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
    @State private var problem = ""
    /// Record series asks first: it records every airing from now on.
    @State private var confirmSeries = false
    let channel: Channel
    let airing: Airing?

    var body: some View {
        let kind = airing?.kind ?? .other
        let on = airing?.isOn(at: store.now) ?? true
        let upcoming = airing.map { $0.start > store.now } ?? false
        let once = airing.flatMap { Pass.once(in: store.passes, for: $0) }
        let series = airing.flatMap { Pass.series(in: store.passes, for: $0) }
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                if let airing, let art = store.artURL(airing, width: 960) {
                    ZStack(alignment: .bottomLeading) {
                        ProgramPicture(url: art, width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0)
                        LinearGradient(colors: [.clear, .black.opacity(0.55)], startPoint: .center, endPoint: .bottom)
                        HStack {
                            ChannelBadge(channel, large: true)
                            Spacer()
                            if on {
                                LiveDot()
                            }
                        }
                        .padding(16)
                    }
                    .frame(maxWidth: .infinity)
                    .frame(height: 220)
                    .clipShape(.rect(cornerRadius: Tokens.Radius.lg))
                } else {
                    HStack {
                        ChannelBadge(channel, large: true)
                        Spacer()
                        if on, airing != nil {
                            LiveDot()
                        }
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
                    if let airing, upcoming, series == nil {
                        if let once {
                            Button {
                                act { try await store.removePass(once.id) }
                            } label: {
                                Label("Don't record", systemImage: "xmark.circle").frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.glass)
                            .controlSize(.large)
                        } else {
                            Button {
                                act { try await store.recordOnce(airing) }
                            } label: {
                                Label("Record", systemImage: "record.circle").frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.glass)
                            .controlSize(.large)
                        }
                    }
                    if let airing {
                        Button {
                            if series == nil {
                                confirmSeries = true
                            }
                        } label: {
                            Label(series != nil ? "Series is recording" : kind == .sports ? "Record every airing" : "Record series", systemImage: "repeat")
                                .frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.glass)
                        .controlSize(.large)
                        #if os(iOS)
                            .disabled(series != nil)
                        #endif
                            .accessibilityIdentifier("program-record-series")
                            .confirmationDialog(
                                kind == .sports ? "Record every airing of \(airing.title)?" : "Record every episode of \(airing.title)?",
                                isPresented: $confirmSeries,
                                titleVisibility: .visible
                            ) {
                                Button(kind == .sports ? "Record every airing" : "Record series") {
                                    act { try await store.recordSeries(airing) }
                                }
                                .accessibilityIdentifier("program-record-series-confirm")
                                Button("Cancel", role: .cancel) {}
                            } message: {
                                Text("New airings record as they come up. You can stop it in Recordings › Upcoming.")
                            }
                    }
                    if !problem.isEmpty {
                        Text(problem).font(.footnote).foregroundStyle(.red)
                    }
                }
                .padding(.top, 8)
                #if os(tvOS)
                    // Room for the focused button to grow without touching the sheet's edge.
                    .padding(.bottom, 36)
                    .padding(.horizontal, 12)
                #endif
            }
            .padding(24)
        }
        .task(id: airing?.id) { await store.refreshPasses() }
        .background {
            RadialGradient(colors: [kind.color.opacity(0.35), .clear], center: .topLeading, startRadius: 0, endRadius: 400)
                .ignoresSafeArea()
        }
    }

    private func act(_ work: @escaping () async throws -> Void) {
        Task {
            do {
                try await work()
                problem = ""
            } catch {
                problem = error.localizedDescription
            }
        }
    }
}

#if os(iOS)
    /// iPad: the docked picture stays on screen above the guide, so browsing never
    /// hides what is playing. It takes no tuner of its own: with nothing playing, a
    /// picked channel shows its last frame until Preview starts it here.
    private struct GuidePreview: View {
        @Environment(AppStore.self) private var store
        @Environment(NowPlaying.self) private var nowPlaying
        @Environment(\.scenePhase) private var scenePhase
        var picked: Channel?
        @State private var minute = Int(Date().timeIntervalSince1970) / 60
        @State private var lastFrame: Image?

        private var playing: Channel? {
            nowPlaying.docked ? nowPlaying.channel : nil
        }

        /// A layer on screen makes the system pause the player in the background,
        /// so the docked sound would stop with the app.
        private var showsPlayer: Bool {
            playing != nil && scenePhase == .active
        }

        var body: some View {
            if let channel = playing ?? picked {
                HStack(alignment: .top, spacing: 16) {
                    picture(channel)
                        .frame(width: 300, height: 169)
                        .clipShape(.rect(cornerRadius: Tokens.Radius.md))
                        .contentShape(.rect)
                        .onTapGesture {
                            if playing != nil {
                                nowPlaying.expanded = true
                            }
                        }
                    details(channel)
                    Spacer(minLength: 0)
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 8)
                .task(id: showsPlayer ? -1 : channel.id) {
                    guard !showsPlayer else { return }
                    lastFrame = nil
                    // The server grabs a new frame about once a minute.
                    while !Task.isCancelled {
                        minute = Int(Date().timeIntervalSince1970) / 60
                        try? await Task.sleep(for: .seconds(30))
                    }
                }
            }
        }

        @ViewBuilder
        private func picture(_ channel: Channel) -> some View {
            if showsPlayer {
                PlayerLayerBox(player: nowPlaying.live.player)
                    .background(.black)
                    .accessibilityElement()
                    .accessibilityLabel("\(channel.displayNumber) \(channel.displayName), playing")
                    .accessibilityHint("Opens the player")
                    .accessibilityAddTraits(.isButton)
                    .accessibilityAction { nowPlaying.expanded = true }
                    .accessibilityIdentifier("guide-preview")
            } else {
                let frame = store.api?.frameURL(channelID: channel.id, width: 480, listed: store.frameIDs)
                ZStack {
                    Tokens.ColorToken.surface1
                    Text(channel.displayNumber)
                        .font(.system(size: 56, weight: .black))
                        .monospacedDigit()
                        .foregroundStyle(.white.opacity(0.12))
                    if let frame {
                        // The last frame stays up while the next one loads.
                        AsyncImage(url: frame.appending(queryItems: [URLQueryItem(name: "t", value: String(minute))])) { phase in
                            if let image = phase.image {
                                image.resizable().scaledToFill()
                                    .onAppear { lastFrame = image }
                            } else if let lastFrame {
                                lastFrame.resizable().scaledToFill()
                            }
                        }
                    }
                }
                .accessibilityHidden(true)
                .accessibilityIdentifier("guide-preview-frame")
            }
        }

        private func details(_ channel: Channel) -> some View {
            let airing = store.index.on(channel.id, at: store.now)
            return VStack(alignment: .leading, spacing: 6) {
                Text("\(channel.displayNumber)  \(channel.displayName)")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.secondary)
                Text(airing?.title ?? "No listing")
                    .font(.title3.weight(.bold))
                    .lineLimit(2)
                if let airing {
                    Text(airing.minutesLeft(at: store.now))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                HStack(spacing: 10) {
                    if playing != nil {
                        Button("Full screen", systemImage: "arrow.up.left.and.arrow.down.right") { nowPlaying.expanded = true }
                            .accessibilityIdentifier("guide-preview-full")
                    }
                    // The program beside the guide has Watch. This plays the picked channel here.
                    if let picked, picked.id != playing?.id {
                        Button(nowPlaying.channel == nil ? "Preview" : "Preview \(picked.displayNumber)", systemImage: "pip") {
                            nowPlaying.play(picked, expanded: false)
                        }
                        .accessibilityHint("Plays it here while you browse")
                        .accessibilityIdentifier("guide-preview-start")
                    }
                }
                .buttonStyle(.glass)
                .padding(.top, 4)
            }
            .accessibilityElement(children: .contain)
        }
    }
#endif

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
