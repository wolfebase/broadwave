@testable import BroadwaveKit
import Foundation
import Testing

@Test func channelUpIsThePreviousChannel() {
    #expect(ChannelStep.offset(up: true) == -1)
    #expect(ChannelStep.offset(up: false) == 1)
}

@Test func startOverNeedsTheShowStart() {
    let start = Date(timeIntervalSince1970: 1_000_000)
    #expect(StartOver.choice(showStart: nil, seekableFrom: start, recordingStarted: start) == nil)
    #expect(StartOver.choice(showStart: start, seekableFrom: start.addingTimeInterval(30), recordingStarted: nil) == nil)
    #expect(StartOver.choice(showStart: start, seekableFrom: start, recordingStarted: nil) == .live)
    #expect(StartOver.choice(showStart: start, seekableFrom: start.addingTimeInterval(-20), recordingStarted: start) == .live)
    #expect(StartOver.choice(showStart: start, seekableFrom: start.addingTimeInterval(5), recordingStarted: start) == .recording)
    #expect(StartOver.choice(showStart: start, seekableFrom: nil, recordingStarted: start.addingTimeInterval(-60)) == .recording)
    #expect(StartOver.choice(showStart: start, seekableFrom: nil, recordingStarted: start.addingTimeInterval(1)) == nil)
}

@Test func anEarlierShowingDoesNotHoldThisStart() {
    let start = Date(timeIntervalSince1970: 1_000_000)
    let airing = Airing(id: 1, channelId: 4, title: "News", programId: "news-1", start: start, end: start.addingTimeInterval(1800))
    let yesterday = Recording(
        id: 1, channelId: 4, guideNumber: "4.1", title: "News", programId: "news-0", status: "complete",
        startedAt: start.addingTimeInterval(-24 * 60 * 60), endedAt: start.addingTimeInterval(-23 * 60 * 60)
    )
    let late = Recording(
        id: 2, channelId: 4, guideNumber: "4.1", title: "News", programId: "news-1", status: "recording",
        startedAt: start.addingTimeInterval(120)
    )
    let held = Recording(
        id: 3, channelId: 4, guideNumber: "4.1", title: "News", programId: "news-1", status: "recording",
        startedAt: start.addingTimeInterval(-60)
    )
    let other = Recording(
        id: 4, channelId: 9, guideNumber: "5.1", title: "News", status: "recording", startedAt: start
    )
    #expect(ShowRecording.holdingStart(of: airing, in: [yesterday, late, other]) == nil)
    #expect(ShowRecording.holdingStart(of: airing, in: [yesterday, late, held, other])?.id == 3)
}

@Test func episodeLinePrefersTheGuideLabel() {
    #expect(ProgramLine.episode(label: "Chapter 2", season: 1, episode: 4, subtitle: "A subtitle") == "Chapter 2")
    #expect(ProgramLine.episode(label: "  ", season: 3, episode: 12, subtitle: "Local headlines") == "Season 3, episode 12")
    #expect(ProgramLine.episode(label: nil, season: 0, episode: 0, subtitle: "Local headlines") == "Local headlines")
    #expect(ProgramLine.episode(label: nil, season: nil, episode: nil, subtitle: " ") == nil)
}

@Test func seekWindowStartFollowsTheFrameOnScreen() {
    let now = Date(timeIntervalSince1970: 5000)
    let start = SeekWindow.start(current: now, currentSeconds: 40, earliestSeconds: 10)
    #expect(start == now.addingTimeInterval(-30))
    #expect(SeekWindow.start(current: now, currentSeconds: .nan, earliestSeconds: 0) == nil)
}

@Test func streamFactsLeaveABlankPieceWaiting() {
    let waiting = StreamFacts.make(rendition: nil, encoder: " ", bitrate: nil, dropped: -3, sync: .init())
    #expect(waiting.rendition == "Waiting")
    #expect(waiting.encoder == "Waiting")
    #expect(waiting.bitrate == "Waiting")
    #expect(waiting.dropped == "0")
    #expect(waiting.drift == "Off")

    let live = StreamFacts.make(rendition: "720.aac2", encoder: "copy", bitrate: "4.5M", dropped: 8, sync: .init(milliseconds: -12.4, state: "locked"))
    #expect(live.rendition == "720.aac2")
    #expect(live.encoder == "copy")
    #expect(live.bitrate == "4.5 Mb/s")
    #expect(live.dropped == "8")
    #expect(live.drift == "Locked · -12 ms")
    #expect(StreamFacts.make(rendition: "720", encoder: "libx264", bitrate: "128k", dropped: 0, sync: .init(milliseconds: 0, state: "off")).drift == "Off")
}
