import Foundation

/// Whether a live media playlist holds enough for AVPlayer to start: its
/// hold-back plus one target duration. AVPlayer starts that far behind the
/// live edge, so a fresh tune with less shows nothing until it fills.
public enum LiveReadiness {
    public static func ready(_ playlist: String) -> Bool {
        var media = 0.0
        var target = 0.0
        var hold = 0.0
        for raw in playlist.split(whereSeparator: \.isNewline) {
            let line = raw.trimmingCharacters(in: .whitespaces)
            if line.hasPrefix("#EXTINF:") {
                media += Double(line.dropFirst(8).prefix { $0 != "," }) ?? 0
            } else if line.hasPrefix("#EXT-X-TARGETDURATION:") {
                target = Double(line.dropFirst(22)) ?? 0
            } else if line.hasPrefix("#EXT-X-SERVER-CONTROL:") {
                for pair in line.dropFirst(22).split(separator: ",") {
                    let kv = pair.split(separator: "=", maxSplits: 1)
                    if kv.count == 2, kv[0] == "HOLD-BACK" {
                        hold = Double(kv[1]) ?? 0
                    }
                }
            }
        }
        guard target > 0 else { return false }
        if hold <= 0 {
            hold = 3 * target
        }
        return media >= hold + target
    }
}
