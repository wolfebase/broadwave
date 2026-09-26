import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// First run: find the server on the network, or type its address.
struct ConnectView: View {
    @Environment(AppStore.self) private var store
    @State private var discovery = Discovery()
    @State private var address = ""
    @State private var checking = false
    @State private var problem: String?
    @State private var showAddress = false
    @State private var explained = ConnectView.initiallyExplained()

    private static func initiallyExplained() -> Bool {
        #if DEBUG
            if UserDefaults.standard.bool(forKey: "BroadwaveExplain") {
                return false
            }
            if UserDefaults.standard.bool(forKey: "BroadwaveDiscover") {
                return true
            }
        #endif
        return UserDefaults.standard.bool(forKey: "localNetworkExplained")
    }

    var body: some View {
        if explained {
            finder
        } else {
            explainer
        }
    }

    private var explainer: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Find your server")
                .font(.largeTitle.weight(.heavy))
            Text("Broadwave looks on your home network for the computer running it. This device will ask to allow this.")
                .foregroundStyle(.secondary)
            Text("Nothing leaves the house.")
                .foregroundStyle(.secondary)
            Button("Look for my server") {
                UserDefaults.standard.set(true, forKey: "localNetworkExplained")
                explained = true
            }
            .buttonStyle(.glassProminent)
        }
        .padding(24)
        .frame(maxWidth: 640, alignment: .leading)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(Tokens.ColorToken.canvas.ignoresSafeArea())
    }

    private var finder: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 28) {
                    VStack(alignment: .leading, spacing: 10) {
                        HStack(spacing: 10) {
                            Circle().fill(Tokens.ColorToken.tally).frame(width: 12, height: 12)
                                .shadow(color: Tokens.ColorToken.tally, radius: 8)
                            Text("Broadwave").font(.title2.weight(.heavy))
                        }
                        Text("Your antenna, on every screen.")
                            .font(.largeTitle.weight(.heavy))
                        Text("Pick your server. It's the computer running Broadwave next to your HDHomeRun.")
                            .foregroundStyle(.secondary)
                    }

                    VStack(alignment: .leading, spacing: 12) {
                        Button("Try the demo") {
                            Task { await tryDemo() }
                        }
                        .buttonStyle(.glassProminent)
                        .disabled(checking)
                        Text("Sample films on this device.")
                            .foregroundStyle(.secondary)
                    }

                    if !store.remembered.isEmpty {
                        VStack(alignment: .leading, spacing: 12) {
                            Text("Saved").font(.headline)
                            ForEach(store.remembered) { server in
                                serverButton(server)
                            }
                        }
                    }

                    VStack(alignment: .leading, spacing: 12) {
                        HStack {
                            Text("On your network").font(.headline)
                            if discovery.searching, discovery.servers.isEmpty, !discovery.looked {
                                ProgressView().padding(.leading, 6)
                            }
                        }
                        if discovery.servers.isEmpty {
                            Text(discovery.looked ? "No server answered on this network." : "Looking…")
                                .foregroundStyle(.secondary)
                        }
                        ForEach(discovery.servers) { server in
                            serverButton(server)
                        }
                    }

                    addressSection
                }
                .padding(24)
                .frame(maxWidth: 640, alignment: .leading)
                .frame(maxWidth: .infinity)
            }
            .background(Tokens.ColorToken.canvas.ignoresSafeArea())
        }
        .onAppear { discovery.start() }
        .onDisappear { discovery.stop() }
    }

    @ViewBuilder
    private var addressSection: some View {
        #if os(tvOS)
            if showAddress {
                addressFields
            } else {
                Button("Enter an address") { showAddress = true }
                    .buttonStyle(.glass)
            }
        #else
            addressFields
        #endif
    }

    private var addressFields: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Or enter an address").font(.headline)
            HStack {
                TextField("192.168.1.20:8477", text: $address)
                    .textContentType(.URL)
                #if os(iOS)
                    .keyboardType(.URL)
                    .textInputAutocapitalization(.never)
                #endif
                    .autocorrectionDisabled()
                    .padding(12)
                    .background(Tokens.ColorToken.surface2, in: .rect(cornerRadius: Tokens.Radius.sm))
                Button("Connect") {
                    Task { await check(manualURL(), name: nil, id: nil) }
                }
                .buttonStyle(.glassProminent)
                .disabled(address.isEmpty || checking)
            }
            if let problem {
                Text(problem).font(.footnote).foregroundStyle(Tokens.ColorToken.tally)
            }
        }
    }

    private func serverButton(_ server: FoundServer) -> some View {
        Button {
            Task { await check(server.url, name: server.name, id: server.id) }
        } label: {
            HStack {
                Image(systemName: "antenna.radiowaves.left.and.right")
                VStack(alignment: .leading) {
                    Text(server.name).font(.headline).multilineTextAlignment(.leading)
                    Text(server.url.host() ?? "").font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                Image(systemName: "chevron.right").foregroundStyle(.tertiary)
            }
            .padding()
            .frame(maxWidth: .infinity)
        }
        .buttonStyle(.glass)
    }

    private func manualURL() -> URL? {
        var raw = address.trimmingCharacters(in: .whitespaces)
        if !raw.contains("://") {
            raw = "http://" + raw
        }
        guard var comps = URLComponents(string: raw), comps.user == nil, comps.password == nil else { return nil }
        if comps.port == nil {
            comps.port = 8477
        }
        return comps.url
    }

    private func tryDemo() async {
        checking = true
        defer { checking = false }
        await store.startDemo()
        if store.server?.id != "demo" {
            problem = store.error ?? "The demo did not start."
        }
    }

    private func check(_ url: URL?, name: String?, id: String?) async {
        guard let url else {
            problem = "That doesn't look like an address."
            return
        }
        checking = true
        defer { checking = false }
        do {
            let info = try await APIClient(base: url).server()
            let display = info.name.isEmpty ? (name ?? "Broadwave") : info.name
            store.connect(FoundServer(id: info.id, name: display, url: url, key: info.discoveryKey))
        } catch {
            if let id, let moved = await store.locate(id) {
                store.connect(moved)
                return
            }
            problem = "No Broadwave server answered at \(url.host() ?? url.absoluteString)."
        }
    }
}
