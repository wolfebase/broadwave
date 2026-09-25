import AVFoundation
import AVKit
import BroadwaveKit
import BroadwaveUI
import CoreMedia
import SwiftUI

struct PictureStats: Equatable {
    var width = 0
    var height = 0
    var dropped = 0
    var buffer = 0.0
    var fps: Float = 0
}

@MainActor
@Observable
final class LivePlayer {
    let player = AVPlayer()
    private(set) var session: WatchSession?
    private(set) var sync: SyncEngine?
    var error: String?
    private(set) var needsConfirm = false
    private(set) var attempt = 0
    private var confirmNext = false
    private var channelID: Int64?
    private var api: APIClient?
    private var started = Date()
    private var tick: Any?
    private var stallObserver: NSObjectProtocol?
    private(set) var firstFrameMs: Int?
    private(set) var stalls = 0
    private(set) var stallMs = 0
    private var stallStarted: Date?
    private var lastBeat = Date()
    /// Nominal frame rate once the asset reports it. Zero until then.
    private(set) var refreshRate: Float = 0
    private(set) var picture = PictureStats()
    private var statsTask: Task<Void, Never>?

    func start(_ channel: Channel, store: AppStore) async {
        await stop()
        guard let api = store.api else { return }
        self.api = api
        channelID = channel.id
        let allow = confirmNext
        confirmNext = false
        needsConfirm = false
        error = nil
        do {
            let session = try await api.watch(channelID: channel.id, caps: Capabilities.current(), prefs: store.prefs, confirmLive: allow)
            guard channelID == channel.id else {
                await api.stopWatching(channelID: channel.id, rendition: session.rendition)
                return
            }
            self.session = session
            let item = AVPlayerItem(url: api.url(session.playlist))
            item.externalMetadata = metadata(channel: channel, airing: store.index.on(channel.id, at: Date()))
            PlayerTuning.apply(item, network: Capabilities.current().network ?? "lan", tile: false)
            player.replaceCurrentItem(with: item)
            player.automaticallyWaitsToMinimizeStalling = true
            watchStartup(item)
            player.play()
            watchPicture()
            if store.syncEnabled, Compatibility.gateFeature(store.info, "wholeHomeSync") == nil, let socket = store.socket {
                let engine = SyncEngine(player: player, socket: socket, room: "channel:\(channel.id)", channelID: channel.id)
                engine.start()
                sync = engine
            }
        } catch let error as APIError where error.code == "recording_soon" {
            needsConfirm = true
            self.error = error.message
        } catch {
            needsConfirm = false
            self.error = error.localizedDescription
        }
    }

    func confirmWatch() {
        confirmNext = true
        needsConfirm = false
        error = nil
        attempt += 1
    }

    func stop() async {
        statsTask?.cancel()
        statsTask = nil
        picture = PictureStats()
        sync?.stop()
        sync = nil
        if let tick {
            player.removeTimeObserver(tick)
        }
        tick = nil
        if let stallObserver {
            NotificationCenter.default.removeObserver(stallObserver)
        }
        stallObserver = nil
        player.pause()
        player.replaceCurrentItem(with: nil)
        if let api, let id = channelID, let session {
            await api.stopWatching(channelID: id, rendition: session.rendition)
        }
        session = nil
        channelID = nil
    }

