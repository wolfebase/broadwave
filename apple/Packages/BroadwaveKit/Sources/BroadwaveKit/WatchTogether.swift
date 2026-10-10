import Foundation

/// What a SharePlay session shares: a channel on one server. SharePlay carries
/// no picture. Each device plays from the server itself and joins the channel's
/// room, so Whole-Home Sync keeps every screen on the same frame. A channel
/// change replaces the session's invite, which reaches everyone in it.
public struct WatchTogether: Codable, Sendable, Hashable {
    public var serverID: String
    public var serverName: String
    public var channelID: Int64
    public var channelName: String

    public init(serverID: String, serverName: String, channelID: Int64, channelName: String) {
        self.serverID = serverID
        self.serverName = serverName
        self.channelID = channelID
        self.channelName = channelName
    }

    /// The invite for a live channel, or nil when there is nothing another device
    /// could open: the demo, a server still connecting, or a bad channel id.
    public static func invite(server: FoundServer?, channel: Channel?) -> WatchTogether? {
        guard let server, let channel, channel.id > 0, server.id != "demo", server.id != "pending", !server.id.isEmpty else { return nil }
        let number = channel.displayNumber.isEmpty ? channel.guideNumber : channel.displayNumber
        let name = [number, channel.displayName].filter { !$0.isEmpty }.joined(separator: " ")
        return WatchTogether(serverID: server.id, serverName: server.name, channelID: channel.id, channelName: name)
    }
}

/// What this device does with an invite.
public enum TogetherJoin: Equatable, Sendable {
    /// Play the channel on the server this device uses now.
    case watch(Int64)
    /// Switch to a server this device already knows, then play the channel.
    case connect(FoundServer, Int64)
    /// This device can't open it. The message says why.
    case cannot(String)

    /// Only a server this device was set up with. SharePlay never relays a broadcast.
    public static func decide(_ invite: WatchTogether, server: FoundServer?, remembered: [FoundServer]) -> TogetherJoin {
        guard invite.channelID > 0, !invite.serverID.isEmpty, invite.serverID != "demo" else {
            return .cannot("This SharePlay can't be opened here.")
        }
        if server?.id == invite.serverID {
            return .watch(invite.channelID)
        }
        if let known = remembered.first(where: { $0.id == invite.serverID }) {
            return .connect(known, invite.channelID)
        }
        let name = invite.serverName.trimmingCharacters(in: .whitespacesAndNewlines)
        let place = name.isEmpty ? "another Broadwave server" : name
        return .cannot("\(invite.channelName) is on \(place), which this device isn't set up with.")
    }
}
