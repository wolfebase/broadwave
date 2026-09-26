import Foundation

/// Picks a new address when the same server shows up somewhere else.
public enum ServerFollow {
    /// The saved server, at a different host or port, when the reply is signed
    /// by the key stored from GET /server. A matching id alone is not enough.
    public static func updated(_ saved: FoundServer, found: [FoundServer]) -> FoundServer? {
        guard let key = saved.key, !key.isEmpty, saved.id != "demo", saved.id != "pending" else { return nil }
        for server in found {
            let opened = server.url.absoluteString
            guard server.id == saved.id, !samePlace(server.url, saved.url),
                  FinderPacket.isLocal(server.url),
                  let nonce = server.nonce, let sig = server.signature, let signed = server.signedURL,
                  signed == opened,
                  FinderPacket.accepts(sig, nonce: nonce, url: signed, id: server.id, key: key)
            else { continue }
            var next = server
            next.key = key
            next.signature = nil
            next.nonce = nil
            next.signedURL = nil
            return next
        }
        return nil
    }

    public static func samePlace(_ a: URL, _ b: URL) -> Bool {
        let ah = a.host()?.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        let bh = b.host()?.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        return ah == bh && port(a) == port(b) && a.scheme == b.scheme
    }

    private static func port(_ url: URL) -> Int {
        if let port = url.port {
            return port
        }
        return url.scheme == "https" ? 443 : 80
    }
}

/// broadwave://connect?url=http://host:port
public enum ConnectLink {
    public static func serverURL(from url: URL) -> URL? {
        guard url.scheme == "broadwave", url.host() == "connect" else { return nil }
        guard let raw = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first(where: { $0.name == "url" })?.value else {
            return nil
        }
        guard let server = URL(string: raw), server.scheme == "http" || server.scheme == "https", server.user == nil, server.password == nil, server.host() != nil else {
            return nil
        }
        return server
    }
}

/// Servers this device has connected to, newest first.
public enum RememberedServers {
    public static func upsert(_ list: [FoundServer], _ server: FoundServer) -> [FoundServer] {
        guard server.id != "demo", server.id != "pending", !server.id.isEmpty else { return list }
        var next = list.filter { $0.id != server.id }
        next.insert(server, at: 0)
        if next.count > 8 {
            next.removeLast(next.count - 8)
        }
        return next
    }
}
