import OTAKit
import OTAUI
import SwiftUI

/// First run: find the server on the network, or type its address.
struct ConnectView: View {
    @Environment(AppStore.self) private var store
    @State private var discovery = Discovery()
    @State private var address = ""
    @State private var checking = false
    @State private var problem: String?

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 28) {
                    VStack(alignment: .leading, spacing: 10) {
                        HStack(spacing: 10) {
                            Circle().fill(Tokens.ColorToken.tally).frame(width: 12, height: 12)
                                .shadow(color: Tokens.ColorToken.tally, radius: 8)
                            Text("OTA Viewer").font(.title2.weight(.heavy))
                        }
                        Text("Your antenna, on every screen.")
                            .font(.largeTitle.weight(.heavy))
                        Text("Pick your server. It's the computer running OTA Viewer next to your HDHomeRun.")
                            .foregroundStyle(.secondary)
                    }

                    VStack(alignment: .leading, spacing: 12) {
                        HStack {
                            Text("On your network").font(.headline)
                            if discovery.searching && discovery.servers.isEmpty { ProgressView().padding(.leading, 6) }
                        }
                        if discovery.servers.isEmpty {
                            Text("Looking…").foregroundStyle(.secondary)
                        }
                        ForEach(discovery.servers) { server in
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
                    }

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
                .padding(24)
                .frame(maxWidth: 640, alignment: .leading)
                .frame(maxWidth: .infinity)
            }
            .background(Tokens.ColorToken.canvas.ignoresSafeArea())
        }
        .onAppear { discovery.start() }
        .onDisappear { discovery.stop() }
    }

    private func manualURL() -> URL? {
        var raw = address.trimmingCharacters(in: .whitespaces)
        if !raw.contains("://") { raw = "http://" + raw }
        guard var comps = URLComponents(string: raw) else { return nil }
        if comps.port == nil { comps.port = 8477 }
        return comps.url
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
            store.connect(FoundServer(id: id ?? info.id, name: name ?? info.name, url: url))
        } catch {
            problem = "No OTA Viewer server answered at \(url.host() ?? url.absoluteString)."
        }
    }
}
