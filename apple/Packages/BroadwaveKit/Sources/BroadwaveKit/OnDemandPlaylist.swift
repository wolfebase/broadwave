import AVFoundation
import Foundation

/// A recording plays as on-demand video: 0:00 to its length, time left, no LIVE badge,
/// no wall clock. The server serves a recording as an EVENT playlist that grows while it
/// transcodes, with each segment dated by when it aired, and AVKit draws that as a live
/// broadcast. The player reads the playlist through `OnDemandLoader`, which drops the
/// dates and, when the length is known, lists the whole recording up front. A segment
/// the encode has not written yet is waited for, then fetched from the server as usual.
public enum OnDemandPlaylist {
    /// The playlist the player asks for. The loader fetches the real one.
    static let scheme = "bwvod"
    /// A segment the encode has not written yet.
    static let waitScheme = "bwvodwait"
    /// The encode's segment length (`-hls_time 2`, a key frame every 2 s) until the
    /// playlist shows the real one: 2.002 s at 29.97 frames a second.
    static let segmentLength = 2.0

    public struct Rewritten: Equatable, Sendable {
        public var text: String
        /// The server's playlist was complete.
        public var ended: Bool
    }

    /// Rewrites one playlist the server sent. `base` is its real address, which segment
    /// lines resolve against. `length` is the recording's length in seconds; without it
    /// the playlist keeps growing as the server's does, minus the dates.
    public static func rewrite(_ text: String, base: URL, length: Double?) -> Rewritten {
        let lines = text.replacingOccurrences(of: "\r\n", with: "\n").split(separator: "\n", omittingEmptySubsequences: false).map(String.init)
        if lines.contains(where: { $0.hasPrefix("#EXT-X-STREAM-INF") }) {
            return Rewritten(text: multivariant(lines, base: base), ended: true)
        }
        var out: [String] = []
        var covered = 0.0
        var lastIndex = -1
        var ended = false
        var typed = false
        var longest = 0.0
        var step: Double?
        var pending: Double?
        for raw in lines {
            let line = raw.trimmingCharacters(in: .whitespaces)
            if line.isEmpty || line.hasPrefix("#EXT-X-PROGRAM-DATE-TIME:") {
                continue
            }
            if line == "#EXT-X-ENDLIST" {
                ended = true
                continue
            }
            if line.hasPrefix("#EXT-X-PLAYLIST-TYPE:") {
                typed = true
                out.append("#EXT-X-PLAYLIST-TYPE:\(length != nil ? "VOD" : String(line.dropFirst("#EXT-X-PLAYLIST-TYPE:".count)))")
                continue
            }
            if line.hasPrefix("#EXTINF:") {
                let value = Double(line.dropFirst("#EXTINF:".count).split(separator: ",").first ?? "") ?? 0
                covered += value
                longest = max(longest, value)
                pending = value
                out.append(line)
                continue
            }
            if line.hasPrefix("#EXT-X-MAP:") || line.hasPrefix("#EXT-X-KEY:") {
                out.append(absoluteURIAttribute(line, base: base))
                continue
            }
            if line.hasPrefix("#") {
                out.append(line)
                continue
            }
            if let index = segmentIndex(line) {
                lastIndex = max(lastIndex, index)
                if step == nil, let pending, pending > 0.5 {
                    step = pending
                }
            }
            out.append(URL(string: line, relativeTo: base)?.absoluteString ?? line)
        }
        if ended {
            if !typed {
                insertAfterHeader(&out, "#EXT-X-PLAYLIST-TYPE:VOD")
            }
            out.append("#EXT-X-ENDLIST")
            return Rewritten(text: out.joined(separator: "\n") + "\n", ended: true)
        }
        guard let length, length > covered + 0.05 else {
            if length != nil {
                if !typed {
                    insertAfterHeader(&out, "#EXT-X-PLAYLIST-TYPE:VOD")
                }
                out.append("#EXT-X-ENDLIST")
            }
            return Rewritten(text: out.joined(separator: "\n") + "\n", ended: false)
        }
        if !typed {
            insertAfterHeader(&out, "#EXT-X-PLAYLIST-TYPE:VOD")
        }
        // The segments still to come keep the encode's own length, so their places on
        // the timeline match the pictures in them.
        let each = step ?? segmentLength
        var at = covered
        var index = lastIndex + 1
        while at < length - 0.05 {
            let piece = min(each, length - at)
            out.append(String(format: "#EXTINF:%.3f,", piece))
            out.append(waitURL(for: segmentName(index), base: base, at: at).absoluteString)
            at += piece
            index += 1
        }
        longest = max(longest, each)
        if let target = out.firstIndex(where: { $0.hasPrefix("#EXT-X-TARGETDURATION:") }) {
            let listed = Int(out[target].dropFirst("#EXT-X-TARGETDURATION:".count)) ?? 0
            out[target] = "#EXT-X-TARGETDURATION:\(max(listed, Int(longest.rounded(.up))))"
        }
        out.append("#EXT-X-ENDLIST")
        return Rewritten(text: out.joined(separator: "\n") + "\n", ended: false)
    }

    /// The segment's number in `seg00012.ts`. Gap segments and anything else have none.
    static func segmentIndex(_ uri: String) -> Int? {
        let name = (uri.split(separator: "?").first.map(String.init) ?? uri).split(separator: "/").last.map(String.init) ?? uri
        guard name.hasPrefix("seg"), name.hasSuffix(".ts") else { return nil }
        return Int(name.dropFirst(3).dropLast(3))
    }

    static func segmentName(_ index: Int) -> String {
        String(format: "seg%05d.ts", index)
    }

    /// The custom address a waiting segment is asked for at. `at` is where it starts
    /// in the recording, which says when a recording still on the air reaches it.
    static func waitURL(for name: String, base: URL, at: Double) -> URL {
        let real = URL(string: name, relativeTo: base)?.absoluteURL ?? base
        var parts = URLComponents(url: real, resolvingAgainstBaseURL: true)
        parts?.scheme = waitScheme
        parts?.queryItems = [URLQueryItem(name: "t", value: String(format: "%.3f", at))]
        return parts?.url ?? real
    }

    /// Where a waiting segment starts in the recording, from its address.
    static func waitStart(_ url: URL) -> Double? {
        URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first { $0.name == "t" }?.value.flatMap(Double.init)
    }

    /// The address the player asks for in place of `real`.
    static func playerURL(_ real: URL) -> URL {
        var parts = URLComponents(url: real, resolvingAgainstBaseURL: true)
        parts?.scheme = scheme
        return parts?.url ?? real
    }

    /// The real address behind a custom one.
    static func realURL(_ url: URL, scheme real: String) -> URL {
        var parts = URLComponents(url: url, resolvingAgainstBaseURL: true)
        parts?.scheme = real
        return parts?.url ?? url
    }

    /// A multivariant playlist: its media playlists go through the loader too.
    private static func multivariant(_ lines: [String], base: URL) -> String {
        var out: [String] = []
        for raw in lines {
            let line = raw.trimmingCharacters(in: .whitespaces)
            if line.isEmpty {
                continue
            }
            if line.hasPrefix("#EXT-X-MEDIA:") || line.hasPrefix("#EXT-X-I-FRAME-STREAM-INF:") {
                out.append(uriAttribute(line) { uri in
                    let real = URL(string: uri, relativeTo: base)?.absoluteURL
                    guard let real else { return uri }
                    return real.pathExtension == "m3u8" ? playerURL(real).absoluteString : real.absoluteString
                })
            } else if line.hasPrefix("#") {
                out.append(line)
            } else if let real = URL(string: line, relativeTo: base)?.absoluteURL {
                out.append(playerURL(real).absoluteString)
            } else {
                out.append(line)
            }
        }
        return out.joined(separator: "\n") + "\n"
    }

    private static func absoluteURIAttribute(_ line: String, base: URL) -> String {
        uriAttribute(line) { URL(string: $0, relativeTo: base)?.absoluteString ?? $0 }
    }

    private static func uriAttribute(_ line: String, _ change: (String) -> String) -> String {
        guard let start = line.range(of: "URI=\""), let end = line[start.upperBound...].firstIndex(of: "\"") else { return line }
        let uri = String(line[start.upperBound ..< end])
        return line.replacingCharacters(in: start.upperBound ..< end, with: change(uri))
    }

    private static func insertAfterHeader(_ out: inout [String], _ line: String) {
        let at = out.firstIndex(where: { $0.hasPrefix("#EXTINF") || !$0.hasPrefix("#") }) ?? out.count
        out.insert(line, at: at)
    }
}

/// Serves `OnDemandPlaylist` to AVPlayer. One per recording item; the item's asset keeps it.
public final class OnDemandLoader: NSObject, AVAssetResourceLoaderDelegate, @unchecked Sendable {
    private let real: URL
    private let headers: [String: String]
    private let length: Double?
    /// When a recording still on the air started. Its segments come at the speed of the
    /// broadcast, so each one is waited for until it has aired.
    private let aired: Date?
    private let session: URLSession
    private let lock = NSLock()
    private var latest: (text: String, at: Date)?
    /// How long a segment the encode has not reached yet is waited for.
    private let wait: TimeInterval
    private nonisolated(unsafe) static var key: UInt8 = 0

