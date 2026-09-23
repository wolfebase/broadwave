import AVFoundation
import Foundation
import Observation

/// Holds an AVPlayer on a Whole-Home Sync room's timeline.
///
/// Segments carry EXT-X-PROGRAM-DATE-TIME on the channel's shared timeline, so
/// `currentDate()` is the same wall time for the same frame on every screen.
/// Small drift is trimmed with rate, a screen that is ahead pauses for exactly
/// its drift, and a screen that is behind seeks forward by date.
@MainActor
@Observable
public final class SyncEngine {
    public enum State: String, Sendable { case off, waiting, syncing, locked }

    public private(set) var state: State = .off
    public private(set) var drift: Double = 0
    public private(set) var members = 0

    private let player: AVPlayer
    private let socket: EventSocket
    private let room: String
    private let channelID: Int64
    private var room_: RoomState?
    private var handler: UUID?
    private var timer: Timer?
    private var holdUntil = Date.distantPast
    private var lastSeek = Date.distantPast

    static let trimMS = 20.0
    static let seekMS = 400.0
    static let maxTrim = 0.03

    public init(player: AVPlayer, socket: EventSocket, room: String, channelID: Int64) {
        self.player = player
        self.socket = socket
        self.room = room
        self.channelID = channelID
    }

    public func start() {
        handler = socket.on("sync.state") { [weak self] data in
            guard let self, let st = try? JSONDecoder().decode(RoomState.self, from: data), st.room == self.room else { return }
            room_ = st
            members = st.members
            apply()
        }
        socket.join(room: room, channelID: channelID)
        state = .waiting
        if let cached = socket.roomState(room), let st = try? JSONDecoder().decode(RoomState.self, from: cached), st.room == room {
            room_ = st
            members = st.members
            apply()
        }
        timer = Timer.scheduledTimer(withTimeInterval: 0.25, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.apply() }
        }
    }

    public func stop() {
        timer?.invalidate()
        if let handler {
            socket.off("sync.state", handler)
        }
        socket.leave(room: room)
        player.rate = player.rate == 0 ? 0 : 1
        state = .off
    }

    public func command(_ action: String, mediaTime: Double? = nil) {
        socket.command(room: room, action: action, mediaTime: mediaTime)
    }

    /// Program date-time of the frame on screen, Unix ms.
    public func mediaNow() -> Double? {
        guard let date = player.currentItem?.currentDate() else { return nil }
        return date.timeIntervalSince1970 * 1000
    }

    private func apply() {
        guard let st = room_, Date() >= holdUntil else { return }
        guard let item = player.currentItem, item.status == .readyToPlay, let local = mediaNow() else {
            state = .waiting
            return
        }
        let target = st.rate == 0 ? st.anchorMedia : st.target(atServer: socket.serverNow())
        let d = local - target
        drift = d
        if st.rate == 0 {
            player.pause()
            if abs(d) > Self.trimMS * 2 {
                seek(to: target)
            }
            state = .locked
            return
        }
        if abs(d) > Self.seekMS {
            if d > 0 {
                holdUntil = Date().addingTimeInterval(d / 1000)
                player.pause()
                DispatchQueue.main.asyncAfter(deadline: .now() + d / 1000) { [weak self] in self?.player.play() }
            } else {
                seek(to: target)
            }
            state = .syncing
            return
        }
        if player.timeControlStatus == .paused {
            player.play()
        }
        if abs(d) > Self.trimMS {
            player.rate = Float(1 + max(-Self.maxTrim, min(Self.maxTrim, -d / 2000)))
            state = .syncing
        } else {
            if player.rate != 1 {
                player.rate = 1
            }
            state = .locked
        }
    }

    private func seek(to media: Double) {
        guard Date().timeIntervalSince(lastSeek) > 2, let item = player.currentItem else { return }
        lastSeek = Date()
        item.seek(to: Date(timeIntervalSince1970: media / 1000)) { _ in }
    }
}
