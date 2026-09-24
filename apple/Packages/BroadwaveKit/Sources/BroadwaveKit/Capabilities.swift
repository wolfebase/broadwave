import Foundation

public enum Capabilities {
    /// What this device plays. Apple devices decode H.264 and HEVC in hardware and take
    /// Dolby Digital in HLS, so the server can send the original broadcast untouched.
    public static func current(cellular: Bool = false) -> Caps {
        #if os(tvOS)
            let platform = "tvos"
        #elseif os(iOS)
            let platform = "ios"
        #else
            let platform = "macos"
        #endif
        return Caps(platform: platform, video: ["h264", "hevc"], audio: ["aac", "ac3", "eac3"], network: cellular ? "cellular" : "lan")
    }
}
