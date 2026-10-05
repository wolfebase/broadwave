@testable import BroadwaveKit
import Testing

@Test func aScreenIsNotToldAboutItself() {
    #expect(AppStore.namesScreen("New iPad found: Den iPad.", "Den iPad"))
    #expect(AppStore.namesScreen("New iPad found: Den iPad.", " Den iPad "))
    #expect(!AppStore.namesScreen("New iPad found: Den iPad.", "Kitchen iPad"))
    #expect(!AppStore.namesScreen("New iPad found: My Den iPad.", "Den iPad"))
    #expect(!AppStore.namesScreen("New tuner found: Den iPad. Add it?", "Den iPad"))
    #expect(!AppStore.namesScreen("New iPad found: Den iPad.", ""))
}

@MainActor @Test func theNoticeNamingThisScreenIsDropped() {
    let store = AppStore()
    store.screenName = "Den iPad"
    store.noteHome("New iPad found: Den iPad.")
    #expect(store.homeNotice == nil)
    store.noteHome("New Apple TV found: Living Room.")
    #expect(store.homeNotice == "New Apple TV found: Living Room.")
}