    init(real: URL, cookie: HTTPCookie?, length: Double?, aired: Date? = nil, wait: TimeInterval = 30) {
        self.real = real
        self.aired = aired
        headers = cookie.map { HTTPCookie.requestHeaderFields(with: [$0]) } ?? [:]
        self.length = length.flatMap { $0 > 1 ? $0 : nil }
        self.wait = wait
        let config = URLSessionConfiguration.ephemeral
        config.requestCachePolicy = .reloadIgnoringLocalCacheData
        session = URLSession(configuration: config)
    }

    /// An asset that plays `real` as on-demand video of `length` seconds.
    public static func asset(real: URL, cookie: HTTPCookie?, length: Double?, aired: Date? = nil) -> AVURLAsset {
        let loader = OnDemandLoader(real: real, cookie: cookie, length: length, aired: aired)
        var options: [String: Any] = [:]
        if let cookie {
            options[AVURLAssetHTTPCookiesKey] = [cookie]
        }
        let asset = AVURLAsset(url: OnDemandPlaylist.playerURL(real), options: options)
        asset.resourceLoader.setDelegate(loader, queue: DispatchQueue(label: "broadwave.ondemand"))
        // The loader is the delegate, which the resource loader holds weakly.
        objc_setAssociatedObject(asset, &key, loader, .OBJC_ASSOCIATION_RETAIN)
        return asset
    }