    private func watchPicture() {
        statsTask?.cancel()
        statsTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(1))
                guard let self else { return }
                var rate: Float = 0
                if let item = player.currentItem {
                    rate = await videoPicture(item).rate
                }
                samplePicture(rate: rate)
            }
        }
    }

    private func samplePicture(rate: Float) {
        guard let item = player.currentItem else { return }
        var next = PictureStats()
        let size = item.presentationSize
        if size.width > 1 {
            next.width = Int(size.width.rounded())
            next.height = Int(size.height.rounded())
        }
        let now = CMTimeGetSeconds(item.currentTime())
        if now.isFinite {
            for value in item.loadedTimeRanges {
                let range = value.timeRangeValue
                let start = CMTimeGetSeconds(range.start)
                let end = CMTimeGetSeconds(CMTimeRangeGetEnd(range))
                if start.isFinite, end.isFinite, start <= now + 0.05, end >= now {
                    next.buffer = max(next.buffer, end - now)
                }
            }
        }
        if let event = item.accessLog()?.events.last {
            next.dropped = event.numberOfDroppedVideoFrames
        }
        let match = PlayerTuning.matchingDisplay(
            width: next.width,
            height: next.height,
            rate: rate,
            previous: DisplayMatch(refreshRate: refreshRate)
        )
        if match.refreshRate > 1 {
            refreshRate = match.refreshRate
        }
        next.fps = refreshRate
        picture = next
    }

    private func watchStartup(_ item: AVPlayerItem) {
        if let tick {
            player.removeTimeObserver(tick)
        }
        if let stallObserver {
            NotificationCenter.default.removeObserver(stallObserver)
        }
        started = Date()
        firstFrameMs = nil
        stalls = 0
        stallMs = 0
        stallStarted = nil
        lastBeat = Date()
        stallObserver = NotificationCenter.default.addObserver(forName: AVPlayerItem.playbackStalledNotification, object: item, queue: .main) { [weak self] _ in
            Task { @MainActor in
                guard let self else { return }
                if self.stallStarted == nil {
                    self.stallStarted = Date()
                }
                self.stalls += 1
                print("broadwave stall \(self.stalls)")
            }
        }
        tick = player.addPeriodicTimeObserver(forInterval: CMTime(seconds: 0.25, preferredTimescale: 600), queue: .main) { [weak self] _ in
            Task { @MainActor in
                guard let self else { return }
                if self.firstFrameMs == nil, self.player.timeControlStatus == .playing {
                    self.firstFrameMs = Int(Date().timeIntervalSince(self.started) * 1000)
                    print("broadwave ttff \(self.firstFrameMs ?? 0)ms")
                }
                if let start = self.stallStarted, self.player.timeControlStatus == .playing {
                    self.stallMs += Int(Date().timeIntervalSince(start) * 1000)
                    self.stallStarted = nil
                    print("broadwave stall-ms \(self.stallMs)")
                }
                if self.firstFrameMs != nil, Date().timeIntervalSince(self.lastBeat) >= 60 {
                    self.lastBeat = Date()
                    print("broadwave beat stalls=\(self.stalls) stallMs=\(self.stallMs)")
                }
            }
        }
    }

    private func metadata(channel: Channel, airing: Airing?) -> [AVMetadataItem] {
        func item(_ id: AVMetadataIdentifier, _ value: String) -> AVMetadataItem {
            let m = AVMutableMetadataItem()
            m.identifier = id
            m.value = value as NSString
            m.extendedLanguageTag = "und"
            return m
        }
        var out = [item(.commonIdentifierTitle, airing?.title ?? channel.displayName)]
        out.append(item(.iTunesMetadataTrackSubTitle, "\(channel.displayNumber) \(channel.displayName)"))
        if let d = airing?.description {
            out.append(item(.commonIdentifierDescription, d))
        }
        return out
    }
}

