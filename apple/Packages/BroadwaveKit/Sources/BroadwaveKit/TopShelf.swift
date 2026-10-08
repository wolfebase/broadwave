import Foundation

/// What the Apple TV Top Shelf shows: games on now, favorite channels on now,
/// and recordings started and not finished.
public enum TopShelf {
    public struct Item: Equatable, Sendable {
        public var id: String
        public var title: String
        public var image: URL?
        public var link: URL
        /// How far into a recording, 0 to 1. Nil for live TV.
        public var progress: Double?
    }

    public struct Section: Equatable, Sendable {
        public var title: String
        public var items: [Item]
    }

    /// What the server said, read in one go.
    public struct Snapshot: Sendable {
        public var channels: [Channel]
        public var airings: [Airing]
        public var recordings: [Recording]
        /// Channels with a fresh preview frame (`GET /frames`).
        public var framed: Set<Int64>

        public init(channels: [Channel], airings: [Airing] = [], recordings: [Recording] = [], framed: Set<Int64> = []) {
            self.channels = channels
            self.airings = airings
            self.recordings = recordings
            self.framed = framed
        }
    }

    public static func sections(_ snap: Snapshot, api: APIClient, now: Date, limit: Int = 10) -> [Section] {
        let framed = snap.framed
        let shown = snap.channels.filter { $0.enabled && !$0.hidden }
        var current: [Int64: Airing] = [:]
        for airing in snap.airings where airing.start <= now && airing.end > now {
            current[airing.channelId] = airing
        }

        func live(_ channel: Channel) -> Item {
            let airing = current[channel.id]
            let image = api.frameURL(channelID: channel.id, width: 1280, listed: framed).map { fresh($0, now) }
                ?? airing.flatMap { a in a.imageUrl?.isEmpty == false ? api.artURL(kind: "airing", id: a.id, width: 1280) : nil }
                ?? (channel.artUrl?.isEmpty == false ? api.artURL(kind: "channel", id: channel.id, width: 320) : nil)
            let title = airing.map { "\(channel.displayNumber) · \($0.title)" } ?? "\(channel.displayNumber) \(channel.displayName)"
            return Item(id: "channel-\(channel.id)", title: title, image: image, link: watchLink(channel.id), progress: nil)
        }

        let games = shown.filter { current[$0.id]?.kind == .sports }
        let gameIDs = Set(games.map(\.id))
        let favorites = shown.filter { $0.favorite && !gameIDs.contains($0.id) }
        let resume = Library.continueWatching(snap.recordings, limit: limit).map { rec in
            let progress = rec.durationSec.flatMap { d in d > 0 ? min(1, max(0, (rec.position ?? 0) / d)) : nil }
            return Item(
                id: "recording-\(rec.id)",
                title: rec.subtitle.map { "\(rec.title): \($0)" } ?? rec.title,
                image: api.posterURL(recordingID: rec.id),
                link: recordingLink(rec.id),
                progress: progress
            )
        }

        let all = [
            Section(title: "Games on now", items: games.prefix(limit).map(live)),
            Section(title: "Favorites", items: favorites.prefix(limit).map(live)),
            Section(title: "Continue watching", items: resume),
        ]
        return all.filter { !$0.items.isEmpty }
    }

    /// The home screen keeps a shelf image by its URL, and a channel's frame URL never changes.
    /// The minute makes a new one; the server ignores it.
    static func fresh(_ url: URL, _ now: Date) -> URL {
        url.appending(queryItems: [URLQueryItem(name: "t", value: String(Int(now.timeIntervalSince1970) / 60))])
    }

    public static func watchLink(_ channelID: Int64) -> URL {
        URL(string: "broadwave://watch/\(channelID)")!
    }

    public static func recordingLink(_ recordingID: Int64) -> URL {
        URL(string: "broadwave://recording/\(recordingID)")!
    }
}
