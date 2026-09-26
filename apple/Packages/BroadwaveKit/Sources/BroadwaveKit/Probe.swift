import CryptoKit
import Darwin
import Foundation
import Security

/// The five-byte UDP probe the server answers on port 8479.
public enum FinderPacket {
    public static let port: UInt16 = 8479
    public static let request = Data("BWDP?".utf8)
    private static let yes = Data("BWDP!".utf8)

    public struct Reply: Equatable, Sendable {
        public var id: String
        public var name: String
        public var url: URL
        public var signature: String?
        public var nonce: Data?
        public var signedURL: String?
    }

    public static func parse(_ data: Data, nonce: Data? = nil) -> Reply? {
        guard data.count > yes.count, data.count <= 512, data.starts(with: yes) else { return nil }
        struct Body: Decodable {
            var id: String
            var name: String
            var url: String
            var sig: String?
        }
        guard let body = try? JSONDecoder().decode(Body.self, from: data.dropFirst(yes.count)),
              !body.id.isEmpty, body.id.count <= 64, body.id != "demo", body.id != "pending",
              !body.name.isEmpty, body.name.count <= 63,
              let url = URL(string: body.url),
              isLocal(url)
        else { return nil }
        return Reply(id: body.id, name: body.name, url: url, signature: body.sig, nonce: nonce, signedURL: body.url)
    }

    /// True when sig is an Ed25519 signature of this nonce, URL, and id under key.
    /// key is the public key stored from GET /server, never the key in the packet.
    public static func accepts(_ sig: String, nonce: Data, url: String, id: String, key: String) -> Bool {
        guard nonce.count == 16,
              let pub = Data(base64Encoded: key), pub.count == 32,
              let raw = Data(base64Encoded: sig), raw.count == 64,
              let publicKey = try? Curve25519.Signing.PublicKey(rawRepresentation: pub)
        else { return false }
        return publicKey.isValidSignature(raw, for: message(nonce: nonce, url: url, id: id))
    }

    static func message(nonce: Data, url: String, id: String) -> Data {
        var msg = Data()
        msg.append(nonce)
        msg.append(0)
        msg.append(contentsOf: Data(url.utf8))
        msg.append(0)
        msg.append(contentsOf: Data(id.utf8))
        return msg
    }

    /// A probe reply may only name a loopback or private literal. A hostname
    /// or a public address is ignored, so a packet cannot point the app off the LAN.
    public static func isLocal(_ url: URL) -> Bool {
        guard url.scheme == "http" || url.scheme == "https",
              url.user == nil, url.password == nil,
              let host = url.host(), isLocalHost(host)
        else { return false }
        return true
    }

    public static func isLocalHost(_ host: String) -> Bool {
        let parts = host.split(separator: ".")
        guard parts.count == 4,
              let a = UInt8(parts[0]), let b = UInt8(parts[1]),
              UInt8(parts[2]) != nil, UInt8(parts[3]) != nil
        else { return false }
        if a == 10 || a == 127 {
            return true
        }
        if a == 192, b == 168 {
            return true
        }
        if a == 172, (16 ... 31).contains(Int(b)) {
            return true
        }
        return false
    }
}

