import AppIntents
import BroadwaveKit
import BroadwaveUI
import SwiftUI
import WidgetKit

@main
struct BroadwaveWidgets: WidgetBundle {
    var body: some Widget {
        OnNowWidget()
        TeamsWidget()
        RecordingWidget()
        UpNextWidget()
        LiveActivityWidget()
        WatchControl()
    }
}

enum Feed: String, CaseIterable {
    case onNow, teams, recording, upNext

    var name: String {
        switch self {
        case .onNow: "On now"
        case .teams: "Your teams"
        case .recording: "Recording now"
        case .upNext: "Up next"
        }
    }

    var about: String {
        switch self {
        case .onNow: "Your favorite channels and what they show."
        case .teams: "Games of the teams you follow, with the score."
        case .recording: "What is recording right now."
        case .upNext: "What records next."
        }
    }

    var empty: String {
        switch self {
        case .onNow: "Nothing on now."
        case .teams: "No games for your teams in the next day."
        case .recording: "Nothing is recording."
        case .upNext: "Nothing set to record."
        }
    }

    func rows(api: APIClient, now: Date) async throws -> [WidgetFeed.Row] {
        switch self {
        case .onNow:
            async let channels = api.channels()
            async let airings = api.airings(from: now, to: now.addingTimeInterval(60))
            async let recordings = api.recordings()
            async let plan = try? api.schedule()
            let snap = try await TopShelf.Snapshot(channels: channels, airings: airings, recordings: recordings, plan: plan?.items ?? [])
            return WidgetFeed.onNow(snap, now: now)
        case .teams:
            // Nobody followed: say so, and skip a day of the whole guide.
            let follows = try await api.teams()
            guard !follows.isEmpty else {
                throw NoTeams()
            }
            async let channels = api.channels()
            // Games only: a day of every listing can pass the widget's memory limit.
            // From 4 h back, so a game in overtime past its listed end stays.
            async let airings = api.airings(from: now.addingTimeInterval(-4 * 3600), to: now.addingTimeInterval(24 * 3600), sportsOnly: true)
            async let recordings = api.recordings()
            async let plan = try? api.schedule()
            async let scores = try? api.scoreboard()
            let snap = try await TopShelf.Snapshot(channels: channels, airings: airings, recordings: recordings, plan: plan?.items ?? [])
            return await WidgetFeed.teams(snap, follows: follows, scores: scores ?? [], now: now)
        case .recording:
            return try await WidgetFeed.recordingNow(api.recordings())
        case .upNext:
            async let channels = api.channels()
            async let plan = api.schedule()
            return try await WidgetFeed.upNext(plan.items, channels: channels, now: now)
        }
    }
}

struct NoTeams: Error {}

struct FeedEntry: TimelineEntry, Sendable {
    enum State: Sendable { case rows, noServer, unreachable, noTeams }
    var date: Date
    var state: State
    var rows: [WidgetFeed.Row]
}

/// The widgets have nothing to set. An intent configuration gives them async loading.
struct FeedOptions: WidgetConfigurationIntent {
    static let title: LocalizedStringResource = "Broadwave"
    static let isDiscoverable = false
}

struct FeedProvider: AppIntentTimelineProvider {
    let feed: Feed

    func placeholder(in _: Context) -> FeedEntry {
        FeedEntry(date: .now, state: .rows, rows: [
            WidgetFeed.Row(id: "a", number: "4.1", title: "Evening News", link: TopShelf.watchLink(0)),
            WidgetFeed.Row(id: "b", number: "5.1", title: "Quiz Night", link: TopShelf.watchLink(0)),
        ])
    }

    /// The gallery shows the sample at once instead of waiting on the server.
    func snapshot(for _: FeedOptions, in context: Context) async -> FeedEntry {
        context.isPreview ? placeholder(in: context) : await Self.load(feed)
    }

    func timeline(for _: FeedOptions, in _: Context) async -> Timeline<FeedEntry> {
        let entry = await Self.load(feed)
        return Timeline(entries: [entry], policy: .after(WidgetFeed.refresh(after: entry.rows, now: entry.date)))
    }

    static func load(_ feed: Feed) async -> FeedEntry {
        let now = Date()
        guard let base = SharedServer.load() else {
            return FeedEntry(date: now, state: .noServer, rows: [])
        }
        let session = boundedSession()
        defer { session.finishTasksAndInvalidate() }
        let api = APIClient(base: base, session: session)
        do {
            return try await FeedEntry(date: now, state: .rows, rows: feed.rows(api: api, now: now))
        } catch is NoTeams {
            return FeedEntry(date: now, state: .noTeams, rows: [])
        } catch {
            return FeedEntry(date: now, state: .unreachable, rows: [])
        }
    }
}

/// A widget needs init() with no arguments, so each feed is its own type.
struct OnNowWidget: Widget {
    var body: some WidgetConfiguration {
        configuration(.onNow)
    }
}

struct TeamsWidget: Widget {
    var body: some WidgetConfiguration {
        configuration(.teams)
    }
}

struct RecordingWidget: Widget {
    var body: some WidgetConfiguration {
        configuration(.recording)
    }
}

struct UpNextWidget: Widget {
    var body: some WidgetConfiguration {
        configuration(.upNext)
    }
}