    public func resourceLoader(_: AVAssetResourceLoader, shouldWaitForLoadingOfRequestedResource request: AVAssetResourceLoadingRequest) -> Bool {
        guard let url = request.request.url else { return false }
        let box = RequestBox(request)
        switch url.scheme {
        case OnDemandPlaylist.scheme:
            Task { await self.servePlaylist(box, url: OnDemandPlaylist.realURL(url, scheme: real.scheme ?? "http")) }
            return true
        case OnDemandPlaylist.waitScheme:
            Task { await self.serveSegment(box, url: OnDemandPlaylist.realURL(url, scheme: real.scheme ?? "http")) }
            return true
        default:
            return false
        }
    }

    private func servePlaylist(_ box: RequestBox, url: URL) async {
        let deadline = Date().addingTimeInterval(20)
        while !box.request.isCancelled {
            if let text = await fetch(url) {
                let isMain = url.path == real.path
                if isMain {
                    remember(text)
                }
                let rewritten = OnDemandPlaylist.rewrite(text, base: url, length: isMain ? length : nil)
                box.request.response = HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil, headerFields: ["Content-Type": "application/vnd.apple.mpegurl"])
                box.request.dataRequest?.respond(with: Data(rewritten.text.utf8))
                box.request.finishLoading()
                return
            }
            if Date() > deadline {
                break
            }
            // The encode has not written its first segment yet.
            try? await Task.sleep(for: .milliseconds(400))
        }
        box.request.finishLoading(with: URLError(.resourceUnavailable))
    }

