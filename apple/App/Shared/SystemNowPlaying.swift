import AVFoundation
import BroadwaveKit
import Foundation
#if compiler(>=6.4)
    import AppIntents
    import NowPlaying
#endif

/// The live channel in the system's Now Playing: the Lock Screen and Control Center on
/// iPhone and iPad, Control Center and the remote on Apple TV. Channel up and down are
/// the next and previous buttons, and the program shows as live, with no scrubber.
///
/// iOS and tvOS 27 use the Now Playing framework. Before 27 the one-channel player's
/// AVPlayerViewController publishes it, as it always has.
@MainActor
final class SystemNowPlaying {
    struct Show: Equatable {
        var channel: Channel
        var airing: Airing?
        var art: URL?
    }

    struct Commands {
        /// -1 is channel up, as `ChannelStep` has it.
        var step: (Int) -> Void
        var toggle: () -> Void
    }

    /// The live player takes this over from AVPlayerViewController, so the two never publish at once.
    static var publishesLive: Bool {
        #if compiler(>=6.4)
            if #available(iOS 27, tvOS 27, *) {
                return true
            }
        #endif
        return false
    }

    private var shown: Show?
    private var model: AnyObject?
    private var session: AnyObject?
    private var watched: AVPlayer?
    private var rateWatch: NSKeyValueObservation?
    private var paused = false
    private var current: Commands?

    /// Publishes a show, or ends the session for nil. `player` gives play and pause; a
    /// multiview grid passes nil and says with `paused` whether it is paused.
    func publish(_ show: Show?, player: AVPlayer?, paused: Bool, commands: Commands) {
        guard Self.publishesLive else { return }
        #if compiler(>=6.4)
            if #available(iOS 27, tvOS 27, *) {
                guard let show else {
                    end()
                    return
                }
                let media = (model as? LiveMedia) ?? LiveMedia()
                if model == nil {
                    model = media
                }
                if shown != show {
                    shown = show
                    media.content = Self.content(show)
                    Self.note("content \(show.channel.displayNumber) \(show.airing?.title ?? show.channel.displayName) art=\(show.art != nil)")
                }
                current = commands
                if media.commands.isEmpty {
                    media.commands = [
                        .togglePlayPause { [weak self] in self?.current?.toggle() },
                        .next { [weak self] in self?.current?.step(ChannelStep.offset(up: true)) },
                        .previous { [weak self] in self?.current?.step(ChannelStep.offset(up: false)) },
                    ]
                }
                self.paused = paused
                watch(player, media: media)
                if session == nil {
                    let live = MediaSession(media)
                    session = live
                    Task {
                        do {
                            try await live.requestToBecomeApplicationPrimary()
                            #if os(iOS)
                                try await live.requestToBecomeSystemPrimary()
                            #endif
                        } catch {
                            Self.note("primary failed \(error)")
                        }
                        Self.note("session primary=\(live.isApplicationPrimary) \(show.channel.displayNumber) \(show.airing?.title ?? "-")")
                    }
                }
            }
        #endif
    }

    /// `-BroadwaveSyncLog 1` only: what the system was handed.
    private static func note(_ line: String) {
        guard UserDefaults.standard.bool(forKey: "BroadwaveSyncLog") else { return }
        print("broadwave nowplaying \(line)")
        fflush(stdout)
    }

    func end() {
        current = nil
        rateWatch = nil
        watched = nil
        shown = nil
        session = nil
        model = nil
    }

    #if compiler(>=6.4)
        @available(iOS 27, tvOS 27, *)
        private func watch(_ player: AVPlayer?, media _: LiveMedia) {
            if watched !== player {
                watched = player
                rateWatch = player?.observe(\.timeControlStatus, options: [.initial, .new]) { [weak self] _, _ in
                    Task { @MainActor in self?.noteState() }
                }
            }
            noteState()
        }

        private func noteState() {
            guard #available(iOS 27, tvOS 27, *), let media = model as? LiveMedia else { return }
            let state: MediaPlaybackSnapshot.PlaybackState = switch watched?.timeControlStatus {
            case .playing?: .playing(rate: 1)
            case .waitingToPlayAtSpecifiedRate?: .buffering
            case .paused?: .paused
            case nil: paused ? .paused : .playing(rate: 1)
            @unknown default: .playing(rate: 1)
            }
            let snapshot = MediaPlaybackSnapshot(state: state, timestamp: Date())
            if media.playbackSnapshot != snapshot {
                media.playbackSnapshot = snapshot
            }
        }

        /// "Jeopardy!" over "4.1 WDAF · Season 41, episode 12", live, with the program's art.
        @available(iOS 27, tvOS 27, *)
        private static func content(_ show: Show) -> any MediaContentRepresentable {
            let channel = show.channel
            let name = "\(channel.displayNumber) \(channel.displayName)"
            let episode = show.airing.flatMap {
                ProgramLine.episode(label: $0.episodeLabel, season: $0.season, episode: $0.episode, subtitle: $0.subtitle)
            }
            let id = "channel-\(channel.id)-\(show.airing?.id ?? 0)"
            var content = GenericContent(
                id: id,
                title: show.airing?.title ?? channel.displayName,
                subtitle: [name, episode].compactMap(\.self).joined(separator: " · "),
                type: .video,
                duration: .live,
                artwork: show.art.map(artwork)
            )
            content.genre = show.airing?.category.flatMap { $0.isEmpty ? nil : $0 }
            // Siri and Apple Intelligence then know which channel "this" is.
            content.appEntityIdentifiers = [EntityIdentifier(for: ChannelEntity.self, identifier: Int(channel.id))]
            return content
        }

        @available(iOS 27, tvOS 27, *)
        private static func artwork(_ url: URL) -> Artwork {
            Artwork(id: url.absoluteString) { _ in
                let (data, _) = try await URLSession.shared.data(from: url)
                return try ArtworkRepresentation(data: data)
            }
        }
    #endif
}

#if compiler(>=6.4)
    @available(iOS 27, tvOS 27, *)
    @MainActor
    @Observable
    private final class LiveMedia: MediaSessionRepresentable {
        let id = "com.wolfeup.broadwave.live"
        var content: (any MediaContentRepresentable)?
        var playbackSnapshot: MediaPlaybackSnapshot?
        var commands: [MediaCommand] = []
    }
#endif
