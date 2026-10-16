import Foundation

/// The full-screen Top Shelf: what is on now, one live still per channel,
/// with the recordings someone is part way through after it.
public extension TopShelf {
    struct Slide: Equatable, Sendable {
        public var id: String
        public var title: String
        /// The line over the title, such as "Live on 4.1 FOX".
        public var context: String
        public var summary: String?
        public var genre: String?
        public var image: URL?
        /// Play goes here.
        public var play: URL
        /// More Info goes here.
        public var info: URL
        /// Length of a recording. Zero for live TV: a live showing has no length to play.
        public var duration: TimeInterval
        /// Name and value pairs under the summary, such as "Ends" and "8:30 PM".
        public var facts: [Fact]
        /// A recording's own picture. Never a live channel's: it would hold a tuner.
        public var preview: URL?
        /// How far into a recording, 0 to 1. Nil for live TV.
        public var progress: Double?
        public var hd: Bool

        public struct Fact: Equatable, Sendable {
            public var name: String
            public var value: String
        }
    }

    /// Channels on now, the ones watched last first, then favorites, then games, then the
    /// rest that have a live still. Recordings to finish come after them.
    static func carousel(
        _ snap: Snapshot, api: APIClient, now: Date, recent: [Int64] = [], limit: Int = 10
    ) -> [Slide] {
        let shown = snap.channels.filter { $0.enabled && !$0.hidden }
        let current = onNow(snap.airings, at: now)
        let byID = Dictionary(shown.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })

        var order: [Channel] = []
        var seen = Set<Int64>()
        func take(_ channel: Channel?) {
            guard let channel, seen.insert(channel.id).inserted else { return }
            order.append(channel)
        }
        recent.forEach { take(byID[$0]) }
        shown.filter(\.favorite).forEach(take)
        shown.filter { current[$0.id]?.kind == .sports }.forEach(take)
        // The rest only with a still or a listing: a bare channel name makes a poor full-screen slide.
        shown.filter { snap.framed.contains($0.id) && current[$0.id] != nil }.forEach(take)

