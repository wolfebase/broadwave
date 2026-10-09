@testable import BroadwaveKit
import Foundation
import Testing

private func rec(
    _ id: Int64, title: String = "Mystery Hour", subtitle: String? = nil, status: String = "complete", day: Double = 0,
    season: Int? = nil, episode: Int? = nil, missing: Bool? = nil, intro: (Double?, Double?) = (nil, nil)
) -> Recording {
    Recording(
        id: id, channelId: 1, guideNumber: "4.1", title: title, subtitle: subtitle, status: status,
        startedAt: Date(timeIntervalSince1970: 1_790_000_000 + day * 86400), missing: missing,
        season: season, episode: episode, introStart: intro.0, introEnd: intro.1
    )
}

@Test func theNextEpisodeIsTheShowsNextInAirOrder() {
    let list = [
        rec(1, day: 5, season: 1, episode: 2),
        rec(2, day: 1, season: 2, episode: 1),
        rec(3, day: 9, season: 1, episode: 1),
        rec(4, title: "Another Show", season: 1, episode: 3),
        rec(5, title: " mystery hour", season: 1, episode: 4),
    ]
    #expect(Library.nextEpisode(after: list[2], in: list)?.id == 1)
    #expect(Library.nextEpisode(after: list[0], in: list)?.id == 5)
    #expect(Library.nextEpisode(after: list[4], in: list)?.id == 2)
    #expect(Library.nextEpisode(after: list[1], in: list) == nil)
}

@Test func theNextEpisodeSkipsOnesStillRecordingOrMissing() {
    let list = [
        rec(1, season: 1, episode: 1),
        rec(2, status: "recording", season: 1, episode: 2),
        rec(3, season: 1, episode: 3, missing: true),
        rec(4, season: 1, episode: 4),
    ]
    #expect(Library.nextEpisode(after: list[0], in: list)?.id == 4)
}

@Test func episodesWithoutNumbersFollowTheDayTheyAired() {
    let list = [rec(1, day: 2), rec(2, day: 0), rec(3, day: 1)]
    #expect(Library.nextEpisode(after: list[1], in: list)?.id == 3)
    #expect(Library.nextEpisode(after: list[2], in: list)?.id == 1)
    #expect(Library.nextEpisode(after: list[0], in: list) == nil)
}

@Test func theNextEpisodeWorksWhenTheCurrentOneIsNotInTheList() {
    let current = rec(9, season: 1, episode: 2)
    #expect(Library.nextEpisode(after: current, in: [rec(1, season: 1, episode: 1), rec(2, season: 1, episode: 3)])?.id == 2)
    #expect(Library.nextEpisode(after: current, in: []) == nil)
}

@Test func skipIntroShowsInsideTheIntroButNotItsLastSecond() {
    let one = rec(1, intro: (30, 90))
    #expect(one.intro == 30 ... 90)
    #expect(!one.showsSkipIntro(at: 29.9))
    #expect(one.showsSkipIntro(at: 30))
    #expect(one.showsSkipIntro(at: 88.9))
    #expect(!one.showsSkipIntro(at: 89))
    #expect(!one.showsSkipIntro(at: 120))
    #expect(rec(1, intro: (30, nil)).intro == nil)
    #expect(rec(1, intro: (90, 30)).intro == nil)
    #expect(!rec(1, intro: (90, 30)).showsSkipIntro(at: 50))
}

@Test func anIntroAtTheStartOfTheFileComesAsItsEndOnly() {
    let first = rec(1, intro: (nil, 45))
    #expect(first.intro == 0 ... 45)
    #expect(first.showsSkipIntro(at: 0))
    #expect(first.showsSkipIntro(at: 43.9))
    #expect(!first.showsSkipIntro(at: 44))
    #expect(rec(1, intro: (nil, 0)).intro == nil)
    #expect(rec(1, intro: (nil, nil)).intro == nil)
}