struct PlayerScreen: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    #if os(iOS)
        @Environment(\.verticalSizeClass) private var verticalSize
    #endif
    @State private var live = LivePlayer()
    @State private var showStream = false
    #if os(iOS)
        @State private var showGuide = false
    #endif

    /// Portrait keeps the channel, the program, and labeled controls on screen.
    /// A short landscape phone keeps the icon bar so the picture stays clear.
    private var portraitChrome: Bool {
        #if os(iOS)
            verticalSize != .compact
        #else
            false
        #endif
    }

    var body: some View {
        ZStack(alignment: .top) {
            Color.black.ignoresSafeArea()
            SystemPlayer(
                player: live.player,
                hint: displayHint(live.session?.stream),
                menu: channelMenu,
                audio: audioMenu,
                streamOn: showStream,
                onStream: { showStream.toggle() },
                onTogether: {
                    if let channel = nowPlaying.channel {
                        nowPlaying.watchTogether([channel])
                    }
                }
            )
            .ignoresSafeArea()
            #if os(iOS)
                overlay
            #endif
            if showStream, !portraitChrome {
                StreamPanel(stream: live.session?.stream, stats: live.picture, sync: live.sync)
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomLeading)
                    .padding(24)
            }
            if let error = live.error {
                VStack(spacing: 12) {
                    Text(error)
                        .multilineTextAlignment(.center)
                    if live.needsConfirm {
                        Button("Watch anyway") {
                            live.confirmWatch()
                        }
                        .buttonStyle(.borderedProminent)
                    }
                }
                .padding()
                .glassEffect(in: .rect(cornerRadius: Tokens.Radius.md))
                .padding(.top, 80)
            }
        }
        .task(id: "\(nowPlaying.channel?.id ?? 0) \(store.prefs.track ?? "") \(store.prefs.even) \(live.attempt)") {
            #if DEBUG
                // Layout checks must not take a tuner. -BroadwaveChrome YES skips the session.
                if UserDefaults.standard.bool(forKey: "BroadwaveChrome") {
                    return
                }
            #endif
            if let channel = nowPlaying.channel {
                await live.start(channel, store: store)
            }
        }
        #if DEBUG
        .onAppear {
            // Simulator checks: -BroadwaveStream YES opens the panel without a remote.
            if UserDefaults.standard.bool(forKey: "BroadwaveStream") {
                showStream = true
            }
            #if os(iOS)
                if UserDefaults.standard.bool(forKey: "BroadwaveChannels") {
                    showGuide = true
                }
            #endif
        }
        #endif
        .onDisappear {
            Task { await live.stop() }
        }
    }

    private func step(_ dir: Int) {
        guard let current = nowPlaying.channel, let i = store.channels.firstIndex(of: current) else { return }
        let next = store.channels[(i + dir + store.channels.count) % store.channels.count]
        nowPlaying.channel = next
    }

    private var audioMenu: [ChannelMenuEntry] {
        let current = store.prefs.track ?? "main"
        func choice(_ num: Int64, _ id: String, _ title: String) -> ChannelMenuEntry {
            ChannelMenuEntry(id: num, title: title, current: current == id) {
                var prefs = store.prefs
                prefs.track = id == "main" ? nil : id
                store.prefs = prefs
            }
        }
        var items = [choice(1, "main", "Main"), choice(2, "language", "Second language"), choice(3, "described", "Described video")]
        items.append(ChannelMenuEntry(id: -1, title: store.prefs.even ? "Even volume on" : "Even volume", current: store.prefs.even) {
            var prefs = store.prefs
            prefs.even.toggle()
            store.prefs = prefs
        })
        return items
    }

    /// tvOS: a Channels menu in the transport bar lets you surf without leaving the player.
    private var channelMenu: [ChannelMenuEntry] {
        store.channels.map { c in
            ChannelMenuEntry(id: c.id, title: "\(c.displayNumber)  \(store.index.on(c.id, at: store.now)?.title ?? c.displayName)", current: c.id == nowPlaying.channel?.id) {
                nowPlaying.channel = c
            }
        }
    }

    #if os(iOS)
        @ViewBuilder
        private var overlay: some View {
            if portraitChrome {
                portraitOverlay
            } else {
                landscapeBar
            }
        }

        private var portraitOverlay: some View {
            VStack(alignment: .leading, spacing: 12) {
                HStack(spacing: 10) {
                    Button("Minimize", systemImage: "chevron.down") { nowPlaying.expanded = false }
                        .labelStyle(.titleAndIcon)
                        .buttonStyle(.glass)
                        .accessibilityLabel("Minimize")
                    Spacer()
                    syncPill
                }
                HStack(alignment: .top, spacing: 12) {
                    programBlock
                    VStack(spacing: 8) {
                        controlButton("Previous", systemImage: "chevron.up", id: "portrait-previous") { step(-1) }
                            .accessibilityLabel("Previous channel")
                        controlButton("Next", systemImage: "chevron.down", id: "portrait-next") { step(1) }
                            .accessibilityLabel("Next channel")
                    }
                    .frame(width: 88)
                }
                Spacer(minLength: 0)
                if showStream {
                    StreamPanel(stream: live.session?.stream, stats: live.picture, sync: live.sync)
                }
                if showGuide {
                    miniGuide
                }
                controlGrid
            }
            .padding(.horizontal)
            .padding(.top, 8)
            .padding(.bottom, 12)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        }

        private var programBlock: some View {
            VStack(alignment: .leading, spacing: 2) {
                if let channel = nowPlaying.channel {
                    let airing = store.index.on(channel.id, at: store.now)
                    Text("\(channel.displayNumber)  \(channel.displayName)")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(.white.opacity(0.86))
                    Text(airing?.title ?? "No listing")
                        .font(.title2.weight(.bold))
                        .lineLimit(2)
                        .fixedSize(horizontal: false, vertical: true)
                    if let airing {
                        Text(airing.minutesLeft(at: store.now))
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("portrait-program")
        }

        private var miniGuide: some View {
            ScrollView {
                VStack(spacing: 0) {
                    ForEach(store.channels) { channel in
                        let airing = store.index.on(channel.id, at: store.now)
                        let title = airing?.title ?? "No listing"
                        Button {
                            nowPlaying.channel = channel
                            showGuide = false
                        } label: {
                            HStack(alignment: .firstTextBaseline, spacing: 10) {
                                Text(channel.displayNumber)
                                    .font(.headline.monospacedDigit())
                                    .frame(width: 52, alignment: .leading)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(channel.displayName).font(.subheadline.weight(.semibold)).lineLimit(1)
                                    Text(title).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                                }
                                Spacer(minLength: 0)
                                if channel.id == nowPlaying.channel?.id {
                                    Image(systemName: "checkmark").foregroundStyle(Tokens.ColorToken.tally)
                                }
                            }
                            .padding(.horizontal, 12)
                            .padding(.vertical, 8)
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel("\(channel.displayNumber) \(channel.displayName), \(title)")
                    }
                }
            }
            .frame(maxHeight: 280)
            .glassEffect(in: .rect(cornerRadius: Tokens.Radius.lg))
            .accessibilityLabel("Channels")
            .accessibilityIdentifier("portrait-guide")
        }

        private var controlGrid: some View {
            LazyVGrid(columns: [GridItem(.flexible(), spacing: 8), GridItem(.flexible(), spacing: 8), GridItem(.flexible(), spacing: 8)], spacing: 8) {
                controlButton(showGuide ? "Close guide" : "Channels", systemImage: "list.bullet", id: "portrait-channels") {
                    showGuide.toggle()
                }
                .accessibilityAddTraits(showGuide ? .isSelected : [])
                controlButton("Side by side", systemImage: "rectangle.split.2x1", id: "portrait-together") {
                    if let channel = nowPlaying.channel {
                        nowPlaying.watchTogether([channel])
                    }
                }
                recordControl
                audioControl
                controlButton(showStream ? "Hide stream" : "Stream", systemImage: "info.circle", id: "portrait-stream") {
                    showStream.toggle()
                }
            }
        }

        private var recordControl: some View {
            let recording = nowPlaying.channel.flatMap { store.activeRecording(on: $0) } != nil
            return controlButton(recording ? "Recording" : "Record", systemImage: recording ? "record.circle.fill" : "record.circle", id: "portrait-record") {
                if let channel = nowPlaying.channel {
                    Task { await store.toggleRecord(channel) }
                }
            }
            .foregroundStyle(Tokens.ColorToken.tally)
            .accessibilityLabel(recording ? "Stop recording" : "Record")
        }

        private var audioControl: some View {
            Menu {
                ForEach(audioMenu) { entry in
                    Button(action: entry.action) {
                        if entry.current {
                            Label(entry.title, systemImage: "checkmark")
                        } else {
                            Text(entry.title)
                        }
                    }
                }
            } label: {
                Label("Audio", systemImage: "speaker.wave.2")
                    .labelStyle(StackedControlLabel())
            }
            .buttonStyle(.glass)
            .accessibilityLabel("Audio")
            .accessibilityIdentifier("portrait-audio")
        }

        private func controlButton(_ title: String, systemImage: String, id: String, action: @escaping () -> Void) -> some View {
            Button(action: action) {
                Label(title, systemImage: systemImage)
                    .labelStyle(StackedControlLabel())
            }
            .buttonStyle(.glass)
            .accessibilityLabel(title)
            .accessibilityIdentifier(id)
        }

        private var landscapeBar: some View {
            HStack(spacing: 10) {
                Button("Minimize", systemImage: "chevron.down") { nowPlaying.expanded = false }
                    .labelStyle(.iconOnly)
                    .buttonStyle(.glass)
                    .accessibilityLabel("Minimize")
                Spacer()
                syncPill
                Button(showStream ? "Hide stream" : "Stream", systemImage: "info.circle") { showStream.toggle() }
                    .labelStyle(.iconOnly)
                    .buttonStyle(.glass)
                    .accessibilityLabel(showStream ? "Hide stream" : "Stream")
                Button("Side by side", systemImage: "rectangle.split.2x1") {
                    if let channel = nowPlaying.channel {
                        nowPlaying.watchTogether([channel])
                    }
                }
                .labelStyle(.iconOnly)
                .buttonStyle(.glass)
                .accessibilityLabel("Side by side")
                Menu("Audio", systemImage: "speaker.wave.2") {
                    ForEach(audioMenu) { entry in
                        Button(action: entry.action) {
                            if entry.current {
                                Label(entry.title, systemImage: "checkmark")
                            } else {
                                Text(entry.title)
                            }
                        }
                    }
                }
                .labelStyle(.iconOnly)
                .buttonStyle(.glass)
                .accessibilityLabel("Audio")
                GlassEffectContainer {
                    HStack(spacing: 6) {
                        Button("Previous channel", systemImage: "chevron.up") { step(-1) }
                        Button("Next channel", systemImage: "chevron.down") { step(1) }
                    }
                    .labelStyle(.iconOnly)
                    .buttonStyle(.glass)
                }
                if let channel = nowPlaying.channel {
                    let recording = store.activeRecording(on: channel) != nil
                    Button(recording ? "Stop recording" : "Record", systemImage: recording ? "record.circle.fill" : "record.circle") {
                        Task { await store.toggleRecord(channel) }
                    }
                    .labelStyle(.iconOnly)
                    .buttonStyle(.glass)
                    .foregroundStyle(Tokens.ColorToken.tally)
                    .accessibilityLabel(recording ? "Stop recording" : "Record")
                }
            }
            .padding(.horizontal)
            .padding(.top, 8)
        }

        @ViewBuilder private var syncPill: some View {
            if let sync = live.sync, sync.state != .off {
                HStack(spacing: 5) {
                    Image(systemName: "arrow.triangle.2.circlepath")
                    if sync.members > 1 {
                        Text("\(sync.members)").monospacedDigit()
                    }
                }
                .font(.footnote.weight(.bold))
                .fixedSize()
                .padding(.horizontal, 12)
                .padding(.vertical, 9)
                .glassEffect(.regular.tint(sync.state == .locked ? Tokens.ColorToken.success.opacity(0.4) : nil))
                .accessibilityLabel(sync.members > 1 ? "Synced with \(sync.members) screens" : "Synced")
            }
        }
    #endif
}

#if os(iOS)
    /// Icon over a short name. Portrait controls have to be readable without VoiceOver.
    private struct StackedControlLabel: LabelStyle {
        func makeBody(configuration: Configuration) -> some View {
            VStack(spacing: 4) {
                configuration.icon
                configuration.title
                    .font(.caption2.weight(.semibold))
                    .lineLimit(2)
                    .multilineTextAlignment(.center)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 8)
        }
    }
#endif

struct ChannelMenuEntry: Identifiable {
    let id: Int64
    let title: String
    let current: Bool
    let action: () -> Void
}

/// AVPlayerViewController: system PiP, AirPlay, captions, audio tracks, and Now Playing.
struct SystemPlayer: UIViewControllerRepresentable {
    let player: AVPlayer
    var hint = DisplayMatch()
    var menu: [ChannelMenuEntry] = []
    var audio: [ChannelMenuEntry] = []
    var streamOn = false
    var onStream: () -> Void = {}
    var onTogether: () -> Void = {}

    func makeCoordinator() -> Coordinator {
        Coordinator()
    }

    func makeUIViewController(context: Context) -> AVPlayerViewController {
        let vc = AVPlayerViewController()
        vc.player = player
        vc.allowsPictureInPicturePlayback = true
        #if os(iOS)
            vc.canStartPictureInPictureAutomaticallyFromInline = true
        #endif
        #if os(tvOS)
            vc.appliesPreferredDisplayCriteriaAutomatically = false
            context.coordinator.start(vc)
        #endif
        return vc
    }

    #if os(tvOS)
        static func dismantleUIViewController(_ vc: AVPlayerViewController, coordinator: Coordinator) {
            coordinator.stop()
            vc.view.window?.avDisplayManager.preferredDisplayCriteria = nil
        }

        /// SDR 8-bit at the asset's size. Broadcasts are not HDR.
        private static func displayCriteria(_ match: DisplayMatch, format: CMFormatDescription?) -> AVDisplayCriteria? {
            guard PlayerTuning.displayReady(match) else { return nil }
            if let format {
                let dims = CMVideoFormatDescriptionGetDimensions(format)
                if Int(dims.width) == match.width, Int(dims.height) == match.height {
                    return AVDisplayCriteria(refreshRate: match.refreshRate, formatDescription: format)
                }
            }
            var desc: CMFormatDescription?
            let status = CMVideoFormatDescriptionCreate(
                allocator: kCFAllocatorDefault,
                codecType: kCMVideoCodecType_H264,
                width: Int32(match.width),
                height: Int32(match.height),
                extensions: nil,
                formatDescriptionOut: &desc
            )
            guard status == noErr, let desc else { return nil }
            return AVDisplayCriteria(refreshRate: match.refreshRate, formatDescription: desc)
        }
    #endif

    func updateUIViewController(_ vc: AVPlayerViewController, context: Context) {
        if vc.player !== player {
            vc.player = player
        }
        #if os(tvOS)
            context.coordinator.start(vc)
            context.coordinator.noteHint(hint, on: vc)
            context.coordinator.sample(vc)
            let actions = menu.map { entry in
                UIAction(title: entry.title, state: entry.current ? .on : .off) { _ in entry.action() }
            }
            let audioActions = audio.map { entry in
                UIAction(title: entry.title, state: entry.current ? .on : .off) { _ in entry.action() }
            }
            let together = UIAction(title: "Side by side", image: UIImage(systemName: "rectangle.split.2x1")) { _ in onTogether() }
            let stream = UIAction(title: streamOn ? "Hide stream" : "Stream", image: UIImage(systemName: "info.circle"), state: streamOn ? .on : .off) { _ in onStream() }
            let audioMenu = UIMenu(title: "Audio", image: UIImage(systemName: "speaker.wave.2"), children: audioActions)
            vc.transportBarCustomMenuItems = [UIMenu(title: "Channels", image: UIImage(systemName: "list.bullet"), children: actions), audioMenu, stream, together]
        #endif
    }

    /// Reads the current item until its size and rate show up, then sets
    /// Match Frame Rate. An early 59.94 is not applied, and a closed player
    /// clears the mode while the window still exists.
    @MainActor
    final class Coordinator {
        #if os(tvOS)
            private var task: Task<Void, Never>?
            private var match = DisplayMatch()
            private var applied: DisplayMatch?
            private var format: CMFormatDescription?
            private var logged = ""

            func start(_ vc: AVPlayerViewController) {
                guard task == nil else { return }
                task = Task { @MainActor in
                    while !Task.isCancelled {
                        await refresh(vc)
                        try? await Task.sleep(for: .milliseconds(500))
                    }
                }
            }

            func stop() {
                task?.cancel()
                task = nil
                match = DisplayMatch()
                applied = nil
                format = nil
            }

            func sample(_ vc: AVPlayerViewController) {
                guard vc.player?.currentItem == nil else { return }
                match = DisplayMatch()
                format = nil
                apply(vc)
            }

            private func refresh(_ vc: AVPlayerViewController) async {
                guard let item = vc.player?.currentItem else {
                    match = DisplayMatch()
                    format = nil
                    apply(vc)
                    return
                }
                let picture = await videoPicture(item)
                format = picture.format
                match = PlayerTuning.matchingDisplay(
                    width: picture.width,
                    height: picture.height,
                    rate: picture.rate,
                    previous: match
                )
                apply(vc)
            }

            func noteHint(_ hint: DisplayMatch, on vc: AVPlayerViewController) {
                let next = PlayerTuning.hintedDisplay(hint, current: match)
                guard next != match else { return }
                match = next
                apply(vc)
            }

            private func apply(_ vc: AVPlayerViewController) {
                guard let manager = vc.view.window?.avDisplayManager else {
                    applied = nil
                    return
                }
                guard let criteria = SystemPlayer.displayCriteria(match, format: format) else {
                    if applied != nil {
                        manager.preferredDisplayCriteria = nil
                        applied = nil
                        logDisplay("clear")
                    }
                    return
                }
                if applied == match {
                    return
                }
                manager.preferredDisplayCriteria = criteria
                applied = match
                logDisplay("\(match.width)x\(match.height) \(match.refreshRate)")
            }

            private func logDisplay(_ message: String) {
                guard message != logged else { return }
                logged = message
                print("broadwave display \(message)")
                fflush(stdout)
            }
        #endif
    }
}

private func displayHint(_ stream: StreamInfo?) -> DisplayMatch {
    DisplayMatch(
        width: stream?.outputWidth ?? 0,
        height: stream?.outputHeight ?? 0,
        refreshRate: fpsValue(stream?.outputFps)
    )
}

private func fpsValue(_ text: String?) -> Float {
    guard let text, let value = Float(text), value > 1 else { return 0 }
    return value
}

private struct VideoPicture {
    var width = 0
    var height = 0
    var rate: Float = 0
    var format: CMFormatDescription?
}

@MainActor
private func videoPicture(_ item: AVPlayerItem) async -> VideoPicture {
    var picture = VideoPicture()
    let tracks = await (try? item.asset.loadTracks(withMediaType: .video)) ?? []
    if let asset = tracks.first {
        let rate = await (try? asset.load(.nominalFrameRate)) ?? 0
        if rate > 1 {
            picture.rate = rate
        }
        let formats = await (try? asset.load(.formatDescriptions)) ?? []
        if let format = formats.first {
            let dims = CMVideoFormatDescriptionGetDimensions(format)
            if dims.width > 1, dims.height > 1 {
                picture.width = Int(dims.width)
                picture.height = Int(dims.height)
                picture.format = format
            }
        }
    }
    if picture.width <= 1 {
        let size = item.presentationSize
        if size.width > 1, size.height > 1 {
            picture.width = Int(size.width.rounded())
            picture.height = Int(size.height.rounded())
        }
    }
    return picture
}

struct StreamPanel: View {
    let stream: StreamInfo?
    let stats: PictureStats
    let sync: SyncEngine?

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Stream").font(.headline)
            line("Source", sourceText)
            line("Output", outputText)
            line("Dropped frames", "\(stats.dropped)")
            line("Buffer", String(format: "%.1fs", stats.buffer))
            line("Sync", syncText)
        }
        .padding(16)
        .frame(maxWidth: 420, alignment: .leading)
        .glassEffect(in: .rect(cornerRadius: Tokens.Radius.lg))
        .accessibilityElement(children: .combine)
    }

    private func line(_ name: String, _ value: String) -> some View {
        HStack(alignment: .firstTextBaseline) {
            Text(name).foregroundStyle(.secondary)
            Spacer(minLength: 12)
            Text(value).fontWeight(.semibold).multilineTextAlignment(.trailing)
        }
        .font(.footnote)
    }

    private var sourceText: String {
        guard let stream else { return "Waiting" }
        return joinFacts([
            stream.sourceVideo,
            sizeText(stream.sourceWidth, stream.sourceHeight),
            scanWord(stream.scan),
            stream.sourceFps,
        ])
    }

    private var outputText: String {
        guard let stream else { return "Waiting" }
        let width = stats.width > 0 ? stats.width : stream.outputWidth
        let height = stats.height > 0 ? stats.height : stream.outputHeight
        let fps = stream.outputFps ?? (stats.fps > 1 ? String(format: "%.2f", stats.fps) : nil)
        let decode = stream.decode == "gpu" ? "GPU" : stream.decode == "cpu" ? "CPU" : stream.video == "copy" ? "Direct" : nil
        return joinFacts([sizeText(width, height), fps, stream.encoder, bitrateText(stream.bitrate), decode])
    }

    private var syncText: String {
        guard let sync, sync.state != .off else { return "Off" }
        let raw = sync.state.rawValue
        let word = raw.prefix(1).uppercased() + raw.dropFirst()
        return "\(word) · \(Int(sync.drift.rounded())) ms"
    }
}

