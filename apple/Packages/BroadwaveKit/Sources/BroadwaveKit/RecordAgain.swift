import Foundation

/// What Record it again says, the same words as the web library.
public enum RecordAgain: Sendable {
    /// The recording and its next airing have no name to store on the pass.
    public struct MissingTitle: Error, Sendable {}

    public static let label = String(localized: "Record it again", bundle: .module)
    public static let noAiring = String(localized: "The guide has no other airing yet.", bundle: .module)
    public static let failed = String(localized: "Could not schedule it. Try again.", bundle: .module)

    /// "Records again Thu 7:00 PM.", with the date ("Thu, Oct 15") past six days out.
    public static func scheduled(_ start: Date, now: Date = Date(), locale: Locale = .current, timeZone: TimeZone = .current) -> String {
        // One format puts "at" between a date and a clock. The web joins them
        // with a space.
        let far = start.timeIntervalSince(now) > 6 * 86400
        var day = Date.FormatStyle.dateTime.weekday(.abbreviated)
        if far {
            day = day.month(.abbreviated).day()
        }
        var clock = Date.FormatStyle.dateTime.hour().minute()
        day.locale = locale
        day.timeZone = timeZone
        clock.locale = locale
        clock.timeZone = timeZone
        let dayText = start.formatted(day)
        let clockText = start.formatted(clock)
        return String(localized: "Records again \(dayText) \(clockText).", bundle: .module)
    }

    /// The name stored on the once-pass. A blank show title uses the airing's
    /// name, then the episode name. Empty when none of those is set.
    public static func passTitle(recording rec: Recording, airing: Airing) -> String {
        for name in [rec.title, airing.title, rec.subtitle ?? "", airing.subtitle ?? ""] {
            let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
            if !trimmed.isEmpty {
                return trimmed
            }
        }
        return ""
    }

    /// Offered on a finished recording that lost enough signal and names its
    /// episode (a program id or an episode name), which is how the server finds
    /// the next airing. A file that is already gone can still be recorded again.
    public static func offered(_ rec: Recording) -> Bool {
        let named = !(rec.programId ?? "").isEmpty || !(rec.subtitle ?? "").isEmpty
        return rec.isDamaged && !rec.isRecording && named
    }

    struct Answer: Decodable {
        var airing: Airing?
    }
}
