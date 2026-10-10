import AppIntents
import BroadwaveKit
import Foundation

// Siri, Shortcuts, and the Control Center control. Each intent asks the saved
// server (the shared keychain entry the widgets use) and opens the app by link.

enum IntentFailure: Error, CustomLocalizedStringResourceConvertible {
    case noServer
    case unreachable
    case noGame(String)
    case noShow(String)

    var localizedStringResource: LocalizedStringResource {
        switch self {
        case .noServer: "Open Broadwave to pick your server."
        case .unreachable: "Can't reach your server."
        case let .noGame(team): "No \(team) game in the next 12 hours."
        case let .noShow(name): "\(name) isn't in the guide."
        }
    }
}

/// One session for every intent, with a whole-request cap: Siri gives an intent a few seconds.
private let intentSession: URLSession = {
    let config = URLSessionConfiguration.ephemeral
    config.timeoutIntervalForResource = 10
    return URLSession(configuration: config)
}()

func intentAPI() throws -> APIClient {
    guard let base = SharedServer.load() else {
        throw IntentFailure.noServer
    }
    return APIClient(base: base, session: intentSession)
}

/// A server call whose failure Siri should say plainly, not as a URL error.
func ask<T>(_ call: () async throws -> T) async throws -> T {
    do {
        return try await call()
    } catch {
        throw IntentFailure.unreachable
    }
}

/// The server's channels, or the lineup the app last saw when the server can't
/// be asked, so a saved Shortcut or Control Center channel still opens the app.
func lineup(_ call: (APIClient) async throws -> [Channel]) async throws -> [Channel] {
    let api = try intentAPI()
    do {
        return try await call(api)
    } catch {
        let kept = SharedLineup.load()
        guard !kept.isEmpty else { throw IntentFailure.unreachable }
        return kept
    }
}

struct ChannelEntity: AppEntity {
    static let typeDisplayRepresentation: TypeDisplayRepresentation = "Channel"
    static let defaultQuery = ChannelQuery()

    /// The channel id. AppIntents takes Int, not Int64.
    var id: Int
    var number: String
    var name: String
    var network: String?

    init(_ channel: Channel) {
        id = Int(channel.id)
        number = channel.displayNumber
        name = channel.displayName
        network = channel.network
    }

    /// Siri matches what it heard against the title and these: "9", "channel 9", "9.1", the name, the network.
    var displayRepresentation: DisplayRepresentation {
        var synonyms: [LocalizedStringResource] = ["\(number)", "channel \(number)", "\(name)"]
        if let major = number.split(separator: ".").first, major != Substring(number) {
            synonyms += ["\(String(major))", "channel \(String(major))"]
        }
        if let network {
            synonyms.append("\(network)")
        }
        return DisplayRepresentation(title: "\(number) \(name)", synonyms: synonyms)
    }
}

struct ChannelQuery: EntityStringQuery {
    func entities(for identifiers: [Int]) async throws -> [ChannelEntity] {
        let wanted = Set(identifiers.map(Int64.init))
        let channels = try await lineup { try await $0.allChannels() }
        return channels.filter { wanted.contains($0.id) }.map(ChannelEntity.init)
    }

    /// The whole lineup, hidden channels too, so an encrypted 3.0 number finds its clear twin.
    func entities(matching string: String) async throws -> [ChannelEntity] {
        let channels = try await lineup { try await $0.allChannels() }
        if let hit = Voice.channel(string, in: channels) {
            return [ChannelEntity(hit)]
        }
        return channels.filter { $0.enabled && !$0.hidden && $0.displayName.localizedCaseInsensitiveContains(string) }
            .map(ChannelEntity.init)
    }

    /// Favorites, then the first of the rest. A big playlist would make Siri's list too long.
    func suggestedEntities() async throws -> [ChannelEntity] {
        guard let shown = try? await lineup({ try await $0.channels() }) else { return [] }
        let visible = shown.filter { $0.enabled && !$0.hidden }
        return (visible.filter(\.favorite) + visible.filter { !$0.favorite }.prefix(30)).map(ChannelEntity.init)
    }
}

/// A followed team, so "Watch the Chiefs game" can be one phrase.
struct TeamEntity: AppEntity {
    static let typeDisplayRepresentation: TypeDisplayRepresentation = "Team"
    static let defaultQuery = TeamQuery()

    var id: String
    var short: String?
    var abbr: String?

    init(_ team: TeamFollow) {
        id = team.name
        short = team.short
        abbr = team.abbr
    }

    var displayRepresentation: DisplayRepresentation {
        DisplayRepresentation(title: "\(short ?? id)", synonyms: [short, abbr, id].compactMap(\.self).map { "\($0)" })
    }

    /// The words a listing uses: the short name ("Chiefs") over the full one.
    var listed: String {
        short.flatMap { $0.isEmpty ? nil : $0 } ?? id
    }
}

struct TeamQuery: EntityStringQuery {
    func entities(for identifiers: [String]) async throws -> [TeamEntity] {
        try await followed().filter { identifiers.contains($0.id) }
    }

