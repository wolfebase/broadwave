import BroadwaveKit
import BroadwaveUI
import SwiftUI
import UIKit

/// Tuners, screens, and servers on this network. One action each.
struct HomeListView: View {
    var hideAdded = false
    @Environment(AppStore.self) private var store
    @State private var places: [APIClient.HomePlace] = []
    @State private var tunerAddress = ""
    @State private var sharing = false
    @State private var note = ""
    @State private var busy = true

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Your home").font(.title3.weight(.bold))
            Text("Tuners, screens, and servers on this network. Nothing is added until you tap.")
                .foregroundStyle(.secondary)
            if shown.isEmpty {
                Text(busy ? "Searching…" : "Nothing else answered yet.").foregroundStyle(.secondary)
            }
            ForEach(shown) { place in
                HStack(alignment: .center, spacing: 12) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(place.name).font(.headline)
                        let line = [place.detail, place.addr].compactMap(\.self).filter { !$0.isEmpty }.joined(separator: " · ")
                        if !line.isEmpty {
                            Text(line).font(.caption).foregroundStyle(.secondary)
                        }
                    }
                    Spacer()
                    action(place)
                }
            }
            if !note.isEmpty {
                Text(note).font(.caption).foregroundStyle(.secondary)
            }
            Button("Scan again") { Task { await load() } }
                .buttonStyle(.glass)
                .disabled(busy)
        }
        .task { await load() }
    }

    private var shown: [APIClient.HomePlace] {
        hideAdded ? places.filter { !($0.group == "tuner" && $0.action == "added") } : places
    }

    @ViewBuilder
    private func action(_ place: APIClient.HomePlace) -> some View {
        switch place.action {
        case "add":
            Button("Add") { Task { await add(place) } }
                .buttonStyle(.glass)
                .disabled(busy)
        case "use":
            Button("Use as tuner") { use() }
                .buttonStyle(.glass)
        case "added":
            Text("Added").foregroundStyle(Tokens.ColorToken.success)
        case "here":
            Text("On this server").foregroundStyle(.secondary)
        default:
            Text("On this network").foregroundStyle(.secondary)
        }
    }

    private func load() async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        do {
            let scan = try await api.home(fresh: true)
            places = scan.places
            tunerAddress = scan.tunerAddress
            sharing = scan.sharing
            note = ""
        } catch is CancellationError {
            return
        } catch {
            try? await Task.sleep(for: .milliseconds(400))
            if let scan = try? await api.home(fresh: true) {
                places = scan.places
                tunerAddress = scan.tunerAddress
                sharing = scan.sharing
                note = ""
                return
            }
            note = "This network did not answer."
        }
    }

    private func add(_ place: APIClient.HomePlace) async {
        guard let api = store.api else { return }
        busy = true
        defer { busy = false }
        do {
            _ = try await api.discover(ip: place.addr ?? "")
            await store.refresh()
            await load()
            note = "Tuner added."
        } catch {
            note = error.localizedDescription
        }
    }

    private func use() {
        let line = sharing
            ? "Add an HDHomeRun at \(tunerAddress)."
            : "Turn on Act as an HDHomeRun, then add \(tunerAddress)."
        note = line
        #if !os(tvOS)
            UIPasteboard.general.string = tunerAddress
        #endif
    }
}
