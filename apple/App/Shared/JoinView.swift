import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// Joins a home server from away, with an invite code from the owner.
/// Also takes the six-digit code an admin shows in Settings → Devices.
struct JoinView: View {
    @Environment(AppStore.self) private var store
    @State private var address: String
    @State private var code: String
    @State private var working = false
    @State private var problem: String?
    /// The six-digit code this screen shows while it waits for the owner.
    @State private var shownCode: String?
    @State private var codeAsk = 0
    var showsHeading = true
    var onJoined: (() -> Void)?

    init(address: String = "", code: String = "", showsHeading: Bool = true, onJoined: (() -> Void)? = nil) {
        _address = State(initialValue: address)
        _code = State(initialValue: code)
        self.showsHeading = showsHeading
        self.onJoined = onJoined
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if showsHeading {
                Text("Join a home server").font(.headline)
            }
            Text("Ask the owner for an invite code. It works once and lasts a week.")
                .foregroundStyle(.secondary)
            field("tv.example.com", text: $address, id: "join-address")
            #if os(iOS)
                .keyboardType(.URL)
                .textInputAutocapitalization(.never)
            #endif
            field("ABCD-EFGH-JKMN", text: $code, id: "join-code")
            #if os(iOS)
                .textInputAutocapitalization(.characters)
            #endif
            Button("Join") {
                Task { await join() }
            }
            .buttonStyle(.glassProminent)
            .disabled(address.isEmpty || code.isEmpty || working)
            .accessibilityIdentifier("join-submit")
            if working {
                ProgressView()
            }
            #if os(tvOS)
                showCodeSection
            #endif
            if let problem {
                Text(problem)
                    .font(.footnote)
                    .foregroundStyle(Tokens.ColorToken.tally)
                    .accessibilityIdentifier("join-problem")
            }
        }
        .task(id: codeAsk) {
            guard codeAsk > 0 else { return }
            await waitForApproval()
        }
    }

    private func field(_ prompt: LocalizedStringKey, text: Binding<String>, id: String) -> some View {
        TextField(prompt, text: text)
            .autocorrectionDisabled()
            .padding(12)
            .background(Tokens.ColorToken.surface2, in: .rect(cornerRadius: Tokens.Radius.sm))
            .accessibilityIdentifier(id)
    }

    #if os(tvOS)
        /// Typing on a TV is slow. The owner can approve a code shown here instead.
        @ViewBuilder
        private var showCodeSection: some View {
            if let shownCode {
                VStack(alignment: .leading, spacing: 8) {
                    Text(shownCode)
                        .font(.system(size: 72, weight: .heavy, design: .monospaced))
                        .accessibilityIdentifier("join-shown-code")
                    Text("Ask the owner to approve this code in Broadwave Settings, under Devices.")
                        .foregroundStyle(.secondary)
                }
            } else {
                Button("Show a code instead") {
                    problem = nil
                    codeAsk += 1
                }
                .buttonStyle(.glass)
                .disabled(address.isEmpty || working)
            }
        }
    #endif

    private func join() async {
        working = true
        defer { working = false }
        problem = await JoinFlow.join(address: address, code: code, store: store)
        if problem == nil {
            onJoined?()
        }
    }

    /// Shows a code and waits until an admin approves it, it runs out, or this screen closes.
    private func waitForApproval() async {
        guard let url = ServerAddress.url(address, remote: true) else {
            problem = String(localized: "That doesn't look like an address.")
            return
        }
        let api = APIClient(base: url)
        do {
            let info = try await api.server()
            let ticket = try await api.startPair(name: JoinFlow.deviceName, kind: JoinFlow.deviceKind)
            shownCode = ticket.code
            defer { shownCode = nil }
            while !Task.isCancelled {
                try await Task.sleep(for: .seconds(2))
                let poll = try await api.pollPair(ticket)
                switch poll.state {
                case "approved":
                    guard let token = poll.token, !token.isEmpty else {
                        problem = String(localized: "That server did not pair this device.")
                        return
                    }
                    store.connect(JoinFlow.found(info, url: url), token: token, device: poll.device?.id)
                    onJoined?()
                    return
                case "denied":
                    problem = String(localized: "The owner turned this code down.")
                    return
                case "expired":
                    problem = String(localized: "That code ran out. Show a new one.")
                    return
                default:
                    continue
                }
            }
        } catch is CancellationError {
            return
        } catch {
            problem = error.localizedDescription
        }
    }
}

/// The steps behind Join: check the address is Broadwave, trade the code for a
/// token, keep the token, and connect.
@MainActor
enum JoinFlow {
    static var deviceName: String {
        let name = ScreenIdentity.name.trimmingCharacters(in: .whitespacesAndNewlines)
        if !name.isEmpty {
            return String(name.prefix(63))
        }
        #if os(tvOS)
            return "Apple TV"
        #else
            return "iPhone"
        #endif
    }

    static var deviceKind: String {
        #if os(tvOS)
            "tv"
        #else
            "phone"
        #endif
    }

    static func found(_ info: ServerInfo, url: URL) -> FoundServer {
        FoundServer(id: info.id, name: info.name.isEmpty ? "Broadwave" : info.name, url: url, key: info.discoveryKey)
    }

    /// Nil when joined. Otherwise the words to show, the server's own when it sent some.
    static func join(address: String, code: String, store: AppStore) async -> String? {
        guard let url = ServerAddress.url(address, remote: true) else {
            return String(localized: "That doesn't look like an address.")
        }
        let typed = InviteCode.normalized(code)
        guard !typed.isEmpty else {
            return String(localized: "Enter the invite code.")
        }
        let api = APIClient(base: url)
        let info: ServerInfo
        do {
            info = try await api.server()
        } catch {
            return String(localized: "No Broadwave server answered at \(url.host() ?? url.absoluteString).")
        }
        do {
            let grant = try await api.claimPair(code: typed, name: deviceName, kind: deviceKind)
            store.connect(found(info, url: url), token: grant.token, device: grant.device.id)
            return nil
        } catch {
            return error.localizedDescription
        }
    }
}

/// Shown when the server turns this device away, instead of looking like an outage.
struct SignedOutView: View {
    @Environment(AppStore.self) private var store
    var message: String

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                VStack(alignment: .leading, spacing: 10) {
                    Text("This device isn't signed in")
                        .font(.largeTitle.weight(.heavy))
                    Text(message)
                        .font(.title3.weight(.semibold))
                        .accessibilityIdentifier("signed-out-message")
                    if let name = store.info?.name ?? store.server?.name, !name.isEmpty {
                        Text(name).foregroundStyle(.secondary)
                    }
                }
                JoinView(address: store.server?.url.absoluteString ?? "", showsHeading: false)
                Button("Use a different server", role: .destructive) { store.forget() }
                    .buttonStyle(.glass)
            }
            .padding(24)
            .frame(maxWidth: 640, alignment: .leading)
            .frame(maxWidth: .infinity)
        }
        .background(Tokens.ColorToken.canvas.ignoresSafeArea())
        .accessibilityIdentifier("signed-out")
    }
}
