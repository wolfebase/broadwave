import AVFoundation
import AVKit
import AVRouting
import BroadwaveKit
import BroadwaveUI
import SwiftUI

enum TileLayout: String, CaseIterable, Identifiable {
    case side = "2up"
    case oneTwo = "1+2"
    case oneThree = "1+3"
    case quad
    case pip

    var id: String {
        rawValue
    }

    var label: String {
        switch self {
        case .side: "Side by side"
        case .oneTwo: "One big and two"
        case .oneThree: "One big and three"
        case .quad: "Quad"
        case .pip: "Small over big"
        }
    }

    var slots: Int {
        switch self {
        case .side, .pip: 2
        case .oneTwo: 3
        case .oneThree, .quad: 4
        }
    }

    var equal: Bool {
        self == .side || self == .quad
    }

    /// Two games sit side by side, three use one big and two, four use quad.
    static func fitting(_ count: Int) -> TileLayout {
        if count >= 4 {
            return .quad
        }
        if count == 3 {
            return .oneTwo
        }
        return .side
    }

    static var saved: TileLayout {
        #if DEBUG
            if let raw = UserDefaults.standard.string(forKey: "BroadwaveMultiviewLayout"), let layout = TileLayout(rawValue: raw) {
                return layout
            }
        #endif
        return TileLayout(rawValue: UserDefaults.standard.string(forKey: "broadwave-mv-layout") ?? "") ?? .side
    }
}

/// Each tile is the largest 16:9 box that fits its slot. The black around a tile is the screen, not a bar inside the picture.
enum TileGeometry {
    private static let wide: CGFloat = 16.0 / 9.0
    private static let gap: CGFloat = 8
    private static let stackedGap: CGFloat = 28

    static func frames(layout: TileLayout, count: Int, in box: CGSize, stacked: Bool, split: CGFloat) -> [CGRect] {
        guard box.width > 1, box.height > 1, count > 0 else { return [] }
        if layout == .side, stacked {
            return stackedFrames(count: min(count, 2), in: box, split: split)
        }
        switch layout {
        case .side:
            return splitGrid(columns: min(count, 2), rows: 1, count: min(count, 2), in: box, gap: gap)
        case .quad:
            return splitGrid(columns: 2, rows: 2, count: min(count, 4), in: box, gap: gap)
        case .oneTwo:
            return feature(count: min(count, 3), rows: 2, sideShare: 0.34, in: box)
        case .oneThree:
            return feature(count: min(count, 4), rows: 3, sideShare: 0.30, in: box)
        case .pip:
            return pip(count: min(count, 2), in: box)
        }
    }

    static func stackedDivider(in box: CGSize, split: CGFloat) -> CGRect {
        let clamped = min(0.75, max(0.25, split))
        let top = (box.height - stackedGap) * clamped
        return CGRect(x: 0, y: top, width: box.width, height: stackedGap)
    }

    private static func fit(_ region: CGRect) -> CGRect {
        guard region.width > 1, region.height > 1 else { return .zero }
        let byWidth = CGSize(width: region.width, height: region.width / wide)
        let size = byWidth.height <= region.height
            ? byWidth
            : CGSize(width: region.height * wide, height: region.height)
        return CGRect(
            x: region.minX + (region.width - size.width) / 2,
            y: region.minY + (region.height - size.height) / 2,
            width: size.width,
            height: size.height
        )
    }

    private static func splitGrid(columns: Int, rows: Int, count: Int, in box: CGSize, gap: CGFloat) -> [CGRect] {
        let cellW = (box.width - gap * CGFloat(columns - 1)) / CGFloat(columns)
        let cellH = (box.height - gap * CGFloat(rows - 1)) / CGFloat(rows)
        return (0 ..< count).map { index in
            let col = index % columns
            let row = index / columns
            let region = CGRect(
                x: CGFloat(col) * (cellW + gap),
                y: CGFloat(row) * (cellH + gap),
                width: cellW,
                height: cellH
            )
            return fit(region)
        }
    }

