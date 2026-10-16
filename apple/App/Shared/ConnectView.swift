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
    @State private var showJoin = false
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
        #if os(tvOS)
            if explained {
                tvFinder
            } else {
                tvExplainer
            }
        #else
            if explained {
                finder
            } else {
                explainer
            }
        #endif
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
                            ForEach(store.remembered, id: \.url) { server in
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
                    joinSection
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

    #if os(tvOS)
        // Apple TV, from ten feet: the brand on the left, big server cards on the right,
        // nothing that moves once it is drawn, and forms on pages of their own.

        private var tvExplainer: some View {
            TVConnectLayout {
                Image(systemName: "antenna.radiowaves.left.and.right")
                    .font(.system(size: 120, weight: .regular))
                    .foregroundStyle(.secondary)
                    .accessibilityHidden(true)
                Text("Find your server")
                    .font(.system(size: 76, weight: .heavy))
            } content: {
                Text("Broadwave looks on your home network for the computer running it. This Apple TV will ask to allow this.")
                    .font(.title3)
                    .foregroundStyle(.secondary)
                Text("Nothing leaves the house.")
                    .font(.title3)
                    .foregroundStyle(.secondary)
                Button("Look for my server") {
                    UserDefaults.standard.set(true, forKey: "localNetworkExplained")
                    explained = true
                }
                .buttonStyle(.borderedProminent)
                .accessibilityIdentifier("connect-look")
            }
        }

        private var tvFinder: some View {
            NavigationStack {
                TVConnectLayout {
                    brand
                    Button("Try the demo", systemImage: "play.fill") {
                        Task { await tryDemo() }
                    }
                    .buttonStyle(TVChipStyle())
                    .disabled(checking)
                    .accessibilityIdentifier("connect-demo")
                    Text("Sample films on this Apple TV.")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                } content: {
                    let saved = store.remembered.filter { !$0.isLoopback }
                    let found = discovery.servers.filter { server in !server.isLoopback && !saved.contains { $0.url == server.url } }
                    if !saved.isEmpty {
                        TVSectionTitle(String(localized: "Saved"))
                        ForEach(saved, id: \.url) { tvServerCard($0) }
                    }
                    HStack(spacing: 16) {
                        TVSectionTitle(String(localized: "On your network"))
                        if discovery.searching, !discovery.looked {
                            ProgressView()
                        }
                    }
                    if found.isEmpty {
                        Text(discovery.looked ? (saved.isEmpty ? "No server answered on this network." : "No other server answered on this network.") : "Looking…")
                            .font(.callout)
                            .foregroundStyle(.secondary)
                            .frame(minHeight: 60, alignment: .topLeading)
                    }
                    ForEach(found) { tvServerCard($0) }
                    HStack(spacing: 32) {
                        Button("Enter an address", systemImage: "keyboard") { showAddress = true }
                            .accessibilityIdentifier("connect-address")
                        Button("Join a home server", systemImage: "person.badge.key") { showJoin = true }
                            .accessibilityIdentifier("join-open")
                    }
                    .buttonStyle(TVChipStyle())
                    .padding(.top, 12)
                    if let problem {
                        Text(problem)
                            .font(.callout)
                            .foregroundStyle(Tokens.ColorToken.tally)
                    }
                }
                .navigationDestination(isPresented: $showAddress) { tvAddressPage }
                .navigationDestination(isPresented: $showJoin) {
                    TVConnectLayout {
                        Image(systemName: "person.badge.key.fill")
                            .font(.system(size: 120, weight: .regular))
                            .foregroundStyle(.secondary)
                            .accessibilityHidden(true)
                        Text("Join a home server")
                            .font(.system(size: 64, weight: .heavy))
                    } content: {
                        JoinView(showsHeading: false, focusAddress: true)
                    }
                }
            }
            .onAppear { discovery.start() }
            .onDisappear { discovery.stop() }
        }

        private var brand: some View {
            VStack(alignment: .leading, spacing: 20) {
                HStack(spacing: 14) {
                    Circle().fill(Tokens.ColorToken.tally).frame(width: 18, height: 18)
                        .shadow(color: Tokens.ColorToken.tally, radius: 10)
                        .accessibilityHidden(true)
                    Text("Broadwave").font(.title2.weight(.heavy))
                }
                Text("Your antenna, on every screen.")
                    .font(.system(size: 76, weight: .heavy))
                    .fixedSize(horizontal: false, vertical: true)
                Text("Pick your server. It's the computer running Broadwave next to your HDHomeRun.")
                    .font(.title3)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }

        private func tvServerCard(_ server: FoundServer) -> some View {
            Button {
                Task { await check(server.url, saved: server) }
            } label: {
                HStack(spacing: 28) {
                    Image(systemName: "server.rack")
                        .font(.title2)
                        .frame(width: 64)
                        .accessibilityHidden(true)
                    VStack(alignment: .leading, spacing: 6) {
                        Text(server.name).font(.title3.weight(.semibold)).lineLimit(1)
                        Text(server.url.host() ?? server.url.absoluteString).font(.callout).opacity(0.7)
                    }
                    Spacer(minLength: 24)
                    if checking {
                        ProgressView()
                    } else {
                        Image(systemName: "chevron.right").font(.headline).opacity(0.6)
                            .accessibilityHidden(true)
                    }
                }
                .padding(.horizontal, 36)
                .padding(.vertical, 28)
            }
            .buttonStyle(TVServerCardStyle())
            .disabled(checking)
            .accessibilityIdentifier("connect-server")
        }

        private var tvAddressPage: some View {
            TVConnectLayout {
                Image(systemName: "keyboard")
                    .font(.system(size: 120, weight: .regular))
                    .foregroundStyle(.secondary)
                    .accessibilityHidden(true)
                Text("Enter an address")
                    .font(.system(size: 64, weight: .heavy))
            } content: {
                Text("The address of the computer running Broadwave, like 192.168.1.20 or 192.168.1.20:8477.")
                    .font(.title3)
                    .foregroundStyle(.secondary)
                TVAddressForm(address: $address, checking: checking, problem: problem) {
                    Task { await check(manualURL(), saved: nil) }
                }
            }
        }
    #endif

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

    /// Away from home: an address the owner shares and an invite code.
    @ViewBuilder
    private var joinSection: some View {
        #if os(tvOS)
            if showJoin {
                JoinView()
            } else {
                Button("Join a home server") { showJoin = true }
                    .buttonStyle(.glass)
                    .accessibilityIdentifier("join-open")
            }
        #else
            JoinView()
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
                    Task { await check(manualURL(), saved: nil) }
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
            Task { await check(server.url, saved: server) }
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

    /// Bare addresses are the home network: http on port 8477. An https address keeps its own port.
    private func manualURL() -> URL? {
        ServerAddress.url(address)
    }

    private func tryDemo() async {
        checking = true
        defer { checking = false }
        await store.startDemo()
        if store.server?.id != "demo" {
            problem = store.error ?? "The demo did not start."
        }
    }

    private func check(_ url: URL?, saved: FoundServer?) async {
        guard let url else {
            problem = "That doesn't look like an address."
            return
        }
        checking = true
        defer { checking = false }
        do {
            let info = try await APIClient(base: url).server()
            let display = info.name.isEmpty ? (saved?.name ?? "Broadwave") : info.name
            store.connect(FoundServer(id: info.id, name: display, url: url, key: info.discoveryKey))
        } catch {
            if let saved, let moved = await store.locate(saved) {
                store.connect(moved, movedFrom: saved.url)
                return
            }
            problem = "No Broadwave server answered at \(url.host() ?? url.absoluteString)."
        }
    }
}

#if os(tvOS)
    /// The connect pages' layout: a fixed column on the left, the choices on the right.
    struct TVConnectLayout<Side: View, Content: View>: View {
        @ViewBuilder var side: Side
        @ViewBuilder var content: Content

        var body: some View {
            HStack(alignment: .center, spacing: 120) {
                VStack(alignment: .leading, spacing: 28) { side }
                    .frame(width: 760, alignment: .leading)
                ScrollView {
                    VStack(alignment: .leading, spacing: 28) { content }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.vertical, 60)
                        .padding(.horizontal, 20)
                }
                .scrollClipDisabled()
                .frame(maxWidth: .infinity)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Tokens.ColorToken.canvas.ignoresSafeArea())
            .toolbar(.hidden, for: .navigationBar)
        }
    }

    /// A server, large and high contrast: white on dark, black on white when focused.
    struct TVServerCardStyle: ButtonStyle {
        func makeBody(configuration: Configuration) -> some View {
            CardBody(configuration: configuration)
        }

        private struct CardBody: View {
            let configuration: ButtonStyleConfiguration
            @Environment(\.isFocused) private var focused

            var body: some View {
                configuration.label
                    .foregroundStyle(focused ? Color.black : Color.white)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(focused ? Color.white : Color.white.opacity(0.1), in: .rect(cornerRadius: 24))
                    .overlay(RoundedRectangle(cornerRadius: 24).stroke(.white.opacity(focused ? 0 : 0.18), lineWidth: 2))
                    .scaleEffect(configuration.isPressed ? 0.98 : focused ? 1.04 : 1)
                    .shadow(color: .black.opacity(focused ? 0.5 : 0), radius: 24, y: 12)
                    .animation(.spring(response: 0.3, dampingFraction: 0.8), value: focused)
            }
        }
    }

    /// One address field that takes focus when its page opens, then Connect.
    struct TVAddressForm: View {
        @Binding var address: String
        let checking: Bool
        let problem: String?
        let connect: () -> Void
        @FocusState private var fieldFocused: Bool

        var body: some View {
            VStack(alignment: .leading, spacing: 28) {
                TextField("192.168.1.20:8477", text: $address)
                    .autocorrectionDisabled()
                    .focused($fieldFocused)
                    .onSubmit(connect)
                    .accessibilityIdentifier("connect-address-field")
                Button("Connect", action: connect)
                    .buttonStyle(.borderedProminent)
                    .disabled(address.isEmpty || checking)
                if let problem {
                    Text(problem).font(.callout).foregroundStyle(Tokens.ColorToken.tally)
                }
            }
            .defaultFocus($fieldFocused, true)
        }
    }
#endif