@Test func upNextNamesTheEpisodeByTagThenNameThenShow() {
    #expect(rec(1, subtitle: "The Pilot", season: 2, episode: 5).upNextLabel == "S2 E5")
    #expect(rec(1, subtitle: "The Pilot").upNextLabel == "The Pilot")
    #expect(rec(1).upNextLabel == "Mystery Hour")
}

@Test func upNextShowsAtTheCreditsOrTenSecondsBeforeTheEnd() {
    #expect(UpNext.cardTime(duration: 3600, creditsStart: 3540) == 3540)
    #expect(UpNext.cardTime(duration: 3600, creditsStart: nil) == 3590)
    #expect(UpNext.cardTime(duration: 3600, creditsStart: 0) == 3590)
    #expect(UpNext.cardTime(duration: 3600, creditsStart: 3600) == 3590)
    #expect(UpNext.cardTime(duration: 6, creditsStart: nil) == 0)
    #expect(UpNext.cardTime(duration: 0, creditsStart: 10) == nil)
    #expect(UpNext.cardTime(duration: .nan, creditsStart: 10) == nil)
}

/// The steps an UpNext takes at each playhead time, in order.
private func steps(_ next: inout UpNext, _ times: [Double], credits: Double? = nil, hasNext: Bool = true) -> [UpNext.Step] {
    times.map { next.observe($0, duration: 3600, creditsStart: credits, hasNext: hasNext) }
}

@Test func upNextDoesNotSkipWhenTheFirstLookIsPastTheCountdown() {
    var next = UpNext(autoplay: true)
    let landed = next.observe(3560, duration: 3600, creditsStart: 3540, hasNext: true)
    let later = next.observe(3570, duration: 3600, creditsStart: 3540, hasNext: true)
    #expect(landed == .card(left: nil))
    #expect(later == .card(left: nil))
    #expect(!next.done)
    let atEnd = next.ended(hasNext: true)
    #expect(atEnd)
}

@Test func upNextCountsDownTenSecondsThenPlays() {
    var next = UpNext(autoplay: true)
    let got = steps(&next, [3539, 3545, 3548.2, 3554.9, 3555, 3556], credits: 3540)
    #expect(got == [.none, .card(left: 10), .card(left: 7), .card(left: 1), .play, .none])
    #expect(next.done)
    let atEnd = next.ended(hasNext: true)
    #expect(!atEnd)
}

@Test func upNextStartsItsCountdownAgainAfterASeekBack() {
    var next = UpNext(autoplay: true)
    let got = steps(&next, [3590, 3594, 1000, 3591])
    #expect(got == [.card(left: 10), .card(left: 6), .none, .card(left: 10)])
}

@Test func upNextWithAutoplayOffShowsTheCardAndNeverPlays() {
    var next = UpNext(autoplay: false)
    let got = steps(&next, [3589, 3595, 3700])
    #expect(got == [.none, .card(left: nil), .card(left: nil)])
    let atEnd = next.ended(hasNext: true)
    #expect(!atEnd)
}

@Test func notNowHidesTheCardAndPlaysNothingAtTheEnd() {
    var next = UpNext(autoplay: true)
    let before = steps(&next, [3592])
    next.dismiss()
    let after = steps(&next, [3599, 3700])
    let atEnd = next.ended(hasNext: true)
    #expect(before == [.card(left: 10)])
    #expect(after == [.none, .none])
    #expect(next.dismissed)
    #expect(!atEnd)
}

@Test func upNextWithoutANextEpisodeShowsNothing() {
    var next = UpNext(autoplay: true)
    let got = steps(&next, [3545, 3599], credits: 3540, hasNext: false)
    let atEnd = next.ended(hasNext: false)
    #expect(got == [.none, .none])
    #expect(!atEnd)
}

@Test func theEndPlaysTheNextEpisodeOnce() {
    var next = UpNext(autoplay: true)
    let first = next.ended(hasNext: true)
    let second = next.ended(hasNext: true)
    #expect(first)
    #expect(!second)
    var now = UpNext(autoplay: true)
    now.played()
    let got = steps(&now, [3595])
    let atEnd = now.ended(hasNext: true)
    #expect(got == [.none])
    #expect(!atEnd)
}
