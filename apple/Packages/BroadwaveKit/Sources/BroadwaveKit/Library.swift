import Foundation

/// The recordings library, by the same rules as the web's `features/library/model.ts`.
public enum Library {
    public enum Sort: String, CaseIterable, Sendable {
        case newest, oldest, title, largest

        public var label: String {
            switch self {
            case .newest: "Newest first"
            case .oldest: "Oldest first"
            case .title: "By name"
            case .largest: "Largest first"
            }
        }
    }

    public enum Kind: String, CaseIterable, Sendable {
        case all, shows, movies, sports

        public var label: String {
            switch self {
            case .all: "Everything"
            case .shows: "Shows"
            case .movies: "Movies"
            case .sports: "Sports"
            }
        }
    }

    public struct Season: Identifiable, Sendable {
        /// 0 holds the recordings the guide gave no season.
        public let season: Int
        public let items: [Recording]
        public var id: Int {
            season
        }

        public var title: String {
            season > 0 ? "Season \(season)" : "Other episodes"
        }
    }

    public struct Show: Identifiable, Sendable {
        public let title: String
        public let items: [Recording]
        public let seasons: [Season]
        public let unwatched: Int
        public let bytes: Int64
        public var id: String {
            title.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        }

        /// "5 recordings · 4 unwatched · 34.4 MB"
        public var line: String {
            let count = items.count == 1 ? "1 recording" : "\(items.count) recordings"
            let left = unwatched == 0 ? "all watched" : "\(unwatched) unwatched"
            return "\(count) · \(left) · \(ByteCountFormatter.string(fromByteCount: bytes, countStyle: .file))"
        }
    }

    /// Started and not finished, most recently played first.
    public static func continueWatching(_ recordings: [Recording], limit: Int = 10) -> [Recording] {
        let started = recordings.filter { !$0.isRecording && !$0.isMissing && ($0.position ?? 0) > 30 && !$0.isWatched }
        return Array(started.sorted { ($0.progressAt ?? $0.startedAt) > ($1.progressAt ?? $1.startedAt) }.prefix(limit))
    }

    public static func filter(_ recordings: [Recording], kind: Kind, unwatchedOnly: Bool) -> [Recording] {
        recordings.filter { (kind == .all || kindOf($0) == kind) && (!unwatchedOnly || !$0.isWatched) }
    }

    /// Movies stay single; everything else groups by title, then by season when the guide gave one.
    public static func build(_ recordings: [Recording], sort: Sort) -> (shows: [Show], movies: [Recording]) {
        let by = order(sort)
        let movies = recordings.filter(\.isMovie).sorted(by: by)
        var keys: [String] = []
        var groups: [String: [Recording]] = [:]
        for rec in recordings where !rec.isMovie {
            let key = rec.title.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            if groups[key] == nil {
                keys.append(key)
            }
            groups[key, default: []].append(rec)
        }
        var shows = keys.map { key -> Show in
            let items = (groups[key] ?? []).sorted(by: by)
            let numbers = Array(Set(items.map { $0.season ?? 0 }))
            let seasons = numbers.sorted { a, b in
                if a == 0 || b == 0 {
                    return b == 0 && a != 0
                }
                return sort == .oldest ? a < b : a > b
            }
            .map { number in Season(season: number, items: items.filter { ($0.season ?? 0) == number }) }
            return Show(
                title: items[0].title,
                items: items,
                seasons: seasons,
                unwatched: items.count(where: { !$0.isWatched }),
                bytes: items.reduce(0) { $0 + ($1.bytes ?? 0) }
            )
        }
        func newest(_ show: Show) -> Date {
            show.items.map(\.startedAt).max() ?? .distantPast
        }
        shows.sort { a, b in
            switch sort {
            case .newest: newest(a) > newest(b)
            case .oldest: newest(a) < newest(b)
            case .title: a.title.localizedCaseInsensitiveCompare(b.title) == .orderedAscending
            case .largest: a.bytes > b.bytes
            }
        }
        return (shows, movies)
    }

    public static func totalBytes(_ recordings: [Recording]) -> Int64 {
        recordings.reduce(0) { $0 + ($1.bytes ?? 0) }
    }

    /// "3 recordings · 1.2 GB"
    public static func summary(_ recordings: [Recording]) -> String {
        let count = recordings.count == 1 ? "1 recording" : "\(recordings.count) recordings"
        return "\(count) · \(ByteCountFormatter.string(fromByteCount: totalBytes(recordings), countStyle: .file))"
    }

    private static func kindOf(_ rec: Recording) -> Kind {
        if rec.isMovie {
            return .movies
        }
        if rec.isSports {
            return .sports
        }
        return .shows
    }

    private static func order(_ sort: Sort) -> (Recording, Recording) -> Bool {
        { a, b in
            switch sort {
            case .newest: aired(b, a) ?? (a.id > b.id)
            case .oldest: aired(a, b) ?? (a.id < b.id)
            case .title:
                switch (a.subtitle ?? a.title).localizedCaseInsensitiveCompare(b.subtitle ?? b.title) {
                case .orderedAscending: true
                case .orderedDescending: false
                case .orderedSame: a.id > b.id
                }
            case .largest: (a.bytes ?? 0) == (b.bytes ?? 0) ? a.id > b.id : (a.bytes ?? 0) > (b.bytes ?? 0)
            }
        }
    }

    /// Season, then episode, then the day it was recorded; nil when all three tie.
    private static func aired(_ a: Recording, _ b: Recording) -> Bool? {
        if (a.season ?? 0) != (b.season ?? 0) {
            return (a.season ?? 0) < (b.season ?? 0)
        }
        if (a.episode ?? 0) != (b.episode ?? 0) {
            return (a.episode ?? 0) < (b.episode ?? 0)
        }
        if a.startedAt != b.startedAt {
            return a.startedAt < b.startedAt
        }
        return nil
    }
}

public extension Recording {
    var isSports: Bool {
        gameId?.isEmpty == false || (category ?? "").lowercased().contains("sport")
    }

    /// "S2 E5", "E5", the guide's own label, or nil.
    var episodeTag: String? {
        if let season, let episode, season > 0, episode > 0 {
            return "S\(season) E\(episode)"
        }
        if let episode, episode > 0 {
            return "E\(episode)"
        }
        if let episodeLabel, !episodeLabel.isEmpty {
            return episodeLabel
        }
        return nil
    }
}
