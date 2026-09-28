import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// What the series passes record next. A skipped airing says why, and offers a
/// later airing when one fits, as the web Schedule does.
struct ScheduleView: View {
    @Environment(AppStore.self) private var store
    @State private var plan: SchedulePlan?
    @State private var events: [Event] = []
    @State private var fixing: String?
    @State private var note: String?

    var body: some View {
        List {
            Section {
                if let plan, plan.items.isEmpty {
                    quiet("No series pass matches an airing in the guide.")
                }
                ForEach(plan?.items ?? [], id: \.key) { item in
                    row(item, tuners: plan?.tunerCount ?? 2)
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
        .navigationTitle("Coming up")
        .task { await load() }
        #if os(iOS)
            .refreshable { await load() }
        #endif
    }

    @ViewBuilder
    private func row(_ item: PlannedAiring, tuners: Int) -> some View {
        let info = VStack(alignment: .leading, spacing: 4) {
            Text(item.airing.start.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day().hour().minute()))
                .font(.caption)
                .foregroundStyle(.secondary)
            Text(item.airing.title).font(.headline).foregroundStyle(.primary)
            Text(item.statusLine(tuners: tuners))
                .font(.subheadline)
                .foregroundStyle(item.skipped ? Tokens.ColorToken.tally : .secondary)
            if let later = item.suggestion {
                Text(item.laterLine(later))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
        }
        if item.suggestion != nil {
            Button {
                Task { await recordLater(item) }
            } label: {
                VStack(alignment: .leading, spacing: 8) {
                    info
                    Label("Record the later airing", systemImage: "calendar.badge.clock")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(Tokens.ColorToken.accent)
                }
            }
            .disabled(fixing == item.key)
        } else {
            info
            #if os(tvOS)
                .focusable()
            #endif
        }
    }

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
