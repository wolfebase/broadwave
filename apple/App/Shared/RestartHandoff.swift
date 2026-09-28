import AVFoundation
import BroadwaveKit

/// A restarted server has lost every watch. The old item plays on from its
/// buffer while a new watch fills, and the swap waits until AVPlayer can start
/// the new playlist or the old picture is about to run dry. A restart then
/// costs a short cut instead of a whole fresh-tune start on a frozen picture.
@MainActor
enum RestartHandoff {
    /// The new watch and whether its playlist was ready, or nil when the watch
    /// failed. `current` turns false when the viewer moved on; the caller then
    /// stops the returned watch.
    static func prepare(
        api: APIClient,
        player: AVPlayer,
        rewatch: () async throws -> WatchSession,
        current: () -> Bool
    ) async -> (session: WatchSession, ready: Bool)? {
        guard let next = try? await rewatch() else { return nil }
        let deadline = Date().addingTimeInterval(8)
        while Date() < deadline, current(), bufferedAhead(player) > 0.5 {
            if let text = await api.playlistText(next.playlist), LiveReadiness.ready(text) {
                return (next, true)
            }
            try? await Task.sleep(for: .milliseconds(250))
        }
        return (next, false)
    }

    /// Seconds of picture the current item holds past the playhead.
    static func bufferedAhead(_ player: AVPlayer) -> Double {
        guard let item = player.currentItem else { return 0 }
        let now = item.currentTime()
        let end = item.loadedTimeRanges.map(\.timeRangeValue).filter { $0.containsTime(now) }.map(\.end.seconds).max()
        guard let end, now.seconds.isFinite, end.isFinite else { return 0 }
        return max(0, end - now.seconds)
    }
}
