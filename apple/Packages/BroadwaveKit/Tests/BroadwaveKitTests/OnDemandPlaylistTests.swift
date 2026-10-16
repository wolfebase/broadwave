@testable import BroadwaveKit
import Foundation
import Testing

private let base = URL(string: "http://tv.local:8477/media/file/9/index.m3u8")!

private let growing = """
#EXTM3U
#EXT-X-VERSION:6
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PLAYLIST-TYPE:EVENT
#EXT-X-INDEPENDENT-SEGMENTS
#EXTINF:2.002000,
#EXT-X-PROGRAM-DATE-TIME:2026-10-15T21:32:00.000Z
seg00000.ts
#EXTINF:2.002000,
#EXT-X-PROGRAM-DATE-TIME:2026-10-15T21:32:02.002Z
seg00001.ts
"""

@Test func aRecordingPlaysWithoutAirDates() {
    let out = OnDemandPlaylist.rewrite(growing, base: base, length: nil)
    #expect(!out.text.contains("PROGRAM-DATE-TIME"))
    #expect(out.text.contains("#EXT-X-PLAYLIST-TYPE:EVENT"))
    #expect(out.text.contains("http://tv.local:8477/media/file/9/seg00001.ts"))
    #expect(!out.text.contains("#EXT-X-ENDLIST"))
    #expect(!out.ended)
}

@Test func aKnownLengthIsListedWholeAsOnDemand() {
    let out = OnDemandPlaylist.rewrite(growing, base: base, length: 9)
    #expect(out.text.contains("#EXT-X-PLAYLIST-TYPE:VOD"))
    #expect(!out.text.contains("EVENT"))
    #expect(out.text.hasSuffix("#EXT-X-ENDLIST\n"))
    // 4.004 s written, so 2.002 + 2.002 + 0.992 more are waited for, numbered on from
    // the last, each at the encode's own length and marked with where it starts.
    #expect(out.text.contains("bwvodwait://tv.local:8477/media/file/9/seg00002.ts?t=4.004"))
    #expect(out.text.contains("bwvodwait://tv.local:8477/media/file/9/seg00004.ts?t=8.008"))
    #expect(!out.text.contains("seg00005"))
    #expect(out.text.contains("#EXTINF:0.992,"))
    let total = out.text.split(separator: "\n").filter { $0.hasPrefix("#EXTINF:") }
        .compactMap { Double($0.dropFirst(8).split(separator: ",")[0]) }.reduce(0, +)
    #expect(abs(total - 9) < 0.01)
}

@Test func aFinishedEncodeIsOnDemandAsIs() {
    let done = growing + "\n#EXT-X-ENDLIST\n"
    let out = OnDemandPlaylist.rewrite(done, base: base, length: 120)
    #expect(out.ended)
    #expect(!out.text.contains("bwvodwait"))
    #expect(out.text.contains("#EXT-X-PLAYLIST-TYPE:VOD"))
    #expect(out.text.components(separatedBy: "#EXT-X-ENDLIST").count == 2)
}

@Test func aResumeKeepsItsGapsAndStart() {
    let resumed = """
    #EXTM3U
    #EXT-X-VERSION:8
    #EXT-X-TARGETDURATION:2
    #EXT-X-PLAYLIST-TYPE:EVENT
    #EXT-X-START:TIME-OFFSET=600.000,PRECISE=YES
    #EXTINF:2.000,
    #EXT-X-GAP
    gap00000.ts
    #EXTINF:2.000,
    seg00000.ts
    """
    let out = OnDemandPlaylist.rewrite(resumed, base: base, length: 8)
    #expect(out.text.contains("#EXT-X-START:TIME-OFFSET=600.000,PRECISE=YES"))
    #expect(out.text.contains("#EXT-X-GAP"))
    #expect(out.text.contains("http://tv.local:8477/media/file/9/gap00000.ts"))
    #expect(out.text.contains("bwvodwait://tv.local:8477/media/file/9/seg00001.ts"))
    #expect(out.text.contains("bwvodwait://tv.local:8477/media/file/9/seg00002.ts"))
    #expect(!out.text.contains("seg00003"))
}

@Test func onDemandAddressesRoundTrip() {
    let player = OnDemandPlaylist.playerURL(base)
    #expect(player.scheme == "bwvod")
    #expect(OnDemandPlaylist.realURL(player, scheme: "http") == base)
    #expect(OnDemandPlaylist.segmentIndex("seg00042.ts") == 42)
    #expect(OnDemandPlaylist.segmentIndex("http://x/media/file/9/seg00007.ts?mk=1") == 7)
    #expect(OnDemandPlaylist.segmentIndex("gap00000.ts") == nil)
    let wait = OnDemandPlaylist.waitURL(for: "seg00003.ts", base: base, at: 6.5)
    #expect(OnDemandPlaylist.waitStart(wait) == 6.5)
    #expect(OnDemandPlaylist.segmentIndex(wait.absoluteString) == 3)
}
