import AVFoundation
import AVKit
import BroadwaveKit
import BroadwaveUI
import SwiftUI

@MainActor
@Observable
final class LivePlayer {
    let player = AVPlayer()
    private(set) var session: WatchSession?
    private(set) var sync: SyncEngine?
    var error: String?
    private var channelID: Int64?
    private var api: APIClient?

    func start(_ channel: Channel, store: AppStore) async {
        await stop()
        guard let api = store.api else { return }
        self.api = api
        channelID = channel.id
        error = nil
        do {
            let session = try await api.watch(channelID: channel.id, caps: Capabilities.current(), prefs: store.prefs)
            guard channelID == channel.id else {
                await api.stopWatching(channelID: channel.id, rendition: session.rendition)
                return
            }
            self.session = session
            let item = AVPlayerItem(url: api.url(session.playlist))
            item.externalMetadata = metadata(channel: channel, airing: store.index.on(channel.id, at: Date()))
            item.preferredForwardBufferDuration = 6
            player.replaceCurrentItem(with: item)
            player.automaticallyWaitsToMinimizeStalling = true
            player.play()
            if store.syncEnabled, let socket = store.socket {
                let engine = SyncEngine(player: player, socket: socket, room: "channel:\(channel.id)", channelID: channel.id)
                engine.start()
                sync = engine
            }
        } catch {
            self.error = error.localizedDescription
        }
    }

    func stop() async {
        sync?.stop()
        sync = nil
        player.pause()
        player.replaceCurrentItem(with: nil)
        if let api, let id = channelID, let session {
            await api.stopWatching(channelID: id, rendition: session.rendition)
        }
        session = nil
        channelID = nil
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
    @State private var live = LivePlayer()

    var body: some View {
        ZStack(alignment: .top) {
            Color.black.ignoresSafeArea()
            SystemPlayer(player: live.player, menu: channelMenu, audio: audioMenu) {
                if let channel = nowPlaying.channel {
                    nowPlaying.watchTogether([channel])
                }
            }
            .ignoresSafeArea()
            #if os(iOS)
                overlay
            #endif
            if let error = live.error {
                Text(error)
                    .padding()
                    .glassEffect(in: .rect(cornerRadius: Tokens.Radius.md))
                    .padding(.top, 80)
            }
        }
        .task(id: "\(nowPlaying.channel?.id ?? 0) \(store.prefs.track ?? "") \(store.prefs.even)") {
            if let channel = nowPlaying.channel {
                await live.start(channel, store: store)
            }
        }
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
        private var overlay: some View {
            HStack(spacing: 10) {
                Button("Minimize", systemImage: "chevron.down") { nowPlaying.expanded = false }
                    .labelStyle(.iconOnly)
                    .buttonStyle(.glass)
                Spacer()
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
                Button("Side by side", systemImage: "rectangle.split.2x1") {
                    if let channel = nowPlaying.channel {
                        nowPlaying.watchTogether([channel])
                    }
                }
                .labelStyle(.iconOnly)
                .buttonStyle(.glass)
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
                }
            }
            .padding(.horizontal)
            .padding(.top, 8)
        }
    #endif
}

struct ChannelMenuEntry: Identifiable {
    let id: Int64
    let title: String
    let current: Bool
    let action: () -> Void
}

/// AVPlayerViewController: system PiP, AirPlay, captions, audio tracks, and Now Playing.
struct SystemPlayer: UIViewControllerRepresentable {
    let player: AVPlayer
    var menu: [ChannelMenuEntry] = []
    var audio: [ChannelMenuEntry] = []
    var onTogether: () -> Void = {}

    func makeUIViewController(context _: Context) -> AVPlayerViewController {
        let vc = AVPlayerViewController()
        vc.player = player
        vc.allowsPictureInPicturePlayback = true
        #if os(iOS)
            vc.canStartPictureInPictureAutomaticallyFromInline = true
        #endif
        #if os(tvOS)
            vc.appliesPreferredDisplayCriteriaAutomatically = true
        #endif
        return vc
    }

    func updateUIViewController(_ vc: AVPlayerViewController, context _: Context) {
        if vc.player !== player {
            vc.player = player
        }
        #if os(tvOS)
            let actions = menu.map { entry in
                UIAction(title: entry.title, state: entry.current ? .on : .off) { _ in entry.action() }
            }
            let audioActions = audio.map { entry in
                UIAction(title: entry.title, state: entry.current ? .on : .off) { _ in entry.action() }
            }
            let together = UIAction(title: "Side by side", image: UIImage(systemName: "rectangle.split.2x1")) { _ in onTogether() }
            let audioMenu = UIMenu(title: "Audio", image: UIImage(systemName: "speaker.wave.2"), children: audioActions)
            vc.transportBarCustomMenuItems = [UIMenu(title: "Channels", image: UIImage(systemName: "list.bullet"), children: actions), audioMenu, together]
        #endif
    }
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
