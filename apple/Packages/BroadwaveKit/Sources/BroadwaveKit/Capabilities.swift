import Darwin
import Foundation

public enum Capabilities {
    /// What this device plays. Apple devices take Dolby Digital in HLS.
    /// HEVC is omitted on machines that cannot decode it (Apple TV HD, A9 and older).
    /// An unrecognized machine keeps HEVC, which is what current iPhones and Apple TV 4K play.
    public static func current(cellular: Bool = false) -> Caps {
        var caps = forMachine(machineIdentifier(), cellular: cellular)
        #if os(tvOS)
            let platform = "tvos"
        #elseif os(iOS)
            let platform = "ios"
        #else
            let platform = "macos"
            // This process is not an iPhone or Apple TV. The Mac plays HEVC.
            caps = Caps(platform: platform, video: ["h264", "hevc"], audio: caps.audio, maxHeight: nil, network: caps.network)
        #endif
        if platform != "macos" {
            caps = Caps(platform: platform, video: caps.video, audio: caps.audio, maxHeight: caps.maxHeight, network: caps.network)
        }
        return caps
    }

    /// Caps for a hardware identifier (`hw.machine` or `SIMULATOR_MODEL_IDENTIFIER`).
    public static func forMachine(_ machine: String, cellular: Bool = false) -> Caps {
        let network = cellular ? "cellular" : "lan"
        let audio = ["aac", "ac3", "eac3"]
        let hevc = ["h264", "hevc"]
        let h264 = ["h264"]
        if let gen = generation(machine, family: "AppleTV") {
            if gen <= 5 {
                return Caps(platform: "tvos", video: h264, audio: audio, maxHeight: 1080, network: network)
            }
            return Caps(platform: "tvos", video: hevc, audio: audio, maxHeight: 2160, network: network)
        }
        if let gen = generation(machine, family: "iPhone"), gen <= 8 {
            return Caps(platform: "ios", video: h264, audio: audio, network: network)
        }
        if let gen = generation(machine, family: "iPad"), gen <= 6 {
            return Caps(platform: "ios", video: h264, audio: audio, network: network)
        }
        let platform = machine.hasPrefix("iPhone") || machine.hasPrefix("iPad") || machine.hasPrefix("iPod") ? "ios" : "tvos"
        return Caps(platform: platform, video: hevc, audio: audio, network: network)
    }

    /// `SIMULATOR_MODEL_IDENTIFIER` when set, otherwise `hw.machine`.
    static func machineIdentifier() -> String {
        if let sim = ProcessInfo.processInfo.environment["SIMULATOR_MODEL_IDENTIFIER"], !sim.isEmpty {
            return sim
        }
        var size = 0
        if sysctlbyname("hw.machine", nil, &size, nil, 0) != 0 || size <= 1 {
            return ""
        }
        var buf = [CChar](repeating: 0, count: size)
        if sysctlbyname("hw.machine", &buf, &size, nil, 0) != 0 {
            return ""
        }
        let bytes = buf.prefix { $0 != 0 }.map { UInt8(bitPattern: $0) }
        return String(bytes: bytes, encoding: .utf8) ?? ""
    }

    /// Major number of `iPhone8,1` is 8. A string that is not that family returns nil.
    private static func generation(_ machine: String, family: String) -> Int? {
        guard machine.hasPrefix(family) else { return nil }
        let rest = machine.dropFirst(family.count)
        let digits = rest.prefix { $0.isNumber }
        guard !digits.isEmpty else { return nil }
        return Int(digits)
    }
}
