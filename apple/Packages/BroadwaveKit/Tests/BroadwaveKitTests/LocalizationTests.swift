@testable import BroadwaveKit
import Foundation
import Testing

@Test func theStringCatalogUsesEnglish() throws {
    var url = URL(fileURLWithPath: #filePath)
    for _ in 0 ..< 3 {
        url.deleteLastPathComponent()
    }
    let data = try Data(contentsOf: url.appendingPathComponent("Sources/BroadwaveKit/Localizable.xcstrings"))
    let catalog = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
    #expect(catalog["sourceLanguage"] as? String == "en")
    let strings = try #require(catalog["strings"] as? [String: Any])
    #expect(!strings.isEmpty)
    for phrase in [
        "Every day", "Weekdays", "Weekends", "Recording", "Failed", "Stopped early", "Imported",
        "Nothing in the guide right now.",
    ] {
        #expect(catalogHas(phrase, strings))
    }
    #expect(String(localized: "Every day", bundle: .module) == "Every day")
}

@Test func countsInCatalogSentencesAreNotGrouped() {
    #expect(APIClient.ScanProgress(scanning: false, found: 1234, progress: nil).line == "1234 channels found.")
    #expect(APIClient.ScanProgress(scanning: true, found: 0, progress: 40).line == "40% done.")
}

@Test func signalLinesStayTheEnglishSentences() {
    func recording(errors: Int64) -> Recording {
        Recording(
            id: 1, channelId: 1, guideNumber: "4.1", title: "News", status: "complete",
            startedAt: Date(timeIntervalSince1970: 0),
            health: RecordingHealth(continuityErrors: errors, transportErrors: 0, syncLosses: 0, packets: 8)
        )
    }
    #expect(recording(errors: 1).signalLine == "Signal broke up once")
    #expect(recording(errors: 3).signalLine == "Signal broke up 3 times")
}

@Test func demoSentencesAreInTheCatalog() throws {
    var url = URL(fileURLWithPath: #filePath)
    for _ in 0 ..< 3 {
        url.deleteLastPathComponent()
    }
    let data = try Data(contentsOf: url.appendingPathComponent("Sources/BroadwaveKit/Localizable.xcstrings"))
    let catalog = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
    let strings = try #require(catalog["strings"] as? [String: Any])
    let phrases = [
        "A Blender Foundation film, under CC BY.",
        "Demo",
        "Kept",
        "Only a group pauses every screen.",
        "Open movie",
        "Original picture",
        "Say whether to keep it.",
        "Settings need text values.",
        "That channel is not in the demo.",
        "That picture is not in the demo.",
        "That recording is not in the demo.",
        "That recording is not in the list.",
        "That screen isn't open right now.",
        "The demo films are not on this device.",
        "The demo can't do that.",
        "This recording can't play in the demo.",
        "The demo plays samples. It does not record.",
    ]
    let missing = phrases.filter { !catalogHas($0, strings) }
    #expect(missing.isEmpty)
    #expect(String(localized: "Only a group pauses every screen.", bundle: .module) == "Only a group pauses every screen.")
}

private func catalogHas(_ phrase: String, _ strings: [String: Any]) -> Bool {
    if strings[phrase] != nil {
        return true
    }
    for value in strings.values {
        guard let entry = value as? [String: Any],
              let localizations = entry["localizations"] as? [String: Any],
              let en = localizations["en"] as? [String: Any],
              let unit = en["stringUnit"] as? [String: Any],
              let text = unit["value"] as? String else { continue }
        if text == phrase {
            return true
        }
    }
    return false
}
