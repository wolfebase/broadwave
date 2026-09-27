import BroadwaveKit
import SwiftUI
#if os(iOS)
    import UniformTypeIdentifiers
#endif

/// Playlists, servers, stream links, and folders, as on the web's Sources page.
struct AddSourceView: View {
    var added: () -> Void = {}
    @Environment(AppStore.self) private var store
    @State private var kind = "m3u"
    @State private var name = ""
    @State private var address = ""
    @State private var username = ""
    @State private var password = ""
    @State private var groups = ""
    @State private var file: (name: String, data: Data)?
    @State private var importing = false
    @State private var pick: APIClient.SourceAdd?
    @State private var chosen: Set<Int> = []
    @State private var busy = false
    @State private var note = ""

    static let kinds: [(tag: String, title: String)] = [
        ("m3u", "Playlist"),
        ("xtream", "Xtream Codes"),
        ("tvheadend", "tvheadend"),
        ("channels", "Channels DVR"),
        ("threadfin", "Threadfin"),
        ("xteve", "xTeVe"),
        ("ersatztv", "ErsatzTV"),
        ("dispatcharr", "Dispatcharr"),
        ("link", "Stream link"),
        ("folder", "Media folder"),
    ]

    private var needsLogin: Bool {
        kind == "xtream" || kind == "tvheadend"
    }

    private var takesGroups: Bool {
        ["m3u", "xtream", "tvheadend", "channels"].contains(kind)
    }

    private var ready: Bool {
        let hasAddress = !address.trimmingCharacters(in: .whitespaces).isEmpty || (kind == "m3u" && file != nil)
        let hasLogin = !needsLogin || (!username.trimmingCharacters(in: .whitespaces).isEmpty && !password.isEmpty)
        return hasAddress && hasLogin && !busy
    }

    var body: some View {
        Form {
            Section {
                Picker("Kind", selection: $kind) {
                    ForEach(Self.kinds, id: \.tag) { Text($0.title).tag($0.tag) }
                }
                TextField("Name", text: $name)
                TextField("Address", text: $address, prompt: Text(placeholder))
                    .textContentType(.URL)
                if needsLogin {
                    TextField("Username", text: $username)
                    SecureField("Password", text: $password)
                }
                #if os(iOS)
                    if kind == "m3u" {
                        if let file {
                            LabeledContent("Playlist file", value: file.name)
                            Button("Use the address instead", role: .destructive) {
                                self.file = nil
                                forgetPick()
                            }
                        } else {
                            Button("Choose a playlist file") { importing = true }
                        }
                    }
                #endif
                if takesGroups {
                    TextField("Groups", text: $groups, prompt: Text("News, Sports, -Shopping"))
                }
            } footer: {
                Text(hint)
            }
            if let options = pick?.options, !options.isEmpty {
                Section {
                    ForEach(Array(options.enumerated()), id: \.offset) { index, option in
                        Toggle(option, isOn: Binding(
                            get: { chosen.contains(index) },
                            set: { on in
                                if on {
                                    chosen.insert(index)
                                } else {
                                    chosen.remove(index)
                                }
                            }
                        ))
                    }
                } header: {
                    Text("Choose what to keep")
                } footer: {
                    if let message = pick?.message, !message.isEmpty {
                        Text(message)
                    }
                }
            }
            Section {
                Button(pick == nil ? "Add" : "Add what I chose") { Task { await add() } }
                    .disabled(!ready || (pick != nil && chosen.isEmpty))
            } footer: {
                if busy {
                    Text("Adding…")
                } else if !note.isEmpty {
                    Text(note)
                }
            }
        }
        #if os(iOS)
        .textInputAutocapitalization(.never)
        .fileImporter(isPresented: $importing, allowedContentTypes: [.data]) { result in
            load(result)
        }
        #endif
        .autocorrectionDisabled()
        .navigationTitle("Add a source")
        .onChange(of: kind) { _, _ in forgetPick() }
        .onChange(of: address) { _, _ in forgetPick() }
        .onChange(of: groups) { _, _ in forgetPick() }
    }

    private var placeholder: String {
        switch kind {
        case "folder": "/media/tv"
        case "xtream", "tvheadend", "channels", "threadfin", "xteve", "ersatztv", "dispatcharr": "http://server:port"
        case "link": "https://example.com/live.m3u8"
        default: "https://example.com/playlist.m3u"
        }
    }

    private var hint: String {
        switch kind {
        case "m3u", "xtream": "Groups: News, Sports, -Shopping. Leave blank to keep every group."
        case "channels": "Uses your Channels DVR tuners."
        case "folder": "A folder the server can read. Its videos join the library."
        case "link": "One stream becomes one channel."
        default: "The server reads the lineup only. It does not open a tuner from this screen."
        }
    }

    #if os(iOS)
        private func load(_ result: Result<URL, Error>) {
            guard case let .success(url) = result else { return }
            forgetPick()
            Task {
                let read = await Self.read(url)
                switch read {
                case let .success(data):
                    file = (url.lastPathComponent, data)
                    if name.isEmpty {
                        name = url.deletingPathExtension().lastPathComponent
                    }
                case let .failure(message):
                    note = message.text
                }
            }
        }

        struct ReadFailure: Error {
            var text: String
        }

        /// The server reads 32 MB of an upload; a larger file would be cut short.
        nonisolated static func read(_ url: URL) async -> Result<Data, ReadFailure> {
            let scoped = url.startAccessingSecurityScopedResource()
            defer {
                if scoped {
                    url.stopAccessingSecurityScopedResource()
                }
            }
            let size = (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0
            if size > 32 << 20 {
                return .failure(ReadFailure(text: "That file is over 32 MB. Add the playlist by address instead."))
            }
            guard let data = try? Data(contentsOf: url) else {
                return .failure(ReadFailure(text: "That file could not be read."))
            }
            return .success(data)
        }
    #endif

    private func forgetPick() {
        pick = nil
        chosen = []
    }

    private func add() async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        var useGroups = groups.trimmingCharacters(in: .whitespaces)
        var useKeep = ""
        if let pick {
            (useGroups, useKeep) = pick.chosen(chosen)
        }
        do {
            let result: APIClient.SourceAdd = if kind == "m3u", let file {
                try await api.addPlaylistFile(name: name, fileName: file.name, data: file.data, groups: useGroups, keep: useKeep)
            } else {
                try await api.addSource(
                    kind: kind, name: name, url: address.trimmingCharacters(in: .whitespaces),
                    username: username.trimmingCharacters(in: .whitespaces), password: password,
                    groups: useGroups, keep: useKeep
                )
            }
            if result.pick == true {
                pick = result
                chosen = []
                note = result.message ?? "Choose what to keep."
                return
            }
            pick = nil
            chosen = []
            password = ""
            address = ""
            file = nil
            if kind == "folder" {
                note = "Added \(result.added ?? 0) files to the library."
            } else {
                note = "Source added. It shows up with the lineup."
            }
            await store.refresh()
            added()
        } catch {
            note = error.localizedDescription
        }
    }
}
