import BroadwaveKit
import Foundation
@preconcurrency import TVServices

/// Fills the Top Shelf when Broadwave sits in the top row of the Apple TV home screen.
final class ContentProvider: TVTopShelfContentProvider {
    override func loadTopShelfContent(completionHandler: @escaping ((any TVTopShelfContent)?) -> Void) {
        // The system takes the answer on any thread. The async override cannot hand back a
        // non-Sendable value, and Xcode 26's isolation checker fails on a Sendable wrapper.
        nonisolated(unsafe) let reply = completionHandler
        Task {
            guard let sections = await Self.sections() else {
                reply(nil)
                return
            }
            reply(TVTopShelfSectionedContent(sections: sections.map { section in
                let collection = TVTopShelfItemCollection(items: section.items.map(Self.item))
                collection.title = section.title
                return collection
            }))
        }
    }

    private static func sections() async -> [TopShelf.Section]? {
        guard let base = SharedServer.load() else { return nil }
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 8
        let api = APIClient(base: base, session: URLSession(configuration: config))
        let now = Date()
        async let channels = try? api.channels()
        async let airings = try? api.airings(from: now, to: now.addingTimeInterval(60))
        async let recordings = try? api.recordings()
        async let frames = try? api.frames()
        guard let lineup = await channels else { return nil }
        let snap = await TopShelf.Snapshot(
            channels: lineup, airings: airings ?? [], recordings: recordings ?? [], framed: Set(frames?.channels ?? [])
        )
        let sections = TopShelf.sections(snap, api: api, now: now)
        return sections.isEmpty ? nil : sections
    }

    private static func item(_ entry: TopShelf.Item) -> TVTopShelfSectionedItem {
        let item = TVTopShelfSectionedItem(identifier: entry.id)
        item.title = entry.title
        item.imageShape = .hdtv
        if let image = entry.image {
            item.setImageURL(image, for: [.screenScale1x, .screenScale2x])
        }
        item.displayAction = TVTopShelfAction(url: entry.link)
        item.playAction = TVTopShelfAction(url: entry.link)
        if let progress = entry.progress {
            item.playbackProgress = progress
        }
        return item
    }
}
