import AVFoundation
import AVKit
import BroadwaveKit
import BroadwaveUI
import CoreMedia
import os
import SwiftUI
#if os(iOS)
    import UIKit
#endif

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
    /// False from a new watch until the picture has moved 0.3 s. A new room
    /// holds its first frame for the start, and that is not a picture yet.
    private(set) var moving = false
    private var movingFrom: Double?
    private(set) var stalls = 0
    private(set) var stallMs = 0
    private var stallStarted: Date?
    private var lastBeat = Date()
    /// Nominal frame rate once the asset reports it. Zero until then.
    private(set) var refreshRate: Float = 0
    private(set) var picture = PictureStats()
    private var statsTask: Task<Void, Never>?
    /// The watch still waiting for its first segment. A newer channel cancels it
    /// so that tuner is not held until the request times out.
    private var watchTask: Task<WatchSession, Error>?
    private var watchToken = 0
    private let outage = ServerWatch()
    private var outageSampled = Date.distantPast
    /// Set for a quiet retry of a stopped picture. The message stays until the new picture moves.
    private var quietRetry = false
    private var holdPicture = false
    private let playLog = Logger(subsystem: "com.wolfeup.broadwave", category: "play")
    #if os(iOS)
        private var lifecycle: [NSObjectProtocol] = []
    #endif

    func playLogNote(_ message: String) {
        playLog.info("\(message, privacy: .public)")
        print("broadwave \(message)")
        fflush(stdout)
    }

    func start(_ channel: Channel, store: AppStore) async {
        watchToken += 1
        let token = watchToken
        watchTask?.cancel()
        let quiet = quietRetry && channelID == channel.id
        quietRetry = false
        let retry = channelID == channel.id
        let held = retry ? error : nil
        holdPicture = false
        await stop(endPicture: !quiet)
        guard !Task.isCancelled, token == watchToken else { return }
        guard let api = store.api else { return }
        self.api = api
        channelID = channel.id
        let allow = confirmNext
        confirmNext = false
        needsConfirm = false
        error = held
        let client = api
        let id = channel.id
        outage.bind(
            snap: { assumeLost, playlist in
                await playbackSnap(api: client, channelID: id, assumeLost: assumeLost, playlist: playlist)
            },
            onMessage: { [weak self] decision in self?.showOutage(decision) },
            onRecover: { [weak self] quiet in
                guard let self else { return }
                quietRetry = quiet
                attempt += 1
            }
        )
        watchLifecycle()
        let caps = Capabilities.current()
        let prefs = store.prefs
        let task = Task { try await api.watch(channelID: channel.id, caps: caps, prefs: prefs, confirmLive: allow) }
        watchTask = task
        defer {
            if watchToken == token {
                watchTask = nil
            }
        }
        do {
            let session = try await withTaskCancellationHandler {
                try await task.value
            } onCancel: {
                task.cancel()
            }
            guard !Task.isCancelled, token == watchToken, channelID == channel.id else {
                await api.stopWatching(channelID: channel.id, rendition: session.rendition)
                return
            }
            self.session = session
            outage.watching(session.playlist)
            if quiet {
                holdPicture = true
            } else {
                error = nil
                holdPicture = false
            }
            let item = AVPlayerItem(url: api.url(session.playlist))
            item.externalMetadata = metadata(channel: channel, airing: store.index.on(channel.id, at: Date()))
            PlayerTuning.apply(item, network: Capabilities.current().network ?? "lan", tile: false)
            player.replaceCurrentItem(with: item)
            player.automaticallyWaitsToMinimizeStalling = true
            watchStartup(item)
            player.play()
            watchPicture()
            playLog.info("channel \(channel.displayNumber, privacy: .public)")
            if store.syncEnabled, Compatibility.gateFeature(store.info, "wholeHomeSync") == nil, let socket = store.socket {
                let engine = SyncEngine(player: player, socket: socket, room: "channel:\(channel.id)", channelID: channel.id)
                engine.start()
                sync = engine
            }
        } catch is CancellationError {
            return
        } catch let error as APIError where error.code == "recording_soon" {
            guard token == watchToken, channelID == channel.id else { return }
            if quiet {
                outage.endPictureRetry()
            }
            needsConfirm = true
            self.error = error.message
        } catch let error as URLError {
            guard token == watchToken, channelID == channel.id, error.code != .cancelled else { return }
            if quiet {
                outage.endPictureRetry()
            }
            needsConfirm = false
            outage.failToReach(online: PlaybackOutage.deviceOnline(error))
        } catch let error as APIError {
            guard token == watchToken, channelID == channel.id else { return }
            needsConfirm = false
            let decision = PlaybackOutage.viewerFailure(code: error.code, status: error.status, message: error.message, online: true)
            if quiet, PlaybackOutage.holdPictureMessage(decision) {
                outage.pictureRetryFailed()
                return
            }
            if quiet {
                outage.endPictureRetry()
            }
            if decision.recovery != nil {
                outage.fail(decision)
            } else if retry || decision.message == PlaybackOutage.pictureStopped, PlaybackOutage.holdPictureMessage(decision) {
                // The stream's own words ("503") are not a cause. Keep asking.
                outage.fail(OutageDecision(message: PlaybackOutage.pictureStopped, recovery: nil))
            } else {
                self.error = decision.message
            }
        } catch {
            guard token == watchToken, channelID == channel.id else { return }
            needsConfirm = false
            let decision = OutageDecision(message: PlaybackOutage.viewerMessage(error.localizedDescription), recovery: nil)
            if quiet, PlaybackOutage.holdPictureMessage(decision) {
                outage.pictureRetryFailed()
                return
            }
            if quiet {
                outage.endPictureRetry()
            }
            if retry, PlaybackOutage.holdPictureMessage(decision) {
                outage.fail(OutageDecision(message: PlaybackOutage.pictureStopped, recovery: nil))
            } else {
                self.error = decision.message
            }
        }
    }

    /// A recoverable outage clears the item so the next start owns the player.
    /// The screen stays up, and the message stays until that start has a picture.
    private func showOutage(_ decision: OutageDecision) {
        error = decision.message
        guard decision.recovery != nil else { return }
        sync?.stop()
        sync = nil
        player.pause()
        player.replaceCurrentItem(with: nil)
    }

    func confirmWatch() {
        confirmNext = true
        needsConfirm = false
        error = nil
        attempt += 1
    }

    /// The viewer's own Try again. A new two minutes starts if the picture stops again.
    func retry() {
        quietRetry = false
        attempt += 1
    }

    func stop(endPicture: Bool = true) async {
        watchTask?.cancel()
        watchTask = nil
        statsTask?.cancel()
        statsTask = nil
        picture = PictureStats()
        moving = false
        movingFrom = nil
        if endPicture {
            outage.reset()
            holdPicture = false
        } else {
            outage.resetStall()
        }
        #if os(iOS)
            for token in lifecycle {
                NotificationCenter.default.removeObserver(token)
            }
            lifecycle = []
        #endif
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
        let id = channelID
        let ended = session
        channelID = nil
        session = nil
        if let api, let id, let ended {
            await api.stopWatching(channelID: id, rendition: ended.rendition)
        }
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
                // The playhead observer does not fire while the picture is stuck,
                // so a dead server would never be named. Two samples a few ms apart
                // would read as a stuck picture, so only sample when it has gone quiet.
                if player.currentItem != nil, Date().timeIntervalSince(outageSampled) > 0.9 {
                    sampleOutage()
                }
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
        moving = false
        movingFrom = nil
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
                self.noteMoving()
                if let start = self.stallStarted, self.player.timeControlStatus == .playing {
                    self.stallMs += Int(Date().timeIntervalSince(start) * 1000)
                    self.stallStarted = nil
                    print("broadwave stall-ms \(self.stallMs)")
                }
                if self.firstFrameMs != nil, Date().timeIntervalSince(self.lastBeat) >= 60 {
                    self.lastBeat = Date()
                    print("broadwave beat stalls=\(self.stalls) stallMs=\(self.stallMs)")
                }
                if self.player.currentItem != nil {
                    self.sampleOutage()
                }
            }
        }
    }

    /// Logs resign, background, and the return so a simulator soak can see a
    /// lock or a home press. Only when `-BroadwaveSyncLog 1` is set.
    private func watchLifecycle() {
        #if os(iOS)
            guard lifecycle.isEmpty, UserDefaults.standard.bool(forKey: "BroadwaveSyncLog") else { return }
            let center = NotificationCenter.default
            let names: [(Notification.Name, String)] = [
                (UIApplication.willResignActiveNotification, "resign"),
                (UIApplication.didBecomeActiveNotification, "active"),
                (UIApplication.didEnterBackgroundNotification, "background"),
                (UIApplication.willEnterForegroundNotification, "foreground"),
            ]
            for (name, label) in names {
                lifecycle.append(center.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
                    Task { @MainActor in
                        self?.playLogNote("lifecycle \(label)")
                    }
                })
            }
        #endif
    }

    private func noteMoving() {
        guard !moving else { return }
        let now = player.currentTime().seconds
        guard player.timeControlStatus == .playing, now.isFinite else {
            movingFrom = nil
            return
        }
        guard let from = movingFrom else {
            movingFrom = now
            return
        }
        if now - from >= 0.3 {
            moving = true
            print("broadwave moving \(Int(Date().timeIntervalSince(started) * 1000))ms")
            fflush(stdout)
        }
    }

    private func sampleOutage() {
        outageSampled = Date()
        let item = player.currentItem
        outage.note(
            time: item?.currentTime().seconds,
            waiting: player.timeControlStatus == .waitingToPlayAtSpecifiedRate,
            paused: player.timeControlStatus == .paused,
            failed: item?.status == .failed
        )
        notePictureMoved()
    }

    /// The quiet message stays until this watch is actually playing. The next
    /// sample is too late: a sync hold pauses the item before the playhead moves.
    private func notePictureMoved() {
        guard holdPicture, player.timeControlStatus == .playing else { return }
        error = nil
        holdPicture = false
        outage.endPictureRetry()
        print("broadwave picture-back")
        fflush(stdout)
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

    private var tuning: Bool {
        #if DEBUG
            // Layout checks never start a picture.
            if UserDefaults.standard.bool(forKey: "BroadwaveChrome") {
                return false
            }
        #endif
        return live.error == nil && !live.moving
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
                },
                rejoin: live.sync?.detached == true ? { live.sync?.rejoin() } : nil
            )
            .ignoresSafeArea()
            if tuning, let channel = nowPlaying.channel {
                TuningCard(channel: channel, show: store.index.on(channel.id, at: Date())?.title)
                    .id(channel.id)
            }
            #if os(iOS)
                overlay
            #endif
            #if os(tvOS) && DEBUG
                // Store shots need the title on screen. The system bar hides itself.
                if UserDefaults.standard.bool(forKey: "BroadwaveInfo") {
                    tvInfo
                }
                // UI tests cannot reach the transport bar's menu; this presses
                // "Back in sync" 10 s after the viewer leaves sync.
                if UserDefaults.standard.bool(forKey: "BroadwaveRejoinTest") {
                    // The test reads the engine here instead of trusting that a press landed.
                    Text("\(live.sync?.state.rawValue ?? "none") detached=\(live.sync?.detached == true)")
                        .font(.system(size: 2))
                        .foregroundStyle(.clear)
                        .accessibilityIdentifier("syncProbe")
                    if live.sync?.detached == true {
                        Color.clear.task {
                            try? await Task.sleep(for: .seconds(10))
                            live.sync?.rejoin()
                        }
                    }
                }
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
                        .accessibilityIdentifier("playback-outage")
                    if live.needsConfirm {
                        Button("Watch anyway") {
                            live.confirmWatch()
                        }
                        .buttonStyle(.borderedProminent)
                    } else {
                        Button("Try again") {
                            live.retry()
                        }
                        .buttonStyle(.borderedProminent)
                    }
                }
                .padding()
                .glassEffect(in: .rect(cornerRadius: Tokens.Radius.md))
                .padding(.top, 80)
            }
        }
        #if os(iOS)
        .onChange(of: verticalSize) { _, size in
            guard UserDefaults.standard.bool(forKey: "BroadwaveSyncLog") else { return }
            live.playLogNote("size \(size == .compact ? "landscape" : "portrait")")
        }
        #endif
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
        #if DEBUG
            #if os(iOS)
                // The host writes portrait or landscape. Rotating the Simulator window would
                // steal the frontmost device, which may not be this one.
                .task {
                    guard UserDefaults.standard.bool(forKey: "BroadwaveExercise") else { return }
                    await watchExercise()
                }
            #endif
                // Keystrokes go to whichever simulator is in front. A file steps the channel on this one.
                .task {
                    guard UserDefaults.standard.bool(forKey: "BroadwaveChannelZap") else { return }
                    let file = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
                        .appendingPathComponent("broadwave-zap.txt")
                    live.playLogNote("zap-file \(file.path)")
                    var seen = ""
                    while !Task.isCancelled {
                        try? await Task.sleep(for: .milliseconds(300))
                        guard let text = try? String(contentsOf: file, encoding: .utf8) else { continue }
                        let line = text.trimmingCharacters(in: .whitespacesAndNewlines)
                        if line.isEmpty || line == seen {
                            continue
                        }
                        seen = line
                        let verb = line.split(separator: " ").first.map(String.init) ?? line
                        if verb == "up" {
                            step(-1)
                        } else if verb == "down" {
                            step(1)
                        }
                        live.playLogNote("zap \(line) \(nowPlaying.channel?.displayNumber ?? "")")
                    }
                }
        #endif
    }

    #if os(iOS)
        private func watchExercise() async {
            let file = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
                .appendingPathComponent("broadwave-exercise.txt")
            live.playLogNote("exercise-file \(file.path)")
            var seen = ""
            while !Task.isCancelled {
                try? await Task.sleep(for: .milliseconds(300))
                guard let text = try? String(contentsOf: file, encoding: .utf8) else { continue }
                let line = text.trimmingCharacters(in: .whitespacesAndNewlines)
                if line.isEmpty || line == seen {
                    continue
                }
                seen = line
                let verb = line.split(separator: " ").first.map(String.init) ?? line
                let mask: UIInterfaceOrientationMask
                if verb == "landscape" {
                    mask = .landscapeRight
                } else if verb == "portrait" {
                    mask = .portrait
                } else {
                    continue
                }
                guard let scene = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene }).first else {
                    live.playLogNote("exercise \(verb) no-scene")
                    continue
                }
                scene.requestGeometryUpdate(.iOS(interfaceOrientations: mask)) { error in
                    print("broadwave exercise \(verb) \(error.localizedDescription)")
                }
                live.playLogNote("exercise \(verb)")
            }
        }
    #endif

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

    #if os(tvOS) && DEBUG
        /// Channel, title, and how far the show has run. Stays up for a screenshot.
        private var tvInfo: some View {
            VStack(alignment: .leading, spacing: 10) {
                if let channel = nowPlaying.channel {
                    let airing = store.index.on(channel.id, at: store.now)
                    Text("\(channel.displayNumber)  \(channel.displayName)")
                        .font(.headline)
                        .accessibilityIdentifier("player-chrome")
                    Text(airing?.title ?? "No listing")
                        .font(.title2.weight(.bold))
                        .lineLimit(1)
                    if let airing {
                        Text(airing.minutesLeft(at: store.now))
                            .font(.callout)
                            .foregroundStyle(.secondary)
                        let done = airing.progress(at: store.now)
                        GeometryReader { geo in
                            ZStack(alignment: .leading) {
                                Capsule().fill(.white.opacity(0.28))
                                Capsule().fill(.white).frame(width: max(4, geo.size.width * done))
                            }
                        }
                        .frame(height: 8)
                        .accessibilityLabel("Progress")
                    }
                }
            }
            .padding(28)
            .frame(width: 640, alignment: .leading)
            .background(.black.opacity(0.62), in: .rect(cornerRadius: 24))
            .padding(56)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomLeading)
        }
    #endif

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
                                    Image(systemName: "checkmark")
                                        .foregroundStyle(Tokens.ColorToken.tally)
                                        .accessibilityHidden(true)
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
            if let sync = live.sync, sync.detached {
                Button("Back in sync", systemImage: "arrow.triangle.2.circlepath") { sync.rejoin() }
                    .labelStyle(.iconOnly)
                    .buttonStyle(.glass)
                    .accessibilityLabel("Back in sync")
            } else if let sync = live.sync, sync.state != .off {
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
                .accessibilityElement(children: .ignore)
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
                // The symbol's own name ("Volume High") would be read beside the control's name.
                configuration.icon.accessibilityHidden(true)
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

/// A recording's menu entry: an action, or a menu of actions when it has children.
struct FileMenuEntry: Identifiable {
    let id: String
    let title: String
    let symbol: String
    /// What VoiceOver reads when the title leans on its menu, as a break's times do.
    var spoken: String?
    var children: [FileMenuEntry] = []
    var action: () -> Void = {}

    #if os(tvOS)
        @MainActor var element: UIMenuElement {
            if children.isEmpty {
                let item = UIAction(title: title, image: UIImage(systemName: symbol)) { _ in action() }
                item.accessibilityLabel = spoken
                return item
            }
            return UIMenu(title: title, image: UIImage(systemName: symbol), children: children.map(\.element))
        }
    #endif
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
    /// Set while this screen has left sync; the menu offers the way back.
    var rejoin: (() -> Void)?
    /// Set while a recording is in a commercial break the viewer skips by hand.
    var skipBreak: (() -> Void)?
    /// A recording has no channels, audio picks, stream panel, or side by side.
    var liveMenu = true
    /// A recording's own entries, shown instead of the live menu.
    var fileMenu: [FileMenuEntry] = []

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
            // The action calls the newest closure, so moving from one break to the next skips the right one.
            let coordinator = context.coordinator
            coordinator.skip = skipBreak
            if (skipBreak != nil) != coordinator.skipShown {
                coordinator.skipShown = skipBreak != nil
                vc.contextualActions = skipBreak == nil ? [] : [
                    UIAction(title: "Skip break", image: UIImage(systemName: "forward.end")) { [weak coordinator] _ in coordinator?.skip?() },
                ]
            }
            guard liveMenu else {
                let key = fileMenu.flatMap { entry in ["\(entry.id) \(entry.title)"] + entry.children.map { "\($0.id) \($0.title)" } }
                guard key != context.coordinator.menuKey else { return }
                context.coordinator.menuKey = key
                vc.transportBarCustomMenuItems = fileMenu.map(\.element)
                return
            }
            // Setting the items redraws the transport bar and keeps it on screen,
            // so a view update that changes nothing must leave them alone.
            let key = (menu + audio).map { "\($0.id) \($0.title) \($0.current)" } + ["\(streamOn)", "\(rejoin != nil)"]
            guard key != context.coordinator.menuKey else { return }
            context.coordinator.menuKey = key
            let actions = menu.map { entry in
                UIAction(title: entry.title, state: entry.current ? .on : .off) { _ in entry.action() }
            }
            let audioActions = audio.map { entry in
                UIAction(title: entry.title, state: entry.current ? .on : .off) { _ in entry.action() }
            }
            let together = UIAction(title: "Side by side", image: UIImage(systemName: "rectangle.split.2x1")) { _ in onTogether() }
            let stream = UIAction(title: streamOn ? "Hide stream" : "Stream", image: UIImage(systemName: "info.circle"), state: streamOn ? .on : .off) { _ in onStream() }
            let audioMenu = UIMenu(title: "Audio", image: UIImage(systemName: "speaker.wave.2"), children: audioActions)
            var items: [UIMenuElement] = [UIMenu(title: "Channels", image: UIImage(systemName: "list.bullet"), children: actions), audioMenu, stream, together]
            if let rejoin {
                items.insert(UIAction(title: "Back in sync", image: UIImage(systemName: "arrow.triangle.2.circlepath")) { _ in rejoin() }, at: 0)
            }
            vc.transportBarCustomMenuItems = items
        #endif
    }

    /// Reads the current item until its size and rate show up, then sets
    /// Match Frame Rate. An early 59.94 is not applied, and a closed player
    /// clears the mode while the window still exists.
    @MainActor
    final class Coordinator {
        #if os(tvOS)
            var menuKey: [String] = []
            var skipShown = false
            var skip: (() -> Void)?
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
                NotificationCenter.default.post(name: SyncEngine.displayWillChange, object: nil)
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

private struct TunerUsed: LocalizedError {
    var errorDescription: String? {
        "This library channel tried to use an antenna tuner."
    }
}

/// Plays a recording, or a library channel's recordings one after another. Neither uses a tuner.
struct RecordingPlayerScreen: View {
    enum Source {
        case recording(Recording)
        case library(VirtualChannel)
    }

    @Environment(AppStore.self) private var store
    let source: Source
    @State private var player = AVPlayer()
    @State private var error: String?
    @State private var note: String?
    /// Counts notes, so saying the same thing again shows it for the full time.
    @State private var noteCount = 0
    /// The recording on screen, and for a library channel its place in the channel.
    @State private var current: Recording?
    @State private var index = 0
    @State private var count = 1
    @State private var markers: [Marker] = []
    @State private var inBreak: Marker?
    /// The break just skipped. It is not skipped or offered again until playback leaves it,
    /// so a seek in flight does not bring the button back and a break that runs to the end
    /// of the file is not sought forever.
    @State private var passed: Int64?
    /// Where a break the viewer is marking by hand starts.
    @State private var breakStart: Double?
    @AppStorage(BreakSkip.key) private var skip = BreakSkip.auto

    init(recording: Recording) {
        source = .recording(recording)
    }

    init(channel: VirtualChannel) {
        source = .library(channel)
    }

    var body: some View {
        SystemPlayer(player: player, skipBreak: inBreak.map { marker in { Task { await skipPast(marker) } } }, liveMenu: false, fileMenu: menu)
            .ignoresSafeArea()
            .overlay {
                if let error {
                    Text(error).padding().glassEffect()
                }
            }
            .overlay(alignment: .top) {
                if let note {
                    Text(note)
                        .font(.subheadline)
                        .padding(.horizontal, 16)
                        .padding(.vertical, 10)
                        .glassEffect()
                        .padding(.top, 60)
                        .allowsHitTesting(false)
                }
            }
        #if os(iOS)
            .overlay(alignment: .bottomTrailing) {
                if let marker = inBreak {
                    Button("Skip break", systemImage: "forward.end") {
                        Task { await skipPast(marker) }
                    }
                    .buttonStyle(.glass)
                    .padding(.trailing, 24)
                    .padding(.bottom, 110)
                }
            }
            .toolbar {
                ForEach(menu.filter { $0.id == "next" }) { entry in
                    ToolbarItem(placement: .topBarTrailing) {
                        Button(entry.title, systemImage: entry.symbol) { entry.action() }
                    }
                }
                if menu.contains(where: { $0.id != "next" }) {
                    ToolbarItem(placement: .topBarTrailing) {
                        Menu("Breaks", systemImage: "scissors") {
                            ForEach(menu.filter { $0.id != "next" }) { entry in
                                if entry.children.isEmpty {
                                    Button(entry.title, systemImage: entry.symbol) { entry.action() }
                                } else {
                                    Menu(entry.title, systemImage: entry.symbol) {
                                        ForEach(entry.children) { child in
                                            Button(child.title) { child.action() }
                                                .accessibilityLabel(child.spoken ?? child.title)
                                        }
                                    }
                                }
                            }
                        }
                        .accessibilityIdentifier("recording-menu")
                    }
                }
            }
        #endif
            .task(id: index) {
                guard await load() else { return }
                while await (try? Task.sleep(for: .milliseconds(250))) != nil {
                    await followBreaks()
                }
            }
            .task(id: noteCount) {
                guard note != nil else { return }
                try? await Task.sleep(for: .seconds(4))
                if !Task.isCancelled {
                    note = nil
                }
            }
            .onReceive(NotificationCenter.default.publisher(for: AVPlayerItem.didPlayToEndTimeNotification).receive(on: DispatchQueue.main)) { sent in
                // A library channel goes on to its next recording, as the web does. The last one stays.
                guard case .library = source, (sent.object as? AVPlayerItem) === player.currentItem, index + 1 < count else { return }
                index += 1
            }
            .onDisappear {
                let pos = player.currentTime().seconds
                player.pause()
                if case let .recording(rec) = source, let api = store.api, pos.isFinite, pos > 0 {
                    Task { await api.saveProgress(recordingID: rec.id, position: pos) }
                }
            }
        #if os(iOS)
            .toolbarVisibility(.hidden, for: .tabBar)
        #endif
    }

    /// Starts the recording on screen. A library channel asks the server for the one at `index`.
    /// A load the viewer has already moved past changes nothing.
    private func load() async -> Bool {
        guard let api = store.api else { return false }
        passed = nil
        inBreak = nil
        breakStart = nil
        do {
            let rec: Recording
            let found: [Marker]
            let playlist: String
            var position = 0.0
            var total = 1
            switch source {
            case let .recording(one):
                let start = try await api.play(recordingID: one.id)
                (rec, found, playlist, position) = (one, start.markers ?? [], start.playlist, start.position)
            case let .library(channel):
                let start = try await api.playVirtual(channel.id, index: index)
                guard start.usesTuner != true else {
                    throw TunerUsed()
                }
                (rec, found, playlist, total) = (start.recording, start.markers ?? [], start.playlist, max(1, start.count))
            }
            guard !Task.isCancelled else { return false }
            error = nil
            current = rec
            markers = found
            count = total
            let item = AVPlayerItem(url: api.url(playlist))
            PlayerTuning.apply(item, network: Capabilities.current().network ?? "lan", tile: false)
            item.externalMetadata = metadata(rec)
            player.replaceCurrentItem(with: item)
            if position > 5 {
                await player.seek(to: CMTime(seconds: position, preferredTimescale: 600))
            }
            player.play()
            return true
        } catch {
            guard !Task.isCancelled else { return false }
            // Nothing half loaded stays behind the message: no picture, breaks, or menu.
            player.replaceCurrentItem(with: nil)
            current = nil
            markers = []
            self.error = PlaybackOutage.viewerMessage(error.localizedDescription)
            return false
        }
    }

    private func metadata(_ rec: Recording) -> [AVMetadataItem] {
        let title = AVMutableMetadataItem()
        title.identifier = .commonIdentifierTitle
        title.value = (rec.subtitle ?? rec.title) as NSString
        title.extendedLanguageTag = "und"
        let line = AVMutableMetadataItem()
        line.identifier = .iTunesMetadataTrackSubTitle
        if case let .library(channel) = source {
            line.value = "\(channel.number) \(channel.name)" as NSString
        } else {
            line.value = rec.guideNumber as NSString
        }
        line.extendedLanguageTag = "und"
        return [title, line]
    }

    /// Next recording, marking a break by hand, and removing one: the transport bar on
    /// Apple TV, a menu in the bar on iPhone and iPad.
    private var menu: [FileMenuEntry] {
        var out: [FileMenuEntry] = []
        // Next stays when a recording fails to load, so one missing file does not end the channel.
        if case .library = source, count > 1 {
            out.append(FileMenuEntry(id: "next", title: "Next recording", symbol: "forward.frame") { index = (index + 1) % count })
        }
        guard current != nil else { return out }
        if let breakStart {
            out.append(FileMenuEntry(id: "end", title: "Break ends here", symbol: "scissors") { Task { await markEnd(from: breakStart) } })
            out.append(FileMenuEntry(id: "cancel", title: "Stop marking", symbol: "xmark") { self.breakStart = nil })
        } else {
            out.append(FileMenuEntry(id: "start", title: "Break starts here", symbol: "scissors") { markStart() })
        }
        if !markers.isEmpty {
            let children = markers.sorted { $0.start < $1.start }.map { marker in
                FileMenuEntry(
                    id: "marker-\(marker.id)", title: marker.span, symbol: "trash",
                    spoken: "Remove the break from \(Marker.clock(marker.start)) to \(Marker.clock(marker.end))"
                ) { Task { await remove(marker) } }
            }
            out.append(FileMenuEntry(id: "remove", title: "Remove a break", symbol: "trash", children: children))
        }
        return out
    }

    private func markStart() {
        let time = player.currentTime().seconds
        guard time.isFinite else { return }
        breakStart = time
        say("Break starts at \(Marker.clock(time)). Play to where it ends, then choose \u{201C}Break ends here.\u{201D}")
    }

    private func markEnd(from start: Double) async {
        guard let api = store.api, let rec = current else { return }
        let time = player.currentTime().seconds
        guard time.isFinite else { return }
        let (from, to) = (min(start, time), max(start, time))
        guard to - from >= 1 else {
            say("A break needs at least a second. Play on, then choose \u{201C}Break ends here.\u{201D}")
            return
        }
        do {
            let made = try await api.addMarker(recordingID: rec.id, start: from, end: to)
            // A library channel may have moved on to its next recording meanwhile.
            guard current?.id == rec.id else { return }
            breakStart = nil
            markers.append(made)
            // The playhead may sit inside the new break when it was marked backward.
            passed = made.id
            say("Marked a break, \(made.span).")
        } catch {
            say(PlaybackOutage.actionMessage(error))
        }
    }

    private func remove(_ marker: Marker) async {
        guard let api = store.api, let rec = current else { return }
        do {
            try await api.deleteMarker(marker.id)
            guard current?.id == rec.id else { return }
            markers.removeAll { $0.id == marker.id }
            if inBreak?.id == marker.id {
                inBreak = nil
            }
            say("Removed the break at \(marker.span).")
        } catch {
            say(PlaybackOutage.actionMessage(error))
        }
    }

    private func say(_ text: String) {
        note = text
        noteCount += 1
        AccessibilityNotification.Announcement(text).post()
    }

    /// Skips a break, offers the skip, or plays it, as the Settings choice says.
    private func followBreaks() async {
        guard !markers.isEmpty, skip != .manual else {
            if inBreak != nil {
                inBreak = nil
            }
            return
        }
        let time = player.currentTime().seconds
        var hit = time.isFinite ? BreakSkip.marker(in: markers, at: time) : nil
        if hit?.id != passed {
            passed = nil
        } else {
            hit = nil
        }
        if let hit, skip == .auto {
            await skipPast(hit)
            return
        }
        if hit != inBreak {
            inBreak = hit
        }
    }

    private func skipPast(_ marker: Marker) async {
        passed = marker.id
        inBreak = nil
        var end = marker.end
        if let length = player.currentItem?.duration.seconds, length.isFinite {
            end = min(end, length)
        }
        await player.seek(to: CMTime(seconds: end, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero)
    }
}

/// Covers the player from a new watch until the picture moves, as the web
/// player does, so the frame a new room holds at its start does not look frozen.
private struct TuningCard: View {
    let channel: Channel
    let show: String?
    @State private var started = Date()

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()
            TimelineView(.periodic(from: started, by: 0.5)) { context in
                VStack(spacing: 10) {
                    Text("\(channel.displayNumber) \(channel.displayName)")
                        .font(.headline)
                    if let show, show != channel.displayName {
                        Text(show)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                    ProgressView()
                    Text("\(TuningSteps.text(after: context.date.timeIntervalSince(started)))…")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                .multilineTextAlignment(.center)
                .padding()
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("tuning")
            }
        }
        .allowsHitTesting(false)
    }
}
