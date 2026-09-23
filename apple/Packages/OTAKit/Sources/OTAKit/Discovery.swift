import Foundation
import Network
import Observation

/// A server found on the network or entered by hand.
public struct FoundServer: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var url: URL

    public init(id: String, name: String, url: URL) {
        self.id = id
        self.name = name
        self.url = url
    }
}

/// Finds Waveguide servers advertised over Bonjour as `_waveguide._tcp`.
@MainActor
@Observable
public final class Discovery {
    public private(set) var servers: [FoundServer] = []
    public private(set) var searching = false

    private var browser: NWBrowser?

    public init() {}

    public func start() {
        guard browser == nil else { return }
        let params = NWParameters.tcp
        params.includePeerToPeer = false
        let browser = NWBrowser(for: .bonjourWithTXTRecord(type: "_waveguide._tcp", domain: nil), using: params)
        browser.browseResultsChangedHandler = { [weak self] results, _ in
            Task { @MainActor in self?.update(results) }
        }
        browser.stateUpdateHandler = { [weak self] state in
            Task { @MainActor in
                switch state {
                case .ready: self?.searching = true
                case .failed, .cancelled: self?.searching = false
                default: break
                }
            }
        }
        browser.start(queue: .main)
        self.browser = browser
    }

    public func stop() {
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
                if let i = self.servers.firstIndex(where: { $0.id == id }) {
                    self.servers[i] = server
                } else {
                    self.servers.append(server)
                }
            }
        }
        let gone = serviceNames.filter { !live.contains($0.key) }.map(\.value)
        servers.removeAll { gone.contains($0.id) }
        serviceNames = serviceNames.filter { live.contains($0.key) }
    }

    /// Opens a connection to learn the address the service lives at.
    nonisolated static func resolve(_ endpoint: NWEndpoint, port: Int) async -> URL? {
        await withCheckedContinuation { cont in
            let conn = NWConnection(to: endpoint, using: .tcp)
            let once = OnceBox()
            conn.stateUpdateHandler = { state in
                switch state {
                case .ready:
                    var url: URL?
                    if case let .hostPort(host, _) = conn.currentPath?.remoteEndpoint {
                        var h = "\(host)"
                        if let pct = h.firstIndex(of: "%") { h = String(h[..<pct]) }
                        if h.contains(":") { h = "[\(h)]" }
                        url = URL(string: "http://\(h):\(port)")
                    }
                    conn.cancel()
                    if once.claim() { cont.resume(returning: url) }
                case .failed, .cancelled:
                    if once.claim() { cont.resume(returning: nil) }
                default:
                    break
                }
            }
            conn.start(queue: .global())
            DispatchQueue.global().asyncAfter(deadline: .now() + 4) {
                conn.cancel()
                if once.claim() { cont.resume(returning: nil) }
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
        if done { return false }
        done = true
        return true
    }
}