    private static func feature(count: Int, rows: Int, sideShare: CGFloat, in box: CGSize) -> [CGRect] {
        let sideW = box.width * sideShare
        let mainW = box.width - sideW - gap
        var out = [fit(CGRect(x: 0, y: 0, width: mainW, height: box.height))]
        let smalls = max(0, count - 1)
        guard smalls > 0 else { return out }
        let cellH = (box.height - gap * CGFloat(rows - 1)) / CGFloat(rows)
        for index in 0 ..< smalls {
            let region = CGRect(
                x: mainW + gap,
                y: CGFloat(index) * (cellH + gap),
                width: sideW,
                height: cellH
            )
            out.append(fit(region))
        }
        return out
    }

    private static func pip(count: Int, in box: CGSize) -> [CGRect] {
        let main = fit(CGRect(origin: .zero, size: box))
        guard count > 1 else { return [main] }
        let pipW = min(min(box.width, main.width) * 0.32, 420)
        let pipH = pipW / wide
        var small = CGRect(x: main.maxX - pipW - 16, y: main.maxY - pipH - 16, width: pipW, height: pipH)
        small.origin.x = min(max(8, small.origin.x), box.width - pipW - 8)
        small.origin.y = min(max(8, small.origin.y), box.height - pipH - 8)
        return [main, small]
    }

    private static func stackedFrames(count: Int, in box: CGSize, split: CGFloat) -> [CGRect] {
        let clamped = min(0.75, max(0.25, split))
        let topH = (box.height - stackedGap) * clamped
        let bottomH = box.height - stackedGap - topH
        var out = [fit(CGRect(x: 0, y: 0, width: box.width, height: topH))]
        if count > 1 {
            out.append(fit(CGRect(x: 0, y: topH + stackedGap, width: box.width, height: bottomH)))
        }
        return out
    }
}

struct SavedSet: Codable, Identifiable, Hashable {
    var name: String
    var channels: [Int64]
    var layout: String?
    var id: String {
        channels.map(String.init).joined(separator: ",")
    }
}

enum SavedMultiview {
    private static let key = "broadwave-multiview"

    static func load() -> [SavedSet] {
        guard let data = UserDefaults.standard.data(forKey: key),
              let sets = try? JSONDecoder().decode([SavedSet].self, from: data) else { return [] }
        return sets
    }

    static func save(name: String, channels: [Int64], layout: String) {
        var sets = load().filter { $0.channels != channels }
        sets.insert(SavedSet(name: name, channels: channels, layout: layout), at: 0)
        if let data = try? JSONEncoder().encode(Array(sets.prefix(8))) {
            UserDefaults.standard.set(data, forKey: key)
        }
    }
}

/// Tiles share one multiview room. `AVPlaybackCoordinationMedium` is not used:
/// it would seek every player to one timeline, and each tile is a different
/// live edge. The room already pauses them together.
@MainActor
@Observable
final class MultiviewSession {
    var layout: TileLayout
    var focusID: Int64
    var guide = false
    var notice = ""
    var split: CGFloat = 0.5
    var dragOrigin: CGFloat?
    let room: String
    private var command: ((String) -> Void)?
    var paused = false

    init(focusID: Int64) {
        layout = .saved
        self.focusID = focusID
        let id = String(UUID().uuidString.prefix(8)).lowercased()
        room = "multiview:\(id)"
    }

    func rememberLayout() {
        UserDefaults.standard.set(layout.rawValue, forKey: "broadwave-mv-layout")
    }

    func bind(_ send: @escaping (String) -> Void) {
        command = send
    }

    func togglePause() {
        command?(paused ? "play" : "pause")
        paused.toggle()
    }

