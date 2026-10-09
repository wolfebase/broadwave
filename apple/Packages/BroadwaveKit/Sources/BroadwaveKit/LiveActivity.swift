#if os(iOS)
    import ActivityKit
    import Foundation

    /// One Live Activity: a recording in progress or a followed game on now.
    /// The app starts and updates it; the widget extension draws it.
    public struct BroadwaveActivity: ActivityAttributes {
        public typealias ContentState = LiveActivityFeed.Content

        public var id: String
        /// The server it came from. Ids are only unique on one server.
        public var server: String
        public var kind: LiveActivityFeed.Kind
        public var channelID: Int64

        public init(id: String, server: String, kind: LiveActivityFeed.Kind, channelID: Int64) {
            self.id = id
            self.server = server
            self.kind = kind
            self.channelID = channelID
        }

        public var link: URL {
            TopShelf.watchLink(channelID)
        }
    }
#endif
