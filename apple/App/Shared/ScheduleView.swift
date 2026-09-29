import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// What the series passes record next. A skipped airing says why, and offers a
/// later airing when one fits, as the web Schedule does. A conflict can also be watched.
struct ScheduleView: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @State private var plan: SchedulePlan?
    @State private var events: [Event] = []
    @State private var fixing: String?
    @State private var note: String?
    #if os(tvOS)
        /// The remote activates a list button only when that button has focus.
        @FocusState private var lineFocus: String?
    #endif

    var body: some View {
        List {
            Section {
                if let plan, plan.items.isEmpty {
                    quiet("No series pass matches an airing in the guide.")
                }
                let shown = plan
                ForEach(lines(in: shown)) { line in
                    lineRow(line, tuners: shown?.tunerCount ?? 2)
                }
                if let note {
                    quiet(note)
                }
            } footer: {
                if plan?.items.contains(where: \.conflict) == true {
                    Text("A skipped show can move to a later airing when one fits. Otherwise raise its priority in Settings, Series passes.")
                }
            }
            Section("Activity") {
                if events.isEmpty {
                    quiet("Guide updates and recordings will be listed here.")
                }
                ForEach(events) { event in
                    HStack(alignment: .firstTextBaseline, spacing: 12) {
                        Text(event.at.formatted(date: .omitted, time: .shortened))
                            .font(.caption.monospacedDigit())
                            .foregroundStyle(.secondary)
                        Text(event.message)
                    }
                    #if os(tvOS)
                    .focusable()
                    #endif
                }
            }
        }
        .navigationTitle("Upcoming")
        .task { await load() }
        #if os(iOS)
            .refreshable { await load() }
        #endif
        #if os(tvOS)
        .onChange(of: plan?.items.map(\.key).joined(separator: ",")) { _, _ in
            claimUpcomingFocus()
        }
        #endif
    }

    private struct UpcomingLine: Identifiable {
        var id: String
        var item: PlannedAiring
        var action: Action

        enum Action {
            case facts, later, watch
        }
    }

    /// Facts, then each action, so the remote and VoiceOver land on the action itself.
    private func lines(in plan: SchedulePlan?) -> [UpcomingLine] {
        (plan?.items ?? []).flatMap { item in
            var rows = [UpcomingLine(id: item.key, item: item, action: .facts)]
            if item.suggestion != nil {
                rows.append(UpcomingLine(id: item.key + "-later", item: item, action: .later))
            }
            if item.conflict {
                rows.append(UpcomingLine(id: item.key + "-watch", item: item, action: .watch))
            }
            return rows
        }
    }

    @ViewBuilder
    private func lineRow(_ line: UpcomingLine, tuners: Int) -> some View {
        switch line.action {
        case .facts:
            VStack(alignment: .leading, spacing: 4) {
                Text(line.item.airing.start.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day().hour().minute()))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text(line.item.airing.title).font(.headline).foregroundStyle(.primary)
                Text(line.item.statusLine(tuners: tuners))
                    .font(.subheadline)
                    .foregroundStyle(line.item.skipped ? Tokens.ColorToken.tally : .secondary)
                if let later = line.item.suggestion {
                    Text(line.item.laterLine(later))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
            }
            .accessibilityElement(children: .combine)
            .accessibilityLabel(line.item.summary(tuners: tuners))
            #if os(tvOS)
                .focusable()
            #endif
        case .later:
            laterButton(line)
        case .watch:
            watchButton(line)
        }
    }

    /// A plain list button, the same shape as a recording row, so Select runs it.
    private func laterButton(_ line: UpcomingLine) -> some View {
        let button = Button {
            Task { await recordLater(line.item) }
        } label: {
            Label("Record the later airing", systemImage: "calendar.badge.clock")
        }
        .disabled(fixing == line.item.key)
        .accessibilityLabel("Record the later airing")
        .accessibilityIdentifier("record-later")
        #if os(tvOS)
            return button.focused($lineFocus, equals: line.id)
        #else
            return button
        #endif
    }

    private func watchButton(_ line: UpcomingLine) -> some View {
        let button = Button {
            watchAnyway(line.item)
        } label: {
            Label("Watch anyway", systemImage: "play")
        }
        .accessibilityLabel("Watch anyway")
        .accessibilityIdentifier("watch-anyway")
        #if os(tvOS)
            return button.focused($lineFocus, equals: line.id)
        #else
            return button
        #endif
    }

    #if os(tvOS)
        /// Lands on the conflict's action when there is one, otherwise the first airing,
        /// so the sidebar closes and Select has a button to press.
        private func claimUpcomingFocus() {
            guard lineFocus == nil else { return }
            let rows = lines(in: plan)
            lineFocus = rows.first { $0.action == .later }?.id ?? rows.first?.id
        }
    #endif

    private func quiet(_ text: String) -> some View {
        Text(text)
            .foregroundStyle(.secondary)
        #if os(tvOS)
            .focusable()
        #endif
    }

    private func load() async {
        guard let api = store.api else { return }
        do {
            plan = try await api.schedule()
            #if os(tvOS)
                claimUpcomingFocus()
            #endif
        } catch {
            if plan == nil {
                note = PlaybackOutage.actionMessage(error)
            }
        }
        await loadEvents()
    }

    private func loadEvents() async {
        if let list = try? await store.api?.events() {
            events = list
        }
    }

    private func watchAnyway(_ item: PlannedAiring) {
        if let channel = store.channels.first(where: { $0.id == item.airing.channelId }) {
            nowPlaying.play(channel)
            return
        }
        let message = "That channel is not in the lineup."
        note = message
        AccessibilityNotification.Announcement(message).post()
    }

    private func recordLater(_ item: PlannedAiring) async {
        guard fixing == nil, let api = store.api, let later = item.suggestion else { return }
        fixing = item.key
        note = nil
        defer { fixing = nil }
        do {
            plan = try await api.fixSchedule(item, later: later)
            await store.refreshPasses()
            await loadEvents()
        } catch {
            // The later airing may have stopped fitting; show what fits now.
            if let fresh = try? await api.schedule() {
                plan = fresh
            }
            let message = PlaybackOutage.actionMessage(error)
            note = message
            AccessibilityNotification.Announcement(message).post()
        }
    }
}