    func prefs(for id: Int64) -> Prefs {
        if id == focusID {
            let audio: Prefs.Sound = layout.equal ? .stereo : .auto
            return Prefs(quality: .focus, audio: audio, picture: "broadcast")
        }
        let quality: Prefs.Quality = (layout == .quad || layout == .pip) ? .tile360 : .tile
        return Prefs(quality: quality, audio: layout.equal ? .stereo : .none, picture: "broadcast")
    }
}

@MainActor
@Observable
final class TilePlayer {
    let player = AVPlayer()
    private(set) var error: String?
    private(set) var detail = ""
    private(set) var dropped = 0
    /// True only after this tile's own item has been told to play. Unmuting the
    /// previous item is not sound yet.
    private(set) var canHear = false
    private var audible = false
    private var attempts = 0
    private var channelID: Int64?
    private var session: WatchSession?
    private var sync: SyncEngine?
    private var api: APIClient?

    struct Request {
        var channel: Channel
        var prefs: Prefs
        var audible: Bool
    }

    func noteDrops(_ count: Int) {
        dropped = count
    }

    func start(_ request: Request, room: String, store: AppStore, bind: @escaping (@escaping (String) -> Void) -> Void) async {
        let channel = request.channel
        let prefs = request.prefs
        canHear = false
        await stop()
        guard let api = store.api else { return }
        self.api = api
        channelID = channel.id
        audible = request.audible
        error = nil
        do {
            let session = try await api.watch(channelID: channel.id, caps: Capabilities.current(), prefs: prefs)
            guard channelID == channel.id else {
                await api.stopWatching(channelID: channel.id, rendition: session.rendition)
                return
            }
            self.session = session
            detail = session.stream.reason
            let item = AVPlayerItem(url: api.url(session.playlist))
            let tile = prefs.quality == .tile || prefs.quality == .tile360
            PlayerTuning.apply(item, network: Capabilities.current().network ?? "lan", tile: tile)
            player.replaceCurrentItem(with: item)
            player.automaticallyWaitsToMinimizeStalling = true
            applyAudible()
            player.play()
            canHear = request.audible
            if let socket = store.socket {
                let engine = SyncEngine(player: player, socket: socket, room: room, channelID: channel.id)
                engine.start()
                sync = engine
                bind { engine.command($0) }
            }
            attempts = 0
        } catch {
            attempts += 1
            if attempts == 1, error.localizedDescription.localizedStandardContains("tuner") {
                try? await Task.sleep(for: .seconds(2))
                guard channelID == channel.id else { return }
                await start(request, room: room, store: store, bind: bind)
                return
            }
            self.error = error.localizedDescription
        }
    }

    func setAudible(_ on: Bool) {
        audible = on
        // Muting is immediate. Unmuting waits until start() has replaced the item,
        // so the previous silent rendition is not counted as sound.
        if !on {
            canHear = false
            applyAudible()
            clearRoute()
        }
    }

    /// Drift from the broadcast timeline, in milliseconds. Nil until sync has a target.
    private(set) var driftMS: Int?
    func noteDrift() {
        guard let sync, sync.state == .locked || sync.state == .syncing else {
            driftMS = nil
            return
        }
        driftMS = Int(sync.drift.rounded())
    }

    func stop() async {
        sync?.stop()
        sync = nil
        clearRoute()
        player.pause()
        player.replaceCurrentItem(with: nil)
        if let api, let id = channelID, let session {
            await api.stopWatching(channelID: id, rendition: session.rendition)
        }
        session = nil
        channelID = nil
        canHear = false
    }