@MainActor
private func configuration(_ feed: Feed) -> some WidgetConfiguration {
    AppIntentConfiguration(kind: feed.rawValue, intent: FeedOptions.self, provider: FeedProvider(feed: feed)) { entry in
        FeedView(feed: feed, entry: entry)
            .containerBackground(Tokens.ColorToken.canvas, for: .widget)
    }
    .configurationDisplayName(feed.name)
    .description(feed.about)
    .supportedFamilies([.systemSmall, .systemMedium, .systemLarge])
}

struct FeedView: View {
    let feed: Feed
    let entry: FeedEntry
    @Environment(\.widgetFamily) private var family

    private var limit: Int {
        switch family {
        case .systemSmall: 1
        case .systemMedium: 2
        default: 4
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Tokens.Space.s2) {
            Text(feed.name)
                .font(.caption.weight(.semibold))
                .foregroundStyle(Tokens.ColorToken.textSecondary)
            switch entry.state {
            case .noServer:
                note("Open Broadwave to pick your server.")
            case .unreachable:
                note("Can't reach your server.")
            case .noTeams:
                note("Follow a team in Broadwave to see its games here.")
            case .rows where entry.rows.isEmpty:
                note(feed.empty)
            case .rows:
                ForEach(entry.rows.prefix(limit)) { row in
                    RowView(row: row, compact: family == .systemSmall, now: entry.date)
                }
            }
            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .widgetURL(family == .systemSmall ? entry.rows.first?.link : nil)
    }

    private func note(_ text: String) -> some View {
        Text(text)
            .font(.subheadline)
            .foregroundStyle(Tokens.ColorToken.text)
    }
}

struct RowView: View {
    let row: WidgetFeed.Row
    let compact: Bool
    let now: Date

    var body: some View {
        HStack(alignment: .center, spacing: Tokens.Space.s2) {
            Link(destination: row.link) {
                VStack(alignment: .leading, spacing: 2) {
                    // A small widget wraps a long title, so the number goes above it.
                    if compact {
                        number
                        title
                    } else {
                        HStack(spacing: Tokens.Space.s1) {
                            number
                            title
                        }
                    }
                    if !line.isEmpty {
                        Text(line).font(.caption).foregroundStyle(Tokens.ColorToken.textSecondary).lineLimit(1)
                    }
                }
                .foregroundStyle(Tokens.ColorToken.text)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityElement(children: .combine)
            }
            if !compact, let ask = row.record {
                Button(intent: RecordIntent(channelID: Int(ask.channelID), title: ask.title, start: Int(ask.start.timeIntervalSince1970))) {
                    Image(systemName: "record.circle")
                        .foregroundStyle(Tokens.ColorToken.tally)
                        .frame(minWidth: 44, minHeight: 44)
                        .contentShape(.rect)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Record \(ask.title)")
            }
        }
    }

    @ViewBuilder private var number: some View {
        if !row.number.isEmpty {
            Text(row.number).font(.caption.monospacedDigit().weight(.bold))
        }
    }

    private var title: some View {
        Text(row.title).font(.subheadline.weight(.semibold)).lineLimit(compact ? 3 : 1)
    }

    /// When, then what the row adds: "Thu 8:00 PM · Conflict", "Until 6:00 PM · Recording".
    private var line: String {
        var parts: [String] = []
        if let start = row.start, start > now {
            parts.append(Calendar.current.isDate(start, inSameDayAs: now)
                ? start.formatted(date: .omitted, time: .shortened)
                : start.formatted(.dateTime.weekday(.abbreviated).hour().minute()))
        } else if let end = row.end {
            parts.append("Until \(end.formatted(date: .omitted, time: .shortened))")
        }
        if !row.detail.isEmpty {
            parts.append(row.detail)
        }
        return parts.joined(separator: " · ")
    }
}

struct RecordIntent: AppIntent {
    static let title: LocalizedStringResource = "Record"
    static let description = IntentDescription("Records one airing, as Record once does in the app.")
    static let isDiscoverable = false

    @Parameter(title: "Channel") var channelID: Int
    @Parameter(title: "Title") var title: String
    /// Unix seconds. The server matches a once pass to the airing's start exactly.
    @Parameter(title: "Start") var start: Int

    init() {}

    init(channelID: Int, title: String, start: Int) {
        self.channelID = channelID
        self.title = title
        self.start = start
    }

    func perform() async throws -> some IntentResult {
        guard let base = SharedServer.load() else {
            throw RecordFailed.noServer
        }
        let session = boundedSession()
        defer { session.finishTasksAndInvalidate() }
        try await APIClient(base: base, session: session).addPass(title: title, channelID: Int64(channelID), airingStart: Date(timeIntervalSince1970: TimeInterval(start)))
        WidgetCenter.shared.reloadAllTimelines()
        return .result()
    }

    enum RecordFailed: Error, CustomLocalizedStringResourceConvertible {
        case noServer

        var localizedStringResource: LocalizedStringResource {
            "Open Broadwave to pick your server."
        }
    }
}

/// A cap on each whole request. APIClient's own 30 s request timeout only limits idle time,
/// and the system gives a widget a few seconds.
func boundedSession() -> URLSession {
    let config = URLSessionConfiguration.ephemeral
    config.timeoutIntervalForResource = 10
    return URLSession(configuration: config)
}
