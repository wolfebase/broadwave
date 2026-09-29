@testable import BroadwaveKit
import Testing

@Test func soundTracksAreNamedByRole() {
    let sounds = SoundTrack.roles([
        SoundTrack.Option(name: "Spanish", isDefault: false, describes: false),
        SoundTrack.Option(name: "English", isDefault: true, describes: false),
        SoundTrack.Option(name: "English (described)", isDefault: false, describes: true),
        SoundTrack.Option(name: "French", isDefault: false, describes: false),
    ])
    #expect(sounds == [
        SoundTrack(role: "main", name: "English", index: 1),
        SoundTrack(role: "language", name: "Spanish", index: 0),
        SoundTrack(role: "described", name: "English (described)", index: 2),
    ])
    #expect(SoundTrack.pick(sounds, role: nil)?.name == "English")
    #expect(SoundTrack.pick(sounds, role: "language")?.name == "Spanish")
}

@Test func aMissingRoleFallsBackToTheMainMix() {
    let sounds = SoundTrack.roles([SoundTrack.Option(name: "English", isDefault: true, describes: false), SoundTrack.Option(name: "Spanish", isDefault: false, describes: false)])
    #expect(SoundTrack.pick(sounds, role: "described")?.name == "English")
    #expect(SoundTrack.pick([], role: "language") == nil)
}

@Test func theAppAsksForAlternates() {
    #expect(Capabilities.current().alternates == true)
    #expect(Capabilities.current(alternates: false).alternates == nil)
}