    private func applyAudible() {
        player.isMuted = !audible
        player.networkResourcePriority = audible ? .high : .low
        guard audible else { return }
        let arbiter = AVRoutingPlaybackArbiter.shared()
        arbiter.preferredParticipantForExternalPlayback = player
        // The iOS declaration first ships in the iOS 27 SDK (Swift 6.4); CI still builds with Xcode 26.
        #if os(tvOS) || compiler(>=6.4)
            if #available(iOS 27, tvOS 26, *) {
                arbiter.preferredParticipantForNonMixableAudioRoutes = player
            }
        #endif
    }

    private func clearRoute() {
        let arbiter = AVRoutingPlaybackArbiter.shared()
        if arbiter.preferredParticipantForExternalPlayback === player {
            arbiter.preferredParticipantForExternalPlayback = nil
        }
        // HomePod and other non-mixable routes use the same focused player.
        #if os(tvOS) || compiler(>=6.4)
            if #available(iOS 27, tvOS 26, *) {
                if arbiter.preferredParticipantForNonMixableAudioRoutes === player {
                    arbiter.preferredParticipantForNonMixableAudioRoutes = nil
                }
            }
        #endif
    }
}

struct MultiviewScreen: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @Environment(\.horizontalSizeClass) private var width
    @Environment(\.verticalSizeClass) private var height
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var session: MultiviewSession
    @State private var blocked: Set<Int64> = []
    @State private var offers: [Int64: MultiviewPlanOffers] = [:]
    @State private var stops: [MultiviewPlanStops] = []
    @State private var planReady = {
        #if DEBUG
            UserDefaults.standard.bool(forKey: "BroadwaveMultiviewTest")
        #else
            false
        #endif
    }()

    @State private var hint = !UserDefaults.standard.bool(forKey: "broadwave-mv-hint-seen")
    @State private var menuChannel: Int64?
    #if os(tvOS)
        @FocusState private var remoteFocus: Int64?
    #endif

    init() {
        _session = State(initialValue: MultiviewSession(focusID: 0))
    }

    private var cap: Int {
        #if os(tvOS)
            4
        #else
            UIDevice.current.userInterfaceIdiom == .pad ? 4 : 2
        #endif
    }

    private var layouts: [TileLayout] {
        #if os(iOS)
            if UIDevice.current.userInterfaceIdiom == .phone {
                return [.side, .pip]
            }
        #endif
        return Array(TileLayout.allCases)
    }

    private var chosen: [Channel] {
        nowPlaying.together.compactMap { id in store.channels.first { $0.id == id } }
    }

    private var ordered: [Channel] {
        let visible = chosen.filter { !blocked.contains($0.id) }
        let capped = Array(visible.prefix(min(session.layout.slots, cap)))
        guard !session.layout.equal, let focus = capped.first(where: { $0.id == session.focusID }) ?? capped.first else {
            return capped
        }
        return [focus] + capped.filter { $0.id != focus.id }
    }

    private var stacked: Bool {
        #if os(iOS)
            session.layout == .side && width == .compact && height == .regular
        #else
            false
        #endif
    }

    var body: some View {
        let tiles = ordered
        ZStack {
            Color.black.ignoresSafeArea()
            VStack(spacing: 10) {
                topBar(tiles)
                if hint {
                    Text("Select a tile to hear it.")
                        .font(.footnote.weight(.semibold))
                        .padding(.horizontal, 14)
                        .padding(.vertical, 8)
                        .glassEffect(in: .capsule)
                        .accessibilityIdentifier("mv-hint")
                }
                if !session.notice.isEmpty {
                    Text(session.notice)
                        .font(.footnote.weight(.semibold))
                        .padding(.horizontal, 14)
                        .padding(.vertical, 8)
                        .glassEffect(in: .capsule)
                        .accessibilityAddTraits(.updatesFrequently)
                }
                // Tiles start a tune on appear. Wait for the plan so a channel it would refuse never takes a tuner.
                if nowPlaying.together.count > 1, !planReady {
                    ProgressView()
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else {
                    grid(tiles)
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                        .safeAreaInset(edge: .bottom, spacing: 8) {
                            VStack(spacing: 8) {
                                layoutBar
                                if session.guide {
                                    channelStrip
                                }
                            }
                        }
                }
            }
            .padding(12)
        }
        .onAppear {
            applyOpenedLayout()
            if session.focusID == 0 {
                session.focusID = nowPlaying.together.first ?? 0
            }
            if nowPlaying.together.count < 2 || UserDefaults.standard.bool(forKey: "BroadwaveMultiviewAdd") {
                session.guide = true
            }
        }
        .onChange(of: nowPlaying.openedLayout) { _, _ in
            applyOpenedLayout()
        }
        .task(id: nowPlaying.together) {
            #if DEBUG
                if UserDefaults.standard.bool(forKey: "BroadwaveMultiviewTest") {
                    planReady = true
                    return
                }
            #endif
            if nowPlaying.together.count > 1 {
                planReady = false
            }
            await refreshPlan()
            planReady = true
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(20))
                if Task.isCancelled {
                    return
                }
                await refreshPlan()
            }
        }
        .task(id: stopKey) {
            guard let soonest = stops.map(\.at).min() else { return }
            let wait = soonest.timeIntervalSinceNow
            if wait > 0 {
                try? await Task.sleep(for: .seconds(wait))
            } else {
                try? await Task.sleep(for: .seconds(4))
            }
            if Task.isCancelled {
                return
            }
            let due = Set(stops.filter { $0.at.timeIntervalSinceNow <= 1 }.map(\.channelId))
            guard !due.isEmpty else { return }
            nowPlaying.together.removeAll { due.contains($0) }
        }
        #if os(tvOS)
        .defaultFocus($remoteFocus, nowPlaying.together.first ?? 0)
        .onPlayPauseCommand {
            session.togglePause()
        }
        .onExitCommand {
            if menuChannel != nil {
                menuChannel = nil
            } else if session.guide {
                session.guide = false
            } else {
                leave()
            }
        }
        .onAppear {
            if remoteFocus == nil {
                remoteFocus = nowPlaying.together.first
            }
        }
        .onChange(of: planReady) { _, ready in
            if ready, remoteFocus == nil {
                remoteFocus = nowPlaying.together.first
            }
        }
        #endif
    }

    private func topBar(_ tiles: [Channel]) -> some View {
        HStack(spacing: 12) {
            Button("Back to one channel", systemImage: "xmark") { leave() }
                .labelStyle(.iconOnly)
                .buttonStyle(.glass)
            Text(session.layout.label)
                .font(.headline)
            Spacer()
            Button(session.paused ? "Play" : "Pause") { session.togglePause() }
                .buttonStyle(.glass)
                .accessibilityIdentifier("multiview-pause")
            if session.paused {
                Text("Paused")
                    .font(.headline)
                    .accessibilityIdentifier("multiview-paused")
            }
            Button("Save") {
                let name = tiles.map(\.displayNumber).joined(separator: " and ")
                SavedMultiview.save(name: name, channels: tiles.map(\.id), layout: session.layout.rawValue)
            }
            .buttonStyle(.glass)
            .disabled(tiles.count < 2)
        }
    }

    private var layoutBar: some View {
        ScrollView(.horizontal) {
            HStack(spacing: 8) {
                ForEach(layouts) { item in
                    Button(item.label) {
                        let change = { session.layout = item; session.rememberLayout() }
                        if reduceMotion {
                            change()
                        } else {
                            withAnimation(.snappy) { change() }
                        }
                    }
                    .buttonStyle(.bordered)
                    .tint(session.layout == item ? Color.accentColor : nil)
                    .accessibilityAddTraits(session.layout == item ? .isSelected : [])
                }
                Button("Channels") { session.guide.toggle() }
                    .buttonStyle(.bordered)
                    .accessibilityAddTraits(session.guide ? .isSelected : [])
            }
        }
        .scrollIndicators(.hidden)
    }

    private var channelStrip: some View {
        ScrollView(.horizontal) {
            HStack(spacing: 8) {
                ForEach(store.channels) { channel in
                    let on = nowPlaying.together.contains(channel.id)
                    let offer = offers[channel.id]
                    let full = offer?.cost == "none"
                    Button {
                        add(channel)
                    } label: {
                        VStack(spacing: 2) {
                            Text(channel.displayNumber).font(.headline.weight(.bold))
                            Text(channel.displayName).font(.caption2).lineLimit(1)
                            if let label = offer?.label, !label.isEmpty {
                                Text(label).font(.caption2).lineLimit(1)
                            }
                        }
                        .frame(minWidth: 88, minHeight: 48)
                    }
                    .buttonStyle(.bordered)
                    .tint(on ? Color.accentColor : nil)
                    .disabled(full)
                    .accessibilityLabel(offerLabel(channel, offer))
                    .accessibilityAddTraits(on ? .isSelected : [])
                }
            }
        }
        .scrollIndicators(.hidden)
        .accessibilityLabel("Add a channel")
    }

    @ViewBuilder
    private func grid(_ tiles: [Channel]) -> some View {
        if tiles.isEmpty {
            Text("Pick two channels.")
                .foregroundStyle(.secondary)
        } else {
            GeometryReader { geo in
                let frames = TileGeometry.frames(
                    layout: session.layout,
                    count: tiles.count,
                    in: geo.size,
                    stacked: stacked,
                    split: session.split
                )
                ZStack(alignment: .topLeading) {
                    ForEach(Array(tiles.enumerated()), id: \.element.id) { index, channel in
                        if frames.indices.contains(index) {
                            tile(channel)
                                .frame(width: frames[index].width, height: frames[index].height)
                                .offset(x: frames[index].minX, y: frames[index].minY)
                        }
                    }
                    if stacked, tiles.count >= 2 {
                        let handle = TileGeometry.stackedDivider(in: geo.size, split: session.split)
                        Rectangle()
                            .fill(.white.opacity(0.35))
                            .frame(width: handle.width, height: handle.height)
                            .offset(x: handle.minX, y: handle.minY)
                            .accessibilityLabel("Divider")
                        #if os(iOS)
                            .gesture(
                                DragGesture()
                                    .onChanged { value in
                                        if session.dragOrigin == nil {
                                            session.dragOrigin = session.split
                                        }
                                        let base = session.dragOrigin ?? session.split
                                        let span = max(geo.size.height - handle.height, 1)
                                        session.split = min(0.75, max(0.25, base + value.translation.height / span))
                                    }
                                    .onEnded { _ in session.dragOrigin = nil }
                            )
                        #endif
                    }
                }
                .frame(width: geo.size.width, height: geo.size.height, alignment: .topLeading)
                .clipped()
            }
        }
    }

    private func dismissHint() {
        guard hint else { return }
        hint = false
        UserDefaults.standard.set(true, forKey: "broadwave-mv-hint-seen")
    }

    private func tile(_ channel: Channel) -> some View {
        let focused = channel.id == (ordered.first { $0.id == session.focusID }?.id ?? ordered.first?.id)
        #if os(tvOS)
            let remoteFocused = remoteFocus == channel.id
        #else
            let remoteFocused = false
        #endif
        return MultiviewTile(
            channel: channel,
            title: store.index.on(channel.id, at: store.now)?.title ?? channel.displayName,
            prefs: session.prefs(for: channel.id),
            room: session.room,
            focused: focused,
            remoteFocused: remoteFocused,
            pip: focused
        ) {
            session.focusID = channel.id
        } bind: { session.bind($0) } onSound: {
            dismissHint()
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        #if os(tvOS)
            .focused($remoteFocus, equals: channel.id)
        #endif
            .contextMenu {
                tileMenu(channel)
            }
    }

    @ViewBuilder
    private func tileMenu(_ channel: Channel) -> some View {
        Button("Make big") {
            session.focusID = channel.id
            menuChannel = nil
        }
        Button("Record") {
            Task { await store.toggleRecord(channel) }
            menuChannel = nil
        }
        Button("Remove") {
            nowPlaying.together.removeAll { $0 == channel.id }
            menuChannel = nil
        }
        .accessibilityIdentifier("tile-menu-remove")
        Button("Full screen") {
            nowPlaying.play(channel)
            menuChannel = nil
        }
    }

    private var stopKey: String {
        stops.map { "\($0.channelId)-\($0.at.timeIntervalSince1970)" }.joined(separator: ",")
    }

    private func offerLabel(_ channel: Channel, _ offer: MultiviewPlanOffers?) -> String {
        let name = "\(channel.displayNumber), \(channel.displayName)"
        guard let offer, !offer.label.isEmpty else { return name }
        return name + ". " + offer.label
    }

    /// Watch together and a saved set name the layout. The phone only has two tiles, so a quad opens side by side there.
    private func applyOpenedLayout() {
        guard let raw = nowPlaying.openedLayout, let picked = TileLayout(rawValue: raw) else { return }
        session.layout = layouts.contains(picked) ? picked : .side
        nowPlaying.openedLayout = nil
    }

    private func add(_ channel: Channel) {
        if offers[channel.id]?.cost == "none" {
            return
        }
        if nowPlaying.together.contains(channel.id) {
            session.focusID = channel.id
            session.guide = false
            return
        }
        let limit = min(session.layout.slots, cap)
        var next = nowPlaying.together
        if next.count >= limit {
            if let drop = next.last(where: { $0 != session.focusID }) {
                next.removeAll { $0 == drop }
            }
        }
        next.append(channel.id)
        nowPlaying.together = next
        session.focusID = channel.id
        session.guide = false
    }

    private func leave() {
        let id = session.focusID
        if let channel = store.channels.first(where: { $0.id == id }) ?? chosen.first {
            nowPlaying.play(channel)
        } else {
            nowPlaying.stop()
        }
    }

    private func refreshPlan() async {
        let ids = nowPlaying.together
        guard let api = store.api else { return }
        guard let plan = try? await api.planMultiview(ids) else { return }
        blocked = Set(plan.blocked.map(\.channelId))
        offers = Dictionary(uniqueKeysWithValues: (plan.offers ?? []).map { ($0.channelId, $0) })
        stops = plan.stops ?? []
        let warning = Array(Set((plan.stops ?? []).map(\.reason))).filter { !$0.isEmpty }.joined(separator: " ")
        session.notice = warning.isEmpty ? (plan.blocked.first?.reason ?? plan.note ?? "") : warning
    }
}

