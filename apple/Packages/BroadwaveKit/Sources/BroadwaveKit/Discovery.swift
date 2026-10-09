import Foundation
import Network
import Observation

/// A server found on the network or entered by hand.
public struct FoundServer: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var url: URL
    /// Base64 Ed25519 public key learned from GET /server. A UDP packet does not set this.
    public var key: String?
    /// Signature from the latest probe. Not stored.
    public var signature: String?
    /// Nonce sent with the probe that produced signature. Not stored.
    public var nonce: Data?
    /// Exact URL string the signature covers. Not stored.
    public var signedURL: String?

    public init(id: String, name: String, url: URL, key: String? = nil, signature: String? = nil, nonce: Data? = nil, signedURL: String? = nil) {
        self.id = id
        self.name = name
        self.url = url
        self.key = key
        self.signature = signature
        self.nonce = nonce
        self.signedURL = signedURL
    }

    private enum CodingKeys: String, CodingKey {
        case id, name, url, key
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(id, forKey: .id)
        try c.encode(name, forKey: .name)
        try c.encode(url, forKey: .url)
        try c.encodeIfPresent(key, forKey: .key)
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = try c.decode(String.self, forKey: .name)
        url = try c.decode(URL.self, forKey: .url)
        key = try c.decodeIfPresent(String.self, forKey: .key)
    }
}

/// Finds Broadwave servers advertised over Bonjour as `_broadwave._tcp`.
@MainActor
@Observable
public final class Discovery {
    public private(set) var servers: [FoundServer] = []
    public private(set) var searching = false
    /// True after the UDP probe has run, or Bonjour already found a server.
    public private(set) var looked = false

    private var browser: NWBrowser?
    private var probeTask: Task<Void, Never>?
    private var probed: Set<String> = []
    /// The first probe ran before Local Network permission and found nothing.
    private var probeMissed = false

    public init() {}

    public func start() {
        guard browser == nil else { return }
        let params = NWParameters.tcp
        params.includePeerToPeer = false
        let browser = NWBrowser(for: .bonjourWithTXTRecord(type: "_broadwave._tcp", domain: nil), using: params)
        browser.browseResultsChangedHandler = { [weak self] results, _ in
            Task { @MainActor in self?.update(results) }
        }
        browser.stateUpdateHandler = { [weak self] state in
            Task { @MainActor in
                switch state {
                case .ready:
                    self?.searching = true
                    self?.probeIfTheFirstMissed()
                case .failed, .cancelled: self?.searching = false
                default: break
                }
            }
        }
        browser.start(queue: .main)
        self.browser = browser
        // Bonjour goes first. The probe still runs after 3s so a server with
        // Bonjour turned off is found even when another server answered mDNS.
        probeTask = Task { @MainActor [weak self] in
            try? await Task.sleep(for: .seconds(3))
            guard !Task.isCancelled, let self, self.browser != nil else { return }
            runProbe()
        }
    }

    private func probeIfTheFirstMissed() {
        guard probeMissed, browser != nil else { return }
        probeMissed = false
        runProbe()
    }

    private func runProbe() {
        // The delayed probe and a retry after a miss share this slot. Cancel the
        // one already running so stop() reaches the probe that is actually out.
        probeTask?.cancel()
        probeTask = Task { @MainActor [weak self] in
            let found = await Task.detached { LANProbe.collect(timeout: 1.5) }.value
            guard !Task.isCancelled, let self, browser != nil else { return }
            merge(found)
            if servers.isEmpty, !searching {
                probeMissed = true
            }
        }
    }

    public func stop() {
        probeTask?.cancel()
        probeTask = nil
        browser?.cancel()
        browser = nil
        searching = false
    }

    private var serviceNames: [String: String] = [:]

    private func update(_ results: Set<NWBrowser.Result>) {
        var live = Set<String>()
        for result in results {
            guard case let .service(name, _, _, _) = result.endpoint else { continue }
            live.insert(name)
            var txt: [String: String] = [:]
            if case let .bonjour(record) = result.metadata {
                txt = record.dictionary
            }
            let port = txt["port"].flatMap(Int.init) ?? 8477
            let id = txt["id"] ?? name
            let display = txt["name"] ?? name
            serviceNames[name] = id
            // The service name is not a hostname; connect once to learn the address.
            Task { @MainActor [weak self] in
                guard let url = await Self.resolve(result.endpoint, port: port) else { return }
                let server = FoundServer(id: id, name: display, url: url)
                guard let self else { return }
                // Resolve takes seconds. Stop, or this service leaving, must not put it back.
                guard Self.keepResolved(browsing: browser != nil, listedID: serviceNames[name], resolvedID: id) else { return }
                NSLog("broadwave discovery: %@ %@", server.name, server.url.absoluteString)
                // A probe already named this server. A later Bonjour resolve can
                // replace that address with a link-local host, so leave it.
                if probed.contains(id), servers.contains(where: { $0.id == id }) {
                    return
                }
                if let i = servers.firstIndex(where: { $0.id == id }) {
                    servers[i] = server
                } else {
                    servers.append(server)
                }
            }
        }
        let gone = Self.droppedServices(names: serviceNames, live: live, probed: probed)
        servers.removeAll { gone.contains($0.id) }
        serviceNames = serviceNames.filter { live.contains($0.key) }
    }