/// Sends BWDP? on the local network and collects replies.
/// Broadcast needs the multicast entitlement on a physical device. A unicast
/// to each address on the local subnet does not, and finds the same servers.
public enum LANProbe {
    public static func collect(timeout: TimeInterval = 1.2) -> [FoundServer] {
        let fd = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP)
        guard fd >= 0 else { return [] }
        defer { close(fd) }
        var yes: Int32 = 1
        _ = setsockopt(fd, SOL_SOCKET, SO_BROADCAST, &yes, socklen_t(MemoryLayout.size(ofValue: yes)))
        var bound = sockaddr_in()
        bound.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        bound.sin_family = sa_family_t(AF_INET)
        bound.sin_port = 0
        bound.sin_addr.s_addr = inet_addr("0.0.0.0")
        let boundOK = withUnsafePointer(to: &bound) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        guard boundOK == 0 else { return [] }
        var tv = timeval(tv_sec: 0, tv_usec: 200_000)
        _ = setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, socklen_t(MemoryLayout.size(ofValue: tv)))

        let nonce = freshNonce()
        var packet = [UInt8](FinderPacket.request)
        if nonce.count == 16 {
            packet.append(contentsOf: nonce)
        }
        func blast(_ host: UInt32) {
            var dest = sockaddr_in()
            dest.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
            dest.sin_family = sa_family_t(AF_INET)
            dest.sin_port = FinderPacket.port.bigEndian
            dest.sin_addr.s_addr = networkOrder(host)
            _ = packet.withUnsafeBufferPointer { buf in
                withUnsafePointer(to: &dest) {
                    $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                        sendto(fd, buf.baseAddress, buf.count, 0, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
                    }
                }
            }
        }

        blast(0xFFFF_FFFF)
        let ifaces = interfaces()
        for iface in ifaces {
            blast(iface.broadcast)
            if !iface.loopback {
                for host in iface.hosts {
                    blast(host)
                }
            }
        }
        blast(0x7F00_0001)

        var found: [String: FoundServer] = [:]
        let deadline = Date().addingTimeInterval(timeout)
        var buf = [UInt8](repeating: 0, count: 512)
        while Date() < deadline {
            let n = buf.withUnsafeMutableBytes { raw -> Int in
                guard let base = raw.baseAddress else { return -1 }
                return recv(fd, base, raw.count, 0)
            }
            if n > 0, let reply = FinderPacket.parse(Data(buf.prefix(n)), nonce: nonce) {
                found[reply.id] = FoundServer(
                    id: reply.id, name: reply.name, url: reply.url,
                    signature: reply.signature, nonce: nonce, signedURL: reply.signedURL
                )
            }
        }
        return Array(found.values)
    }

    private static func freshNonce() -> Data {
        var bytes = [UInt8](repeating: 0, count: 16)
        guard SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes) == errSecSuccess else { return Data() }
        return Data(bytes)
    }

    private struct Iface {
        var broadcast: UInt32
        var loopback: Bool
        var hosts: [UInt32]
    }

    /// Host-order addresses. A mask wider than /24 is capped so a VPN cannot scan the internet.
    private static func interfaces() -> [Iface] {
        var list: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&list) == 0, let list else { return [] }
        defer { freeifaddrs(list) }
        var out: [Iface] = []
        var ptr: UnsafeMutablePointer<ifaddrs>? = list
        while let cur = ptr {
            let next = cur.pointee.ifa_next
            defer { ptr = next }
            let flags = Int32(cur.pointee.ifa_flags)
            if flags & IFF_UP == 0 {
                continue
            }
            guard let sa = cur.pointee.ifa_addr, sa.pointee.sa_family == sa_family_t(AF_INET) else { continue }
            guard let maskPtr = cur.pointee.ifa_netmask else { continue }
            let address = hostOrder(sa.withMemoryRebound(to: sockaddr_in.self, capacity: 1) { $0.pointee.sin_addr.s_addr })
            let mask = hostOrder(maskPtr.withMemoryRebound(to: sockaddr_in.self, capacity: 1) { $0.pointee.sin_addr.s_addr })
            if mask == 0 || address == 0 {
                continue
            }
            let loopback = flags & IFF_LOOPBACK != 0 || (address & 0xFF00_0000) == 0x7F00_0000
            let linkLocal = (address & 0xFFFF_0000) == 0xA9FE_0000
            if linkLocal || (!loopback && !FinderPacket.isLocalHost(dotted(address))) {
                continue
            }
            var use = mask
            if prefix(mask) < 24 {
                use = 0xFFFF_FF00
            }
            let network = address & use
            let broadcast = network | ~use
            var hosts: [UInt32] = []
            if !loopback, broadcast > network &+ 1 {
                var host = network &+ 1
                while host < broadcast, hosts.count < 254 {
                    if host != address {
                        hosts.append(host)
                    }
                    host = host &+ 1
                }
            }
            out.append(Iface(broadcast: broadcast, loopback: loopback, hosts: hosts))
        }
        return out
    }

    private static func dotted(_ host: UInt32) -> String {
        "\(host >> 24).\((host >> 16) & 0xFF).\((host >> 8) & 0xFF).\(host & 0xFF)"
    }

    private static func prefix(_ mask: UInt32) -> Int {
        var n = 0
        var bit: UInt32 = 0x8000_0000
        while bit != 0, mask & bit != 0 {
            n += 1
            bit >>= 1
        }
        return n
    }

    private static func hostOrder(_ raw: in_addr_t) -> UInt32 {
        var value = raw
        let bytes = withUnsafeBytes(of: &value) { Array($0) }
        guard bytes.count == 4 else { return 0 }
        return (UInt32(bytes[0]) << 24) | (UInt32(bytes[1]) << 16) | (UInt32(bytes[2]) << 8) | UInt32(bytes[3])
    }

    private static func networkOrder(_ host: UInt32) -> in_addr_t {
        let bytes: [UInt8] = [
            UInt8((host >> 24) & 0xFF),
            UInt8((host >> 16) & 0xFF),
            UInt8((host >> 8) & 0xFF),
            UInt8(host & 0xFF),
        ]
        var out: in_addr_t = 0
        withUnsafeMutableBytes(of: &out) { dst in
            bytes.withUnsafeBytes { src in dst.copyMemory(from: src) }
        }
        return out
    }
}