struct MultiviewTile: View {
    @Environment(AppStore.self) private var store
    let channel: Channel
    let title: String
    let prefs: Prefs
    let room: String
    let focused: Bool
    /// The Siri Remote is on this tile. Sound is separate: click moves that.
    let remoteFocused: Bool
    let pip: Bool
    let onFocus: () -> Void
    let bind: (@escaping (String) -> Void) -> Void
    let onSound: () -> Void
    @State private var live = TilePlayer()
    private var tileStats: Bool {
        UserDefaults.standard.bool(forKey: "BroadwaveTileStats")
    }

    var body: some View {
        Button(action: onFocus) {
            ZStack(alignment: .bottomLeading) {
                PlayerLayerBox(player: live.player, pip: pip)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .background(.black)
                VStack(alignment: .leading, spacing: 2) {
                    HStack(spacing: 8) {
                        Text(channel.displayNumber).font(.caption.weight(.bold))
                        Text(title).font(.caption).lineLimit(1)
                        if remoteFocused {
                            Text("Focused")
                                .font(.caption.weight(.bold))
                        }
                        Spacer(minLength: 0)
                        if focused {
                            Label("Sound", systemImage: "speaker.wave.2.fill")
                                .font(.caption.weight(.bold))
                                .labelStyle(.titleAndIcon)
                        }
                    }
                    if focused, !live.detail.isEmpty {
                        Text(live.detail)
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                    if tileStats {
                        Text("\(live.dropped) dropped")
                            .font(.caption2)
                            .accessibilityIdentifier("tile-drops-\(channel.id)")
                        if let drift = live.driftMS {
                            Text("\(drift) ms")
                                .font(.caption2)
                                .accessibilityIdentifier("tile-drift-\(channel.id)")
                        }
                    }
                }
                .padding(8)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(.black.opacity(0.45))
                if let error = live.error {
                    Text(error)
                        .font(.footnote)
                        .padding(8)
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                        .background(.black.opacity(0.55))
                }
            }
            .clipShape(.rect(cornerRadius: Tokens.Radius.md))
            .overlay {
                RoundedRectangle(cornerRadius: Tokens.Radius.md)
                    .strokeBorder(focused ? .white : .white.opacity(0.15), lineWidth: focused ? 3 : 1)
            }
        }
        .buttonStyle(.plain)
        #if os(iOS)
            .focusEffectDisabled()
        #endif
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .accessibilityIdentifier("tile-\(channel.id)")
            .accessibilityLabel("\(channel.displayNumber) \(channel.displayName), \(title)")
            .accessibilityValue(tileValue)
            .accessibilityAddTraits(focused ? .isSelected : [])
            .task(id: "\(channel.id)-\(prefs.quality.rawValue)-\(prefs.audio.rawValue)") {
                #if DEBUG
                    if UserDefaults.standard.bool(forKey: "BroadwaveMultiviewTest") {
                        return
                    }
                #endif
                await live.start(TilePlayer.Request(channel: channel, prefs: prefs, audible: focused), room: room, store: store, bind: bind)
            }
            .onChange(of: focused) { _, on in
                live.setAudible(on)
            }
            .task(id: focused) {
                guard focused else { return }
                while !Task.isCancelled {
                    if live.canHear, live.player.timeControlStatus == .playing, !live.player.isMuted {
                        onSound()
                        return
                    }
                    try? await Task.sleep(for: .milliseconds(400))
                }
            }
            .task(id: tileStats) {
                guard tileStats else { return }
                while !Task.isCancelled {
                    if let event = live.player.currentItem?.accessLog()?.events.last {
                        live.noteDrops(event.numberOfDroppedVideoFrames)
                    }
                    live.noteDrift()
                    try? await Task.sleep(for: .seconds(1))
                }
            }
            .onDisappear {
                Task { await live.stop() }
            }
    }

    private var tileValue: String {
        var parts = [remoteFocused ? "Focused" : nil, focused ? "Sound on" : "Sound off"].compactMap(\.self)
        if tileStats {
            parts.append("\(live.dropped) dropped")
            if let drift = live.driftMS {
                parts.append("\(drift) ms")
            }
        }
        return parts.joined(separator: ", ")
    }
}

struct PlayerLayerBox: UIViewRepresentable {
    let player: AVPlayer
    var pip = false

    func makeUIView(context _: Context) -> PlayerHost {
        let view = PlayerHost()
        view.playerLayer?.player = player
        view.playerLayer?.videoGravity = .resizeAspectFill
        return view
    }

    func updateUIView(_ view: PlayerHost, context _: Context) {
        view.playerLayer?.player = player
        #if os(iOS)
            if pip, view.pip == nil, let layer = view.playerLayer, AVPictureInPictureController.isPictureInPictureSupported() {
                view.pip = AVPictureInPictureController(playerLayer: layer)
                view.pip?.canStartPictureInPictureAutomaticallyFromInline = true
            }
            if !pip {
                view.pip = nil
            }
        #endif
    }
}

final class PlayerHost: UIView {
    override static var layerClass: AnyClass {
        AVPlayerLayer.self
    }

    var playerLayer: AVPlayerLayer? {
        layer as? AVPlayerLayer
    }

    #if os(iOS)
        var pip: AVPictureInPictureController?
    #endif
}
