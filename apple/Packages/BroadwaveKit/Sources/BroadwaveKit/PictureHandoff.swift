import Foundation

/// Picture in Picture and the AirPlay control.
///
/// Home moves the picture into the small window and the full player stays up
/// behind it, so a tap on that window comes back to the same channel. Closing
/// the small window from outside the app stops the watch, so the tuner is not
/// held for a picture nobody can see.
public enum PictureHandoff {
    /// VoiceOver name of the system route button.
    public static let airPlayLabel = "AirPlay"

    /// The embedded player is not a presented controller. Dismissing it when the
    /// small window starts would drop the watch, and a tap would have nothing
    /// to come back to.
    public static let dismissWhenPictureInPictureStarts = false

    /// The small window stopped. `restored` is a tap back into the player;
    /// `away` is the app not in the foreground when it closed.
    public static func stopWhenClosed(restored: Bool, away: Bool) -> Bool {
        !restored && away
    }
}
