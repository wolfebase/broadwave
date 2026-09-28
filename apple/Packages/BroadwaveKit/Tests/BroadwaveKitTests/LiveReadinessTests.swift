import BroadwaveKit
import Testing

private func playlist(segments: Int, seconds: Double = 0.5, target: Int = 2, hold: Double? = 6) -> String {
    var lines = ["#EXTM3U", "#EXT-X-VERSION:9", "#EXT-X-TARGETDURATION:\(target)"]
    if let hold {
        lines.append("#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,HOLD-BACK=\(hold),PART-HOLD-BACK=1.500,CAN-SKIP-UNTIL=12.000")
    }
    for i in 0 ..< segments {
        lines.append("#EXTINF:\(seconds),")
        lines.append("seg\(i).m4s")
    }
    return lines.joined(separator: "\n")
}

@Test func aFreshTuneIsNotReadyUntilItHoldsTheHoldBack() {
    // 6 s hold-back and a 2 s target: 8 s of media before AVPlayer can start.
    #expect(!LiveReadiness.ready(playlist(segments: 4)))
    #expect(!LiveReadiness.ready(playlist(segments: 15)))
    #expect(LiveReadiness.ready(playlist(segments: 16)))
}

@Test func partHoldBackIsNotTheHoldBack() {
    // Reading PART-HOLD-BACK=1.5 as the hold-back would call 4 s ready.
    #expect(!LiveReadiness.ready(playlist(segments: 8)))
}

@Test func aPlaylistWithoutAHoldBackUsesThreeTargets() {
    #expect(!LiveReadiness.ready(playlist(segments: 15, hold: nil)))
    #expect(LiveReadiness.ready(playlist(segments: 16, hold: nil)))
}

@Test func somethingThatIsNotAPlaylistIsNotReady() {
    #expect(!LiveReadiness.ready(""))
    #expect(!LiveReadiness.ready("<html>Bad gateway</html>"))
}
