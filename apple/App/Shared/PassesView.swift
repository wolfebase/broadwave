import BroadwaveKit
import SwiftUI

/// Passes in the server's order, with the same rules the web schedule page has.
struct PassesView: View {
    @Environment(AppStore.self) private var store
    @State private var passes: [Pass] = []
    @State private var loaded = false
    @State private var error = ""
    @State private var openFirst = false
    @State private var adding = false

    var body: some View {
        List {
            if !error.isEmpty {
                Section {
                    Text(error).foregroundStyle(.red)
                }
            }
            #if os(tvOS)
                Section {
                    Button("New pass") { adding = true }
                        .accessibilityIdentifier("new-pass")
                }
            #endif
            Section {
                if !loaded {
                    Text("Loading…").foregroundStyle(.secondary)
                } else if passes.isEmpty {
                    Text("A pass records every airing that matches: a title, words in a title, or a category. Set one here or from the guide.")
                        .foregroundStyle(.secondary)
                }
                ForEach(passes) { pass in
                    NavigationLink {
                        PassEditView(pass: pass, passes: $passes)
                    } label: {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(pass.label).font(.headline)
                            Text(pass.details(channel: channelName(pass.channelId)))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                    .accessibilityIdentifier("pass-row-\(pass.id)")
                }
                #if os(iOS)
                .onMove(perform: move)
                #endif
            } footer: {
                if passes.count > 1 {
                    #if os(tvOS)
                        Text("When two passes need the same tuner, the one higher in the list records. Open a pass to move it up or down.")
                    #else
                        Text("When two passes need the same tuner, the one higher in the list records. Tap Edit to change the order.")
                    #endif
                }
            }
        }
        .navigationTitle("Passes")
        #if os(iOS)
            .toolbar {
                if passes.count > 1 {
                    ToolbarItem(placement: .topBarTrailing) { EditButton() }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("New pass", systemImage: "plus") { adding = true }
                        .accessibilityIdentifier("new-pass")
                }
            }
        #endif
            .task { await load() }
            .onDisappear { Task { await store.refreshPasses() } }
            .navigationDestination(isPresented: $adding) {
                PassEditView(pass: nil, passes: $passes)
            }
            .navigationDestination(isPresented: $openFirst) {
                if let pass = passes.first {
                    PassEditView(pass: pass, passes: $passes)
                }
            }
    }

    private func channelName(_ id: Int64?) -> String? {
        guard let id, id != 0 else { return nil }
        return store.channels.first { $0.id == id }.map { "\($0.displayNumber) \($0.displayName)" }
    }

    private func move(_ from: IndexSet, _ to: Int) {
        var next = passes
        next.move(fromOffsets: from, toOffset: to)
        passes = next
        Task {
            guard let api = store.api else { return }
            do {
                passes = try await api.orderPasses(next.map(\.id))
                error = ""
            } catch {
                self.error = error.localizedDescription
                await load()
            }
        }
    }

    private func load() async {
        guard let api = store.api else { return }
        do {
            passes = try await api.passes()
            error = ""
            #if DEBUG
                if !loaded {
                    openFirst = UserDefaults.standard.bool(forKey: "BroadwaveOpenFirst") && !passes.isEmpty
                    adding = UserDefaults.standard.string(forKey: "BroadwaveNewPass") != nil
                }
            #endif
        } catch {
            self.error = error.localizedDescription
        }
        loaded = true
    }
}

/// Edits a saved pass one field at a time, or builds a new one that saves on Add pass.
struct PassEditView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.dismiss) private var dismiss
    @Binding var passes: [Pass]
    @State private var pass: Pass
    @State private var text: String
    @State private var error = ""
    @State private var confirmDelete = false
    @State private var saves = 0
    @State private var adding = false
    @State private var preview: PassPreview?
    @State private var previewNote = ""
    @FocusState private var typing: Bool
    private let isNew: Bool

    static let minutes = [0, 1, 2, 3, 5, 10, 15, 20, 30]
    static let keepCounts = [1, 2, 3, 5, 10, 20]
    static let limits = [0, 1, 2, 3, 5, 10, 20]
    /// Half-hour starts, "00:00" to "23:30".
    static let slots = (0 ..< 48).map { String(format: "%02d:%02d", $0 / 2, $0 % 2 * 30) }

    init(pass: Pass?, passes: Binding<[Pass]>) {
        let start = pass ?? Pass(id: 0, title: "", kind: "series", padBefore: 1, padAfter: 2, episodes: "all", keepMode: "all", keepCount: 5,
                                 limitCount: 0, rerecord: false, commercials: true, timeStart: "", timeEnd: "", matchKind: "title", days: [])
        _pass = State(initialValue: start)
        _text = State(initialValue: start.title)
        _passes = passes
        isNew = pass == nil
    }

    private var series: Bool {
        !pass.isOnce && !pass.isTeam
    }

    /// The rules as they stand, with the title being typed.
    private var rules: NewPass {
        var draft = pass
        if series {
            draft.title = text
        }
        return draft.rules
    }

    var body: some View {
        ScrollViewReader { proxy in
            Form {
                if !error.isEmpty {
                    Section {
                        Text(error).foregroundStyle(.red)
                    }
                }
                if series {
                    matchSection
                }
                if !pass.isOnce {
                    channelSection
                    daysSection
                    timeSection
                }
                recordingsSection
                timingSection
                #if os(tvOS)
                    if !isNew, passes.count > 1 {
                        orderSection
                    }
                #endif
                previewSection.id("preview")
                if isNew {
                    Section {
                        Button("Add pass") { Task { await add() } }
                            .disabled(adding)
                            .accessibilityIdentifier("add-pass")
                    }
                } else {
                    Section {
                        Button("Delete pass", role: .destructive) { confirmDelete = true }
                            .accessibilityIdentifier("delete-pass")
                    } footer: {
                        Text("Recordings it already made stay.")
                    }
                }
            }
            #if DEBUG
            .task {
                // -BroadwaveNewPass category:Sports starts a new pass with that match.
                if isNew, let seed = UserDefaults.standard.string(forKey: "BroadwaveNewPass"), let colon = seed.firstIndex(of: ":") {
                    pass.matchKind = String(seed[..<colon])
                    text = String(seed[seed.index(after: colon)...])
                }
                guard UserDefaults.standard.string(forKey: "BroadwavePassAnchor") == "preview" else { return }
                try? await Task.sleep(for: .seconds(1.5))
                proxy.scrollTo("preview", anchor: .top)
            }
            #else
            .onAppear { _ = proxy }
            #endif
        }
        .navigationTitle(isNew ? "New pass" : pass.title)
        #if os(iOS)
            .toolbar {
                if isNew {
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Add pass") { Task { await add() } }
                            .disabled(adding || text.trimmingCharacters(in: .whitespaces).isEmpty)
                            .accessibilityIdentifier("add-pass")
                    }
                }
            }
        #endif
            .task(id: rules) { await runPreview(rules) }
            .onChange(of: typing) { _, now in
                if !now {
                    commitTitle()
                }
            }
            .onDisappear { commitTitle() }
            .confirmationDialog("Delete the \(pass.title) pass?", isPresented: $confirmDelete, titleVisibility: .visible) {
                Button("Delete", role: .destructive) { Task { await delete() } }
                Button("Cancel", role: .cancel) {}
            }
    }

    // MARK: Sections

    private var matchSection: some View {
        Section {
            Picker("Match", selection: binding(\.matchKind, "title")) {
                Text("Title is").tag("title")
                Text("Title contains").tag("contains")
                Text("Category").tag("category")
            }
            #if os(iOS)
                LabeledContent(fieldName) {
                    titleField
                        .multilineTextAlignment(.trailing)
                        .textInputAutocapitalization(.words)
                }
            #else
                titleField
            #endif
        } header: {
            Text("Match")
        } footer: {
            switch pass.matchKind {
            case "contains": Text("Records every title with these words.")
            case "category": Text("Records every airing the guide files under this category.")
            default: Text("Records every airing with this exact title.")
            }
        }
    }

    private var titleField: some View {
        TextField(fieldName, text: $text, prompt: Text(fieldPrompt))
            .focused($typing)
            .onSubmit(commitTitle)
            .autocorrectionDisabled()
            .accessibilityIdentifier("pass-title")
            .accessibilityLabel(fieldName)
    }

    private var fieldName: String {
        switch pass.matchKind {
        case "contains": "Words"
        case "category": "Category"
        default: "Title"
        }
    }

    private var fieldPrompt: String {
        switch pass.matchKind {
        case "contains": "Bears"
        case "category": "Sports"
        default: "Jeopardy!"
        }
    }

    private var channelSection: some View {
        Section {
            let current = pass.channelId ?? 0
            Picker("Channel", selection: Binding(
                get: { current },
                set: { id in update { $0.channelId = id } }
            )) {
                Text("Any channel").tag(Int64(0))
                ForEach(store.channels.filter { !$0.hidden || $0.id == current }) { ch in
                    Text("\(ch.displayNumber) \(ch.displayName)").tag(ch.id)
                }
            }
            if series {
                Picker("Episodes", selection: binding(\.episodes, "all")) {
                    Text("All").tag("all")
                    Text("New only").tag("new")
                }
            }
        }
    }

    private var daysSection: some View {
        Section {
            let names = Calendar.current.weekdaySymbols
            ForEach(0 ..< 7, id: \.self) { day in
                Toggle(names.indices.contains(day) ? names[day] : "\(day)", isOn: Binding(
                    get: { pass.days?.contains(day) == true },
                    set: { on in
                        update { p in
                            var days = Set(p.days ?? [])
                            if on {
                                days.insert(day)
                            } else {
                                days.remove(day)
                            }
                            p.days = days.sorted()
                        }
                    }
                ))
                .accessibilityIdentifier("pass-day-\(day)")
            }
        } header: {
            Text("Days")
        } footer: {
            Text(Pass.daysLabel(pass.days ?? []))
        }
    }

    private var timeSection: some View {
        let from = pass.timeStart ?? ""
        let until = pass.timeEnd ?? ""
        return Section {
            Picker("From", selection: Binding(
                get: { from },
                set: { value in
                    update { p in
                        p.timeStart = value
                        if value.isEmpty {
                            p.timeEnd = ""
                        } else if (p.timeEnd ?? "").isEmpty || p.timeEnd == value {
                            p.timeEnd = Self.slot(after: value, hours: 3)
                        }
                    }
                }
            )) {
                Text("Any time").tag("")
                ForEach(Self.withValue(Self.slots, from), id: \.self) { Text(Pass.clockLabel($0)).tag($0) }
            }
            .accessibilityIdentifier("pass-time-from")
            if !from.isEmpty {
                Picker("Until", selection: Binding(
                    get: { until },
                    set: { value in update { $0.timeStart = from; $0.timeEnd = value } }
                )) {
                    ForEach(Self.withValue(Self.slots, until).filter { $0 != from }, id: \.self) { Text(Pass.clockLabel($0)).tag($0) }
                }
            }
        } header: {
            Text("Time")
        } footer: {
            Text(from.isEmpty ? "Any time of day." : "Records airings that start in this window. An end before the start runs past midnight.")
        }
    }

    private var recordingsSection: some View {
        Section {
            if !pass.isOnce {
                Picker("Keep", selection: Binding(
                    get: { pass.keepMode ?? "all" },
                    set: { mode in
                        update { p in
                            p.keepMode = mode
                            if mode == "last", (p.keepCount ?? 0) < 1 {
                                p.keepCount = 5
                            }
                        }
                    }
                )) {
                    Text("All").tag("all")
                    Text("Unwatched").tag("unwatched")
                    Text("The newest").tag("last")
                }
                if pass.keepMode == "last" {
                    picker("How many", \.keepCount, Self.keepCounts, fallback: 5) { "\($0)" }
                }
                picker("Stop at", \.limitCount, Self.limits, fallback: 0) { $0 == 0 ? "No limit" : "\($0) unwatched" }
                    .accessibilityIdentifier("pass-stop-at")
                if series {
                    Toggle("Record again after a delete", isOn: Binding(
                        get: { pass.rerecord == true },
                        set: { on in update { $0.rerecord = on } }
                    ))
                }
            }
            Toggle("Find commercials", isOn: Binding(
                get: { pass.commercials != false },
                set: { on in update { $0.commercials = on } }
            ))
        } header: {
            Text("Recordings")
        } footer: {
            Text(pass.isOnce ? "Found commercials can be skipped." : "Unwatched removes a recording once it is watched. Stop at pauses the pass until you watch some.")
        }
    }

    private var timingSection: some View {
        Section {
            picker("Early", \.padBefore, Self.minutes, fallback: 0) { $0 == 0 ? "On time" : "\($0) min" }
            picker("After", \.padAfter, Self.minutes, fallback: 0) { $0 == 0 ? "On time" : "\($0) min" }
        } header: {
            Text("Timing")
        } footer: {
            Text("Early starts the tuner before the listing. After keeps it through the credits.")
        }
    }

    #if os(tvOS)
        private var orderSection: some View {
            let at = passes.firstIndex { $0.id == pass.id } ?? 0
            return Section {
                Button("Move up") { Task { await move(-1) } }
                    .disabled(at == 0)
                    .accessibilityIdentifier("pass-move-up")
                Button("Move down") { Task { await move(1) } }
                    .disabled(at >= passes.count - 1)
                    .accessibilityIdentifier("pass-move-down")
            } header: {
                Text("Order")
            } footer: {
                Text("\(at + 1) of \(passes.count). When two passes need the same tuner, the one higher in the list records.")
            }
        }
    #endif

    @ViewBuilder
    private var previewSection: some View {
        if !rules.title.isEmpty {
            Section {
                if !previewNote.isEmpty {
                    Text(previewNote).foregroundStyle(.secondary)
                } else if let preview {
                    Text(Self.headline(preview))
                    ForEach(Array(preview.items.prefix(5).enumerated()), id: \.offset) { _, item in
                        airingRow(item, why: item.skipped ? (item.conflict ? "Skipped: another pass has the tuner" : item.reason ?? "Skipped") : nil)
                    }
                    if preview.items.count > 5 {
                        Text("and \(preview.items.count - 5) more").font(.caption).foregroundStyle(.secondary)
                    }
                } else {
                    Text("Checking the guide…").foregroundStyle(.secondary)
                }
            } header: {
                Text("Next 2 weeks")
            } footer: {
                if let preview, preview.utcOffset != TimeZone.current.secondsFromGMT() {
                    Text("Days and times are on the server's clock (\(preview.timeZone)).")
                }
            }
            if let preview, previewNote.isEmpty, !preview.bumps.isEmpty {
                Section {
                    ForEach(Array(preview.bumps.enumerated()), id: \.offset) { _, item in
                        airingRow(item, why: nil)
                    }
                } header: {
                    Text("This pass would stop these from recording:")
                }
            }
        }
    }

    private func airingRow(_ item: PlannedAiring, why: String?) -> some View {
        let number = store.channels.first { $0.id == item.airing.channelId }?.displayNumber ?? ""
        let when = item.airing.start.formatted(.dateTime.weekday(.abbreviated).month(.defaultDigits).day().hour().minute())
        return VStack(alignment: .leading, spacing: 2) {
            Text("\(when) · \(number) \(item.airing.title)")
            if let why {
                Text(why).font(.caption).foregroundStyle(.orange)
            }
        }
        .accessibilityElement(children: .combine)
    }

    static func headline(_ preview: PassPreview) -> String {
        let total = preview.items.count
        let records = preview.items.filter { !$0.skipped }.count
        let airings = "\(total) airing\(total == 1 ? "" : "s")"
        if total == 0 {
            return "Nothing in the guide matches in the next 2 weeks."
        }
        if records == total {
            return "Records \(airings) in the next 2 weeks."
        }
        if records == 0 {
            return "Matches \(airings) in the next 2 weeks, but none will record."
        }
        return "Records \(records) of \(airings) in the next 2 weeks."
    }

    // MARK: Values

    /// A value set elsewhere stays selectable.
    private static func withValue<T: Comparable>(_ values: [T], _ current: T) -> [T] {
        values.contains(current) ? values : (values + [current]).sorted()
    }

    private static func withValue(_ values: [String], _ current: String) -> [String] {
        current.isEmpty || values.contains(current) ? values : (values + [current]).sorted()
    }

    private static func slot(after hhmm: String, hours: Int) -> String {
        let bits = hhmm.split(separator: ":").compactMap { Int($0) }
        guard bits.count == 2 else { return "23:00" }
        let mins = (bits[0] * 60 + bits[1] + hours * 60) % (24 * 60)
        return String(format: "%02d:%02d", mins / 60, mins % 60)
    }

    private func picker(_ title: String, _ key: WritableKeyPath<Pass, Int?>, _ values: [Int], fallback: Int, _ label: @escaping (Int) -> String) -> some View {
        let current = pass[keyPath: key] ?? fallback
        return Picker(title, selection: Binding(
            get: { current },
            set: { value in update { $0[keyPath: key] = value } }
        )) {
            ForEach(Self.withValue(values, current), id: \.self) { Text(label($0)).tag($0) }
        }
    }

    private func binding(_ key: WritableKeyPath<Pass, String?>, _ fallback: String) -> Binding<String> {
        Binding(
            get: { pass[keyPath: key] ?? fallback },
            set: { value in update { $0[keyPath: key] = value } }
        )
    }

    // MARK: Saving

    /// A saved pass sends only the changed fields, so a save in flight or an edit on the web is not undone. A new pass waits for Add pass.
    private func update(_ change: (inout Pass) -> Void) {
        var next = pass
        change(&next)
        pass = next
        guard !isNew else { return }
        var patch = Pass(id: pass.id, title: pass.title)
        change(&patch)
        saves += 1
        let mine = saves
        Task { await save(patch.id, mine) { try await $0.updatePass(patch) } }
    }

    private func commitTitle() {
        guard series, !isNew else { return }
        let title = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard title != pass.title else { return }
        guard !title.isEmpty else {
            error = "Name a title, words, or a category."
            text = pass.title
            return
        }
        // A rename has its own request; every other change sends the title the screen loaded.
        pass.title = title
        saves += 1
        let mine = saves
        let id = pass.id
        Task { await save(id, mine) { try await $0.renamePass(id, to: title) } }
    }

    private func save(_ id: Int64, _ mine: Int, _ send: (APIClient) async throws -> [Pass]) async {
        guard let api = store.api else { return }
        do {
            let list = try await send(api)
            if mine == saves, let fresh = list.first(where: { $0.id == id }) {
                pass = fresh
            }
            error = ""
            passes = list
        } catch {
            self.error = error.localizedDescription
            // Show what the server kept.
            if let list = try? await api.passes() {
                passes = list
                if mine == saves, let fresh = list.first(where: { $0.id == id }) {
                    pass = fresh
                    text = fresh.title
                }
            }
        }
    }

    private func add() async {
        let body = rules
        guard !body.title.isEmpty else {
            error = "Name a title, words, or a category."
            return
        }
        guard let api = store.api else { return }
        adding = true
        defer { adding = false }
        do {
            passes = try await api.addSeriesPass(body)
            error = ""
            dismiss()
        } catch {
            self.error = error.localizedDescription
        }
    }

    #if os(tvOS)
        private func move(_ by: Int) async {
            guard let api = store.api else { return }
            var ids = passes.map(\.id)
            guard let at = ids.firstIndex(of: pass.id), ids.indices.contains(at + by) else { return }
            ids.swapAt(at, at + by)
            do {
                passes = try await api.orderPasses(ids)
                error = ""
            } catch {
                self.error = error.localizedDescription
                if let list = try? await api.passes() {
                    passes = list
                }
            }
        }
    #endif

    private func runPreview(_ rules: NewPass) async {
        guard !rules.title.isEmpty, let api = store.api else {
            preview = nil
            return
        }
        try? await Task.sleep(for: .milliseconds(400))
        guard !Task.isCancelled else { return }
        do {
            let next = try await api.previewPass(rules, id: isNew ? nil : pass.id)
            guard !Task.isCancelled else { return }
            preview = next
            previewNote = ""
        } catch {
            guard !Task.isCancelled else { return }
            preview = nil
            previewNote = error.localizedDescription
        }
    }

    private func delete() async {
        guard let api = store.api else { return }
        do {
            passes = try await api.deletePass(pass.id)
            dismiss()
        } catch {
            self.error = error.localizedDescription
        }
    }
}
