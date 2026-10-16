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

/// Servers this device has connected to, newest first. Each entry is kept under
/// its address (scheme, host, and port, as tokens are), so a server that reports
/// another server's id never replaces that server's entry. The same server
/// reached at home and away from home is two entries.
public enum RememberedServers {
    /// `movedFrom` is the address a saved home-network server answered at before
    /// the signed discovery reply and GET /server proved it moved. Only then does
    /// the entry at that old address go.
    public static func upsert(_ list: [FoundServer], _ server: FoundServer, movedFrom: URL? = nil) -> [FoundServer] {
        guard server.id != "demo", server.id != "pending", !server.id.isEmpty,
              let origin = ServerOrigin.of(server.url) else { return list }
        let old = movedFrom.flatMap(ServerOrigin.of)
        let replaced = { (entry: FoundServer) -> Bool in
            let at = ServerOrigin.of(entry.url)
            return at == origin || (old != nil && at == old && entry.id == server.id)
        }
        // A connect link has no key. Replacing the saved one would stop follow after
        // the server moves. Only the same server's key is kept, never another one's.
        var incoming = server
        let kept = list.first { replaced($0) && $0.id == server.id }?.key
        if incoming.key?.isEmpty != false, let kept, !kept.isEmpty {
            incoming.key = kept
        }
        var next = list.filter { !replaced($0) }
        next.insert(incoming, at: 0)
        if next.count > 8 {
            next.removeLast(next.count - 8)
        }
        return next
    }

    /// The saved entry at this address.
    public static func saved(at url: URL, in list: [FoundServer]) -> FoundServer? {
        guard let origin = ServerOrigin.of(url) else { return nil }
        return list.first { ServerOrigin.of($0.url) == origin }
    }

    /// What a refresh should store. Nil when nothing about this server changed.
    public static func refreshed(_ current: FoundServer, id: String, name: String, key: String?) -> FoundServer? {
        let pending = current.id == "pending" || current.id.isEmpty
        let same = current.id == id
        guard pending || same else { return nil }
        let needKey = current.key?.isEmpty != false && key?.isEmpty == false
        let rename = !name.isEmpty && name != current.name
        let rotated = key?.isEmpty == false && key != current.key
        guard pending || needKey || rename || rotated else { return nil }
        return FoundServer(
            id: id,
            name: name.isEmpty ? current.name : name,
            url: current.url,
            key: key?.isEmpty == false ? key : current.key
        )
    }
}
