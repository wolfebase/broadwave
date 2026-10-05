import AVFoundation
import AVKit
import BroadwaveKit
import BroadwaveUI
import CoreMedia
import MediaAccessibility
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
    private var endObserver: NSObjectProtocol?
    /// AVPlayer ended the current item (failedToPlayToEndTime). Read once.
    private var itemEnded = false
    /// The current item has had media loaded past its playhead.
    private var itemPrimed = false
    private(set) var firstFrameMs: Int?
    /// False from a new watch until the picture has moved 0.3 s. A new room
    /// holds its first frame for the start, and that is not a picture yet.
    private(set) var moving = false
    private var movingFrom: Double?
    private(set) var stalls = 0
    private(set) var stallMs = 0
    /// A master's sound tracks by role, once the item lists them. Nil for a
    /// watch without a master, which changes sound with a new watch.
    private(set) var sounds: [SoundTrack]?
    /// The role playing now, including a choice made in the system menu.
    private(set) var soundRole = "main"
    private var soundGroup: AVMediaSelectionGroup?
    private var soundObserver: NSObjectProtocol?
    private var stallStarted: Date?
    private var lastBeat = Date()
    private var clockLogged = Date.distantPast
    /// Nominal frame rate once the asset reports it. Zero until then.
    private(set) var refreshRate: Float = 0
    private(set) var picture = PictureStats()
    /// Earliest time the current item can seek to. Nil until it has a program date.
    private(set) var seekableFrom: Date?
    /// The small window has the picture.
    private(set) var pictureInPicture = false
    /// The display line last asked for, such as "1280x720 59.94". Empty until a picture has a rate.
    var displayLine = ""
    private var statsTask: Task<Void, Never>?
    /// The watch still waiting for its first segment. A newer channel cancels it
    /// so that tuner is not held until the request times out.
    private var watchTask: Task<WatchSession, Error>?
    private var watchToken = 0
    private let outage = ServerWatch()
    private var outageSampled = Date.distantPast
    private var frozen = FrozenPicture()
    /// A picture that stopped is being reloaded or tuned again.
    private(set) var reconnecting = false
    /// AVPlayer set its rate to 0 on its own, not a viewer, the sync engine,
    /// or the system. That is a stopped picture, not a pause.
    private var pausedItself = false
    private var rateObserver: NSObjectProtocol?
    private var routeObserver: NSObjectProtocol?
    /// When the audio route lost its device. AVPlayer pauses for that on its
    /// own, and that pause is the viewer's: playing on would move the sound
    /// to the speaker.
    private var routeLost = Date.distantPast
    /// Set for a quiet retry of a stopped picture. The message stays until the new picture moves.
    private var quietRetry = false
    private var holdPicture = false
    private var restartToken: (socket: EventSocket, id: UUID)?
    /// The room's live delay, from its latest state; nil until it has one.
    private(set) var roomLatency: String?
    private var roomToken: (socket: EventSocket, id: UUID)?
    private var roomName: String?
    /// A watch started while the old picture played on, for the next start to use.
    private var preparedSession: WatchSession?
    private var rewatch: (() async throws -> WatchSession)?
    private var handingOff = false
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
        if !retry {
            frozen = FrozenPicture()
            reconnecting = false
        }
        watchRate()
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
                guard let self, !handingOff else { return }
                quietRetry = quiet
                attempt += 1
            }
        )
        watchLifecycle()
        listenForRestart(store.socket)
        // Automatic is what starts the small window. continuesIfPossible
        // keeps the picture in the background with no window at all.
        player.audiovisualBackgroundPlaybackPolicy = .automatic
        // Picture in Picture stays hidden until the session is playback.
        // longFormVideo is iOS: it is what lets Home start the small window.
        let session = AVAudioSession.sharedInstance()
        do {
            #if os(iOS)
                player.allowsExternalPlayback = true
                try session.setCategory(.playback, mode: .moviePlayback, policy: .longFormVideo)
            #else
                try session.setCategory(.playback)
            #endif
            try session.setActive(true)
        } catch {
            playLogNote("audio session \(error.localizedDescription)")
        }
        if UserDefaults.standard.bool(forKey: "BroadwaveSyncLog") {
            let supported = AVPictureInPictureController.isPictureInPictureSupported()
            playLogNote("pip supported \(supported ? "yes" : "no")")
            #if os(iOS)
                // The unified log buffers while the app is away, so a test cannot
                // see the clock move. This file is written as it plays.
                let url = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
                    .appendingPathComponent("broadwave-clock.txt")
                try? Data().write(to: url)
            #endif
        }
        let caps = Capabilities.current()
        let prefs = store.prefs
        let room = "channel:\(channel.id)"
        // The socket can still be connecting; the engine below checks again.
        let syncs = store.syncEnabled && Compatibility.gateFeature(store.info, "wholeHomeSync") == nil && store.socket != nil
        rewatch = { try await api.watch(channelID: channel.id, caps: caps, prefs: prefs, confirmLive: false) }
        let prepared = preparedSession?.channelId == channel.id ? preparedSession : nil
        if let stale = preparedSession, prepared == nil {
            await api.stopWatching(stale)
        }
        preparedSession = nil
        let task = Task {
            if let prepared {
                return prepared
            }
            return try await api.watch(channelID: channel.id, caps: caps, prefs: prefs, confirmLive: allow, room: syncs ? room : nil)
        }
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
                await api.stopWatching(session)
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
            // The multivariant playlist carries the captions track, so the
            // system subtitle menu and the Closed Captions setting apply.
            let item = AVPlayerItem(url: api.url(session.mainPlaylist ?? session.playlist))
            item.externalMetadata = metadata(channel: channel, airing: store.index.on(channel.id, at: Date()))
            PlayerTuning.apply(item, network: Capabilities.current().network ?? "lan", tile: false)
            player.replaceCurrentItem(with: item)
            player.automaticallyWaitsToMinimizeStalling = true
            watchStartup(item)
            player.play()
            watchPicture()
            playLog.info("channel \(channel.displayNumber, privacy: .public)")
            if session.mainPlaylist != nil {
                Task { await loadSounds(item, prefer: prefs.track) }
            }
            #if DEBUG
                if UserDefaults.standard.bool(forKey: "BroadwaveCaptions") {
                    Task { await showCaptions(item) }
                }
            #endif
            if store.syncEnabled, Compatibility.gateFeature(store.info, "wholeHomeSync") == nil, let socket = store.socket {
                let engine = SyncEngine(player: player, socket: socket, room: room, channelID: channel.id)
                listenForRoom(socket, room: room)
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
        playLogNote("outage \(decision.message)")
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

    /// Jump to a show's start. Pausing first is the viewer's pause, so sync
    /// leaves this screen where it landed instead of pulling it back to live.
    /// The engine notices the pause on a later tick; seeking before it has
    /// would be undone.
    func jump(to date: Date) {
        player.pause()
        let token = watchToken
        Task { @MainActor in
            for _ in 0 ..< 30 {
                try? await Task.sleep(for: .milliseconds(100))
                if sync == nil || sync?.detached == true {
                    break
                }
            }
            guard token == watchToken, player.currentItem != nil else { return }
            player.seek(to: date) { [weak self] finished in
                Task { @MainActor in
                    guard finished, let self, token == self.watchToken else { return }
                    self.player.play()
                    self.playLogNote("start over")
                }
            }
        }
    }

    /// A restarted server has lost this watch. Start it again now instead of
    /// waiting for the picture to run dry and the stall clock to name it.
    private func listenForRestart(_ socket: EventSocket?) {
        if let restartToken {
            restartToken.socket.off("restarted", restartToken.id)
        }
        restartToken = nil
        guard let socket else { return }
        let id = socket.on("restarted") { [weak self] _ in
            guard let self, !handingOff, session != nil || watchTask != nil else { return }
            playLogNote("server restarted; watching again")
            Task { await self.handOff() }
        }
        restartToken = (socket, id)
    }

    private func listenForRoom(_ socket: EventSocket?, room: String?) {
        if let roomToken {
            roomToken.socket.off("sync.state", roomToken.id)
        }
        roomToken = nil
        roomName = room
        roomLatency = nil
        guard let socket, let room else { return }
        if let data = socket.roomState(room), let state = try? JSONDecoder().decode(RoomState.self, from: data) {
            roomLatency = state.latency
        }
        let id = socket.on("sync.state") { [weak self] data in
            guard let state = try? JSONDecoder().decode(RoomState.self, from: data), state.room == room else { return }
            self?.roomLatency = state.latency
        }
        roomToken = (socket, id)
    }

    /// Moves the room this screen is in, for everyone in it.
    func setDelay(_ delay: LiveDelay) {
        guard let roomToken, let roomName else { return }
        roomToken.socket.command(room: roomName, action: "latency", latency: delay.rawValue)
    }

    private func handOff() async {
        handingOff = true
        defer { handingOff = false }
        let token = watchToken
        guard let api, let rewatch, let id = channelID, session != nil else {
            quietRetry = true
            attempt += 1
            return
        }
        // Its room went with the old process.
        sync?.stop()
        sync = nil
        if let next = await RestartHandoff.prepare(api: api, player: player, rewatch: rewatch, current: { token == watchToken }) {
            guard token == watchToken, channelID == id else {
                await api.stopWatching(next.session)
                return
            }
            playLogNote("handoff \(next.ready ? "ready" : "early") old=\(String(format: "%.1f", RestartHandoff.bufferedAhead(player)))s")
            preparedSession = next.session
        }
        guard token == watchToken else { return }
        quietRetry = true
        attempt += 1
    }

    private func noteStop(_ session: WatchSession) {
        guard UserDefaults.standard.bool(forKey: "BroadwaveSyncLog") else { return }
        playLogNote("stop channel=\(session.channelId) boot=\(session.boot ?? "")")
    }

    func notePictureInPicture(_ on: Bool) {
        guard on != pictureInPicture else { return }
        pictureInPicture = on
        playLogNote(on ? "pip start" : "pip stop")
    }

    func stop(endPicture: Bool = true) async {
        if let restartToken {
            restartToken.socket.off("restarted", restartToken.id)
        }
        restartToken = nil
        watchTask?.cancel()
        watchTask = nil
        statsTask?.cancel()
        statsTask = nil
        picture = PictureStats()
        seekableFrom = nil
        sounds = nil
        soundGroup = nil
        soundRole = "main"
        if let soundObserver {
            NotificationCenter.default.removeObserver(soundObserver)
        }
        soundObserver = nil
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
        listenForRoom(nil, room: nil)
        if let tick {
            player.removeTimeObserver(tick)
        }
        tick = nil
        if let stallObserver {
            NotificationCenter.default.removeObserver(stallObserver)
        }
        stallObserver = nil
        if let endObserver {
            NotificationCenter.default.removeObserver(endObserver)
        }
        endObserver = nil
        itemEnded = false
        player.pause()
        player.replaceCurrentItem(with: nil)
        let ended = session
        let prepared = endPicture ? preparedSession : nil
        if endPicture {
            preparedSession = nil
        }
        channelID = nil
        session = nil
        if let api, let ended {
            noteStop(ended)
            await api.stopWatching(ended)
        }
        if let api, let prepared {
            noteStop(prepared)
            await api.stopWatching(prepared)
        }
    }

    private func watchPicture() {
        statsTask?.cancel()
        statsTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(1))
                guard let self else { return }
                var rate: Float = 0
                // A failed asset's track load may never answer, and this loop
                // is what names a stopped picture.
                if let item = player.currentItem, item.status != .failed {
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
        seekableFrom = seekableStart(item)
        noteClock()
    }

    private func seekableStart(_ item: AVPlayerItem) -> Date? {
        guard let date = item.currentDate() else { return nil }
        let now = CMTimeGetSeconds(item.currentTime())
        var earliest: Double?
        for value in item.seekableTimeRanges {
            let start = CMTimeGetSeconds(value.timeRangeValue.start)
            guard start.isFinite else { continue }
            earliest = min(earliest ?? start, start)
        }
        guard let earliest else { return nil }
        return SeekWindow.start(current: date, currentSeconds: now, earliestSeconds: earliest)
    }

    /// One info line a second while sync logging is on. The debug sync line is
    /// buffered, so a test that reads the log as it grows never sees it move.
    private func noteClock() {
        guard UserDefaults.standard.bool(forKey: "BroadwaveSyncLog") else { return }
        guard Date().timeIntervalSince(clockLogged) >= 0.9 else { return }
        guard let item = player.currentItem else { return }
        let ms: Double
        if let date = item.currentDate() {
            ms = date.timeIntervalSince1970 * 1000
        } else {
            let seconds = CMTimeGetSeconds(item.currentTime())
            guard seconds.isFinite else { return }
            ms = seconds * 1000
        }
        clockLogged = Date()
        let stalls = item.accessLog()?.events.last?.numberOfStalls ?? 0
        let media = Int(ms.rounded())
        playLogNote("clock media=\(media) stalls=\(stalls)")
        let url = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("broadwave-clock.txt")
        let line = Data("media=\(media) stalls=\(stalls)\n".utf8)
        if FileManager.default.fileExists(atPath: url.path), let handle = try? FileHandle(forWritingTo: url) {
            defer { try? handle.close() }
            _ = try? handle.seekToEnd()
            try? handle.write(contentsOf: line)
            try? handle.synchronize()
        } else {
            try? line.write(to: url)
        }
    }

    /// Lists a master's sound tracks and starts on the preferred one. AVPlayer
    /// switches between them in place, keeping the picture and the room.
    private func loadSounds(_ item: AVPlayerItem, prefer role: String?) async {
        guard let group = try? await item.asset.loadMediaSelectionGroup(for: .audible), group.options.count > 1,
              player.currentItem === item
        else { return }
        let listed = SoundTrack.roles(group.options.map {
            SoundTrack.Option(name: $0.displayName, isDefault: $0 == group.defaultOption, describes: $0.hasMediaCharacteristic(.describesVideoForAccessibility))
        })
        soundGroup = group
        sounds = listed
        if let role, role != "main", let pick = SoundTrack.pick(listed, role: role) {
            item.select(group.options[pick.index], in: group)
        }
        noteSound(item)
        if let soundObserver {
            NotificationCenter.default.removeObserver(soundObserver)
        }
        soundObserver = NotificationCenter.default.addObserver(forName: AVPlayerItem.mediaSelectionDidChangeNotification, object: item, queue: .main) { [weak self] _ in
            Task { @MainActor in self?.noteSound(item) }
        }
        playLogNote("sounds \(listed.map { "\($0.role)=\($0.name)" }.joined(separator: ", ")) playing \(soundRole)")
        #if DEBUG
            // -BroadwaveAudio language|described switches 10 s in and reports the next 10 s.
            if let role = UserDefaults.standard.string(forKey: "BroadwaveAudio") {
                Task { [weak self] in
                    try? await Task.sleep(for: .seconds(10))
                    guard let self, player.currentItem === item else { return }
                    let stalls = stalls
                    let dropped = item.accessLog()?.events.last?.numberOfDroppedVideoFrames ?? 0
                    selectSound(role)
                    try? await Task.sleep(for: .seconds(10))
                    guard player.currentItem === item else { return }
                    let after = item.accessLog()?.events.last?.numberOfDroppedVideoFrames ?? 0
                    let chosen = item.currentMediaSelection.selectedMediaOption(in: group)?.displayName ?? "none"
                    playLogNote("sound switched \(soundRole) playing \(chosen) stalls=\(self.stalls - stalls) dropped=\(after - dropped) state=\(player.timeControlStatus.rawValue)")
                }
            }
        #endif
    }

    private func noteSound(_ item: AVPlayerItem) {
        guard let group = soundGroup, let sounds, let chosen = item.currentMediaSelection.selectedMediaOption(in: group),
              let index = group.options.firstIndex(of: chosen), let sound = sounds.first(where: { $0.index == index })
        else { return }
        soundRole = sound.role
    }

    /// Plays another of the master's sound tracks without a new watch.
    func selectSound(_ role: String) {
        guard let group = soundGroup, let sounds, let item = player.currentItem, let pick = SoundTrack.pick(sounds, role: role) else { return }
        item.select(group.options[pick.index], in: group)
        soundRole = pick.role
        playLogNote("sound \(pick.role) stalls=\(stalls)")
    }

    #if DEBUG
        /// `-BroadwaveCaptions YES` turns on the system's captions setting, as
        /// Accessibility › Subtitles and Captioning would, and logs the track
        /// the player chose.
        private func showCaptions(_ item: AVPlayerItem) async {
            MACaptionAppearanceSetDisplayType(.user, .alwaysOn)
            guard let group = try? await item.asset.loadMediaSelectionGroup(for: .legible) else {
                playLog.info("captions none")
                return
            }
            try? await Task.sleep(for: .seconds(8))
            let offered = group.options.map(\.displayName).joined(separator: ", ")
            let chosen = item.currentMediaSelection.selectedMediaOption(in: group)?.displayName ?? "none"
            playLog.info("captions offered \(offered, privacy: .public) chosen \(chosen, privacy: .public)")
        }
    #endif

    private func watchStartup(_ item: AVPlayerItem) {
        if let tick {
            player.removeTimeObserver(tick)
        }
        if let stallObserver {
            NotificationCenter.default.removeObserver(stallObserver)
        }
        if let endObserver {
            NotificationCenter.default.removeObserver(endObserver)
        }
        itemEnded = false
        itemPrimed = false
        endObserver = NotificationCenter.default.addObserver(forName: AVPlayerItem.failedToPlayToEndTimeNotification, object: item, queue: .main) { [weak self] _ in
            MainActor.assumeIsolated {
                self?.itemEnded = true
                self?.playLogNote("item ended")
            }
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
                self.noteClock()
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
            paused: player.timeControlStatus == .paused && !pausedItself,
            failed: item?.status == .failed
        )
        noteFrozen(item)
        notePictureMoved()
    }

    private func watchRate() {
        guard rateObserver == nil else { return }
        routeObserver = NotificationCenter.default.addObserver(
            forName: AVAudioSession.routeChangeNotification, object: nil, queue: .main
        ) { [weak self] note in
            let raw = note.userInfo?[AVAudioSessionRouteChangeReasonKey] as? UInt
            guard raw.flatMap(AVAudioSession.RouteChangeReason.init) == .oldDeviceUnavailable else { return }
            MainActor.assumeIsolated {
                self?.routeLost = Date()
            }
        }
        rateObserver = NotificationCenter.default.addObserver(
            forName: AVPlayer.rateDidChangeNotification, object: player, queue: .main
        ) { [weak self] note in
            let reason = note.userInfo?[AVPlayer.rateDidChangeReasonKey] as? AVPlayer.RateDidChangeReason
            MainActor.assumeIsolated {
                guard let self else { return }
                let other: Set<AVPlayer.RateDidChangeReason> = [.setRateCalled, .appBackgrounded, .audioSessionInterrupted]
                let lostRoute = Date().timeIntervalSince(self.routeLost) < 2
                self.pausedItself = self.player.rate == 0 && !lostRoute && !(reason.map(other.contains) ?? false)
                if self.pausedItself {
                    self.playLogNote("paused itself (\(reason?.rawValue ?? "no reason"))")
                }
            }
        }
    }

    /// A picture that should move and does not reloads the item, then starts
    /// a new watch (`FrozenPicture`). An outage message has its own clock.
    private func noteFrozen(_ item: AVPlayerItem?) {
        // The route notice can come after the rate change it caused.
        if Date().timeIntervalSince(routeLost) < 3 {
            pausedItself = false
        }
        let paused = player.timeControlStatus == .paused
        let time = error == nil ? item?.currentTime().seconds : nil
        // A reload plays. The viewer's pause stays; their play finds the item dead.
        let viewerPaused = paused && !pausedItself && sync?.detached == true
        let ended = itemEnded && error == nil && !viewerPaused
        itemEnded = false
        if RestartHandoff.bufferedAhead(player) >= 0.5 {
            itemPrimed = true
        }
        let starved = player.timeControlStatus == .playing && RestartHandoff.bufferedAhead(player) < 0.1
        let step = frozen.note(
            time: time, playing: !paused, stoppedItself: paused && pausedItself, ended: ended, primed: itemPrimed,
            starved: starved, at: Date()
        )
        if !frozen.reconnecting {
            reconnecting = false
        }
        guard let step else { return }
        reconnecting = true
        switch step {
        case .reload:
            playLogNote("frozen: reload")
            reload()
        case .retune:
            playLogNote("frozen: retune")
            quietRetry = false
            attempt += 1
        }
    }

    /// The same watch on a fresh item, which AVPlayer opens at the live edge.
    private func reload() {
        guard let api, let session, let old = player.currentItem else { return }
        let item = AVPlayerItem(url: api.url(session.mainPlaylist ?? session.playlist))
        item.externalMetadata = old.externalMetadata
        PlayerTuning.apply(item, network: Capabilities.current().network ?? "lan", tile: false)
        pausedItself = false
        player.replaceCurrentItem(with: item)
        outage.newItem()
        watchStartup(item)
        player.play()
        if session.mainPlaylist != nil {
            let role = soundRole
            Task { await loadSounds(item, prefer: role) }
        }
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
        // The system Info tab shows the description, so the episode line leads it.
        let episode = airing.flatMap {
            ProgramLine.episode(label: $0.episodeLabel, season: $0.season, episode: $0.episode, subtitle: $0.subtitle)
        }
        let about = [episode, airing?.description].compactMap(\.self).joined(separator: "\n\n")
        if !about.isEmpty {
            out.append(item(.commonIdentifierDescription, about))
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
    private var live: LivePlayer {
        nowPlaying.live
    }

    @State private var showStream = false
    /// Fill crops the picture. Fit shows the whole frame. Pinch on iPhone switches them.
    @State private var fillPicture = false
    #if os(iOS)
        /// Last scale from the pinch, in case `onEnded` reports a smaller move.
        @State private var pinchScale: CGFloat = 1
    #endif
    /// True while the Apple TV transport bar is on screen. Swipes change channel only when it is not.
    @State private var transportShown = true
    /// A recording that holds this showing's start, played from Start over.
    @State private var startOverRecording: Recording?
    @State private var lastChannelStep = Date.distantPast
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
                delay: delayMenu,
                streamOn: showStream,
                onStream: { showStream.toggle() },
                onTogether: {
                    if let channel = nowPlaying.channel {
                        nowPlaying.watchTogether([channel])
                    }
                },
                rejoin: live.sync?.detached == true ? { live.sync?.rejoin() } : nil,
                onPictureInPicture: { live.notePictureInPicture($0) },
                onPictureRestore: {
                    live.playLogNote("pip restore \(nowPlaying.channel.map { String($0.id) } ?? "-")")
                    if nowPlaying.channel != nil {
                        nowPlaying.expanded = true
                    }
                },
                onPictureClosed: {
                    live.playLogNote("pip closed")
                    nowPlaying.stop()
                },
                channelNumber: nowPlaying.channel?.displayNumber ?? "",
                recording: nowPlaying.channel.flatMap { store.activeRecording(on: $0) } != nil,
                canStartOver: startOverChoice != nil,
                fill: fillPicture,
                onRecord: {
                    if let channel = nowPlaying.channel {
                        Task { await store.toggleRecord(channel) }
                    }
                },
                onStartOver: beginStartOver,
                onStep: { step($0) },
                onTransport: { transportShown = $0 },
                onDisplay: { live.displayLine = $0 },
                panelStore: store,
                panelNow: nowPlaying,
                panelLive: live
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
                if UserDefaults.standard.bool(forKey: "BroadwaveChannelNow") {
                    Group {
                        Text(transportShown ? "shown" : "hidden")
                            .accessibilityIdentifier("transportProbe")
                        Text(transportMenuNames.joined(separator: "|"))
                            .accessibilityIdentifier("menuProbe")
                    }
                    .font(.system(size: 2))
                    .foregroundStyle(.clear)
                    .allowsHitTesting(false)
                }
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
            #if DEBUG
                if UserDefaults.standard.bool(forKey: "BroadwaveChannelNow") {
                    Text(nowPlaying.channel?.displayNumber ?? "")
                        .font(.system(size: 2))
                        .foregroundStyle(.clear)
                        .allowsHitTesting(false)
                        .accessibilityIdentifier("channel-now")
                    Text(live.displayLine)
                        .font(.system(size: 2))
                        .foregroundStyle(.clear)
                        .allowsHitTesting(false)
                        .accessibilityIdentifier("displayProbe")
                    #if os(tvOS)
                        Text(AVPictureInPictureController.isPictureInPictureSupported() ? "yes" : "no")
                            .font(.system(size: 2))
                            .foregroundStyle(.clear)
                            .allowsHitTesting(false)
                            .accessibilityIdentifier("pipProbe")
                    #endif
                }
                if UserDefaults.standard.bool(forKey: "BroadwaveSyncProbe") {
                    SyncProbe(sync: live.sync)
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
            } else if live.reconnecting {
                Text("Reconnecting…")
                    .font(.footnote.weight(.semibold))
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .glassEffect(in: .capsule)
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                    .padding(.top, 24)
                    .allowsHitTesting(false)
                    .accessibilityIdentifier("playback-reconnecting")
            } else if let note = nowPlaying.note {
                Text(note)
                    .font(.footnote.weight(.semibold))
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .glassEffect(in: .capsule)
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                    .padding(.top, 24)
                    .allowsHitTesting(false)
                    .accessibilityIdentifier("playback-note")
                    .task(id: note) {
                        try? await Task.sleep(for: .seconds(10))
                        if nowPlaying.note == note {
                            nowPlaying.note = nil
                        }
                    }
            }
        }
        #if os(iOS)
        .onChange(of: verticalSize) { _, size in
            guard UserDefaults.standard.bool(forKey: "BroadwaveSyncLog") else { return }
            live.playLogNote("size \(size == .compact ? "landscape" : "portrait")")
        }
        #endif
        .onChange(of: store.prefs.track) { _, track in
            if live.sounds == nil {
                nowPlaying.trackRestarts += 1
            } else if live.soundRole != (track ?? "main") {
                live.selectSound(track ?? "main")
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
        .fullScreenCover(item: $startOverRecording, onDismiss: { live.retry() }, content: { recording in
            RecordingPlayerScreen(recording: recording)
                .environment(store)
        })
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
        let now = Date()
        // A swipe can also arrive as an arrow press. One move is enough.
        guard now.timeIntervalSince(lastChannelStep) > 0.35 else { return }
        lastChannelStep = now
        guard let current = nowPlaying.channel, let i = store.channels.firstIndex(of: current) else { return }
        let next = store.channels[(i + dir + store.channels.count) % store.channels.count]
        nowPlaying.channel = next
        live.playLogNote("step \(dir) \(next.displayNumber)")
    }

    #if os(tvOS)
        /// Names in the transport menu. The bar's own entries stay out of the accessibility tree.
        private var transportMenuNames: [String] {
            let recording = nowPlaying.channel.flatMap { store.activeRecording(on: $0) } != nil
            var names = ["Channels", "Audio", recording ? "Stop recording" : "Record", "Multiview"]
            if let current = delayMenu.first(where: \.current) {
                names.insert("Live delay \(current.title)", at: 2)
            }
            if startOverChoice != nil {
                names.append("Start over")
            }
            if live.sync?.detached == true {
                names.insert("Back in sync", at: 0)
            }
            return names
        }
    #endif

    /// The live window wins. Otherwise a recording of this showing plays from its start.
    private var startOverChoice: StartOver? {
        guard let channel = nowPlaying.channel, let airing = store.index.on(channel.id, at: store.now) else { return nil }
        let held = ShowRecording.holdingStart(of: airing, in: store.recordings)
        return StartOver.choice(showStart: airing.start, seekableFrom: live.seekableFrom, recordingStarted: held?.startedAt)
    }

    private func beginStartOver() {
        guard let channel = nowPlaying.channel, let airing = store.index.on(channel.id, at: store.now) else { return }
        switch startOverChoice {
        case .live:
            live.jump(to: airing.start)
        case .recording:
            guard let recording = ShowRecording.holdingStart(of: airing, in: store.recordings) else { return }
            Task {
                await live.stop()
                startOverRecording = recording
            }
        case nil:
            break
        }
    }

    /// Empty until the screen is in a room with a known delay.
    private var delayMenu: [ChannelMenuEntry] {
        guard live.sync != nil, let current = live.roomLatency else { return [] }
        return LiveDelay.allCases.enumerated().map { number, delay in
            ChannelMenuEntry(id: Int64(number + 1), title: delay.title, current: current == delay.rawValue) {
                LiveDelay.saved = delay
                live.setDelay(delay)
            }
        }
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
        if let sounds = live.sounds {
            // A master names its tracks; the choice is kept for the next channel.
            items = sounds.enumerated().map { number, sound in
                ChannelMenuEntry(id: Int64(number + 1), title: sound.name, current: live.soundRole == sound.role) {
                    live.selectSound(sound.role)
                    var prefs = store.prefs
                    prefs.track = sound.role == "main" ? nil : sound.role
                    store.prefs = prefs
                }
            }
        }
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
        @ViewBuilder private var delaySection: some View {
            if !delayMenu.isEmpty {
                Section("Live delay") {
                    ForEach(delayMenu) { entry in
                        Button(action: entry.action) {
                            if entry.current {
                                Label(entry.title, systemImage: "checkmark")
                            } else {
                                Text(entry.title)
                            }
                        }
                    }
                }
            }
        }

        private var overlay: some View {
            ZStack {
                gesturePad
                if portraitChrome {
                    portraitOverlay
                } else {
                    landscapeBar
                }
            }
        }

        /// Swipe up for the previous channel, down for the next. Pinch out fills the screen.
        /// The pad sits in the open picture. A pinch starts at the pad's edges, and the chrome covers those.
        private var gesturePad: some View {
            let compact = verticalSize == .compact
            return Color.clear
                .contentShape(Rectangle())
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .gesture(channelDrag)
                .simultaneousGesture(fillPinch)
                .accessibilityElement()
                .accessibilityLabel("Picture")
                .accessibilityValue(fillPicture ? "Fill" : "Fit")
                .accessibilityIdentifier("player-gesture")
                .padding(.top, compact ? 40 : 156)
                .padding(.bottom, compact ? 72 : 200)
                .padding(.leading, 16)
                .padding(.trailing, compact ? 16 : 108)
        }

        private var channelDrag: some Gesture {
            DragGesture(minimumDistance: 24)
                .onEnded { value in
                    let dx = value.translation.width
                    let dy = value.translation.height
                    guard abs(dy) > abs(dx), abs(dy) > 80 else { return }
                    step(dy < 0 ? ChannelStep.offset(up: true) : ChannelStep.offset(up: false))
                }
        }

        private var fillPinch: some Gesture {
            MagnificationGesture()
                .onChanged { pinchScale = $0 }
                .onEnded { scale in
                    let amount = abs(pinchScale - 1) > abs(scale - 1) ? pinchScale : scale
                    pinchScale = 1
                    guard abs(amount - 1) > 0.08 else { return }
                    let fill = amount > 1
                    guard fill != fillPicture else { return }
                    fillPicture = fill
                    live.playLogNote(fill ? "fill" : "fit")
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
                airPlayControl
            }
        }

        private var airPlayControl: some View {
            AirPlayRoute {
                live.playLogNote("airplay open")
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
                delaySection
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
                    delaySection
                }
                .labelStyle(.iconOnly)
                .buttonStyle(.glass)
                .accessibilityLabel("Audio")
                AirPlayRoute(compact: true) {
                    live.playLogNote("airplay open")
                }
                .frame(width: 44, height: 44)
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

#if os(iOS)
    /// One entry of the iPhone and iPad Breaks menu.
    private struct FileMenuItem: View {
        let entry: FileMenuEntry

        var body: some View {
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
#endif

/// AVPlayerViewController: system PiP, AirPlay, captions, audio tracks, and Now Playing.
struct SystemPlayer: UIViewControllerRepresentable {
    let player: AVPlayer
    var hint = DisplayMatch()
    var menu: [ChannelMenuEntry] = []
    var audio: [ChannelMenuEntry] = []
    var delay: [ChannelMenuEntry] = []
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
    var onPictureInPicture: (Bool) -> Void = { _ in }
    var onPictureRestore: () -> Void = {}
    var onPictureClosed: () -> Void = {}
    var channelNumber = ""
    var recording = false
    /// Start over is omitted when neither the picture nor a recording holds the show's start.
    var canStartOver = false
    var fill = false
    var onRecord: () -> Void = {}
    var onStartOver: () -> Void = {}
    var onStep: (Int) -> Void = { _ in }
    var onTransport: (Bool) -> Void = { _ in }
    var onDisplay: (String) -> Void = { _ in }
    var panelStore: AppStore?
    var panelNow: NowPlaying?
    var panelLive: LivePlayer?

    func makeCoordinator() -> Coordinator {
        Coordinator()
    }

    func makeUIViewController(context: Context) -> LivePlayerController {
        let vc = LivePlayerController()
        vc.player = player
        vc.allowsPictureInPicturePlayback = true
        vc.onStep = onStep
        #if os(iOS)
            vc.canStartPictureInPictureAutomaticallyFromInline = true
            vc.delegate = context.coordinator.picture
            context.coordinator.picture.onChange = onPictureInPicture
            context.coordinator.picture.onRestore = onPictureRestore
            context.coordinator.picture.onClosed = onPictureClosed
        #endif
        #if os(tvOS)
            vc.delegate = vc
            vc.onTransport = onTransport
            vc.appliesPreferredDisplayCriteriaAutomatically = false
            context.coordinator.start(vc)
        #endif
        return vc
    }

    #if os(tvOS)
        static func dismantleUIViewController(_ vc: LivePlayerController, coordinator: Coordinator) {
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

    func updateUIViewController(_ vc: LivePlayerController, context: Context) {
        if vc.player !== player {
            vc.player = player
        }
        vc.onStep = onStep
        vc.view.accessibilityIdentifier = "player-channel"
        if !channelNumber.isEmpty {
            vc.view.accessibilityValue = channelNumber
        }
        #if os(iOS)
            let gravity: AVLayerVideoGravity = fill ? .resizeAspectFill : .resizeAspect
            if vc.videoGravity != gravity {
                vc.videoGravity = gravity
            }
            vc.delegate = context.coordinator.picture
            context.coordinator.picture.onChange = onPictureInPicture
            context.coordinator.picture.onRestore = onPictureRestore
            context.coordinator.picture.onClosed = onPictureClosed
        #endif
        #if os(tvOS)
            vc.onTransport = onTransport
            vc.onPictureInPicture = onPictureInPicture
            vc.onPictureRestore = onPictureRestore
            vc.onPictureClosed = onPictureClosed
            if liveMenu, let store = panelStore, let now = panelNow, let live = panelLive {
                context.coordinator.installPanels(on: vc, store: store, now: now, live: live)
            }
            context.coordinator.onDisplay = onDisplay
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
            let key = (menu + audio + delay).map { "\($0.id) \($0.title) \($0.current)" } + ["\(streamOn)", "\(rejoin != nil)", "\(recording)", "\(canStartOver)"]
            guard key != context.coordinator.menuKey else { return }
            context.coordinator.menuKey = key
            let actions = menu.map { entry in
                UIAction(title: entry.title, state: entry.current ? .on : .off) { _ in entry.action() }
            }
            let audioActions = audio.map { entry in
                UIAction(title: entry.title, state: entry.current ? .on : .off) { _ in entry.action() }
            }
            let record = UIAction(title: recording ? "Stop recording" : "Record", image: UIImage(systemName: recording ? "record.circle.fill" : "record.circle")) { _ in onRecord() }
            let together = UIAction(title: "Multiview", image: UIImage(systemName: "rectangle.split.2x1")) { _ in onTogether() }
            let stream = UIAction(title: streamOn ? "Hide stream" : "Stream", image: UIImage(systemName: "info.circle"), state: streamOn ? .on : .off) { _ in onStream() }
            let audioMenu = UIMenu(title: "Audio", image: UIImage(systemName: "speaker.wave.2"), children: audioActions)
            var items: [UIMenuElement] = [UIMenu(title: "Channels", image: UIImage(systemName: "list.bullet"), children: actions), audioMenu, stream, record]
            if !delay.isEmpty {
                let choices = delay.map { entry in
                    UIAction(title: entry.title, state: entry.current ? .on : .off) { _ in entry.action() }
                }
                items.insert(UIMenu(title: "Live delay", image: UIImage(systemName: "clock"), children: choices), at: 2)
            }
            if canStartOver {
                items.append(UIAction(title: "Start over", image: UIImage(systemName: "backward.end")) { _ in onStartOver() })
            }
            items.append(together)
            if let rejoin {
                items.insert(UIAction(title: "Back in sync", image: UIImage(systemName: "arrow.triangle.2.circlepath")) { _ in rejoin() }, at: 0)
            }
            vc.transportBarCustomMenuItems = items
            // The default action is Play From Beginning, which a live picture cannot do
            // unless the window still holds the start. An empty list leaves it off.
            if canStartOver {
                vc.infoViewActions = [UIAction(title: "Start over", image: UIImage(systemName: "backward.end")) { _ in onStartOver() }]
            } else {
                vc.infoViewActions = []
            }
        #endif
    }

    /// Reads the current item until its size and rate show up, then asks the
    /// TV for that broadcast rate. A closed player clears the mode while the
    /// window still exists.
    @MainActor
    final class Coordinator {
        #if os(iOS)
            let picture = PictureDelegate()
        #endif
        #if os(tvOS)
            var menuKey: [String] = []
            var skipShown = false
            var skip: (() -> Void)?
            private var infoPanels: [UIViewController] = []
            private var task: Task<Void, Never>?
            private var match = DisplayMatch()
            private var hint = DisplayMatch()
            private var applied: DisplayMatch?
            private var format: CMFormatDescription?
            private var logged = ""
            var onDisplay: (String) -> Void = { _ in }

            func installPanels(on vc: LivePlayerController, store: AppStore, now: NowPlaying, live: LivePlayer) {
                guard infoPanels.isEmpty else { return }
                infoPanels = PlayerPanels.controllers(store: store, now: now, live: live)
                vc.customInfoViewControllers = infoPanels
                #if DEBUG
                    print("broadwave panels \(infoPanels.count)")
                #endif
            }

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
                hint = DisplayMatch()
                applied = nil
                format = nil
            }

            func sample(_ vc: AVPlayerViewController) {
                guard vc.player?.currentItem == nil else { return }
                match = DisplayMatch()
                hint = DisplayMatch()
                format = nil
                apply(vc)
            }

            private func refresh(_ vc: AVPlayerViewController) async {
                guard let item = vc.player?.currentItem else {
                    match = DisplayMatch()
                    hint = DisplayMatch()
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
                let hintChanged = hint != self.hint
                self.hint = hint
                let next = PlayerTuning.hintedDisplay(hint, current: match)
                let matchChanged = next != match
                match = next
                if hintChanged || matchChanged {
                    apply(vc)
                }
            }

            private func apply(_ vc: AVPlayerViewController) {
                guard let manager = vc.view.window?.avDisplayManager else {
                    applied = nil
                    return
                }
                let asked = PlayerTuning.displayAsked(asset: match, hint: hint)
                guard let criteria = SystemPlayer.displayCriteria(asked, format: format) else {
                    if applied != nil {
                        manager.preferredDisplayCriteria = nil
                        applied = nil
                        logDisplay("clear")
                    }
                    return
                }
                if applied == asked {
                    return
                }
                NotificationCenter.default.post(name: SyncEngine.displayWillChange, object: nil)
                manager.preferredDisplayCriteria = criteria
                applied = asked
                logDisplay("\(asked.width)x\(asked.height) \(PlayerTuning.refreshRateName(asked.refreshRate))")
            }

            private func logDisplay(_ message: String) {
                guard message != logged else { return }
                logged = message
                print("broadwave display \(message)")
                fflush(stdout)
                onDisplay(message)
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

/// What the recording player just did, over the top of the picture for a few seconds.
private struct PlayerNote: View {
    let text: String?

    var body: some View {
        if let text {
            Text(text)
                .font(.subheadline)
                .padding(.horizontal, 16)
                .padding(.vertical, 10)
                .glassEffect()
                .padding(.top, 60)
                .allowsHitTesting(false)
        }
    }
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

    /// AVFoundation may post this off the main thread.
    private static let ended = NotificationCenter.default.publisher(for: AVPlayerItem.didPlayToEndTimeNotification).receive(on: DispatchQueue.main)

    init(recording: Recording) {
        source = .recording(recording)
    }

    #if DEBUG
        private static let log = Logger(subsystem: "com.wolfeup.broadwave", category: "play")

        /// Simulator testing: -BroadwaveSyncLog 1 logs the file player once a second,
        /// and -BroadwaveRecordingSeek <seconds> seeks there 20 s in, as a viewer would.
        private func debugReport(_ ticks: Int) {
            guard UserDefaults.standard.bool(forKey: "BroadwaveSyncLog"), ticks % 4 == 0, let item = player.currentItem else { return }
            let seekTo = UserDefaults.standard.double(forKey: "BroadwaveRecordingSeek")
            if seekTo > 0, ticks == 80 {
                player.seek(to: CMTime(seconds: seekTo, preferredTimescale: 600))
            }
            let events = item.accessLog()?.events ?? []
            let stalls = events.reduce(0) { $0 + max(0, $1.numberOfStalls) }
            let dropped = events.reduce(0) { $0 + max(0, $1.numberOfDroppedVideoFrames) }
            let time = player.currentTime().seconds, rate = player.rate, tc = player.timeControlStatus.rawValue
            let size = item.presentationSize, status = item.status.rawValue
            let err = item.errorLog()?.events.last?.errorComment ?? "-"
            Self.log.notice("file t=\(time) rate=\(rate) tc=\(tc) item=\(status) stalls=\(stalls) dropped=\(dropped) size=\(Int(size.width))x\(Int(size.height)) err=\(err, privacy: .public)")
        }
    #endif

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
            .overlay(alignment: .top) { PlayerNote(text: note) }
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
            .toolbar { fileToolbar }
        #endif
            .task(id: index) {
                guard await load() else { return }
                #if DEBUG
                    var ticks = 0
                #endif
                while await (try? Task.sleep(for: .milliseconds(250))) != nil {
                    await followBreaks()
                    #if DEBUG
                        ticks += 1
                        debugReport(ticks)
                    #endif
                }
            }
            .task(id: noteCount) {
                guard note != nil else { return }
                try? await Task.sleep(for: .seconds(4))
                if !Task.isCancelled {
                    note = nil
                }
            }
            .onReceive(Self.ended) { sent in
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

    #if os(iOS)
        /// Next recording as its own button, and the break entries in a Breaks menu.
        @ToolbarContentBuilder private var fileToolbar: some ToolbarContent {
            let entries = menu
            if let next = entries.first(where: { $0.id == "next" }) {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(next.title, systemImage: next.symbol) { next.action() }
                }
            }
            if let start = entries.first(where: { $0.id == "start-over" }) {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(start.title, systemImage: start.symbol) { start.action() }
                        .accessibilityIdentifier("recording-start-over")
                }
            }
            let breaks = entries.filter { $0.id != "next" && $0.id != "start-over" }
            if !breaks.isEmpty {
                ToolbarItem(placement: .topBarTrailing) {
                    Menu("Breaks", systemImage: "scissors") {
                        ForEach(breaks) { entry in
                            FileMenuItem(entry: entry)
                        }
                    }
                    .accessibilityIdentifier("recording-menu")
                }
            }
        }
    #endif

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
        out.insert(FileMenuEntry(id: "start-over", title: "Start over", symbol: "backward.end") {
            player.seek(to: .zero)
            player.play()
        }, at: 0)
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

#if DEBUG
    /// `-BroadwaveSyncProbe YES`: the engine's state and drift, for UI tests.
    private struct SyncProbe: View {
        let sync: SyncEngine?

        var body: some View {
            Text("\(sync?.state.rawValue ?? "none") drift=\(Int(sync?.drift ?? 0))")
                .font(.system(size: 2))
                .foregroundStyle(.clear)
                .allowsHitTesting(false)
                .accessibilityIdentifier("syncState")
        }
    }
#endif
