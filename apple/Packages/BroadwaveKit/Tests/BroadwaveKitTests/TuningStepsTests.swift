@testable import BroadwaveKit
import Foundation
import Testing

@Test func tuningStepsFollowTheWebPlayer() {
    #expect(TuningSteps.text(after: 0) == "Tuning the antenna")
    #expect(TuningSteps.text(after: 2.9) == "Tuning the antenna")
    #expect(TuningSteps.text(after: 3) == "Starting the picture")
    #expect(TuningSteps.text(after: 8) == "Lining up with live")
    #expect(TuningSteps.text(after: 30) == "Still tuning. A weak signal can take longer")
    #expect(TuningSteps.text(after: -1) == "Tuning the antenna")
}