    /// Waits until the server's playlist lists the segment, then sends the player there.
    /// Past the end of a finished encode, or after `wait`, it is an empty gap segment.
    private func serveSegment(_ box: RequestBox, url: URL) async {
        let name = url.lastPathComponent
        var deadline = Date().addingTimeInterval(wait)
        if let aired, let start = OnDemandPlaylist.waitStart(url) {
            deadline = max(deadline, aired.addingTimeInterval(start + OnDemandPlaylist.segmentLength + wait))
        }
        var plain = URLComponents(url: url, resolvingAgainstBaseURL: false)
        plain?.query = nil
        let url = plain?.url ?? url
        var target: URL?
        while !box.request.isCancelled {
            if let text = await currentPlaylist() {
                if text.contains("\n\(name)") || text.contains("/\(name)") || text.hasPrefix(name) {
                    target = url
                    break
                }
                if text.contains("#EXT-X-ENDLIST") {
                    break
                }
            }
            if Date() > deadline {
                break
            }
            try? await Task.sleep(for: .milliseconds(500))
        }
        guard !box.request.isCancelled else { return }
        let to = target ?? url.deletingLastPathComponent().appendingPathComponent("gap-end.ts")
        box.request.redirect = URLRequest(url: to)
        box.request.response = HTTPURLResponse(url: to, statusCode: 302, httpVersion: nil, headerFields: ["Location": to.absoluteString])
        box.request.finishLoading()
    }

    /// The server's playlist, at most half a second old. Every waiting segment shares it.
    private func currentPlaylist() async -> String? {
        if let latest = lock.withLock({ latest }), Date().timeIntervalSince(latest.at) < 0.5 {
            return latest.text
        }
        guard let text = await fetch(real) else { return nil }
        remember(text)
        return text
    }

    private func remember(_ text: String) {
        lock.withLock { latest = (text, Date()) }
    }

    private func fetch(_ url: URL) async -> String? {
        var request = URLRequest(url: url, timeoutInterval: 10)
        for (name, value) in headers {
            request.setValue(value, forHTTPHeaderField: name)
        }
        guard let (data, response) = try? await session.data(for: request),
              (response as? HTTPURLResponse)?.statusCode == 200,
              let text = String(data: data, encoding: .utf8), text.hasPrefix("#EXTM3U")
        else { return nil }
        return text
    }
}

/// The loading request crosses into a task. AVFoundation allows answering it from any thread.
private final class RequestBox: @unchecked Sendable {
    let request: AVAssetResourceLoadingRequest

    init(_ request: AVAssetResourceLoadingRequest) {
        self.request = request
    }
}

public extension APIClient {
    /// A recording's player item, as on-demand video of `length` seconds.
    /// See `OnDemandPlaylist`. `aired` is the start of a recording still on the air.
    /// Away from home it plays from a media link.
    func onDemandItem(_ path: String, length: Double?, aired: Date? = nil) async -> AVPlayerItem {
        if let linked = await mediaLinkURL(path) {
            return AVPlayerItem(asset: OnDemandLoader.asset(real: linked, cookie: nil, length: length, aired: aired))
        }
        return AVPlayerItem(asset: OnDemandLoader.asset(real: url(path), cookie: tokenCookie, length: length, aired: aired))
    }
}
