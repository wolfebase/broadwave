import BroadwaveKit
import SwiftUI

/// Series passes with the same rules the web schedule page has.
struct PassesView: View {
    @Environment(AppStore.self) private var store
    @State private var passes: [Pass] = []
    @State private var loaded = false
    @State private var error = ""
    @State private var openFirst = false

    var body: some View {
        List {
            if !error.isEmpty {
                Section {
                    Text(error).foregroundStyle(.red)
                }
            }
            Section {
                if !loaded {
                    Text("Loading…").foregroundStyle(.secondary)
                } else if passes.isEmpty {
                    Text("A series pass records every airing of a title. Set one from the guide.")
                        .foregroundStyle(.secondary)
                }
                ForEach(passes) { pass in
                    NavigationLink {
                        PassEditView(pass: pass) { passes = $0 }
                    } label: {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(pass.title).font(.headline)
                            Text(PassEditView.summary(pass, channel: channelName(pass.channelId)))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            } footer: {
                Text("Early starts the tuner before the listing. After keeps it through the credits. When two passes overlap, the higher priority keeps the tuner.")
            }
        }
        .navigationTitle("Series passes")
        .task { await load() }
        .navigationDestination(isPresented: $openFirst) {
            if let pass = passes.first {
                PassEditView(pass: pass) { passes = $0 }
            }
        }
    }

    private func channelName(_ id: Int64?) -> String? {
        guard let id, id != 0 else { return nil }
        return store.channels.first { $0.id == id }.map { "\($0.displayNumber) \($0.displayName)" }
    }

    private func load() async {
        guard let api = store.api else { return }
        do {
            passes = try await api.passes()
            error = ""
            #if DEBUG
                openFirst = UserDefaults.standard.bool(forKey: "BroadwaveOpenFirst") && !passes.isEmpty
            #endif
        } catch {
            self.error = error.localizedDescription
        }
        loaded = true
    }
}

struct PassEditView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.dismiss) private var dismiss
    @State private var pass: Pass
    @State private var error = ""
    @State private var confirmDelete = false
    @State private var saves = 0
    let changed: ([Pass]) -> Void

    static let minutes = [0, 1, 2, 3, 5, 10, 15, 20, 30]
    static let priorities = Array(0 ... 10)
    static let keepCounts = [1, 2, 3, 5, 10, 20]

    init(pass: Pass, changed: @escaping ([Pass]) -> Void) {
        _pass = State(initialValue: pass)
        self.changed = changed
    }

    var body: some View {
        Form {
            if !error.isEmpty {
                Section {
                    Text(error).foregroundStyle(.red)
                }
            }
            Section {
                picker("Early", \.padBefore, Self.minutes) { $0 == 0 ? "On time" : "\($0) min" }
                picker("After", \.padAfter, Self.minutes) { $0 == 0 ? "On time" : "\($0) min" }
                picker("Priority", \.priority, Self.priorities) { "\($0)" }
            } header: {
                Text("Timing")
            }
            Section {
                Picker("Episodes", selection: binding(\.episodes, "all")) {
                    Text("All").tag("all")
                    Text("New only").tag("new")
                }
                Picker("Keep", selection: binding(\.keepMode, "all")) {
                    Text("All").tag("all")
                    Text("Unwatched").tag("unwatched")
                    Text("Last few").tag("last")
                }
                if pass.keepMode == "last" {
                    picker("How many", \.keepCount, Self.keepCounts) { "\($0)" }
                }
                Toggle("Mark commercials", isOn: Binding(
                    get: { pass.commercials != false },
                    set: { on in update { $0.commercials = on } }
                ))
            } header: {
                Text("Recordings")
            } footer: {
                Text("Unwatched removes a recording once it is watched. Last few keeps only the newest ones. Marked commercials can be skipped.")
            }
            Section {
                Button("Delete pass", role: .destructive) { confirmDelete = true }
            } footer: {
                Text("Recordings it already made stay.")
            }
        }
        .navigationTitle(pass.title)
        .confirmationDialog("Delete the \(pass.title) pass?", isPresented: $confirmDelete, titleVisibility: .visible) {
            Button("Delete", role: .destructive) { Task { await delete() } }
            Button("Cancel", role: .cancel) {}
        }
    }

    static func summary(_ pass: Pass, channel: String?) -> String {
        var parts = [channel ?? "Any channel"]
        parts.append(pass.episodes == "new" ? "New only" : "All episodes")
        switch pass.keepMode {
        case "unwatched": parts.append("Keep unwatched")
        case "last": parts.append("Keep last \(max(pass.keepCount ?? 1, 1))")
        default: break
        }
        let early = pass.padBefore ?? 0
        let after = pass.padAfter ?? 0
        if early > 0 || after > 0 {
            parts.append("\(early) min early, \(after) after")
        }
        return parts.joined(separator: " · ")
    }

    /// A value the web set outside the menu stays selectable.
    private func picker(_ title: String, _ key: WritableKeyPath<Pass, Int?>, _ values: [Int], _ label: @escaping (Int) -> String) -> some View {
        let current = pass[keyPath: key] ?? (key == \Pass.keepCount ? 1 : 0)
        let options = values.contains(current) ? values : (values + [current]).sorted()
        return Picker(title, selection: Binding(
            get: { current },
            set: { value in update { $0[keyPath: key] = value } }
        )) {
            ForEach(options, id: \.self) { Text(label($0)).tag($0) }
        }
    }

    private func binding(_ key: WritableKeyPath<Pass, String?>, _ fallback: String) -> Binding<String> {
        Binding(
            get: { pass[keyPath: key] ?? fallback },
            set: { value in update { $0[keyPath: key] = value } }
        )
    }

    /// Sends only the changed field, so a save in flight or an edit on the web is not undone.
    private func update(_ change: (inout Pass) -> Void) {
        var next = pass
        change(&next)
        pass = next
        var patch = Pass(id: pass.id, title: pass.title)
        change(&patch)
        saves += 1
        let mine = saves
        Task { await save(patch, mine) }
    }

    private func save(_ patch: Pass, _ mine: Int) async {
        guard let api = store.api else { return }
        do {
            let list = try await api.updatePass(patch)
            if mine == saves, let fresh = list.first(where: { $0.id == patch.id }) {
                pass = fresh
            }
            error = ""
            changed(list)
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func delete() async {
        guard let api = store.api else { return }
        do {
            try await changed(api.deletePass(pass.id))
            dismiss()
        } catch {
            self.error = error.localizedDescription
        }
    }
}