    private func merge(_ found: [FoundServer]) {
        for server in found {
            probed.insert(server.id)
            NSLog("broadwave discovery: %@ %@", server.name, server.url.absoluteString)
            if let i = servers.firstIndex(where: { $0.id == server.id }) {
                servers[i] = server
            } else {
                servers.append(server)
            }
        }
        looked = true
    }

    /// A resolve that finishes after browsing stopped, or after that service
    /// left the latest results, must not put the server back on the list.
    nonisolated static func keepResolved(browsing: Bool, listedID: String?, resolvedID: String) -> Bool {
        browsing && listedID == resolvedID
    }

    /// An id leaves only when every service that used it has left and a probe
    /// did not also find it. One of two names going quiet is not a departure.
    nonisolated static func droppedServices(names: [String: String], live: Set<String>, probed: Set<String>) -> Set<String> {
        var gone = Set<String>()
        var staying = Set<String>()
        for (name, id) in names {
            if live.contains(name) {
                staying.insert(id)
            } else {
                gone.insert(id)
            }
        }
        return gone.subtracting(staying).subtracting(probed)
    }

    /// What a resolve does when the connection reaches a terminal state.
    /// The handler is dropped so it cannot keep the connection alive.
    struct ResolveAction: Equatable {
        var resumeWithAddress: Bool
        var dropHandler: Bool
    }

    enum ResolveEvent {
        case ready, failed, cancelled
    }

    nonisolated static func closeResolve(_ event: ResolveEvent) -> ResolveAction {
        switch event {
        case .ready:
            ResolveAction(resumeWithAddress: true, dropHandler: true)
        case .failed, .cancelled:
            ResolveAction(resumeWithAddress: false, dropHandler: true)
        }
    }

    /// Opens a connection to learn the address the service lives at.
    /// IPv4 first: a server on a Mac also answers on its IPv6 link-local
    /// address, and a URL cannot carry that address's interface.
    nonisolated static func resolve(_ endpoint: NWEndpoint, port: Int) async -> URL? {
        if let url = await resolve(endpoint, port: port, version: .v4) {
            return url
        }
        return await resolve(endpoint, port: port, version: .any)
    }

    /// The server's address as a URL, or nil for an IPv6 link-local address.
    nonisolated static func serverURL(host: String, port: Int) -> URL? {
        var h = host
        if let pct = h.firstIndex(of: "%") {
            h = String(h[..<pct])
        }
        if h.contains(":") {
            if h.lowercased().hasPrefix("fe80:") {
                return nil
            }
            h = "[\(h)]"
        }
        return URL(string: "http://\(h):\(port)")
    }

    private nonisolated static func resolve(_ endpoint: NWEndpoint, port: Int, version: NWProtocolIP.Options.Version) async -> URL? {
        await withCheckedContinuation { cont in
            let params = NWParameters.tcp
            if let ip = params.defaultProtocolStack.internetProtocol as? NWProtocolIP.Options {
                ip.version = version
            }
            let conn = NWConnection(to: endpoint, using: params)
            let once = OnceBox()
            conn.stateUpdateHandler = { state in
                let event: ResolveEvent
                switch state {
                case .ready: event = .ready
                case .failed: event = .failed
                case .cancelled: event = .cancelled
                default: return
                }
                let action = Self.closeResolve(event)
                var url: URL?
                if action.resumeWithAddress, case let .hostPort(host, _) = conn.currentPath?.remoteEndpoint {
                    url = serverURL(host: "\(host)", port: port)
                }
                let won = once.claim()
                // Clear the handler before cancel. The handler retains the connection,
                // and a cancel delivered on this stack must not resume with nil after
                // the address was already taken.
                if action.dropHandler {
                    conn.stateUpdateHandler = nil
                }
                conn.cancel()
                if won {
                    cont.resume(returning: url)
                }
            }
            conn.start(queue: .global())
            DispatchQueue.global().asyncAfter(deadline: .now() + 4) {
                conn.stateUpdateHandler = nil
                conn.cancel()
                if once.claim() {
                    cont.resume(returning: nil)
                }
            }
        }
    }
}

final class OnceBox: @unchecked Sendable {
    private var done = false
    private let lock = NSLock()

    func claim() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        if done {
            return false
        }
        done = true
        return true
    }
}
