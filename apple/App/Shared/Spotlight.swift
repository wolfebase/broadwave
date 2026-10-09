import SwiftUI
#if os(iOS)
    import BroadwaveKit
    import CoreSpotlight

    /// Channels and recordings in Spotlight. Each item's id is the link it opens.
    enum Spotlight {
        private static let channels = "channels"
        private static let recordings = "recordings"

        /// Replaces what was indexed. The demo and a forgotten server index nothing.
        static func index(server: FoundServer?, channels shown: [Channel], recordings saved: [Recording]) async {
            let index = CSSearchableIndex.default()
            try? await index.deleteSearchableItems(withDomainIdentifiers: [channels, recordings])
            guard let server, server.id != "demo" else { return }
            var items = shown.filter { $0.enabled && !$0.hidden }.map { channel in
                let set = CSSearchableItemAttributeSet(contentType: .content)
                set.title = "\(channel.displayNumber) \(channel.displayName)"
                set.contentDescription = channel.network.map { "\($0) · Watch live" } ?? "Watch live"
                return CSSearchableItem(
                    uniqueIdentifier: TopShelf.watchLink(channel.id).absoluteString, domainIdentifier: channels, attributeSet: set
                )
            }
            items += saved.filter { !$0.isRecording && $0.missing != true }.map { rec in
                let set = CSSearchableItemAttributeSet(contentType: .movie)
                set.title = rec.title
                set.contentDescription = [rec.subtitle, rec.startedAt.formatted(date: .abbreviated, time: .shortened)]
                    .compactMap(\.self).joined(separator: " · ")
                return CSSearchableItem(
                    uniqueIdentifier: TopShelf.recordingLink(rec.id).absoluteString, domainIdentifier: recordings, attributeSet: set
                )
            }
            // A newer pass (the lineup changed again) started while this one built its list.
            guard !Task.isCancelled else { return }
            try? await index.indexSearchableItems(items)
        }

        /// The link a tapped Spotlight result opens.
        static func link(_ activity: NSUserActivity) -> URL? {
            (activity.userInfo?[CSSearchableItemActivityIdentifier] as? String).flatMap(URL.init(string:))
        }
    }
#endif

extension View {
    /// Opens a tapped Spotlight result as its link. Apple TV has no Spotlight results.
    func spotlightLinks(_ open: @escaping (URL) -> Void) -> some View {
        #if os(iOS)
            onContinueUserActivity(CSSearchableItemActionType) { activity in
                if let url = Spotlight.link(activity) {
                    open(url)
                }
            }
        #else
            self
        #endif
    }
}