private func sizeText(_ width: Int?, _ height: Int?) -> String? {
    guard let width, let height, width > 0, height > 0 else { return nil }
    return "\(width)×\(height)"
}

private func scanWord(_ scan: String?) -> String? {
    switch scan {
    case "progressive": "Progressive"
    case "interlaced": "Interlaced"
    case "film": "Film"
    default: nil
    }
}

private func bitrateText(_ rate: String?) -> String? {
    guard let rate, !rate.isEmpty else { return nil }
    if rate.hasSuffix("M") {
        return String(rate.dropLast()) + " Mb/s"
    }
    if rate.hasSuffix("k") {
        return String(rate.dropLast()) + " kb/s"
    }
    return rate
}

private func joinFacts(_ parts: [String?]) -> String {
    let line = parts.compactMap(\.self).filter { !$0.isEmpty }.joined(separator: " · ")
    return line.isEmpty ? "Waiting" : line
}

struct RecordingPlayerScreen: View {
    @Environment(AppStore.self) private var store
    let recording: Recording
    @State private var player = AVPlayer()
    @State private var error: String?

    var body: some View {
        SystemPlayer(player: player)
            .ignoresSafeArea()
            .overlay {
                if let error {
                    Text(error).padding().glassEffect()
                }
            }
            .task {
                guard let api = store.api else { return }
                do {
                    let start = try await api.play(recordingID: recording.id)
                    let item = AVPlayerItem(url: api.url(start.playlist))
                    PlayerTuning.apply(item, network: Capabilities.current().network ?? "lan", tile: false)
                    player.replaceCurrentItem(with: item)
                    if start.position > 5 {
                        await player.seek(to: CMTime(seconds: start.position, preferredTimescale: 600))
                    }
                    player.play()
                } catch {
                    self.error = error.localizedDescription
                }
            }
            .onDisappear {
                let pos = player.currentTime().seconds
                player.pause()
                if let api = store.api, pos.isFinite, pos > 0 {
                    Task { await api.saveProgress(recordingID: recording.id, position: pos) }
                }
            }
        #if os(iOS)
            .toolbarVisibility(.hidden, for: .tabBar)
        #endif
    }
}
