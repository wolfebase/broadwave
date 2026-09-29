@testable import BroadwaveKit
import Foundation
import Testing

@Test func liveDelayDefaultsToBalancedAndKeepsAPick() {
    let defaults = UserDefaults.standard
    let before = defaults.string(forKey: LiveDelay.key)
    defer { defaults.set(before, forKey: LiveDelay.key) }
    defaults.removeObject(forKey: LiveDelay.key)
    #expect(LiveDelay.saved == .balanced)
    // Lowest is out of an Apple screen's reach; a stored one reads as Balanced.
    defaults.set("lowest", forKey: LiveDelay.key)
    #expect(LiveDelay.saved == .balanced)
    LiveDelay.saved = .stable
    #expect(LiveDelay.saved == .stable)
    #expect(LiveDelay.allCases.map(\.title) == ["Balanced", "Stable"])
}
