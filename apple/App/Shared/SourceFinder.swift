import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// Look harder, free channels, and add by address. Setup and Sources share it.
struct SourceFinder: View {
    var disabled = false
    var changed: () async -> Void = {}
    @Environment(AppStore.self) private var store
    @State private var hits: [APIClient.FoundHit] = []
    @State private var feeds: [APIClient.FreeFeed] = []
    @State private var freeGuide = ""
    @State private var address = ""
    @State private var busy = false
    @State private var note = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Button("Look harder") { Task { await look() } }
                    .buttonStyle(.glass)
                    .disabled(busy || disabled)
                Button("Add free channels") { Task { await findFree() } }
                    .buttonStyle(.glass)
                    .disabled(busy || disabled)
            }
            ForEach(hits) { hit in
                HStack {
                    VStack(alignment: .leading) {
                        Text(hit.name).font(.headline)
                        Text(hit.addr).font(.caption).foregroundStyle(.secondary)
                    }
                    Spacer()
                    Button("Add") { Task { await add(hit) } }
                        .buttonStyle(.glass)
                }
            }
            ForEach(feeds) { feed in
                HStack {
                    Text(feed.name)
                    Spacer()
                    Button("Add") { Task { await add(feed) } }
                        .buttonStyle(.glass)
                }
            }
            if !freeGuide.isEmpty {
                Text(freeGuide).font(.caption).foregroundStyle(.secondary)
            }
            HStack {
                field("Tuner address", text: $address)
                Button("Add by address") { Task { await addAddress() } }
                    .buttonStyle(.glassProminent)
                    .disabled(busy || disabled || address.trimmingCharacters(in: .whitespaces).isEmpty)
            }
            if busy {
                Text("Looking…").font(.caption).foregroundStyle(.secondary)
            } else if !note.isEmpty {
                Text(note).font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    private func field(_ title: String, text: Binding<String>) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title).font(.caption).foregroundStyle(.secondary)
            TextField(title, text: text)
                .textContentType(.URL)
            #if os(iOS)
                .keyboardType(.URL)
                .textInputAutocapitalization(.never)
            #endif
                .autocorrectionDisabled()
                .padding(12)
                .background(Tokens.ColorToken.surface2, in: .rect(cornerRadius: Tokens.Radius.sm))
        }
    }

    private func look() async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        do {
            hits = try await api.lookHarder()
            note = hits.isEmpty ? "Nothing else answered." : ""
        } catch {
            hits = []
            note = error.localizedDescription
        }
    }

    private func findFree() async {
        guard let api = store.api else { return }
        busy = true
        freeGuide = ""
        defer { busy = false }
        do {
            let res = try await api.freeSources()
            feeds = res.found
            freeGuide = res.found.isEmpty ? res.guide : ""
        } catch {
            note = "No free-channel server answered."
        }
    }

    private func add(_ hit: APIClient.FoundHit) async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        do {
            if hit.kind == "fastchannels" || hit.kind == "pluto" || hit.kind == "samsung" {
                let addr = hit.addr.contains("://") ? hit.addr : "http://\(hit.addr)"
                let message = try await api.addFree(kind: hit.kind, addr: addr, playlist: "", guide: "", name: hit.name)
                note = message.isEmpty ? "Source added." : message
            } else {
                _ = try await api.discover(ip: hit.addr)
                note = "Source added."
            }
            await changed()
        } catch {
            note = error.localizedDescription
        }
    }

    private func add(_ feed: APIClient.FreeFeed) async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        do {
            let message = try await api.addFree(kind: feed.kind, addr: feed.addr, playlist: feed.playlist, guide: feed.guide, name: feed.name)
            note = message.isEmpty ? "Source added. It shows up with the lineup." : message
            await changed()
        } catch {
            note = error.localizedDescription
        }
    }

    private func addAddress() async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        do {
            _ = try await api.discover(ip: address.trimmingCharacters(in: .whitespaces))
            note = ""
            await changed()
        } catch {
            note = error.localizedDescription
        }
    }
}
