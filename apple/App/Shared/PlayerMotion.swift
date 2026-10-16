import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// The namespace a channel tile and the full-screen player share, so on iPhone and iPad
/// the player grows out of the tile that started it and shrinks back into it.
extension EnvironmentValues {
    @Entry var playerZoom: Namespace.ID?
}

enum PlayerZoom {
    /// The mini player's id. Any other source is a channel.
    static let mini = "mini-player"

    static func id(_ channelID: Int64) -> String {
        "channel-\(channelID)"
    }
}

private struct PlayerZoomSource: ViewModifier {
    @Environment(\.playerZoom) private var zoom
    let id: String

    func body(content: Content) -> some View {
        #if os(iOS)
            if let zoom {
                content.matchedTransitionSource(id: id, in: zoom) { source in
                    source.clipShape(.rect(cornerRadius: Tokens.Radius.lg))
                }
            } else {
                content
            }
        #else
            content
        #endif
    }
}

private struct PlayerZoomDestination: ViewModifier {
    @Environment(\.playerZoom) private var zoom
    let id: String?

    func body(content: Content) -> some View {
        #if os(iOS)
            if let zoom, let id {
                content.navigationTransition(.zoom(sourceID: id, in: zoom))
            } else {
                content
            }
        #else
            content
        #endif
    }
}

extension View {
    /// A tile the player can zoom out of.
    func playerZoomSource(_ channelID: Int64) -> some View {
        modifier(PlayerZoomSource(id: PlayerZoom.id(channelID)))
    }

    func playerZoomSource(id: String) -> some View {
        modifier(PlayerZoomSource(id: id))
    }

    /// The full-screen player, zooming from the tile named by `id`.
    func playerZoomDestination(_ id: String?) -> some View {
        modifier(PlayerZoomDestination(id: id))
    }

    /// Haptics for the moments that matter on iPhone: a channel change, a recording that
    /// starts, and screens that just lined up. Nothing on Apple TV or iPad without a Taptic Engine.
    func playerHaptics(channel: Int64?, recording: Bool, synced: Bool) -> some View {
        sensoryFeedback(.selection, trigger: channel) { old, new in old != nil && new != nil }
            .sensoryFeedback(trigger: recording) { _, on in on ? .start : .stop }
            .sensoryFeedback(.success, trigger: synced) { _, locked in locked }
    }
}

/// Swipe a channel row: Record from the leading edge, Favorite and Watch together from the trailing.
struct ChannelSwipeActions: ViewModifier {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    let channel: Channel

    func body(content: Content) -> some View {
        #if os(iOS)
            let recording = store.activeRecording(on: channel) != nil
            content
                .swipeActions(edge: .leading, allowsFullSwipe: true) {
                    Button(recording ? "Stop" : "Record", systemImage: recording ? "stop.circle" : "record.circle") {
                        Task { await store.toggleRecord(channel) }
                    }
                    .tint(Tokens.ColorToken.tally)
                }
                .swipeActions(edge: .trailing) {
                    Button(channel.favorite ? "Unfavorite" : "Favorite", systemImage: channel.favorite ? "star.slash" : "star.fill") {
                        Task { await store.toggleFavorite(channel) }
                    }
                    .tint(Tokens.ColorToken.warning)
                    Button("Together", systemImage: "rectangle.split.2x1") {
                        if let current = nowPlaying.channel, current.id != channel.id {
                            nowPlaying.watchTogether([current, channel])
                        } else {
                            nowPlaying.watchTogether([channel])
                        }
                    }
                    .tint(.indigo)
                }
        #else
            content
        #endif
    }
}
