import BroadwaveKit
import SwiftUI

extension View {
    /// While a live channel plays, a nearby iPhone, iPad, or Mac can pick it up.
    /// A Mac opens the server's web player. Apple TV takes no Handoff.
    func watchHandoff(_ channel: Channel?, server: FoundServer?, open: @escaping (Int64) -> Void) -> some View {
        #if os(iOS)
            let live = server.flatMap { $0.id == "demo" ? nil : $0 }
            return userActivity(WatchHandoff.activityType, isActive: channel != nil && live != nil) { activity in
                guard let channel, let live else { return }
                let number = channel.displayNumber.isEmpty ? channel.guideNumber : channel.displayNumber
                activity.title = "\(number) \(channel.displayName)"
                activity.userInfo = [WatchHandoff.channelKey: channel.id, WatchHandoff.serverKey: live.id]
                activity.isEligibleForHandoff = true
                activity.webpageURL = WatchHandoff.webpageURL(base: live.url, channelID: channel.id)
            }
            .onContinueUserActivity(WatchHandoff.activityType) { activity in
                if let id = WatchHandoff.channel(activity.userInfo, server: server?.id) {
                    open(id)
                }
            }
        #else
            self
        #endif
    }
}