        let resume = Library.continueWatching(snap.recordings, limit: 2)
        let liveRoom = max(1, limit - resume.count)
        let live = order.lazy
            .filter { snap.framed.contains($0.id) || current[$0.id] != nil }
            .prefix(liveRoom)
            .map { slide($0, airing: current[$0.id], api: api, framed: snap.framed, now: now) }
        return Array(live) + resume.map { recordingSlide($0, api: api) }
    }

    /// The listing on now for each channel. The one that started first stays on, as in the guide.
    static func onNow(_ airings: [Airing], at now: Date) -> [Int64: Airing] {
        var current: [Int64: Airing] = [:]
        for airing in airings where airing.isOn(at: now) {
            if let have = current[airing.channelId], have.start <= airing.start {
                continue
            }
            current[airing.channelId] = airing
        }
        return current
    }

    internal static func slide(_ channel: Channel, airing: Airing?, api: APIClient, framed: Set<Int64>, now: Date) -> Slide {
        let name = "\(channel.displayNumber) \(channel.displayName)"
        // The server keeps a 1280 still; the home screen scales it to the full width.
        let image = api.frameURL(channelID: channel.id, width: 1920, listed: framed).map { fresh($0, now) }
            ?? airing.flatMap { a in a.imageUrl?.isEmpty == false ? api.artURL(kind: "airing", id: a.id, width: 1920) : nil }
            ?? (channel.artUrl?.isEmpty == false ? api.artURL(kind: "channel", id: channel.id, width: 640) : nil)
        var facts: [Slide.Fact] = []
        if let airing {
            let left = max(0, airing.end.timeIntervalSince(now))
            facts.append(Slide.Fact(name: String(localized: "Time left", bundle: .module), value: remaining(left)))
            facts.append(Slide.Fact(
                name: String(localized: "Ends", bundle: .module),
                value: airing.end.formatted(date: .omitted, time: .shortened)
            ))
        }
        facts.append(Slide.Fact(name: String(localized: "Channel", bundle: .module), value: name))
        let episode = airing.flatMap {
            ProgramLine.episode(label: $0.episodeLabel, season: $0.season, episode: $0.episode, subtitle: $0.subtitle)
        }
        let about = [episode, airing?.description].compactMap { text in
            text.flatMap { $0.isEmpty ? nil : $0 }
        }.joined(separator: " · ")
        return Slide(
            id: "channel-\(channel.id)",
            title: airing?.title ?? channel.displayName,
            context: String(localized: "shelf.liveOn", defaultValue: "Live on \(name)", bundle: .module),
            summary: about.isEmpty ? nil : about,
            genre: airing.map(genre),
            image: image,
            play: watchLink(channel.id),
            info: guideLink(channel.id),
            duration: 0,
            facts: facts,
            preview: nil,
            progress: nil,
            hd: channel.hd
        )
    }

    internal static func recordingSlide(_ rec: Recording, api: APIClient) -> Slide {
        let length = rec.durationSec ?? 0
        let progress = length > 0 ? min(1, max(0, (rec.position ?? 0) / length)) : nil
        var facts: [Slide.Fact] = []
        if length > 0, let position = rec.position, position > 0 {
            facts.append(Slide.Fact(name: String(localized: "Time left", bundle: .module), value: remaining(length - position)))
        }
        facts.append(Slide.Fact(
            name: String(localized: "Recorded", bundle: .module),
            value: rec.startedAt.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day())
        ))
        let episode = ProgramLine.episode(label: rec.episodeLabel, season: rec.season, episode: rec.episode, subtitle: rec.subtitle)
        let about = [episode, rec.description].compactMap { text in
            text.flatMap { $0.isEmpty ? nil : $0 }
        }.joined(separator: " · ")
        return Slide(
            id: "recording-\(rec.id)",
            title: rec.title,
            context: String(localized: "Continue watching", bundle: .module),
            summary: about.isEmpty ? nil : about,
            genre: rec.category.flatMap { $0.isEmpty ? nil : $0 },
            image: api.posterURL(recordingID: rec.id),
            play: recordingLink(rec.id),
            info: recordingLink(rec.id),
            duration: max(0, length),
            facts: facts,
            preview: api.recordingPreviewURL(recordingID: rec.id),
            progress: progress,
            hd: true
        )
    }

    /// The listing's own category when it has one ("Comedy"), else the guide's kind ("Sports").
    internal static func genre(_ airing: Airing) -> String {
        let listed = (airing.category ?? "").split(separator: ",").first.map { $0.trimmingCharacters(in: .whitespaces) } ?? ""
        return listed.isEmpty ? airing.kind.label : listed
    }

    /// "25 min", "1 hr 5 min".
    internal static func remaining(_ seconds: TimeInterval) -> String {
        let minutes = max(1, Int((seconds / 60).rounded(.up)))
        return Duration.seconds(minutes * 60).formatted(.units(allowed: [.hours, .minutes], width: .abbreviated))
    }

    /// The guide, opened on this channel.
    static func guideLink(_ channelID: Int64) -> URL {
        URL(string: "broadwave://guide?channel=\(channelID)")!
    }
}

/// Channels watched on this device, newest first, kept beside the server address
/// so the Top Shelf leads with them.
public enum SharedRecents {
    public static let limit = 6

    public static func adding(_ id: Int64, to list: [Int64]) -> [Int64] {
        Array(([id] + list.filter { $0 != id }).prefix(limit))
    }

    public static func note(_ id: Int64) {
        let list = adding(id, to: load())
        SharedServer.write(try? JSONEncoder().encode(list), account: "recent")
    }

    public static func load() -> [Int64] {
        guard let data = SharedServer.read(account: "recent") else { return [] }
        return (try? JSONDecoder().decode([Int64].self, from: data)) ?? []
    }
}
