import Foundation

/// What Record it again says, the same words as the web library.
public enum RecordAgain: Sendable {
    public static let label = "Record it again"
    public static let noAiring = "The guide has no other airing yet."
    public static let failed = "Could not schedule it. Try again."

    /// "Records again Thu 7:00 PM.", with the date ("Thu, Oct 15") past six days out.
    public static func scheduled(_ start: Date, now: Date = Date(), locale: Locale = .current, timeZone: TimeZone = .current) -> String {
        var style = Date.FormatStyle.dateTime.weekday(.abbreviated).hour().minute()
        if start.timeIntervalSince(now) > 6 * 86400 {
            style = style.month(.abbreviated).day()
        }
        style.locale = locale
        style.timeZone = timeZone
        return "Records again \(start.formatted(style))."
    }

    /// Offered on a finished recording that lost enough signal, still has its
    /// file, and names its episode (a program id or an episode name), which is
    /// how the server finds the next airing.
    public static func offered(_ rec: Recording) -> Bool {
        let named = !(rec.programId ?? "").isEmpty || !(rec.subtitle ?? "").isEmpty
        return rec.isDamaged && !rec.isRecording && !rec.isMissing && named
    }

    struct Answer: Decodable {
        var airing: Airing?
    }
}