    func entities(matching string: String) async throws -> [TeamEntity] {
        let said = Voice.tokens(Voice.team(string))
        return try await followed().filter { team in
            [team.short, team.abbr, team.id].compactMap(\.self).contains { Voice.tokens($0) == said }
                || Voice.tokens(team.id).contains(said.joined(separator: " "))
        }
    }

    func suggestedEntities() async throws -> [TeamEntity] {
        await (try? followed()) ?? []
    }

    private func followed() async throws -> [TeamEntity] {
        let api = try intentAPI()
        return try await ask { try await api.teams() }.map(TeamEntity.init)
    }
}

struct WatchChannelIntent: AppIntent {
    static let title: LocalizedStringResource = "Watch a channel"
    static let description = IntentDescription("Opens a channel live in Broadwave.")

    @Parameter(title: "Channel", requestValueDialog: "Which channel?") var channel: ChannelEntity

    init() {}

    init(channel: ChannelEntity) {
        self.channel = channel
    }

    func perform() async throws -> some IntentResult & OpensIntent {
        .result(opensIntent: OpenURLIntent(TopShelf.watchLink(Int64(channel.id))))
    }
}

struct WatchGameIntent: AppIntent {
    static let title: LocalizedStringResource = "Watch a game"
    static let description = IntentDescription("Opens a team's game, on now or the next one today.")

    @Parameter(title: "Team", requestValueDialog: "Which team?") var team: TeamEntity

    func perform() async throws -> some IntentResult & OpensIntent {
        let api = try intentAPI()
        let now = Date()
        let (channels, airings) = try await ask {
            async let channels = api.channels()
            async let airings = api.airings(from: now.addingTimeInterval(-4 * 3600), to: now.addingTimeInterval(12 * 3600), sportsOnly: true)
            return try await (channels, airings)
        }
        guard let game = Voice.game(team.listed, airings: airings, channels: channels, now: now) else {
            throw IntentFailure.noGame(team.listed)
        }
        return .result(opensIntent: OpenURLIntent(TopShelf.watchLink(game.channelId)))
    }
}

struct RecordShowIntent: AppIntent {
    static let title: LocalizedStringResource = "Record a show"
    static let description = IntentDescription("Records every new airing of a show, as Record series does in the app.")

    @Parameter(title: "Show", requestValueDialog: "Which show?") var show: String

    func perform() async throws -> some IntentResult & ProvidesDialog {
        let api = try intentAPI()
        let found = try await ask { try await api.search(show) }
        guard let (airing, exact) = Voice.show(show, in: found.airings, now: Date()) else {
            throw IntentFailure.noShow(show)
        }
        let passes = try await ask { try await api.passes() }
        if passes.contains(where: { Voice.tokens($0.title) == Voice.tokens(airing.title) && $0.airingStart == nil }) {
            return .result(dialog: "Already recording \(airing.title).")
        }
        let number = airing.guideNumber.map { " on \($0)" } ?? ""
        // A title that only has the words ("Masters" for "Jeopardy! Masters") is asked about first.
        if !exact {
            try await requestConfirmation(actionName: .set, dialog: "Record every \(airing.title)\(number)?")
        }
        _ = try await ask { try await api.addPass(title: airing.title, channelID: airing.channelId) }
        return .result(dialog: "Recording \(airing.title)\(number).")
    }
}

struct WhatsOnIntent: AppIntent {
    static let title: LocalizedStringResource = "What's on"
    static let description = IntentDescription("Says what your favorite channels show now.")

    func perform() async throws -> some IntentResult & ProvidesDialog {
        let api = try intentAPI()
        let now = Date()
        let snap = try await ask {
            async let channels = api.channels()
            async let airings = api.airings(from: now, to: now.addingTimeInterval(60))
            return try await TopShelf.Snapshot(channels: channels, airings: airings)
        }
        return .result(dialog: "\(Voice.onNow(snap, now: now))")
    }
}

struct MultiviewIntent: AppIntent {
    static let title: LocalizedStringResource = "Start multiview"
    static let description = IntentDescription("Opens two to four channels side by side.")

    @Parameter(title: "Channels", size: IntentCollectionSize(min: 2, max: 4))
    var channels: [ChannelEntity]

    func perform() async throws -> some IntentResult & OpensIntent {
        let ids = channels.map { String($0.id) }.joined(separator: ",")
        return .result(opensIntent: OpenURLIntent(URL(string: "broadwave://multiview?ch=\(ids)")!))
    }
}

/// The Control Center control's setting: a channel, or none for the guide.
struct WatchControlIntent: ControlConfigurationIntent {
    static let title: LocalizedStringResource = "Watch"
    static let isDiscoverable = false

    @Parameter(title: "Channel") var channel: ChannelEntity?
}

/// What the control's button runs.
struct OpenChannelIntent: AppIntent {
    static let title: LocalizedStringResource = "Open a channel or the guide"
    static let isDiscoverable = false

    @Parameter(title: "Channel") var channel: ChannelEntity?

    init() {}

    init(channel: ChannelEntity?) {
        self.channel = channel
    }

    func perform() async throws -> some IntentResult & OpensIntent {
        let link = channel.map { TopShelf.watchLink(Int64($0.id)) } ?? URL(string: "broadwave://guide")!
        return .result(opensIntent: OpenURLIntent(link))
    }
}
