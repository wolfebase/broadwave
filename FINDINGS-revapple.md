# Findings — revapple

App-target issues and `SyncEngine.swift`. No Xcode build was run. Line numbers are from this worktree. Kit bugs that `swift test` can prove are fixed on this branch instead of listed here.

## Playback

### A recording that fails while loading is treated as playing

`apple/App/Shared/PlayerScreen.swift` around 2427–2438.

`AVPlayerItem.status` is still `.unknown` right after `replaceCurrentItem`. The loop retries only when status is already `.failed`, then calls `play()` and returns. A first play (`position` ≤ 5) never waits, and nothing observes `status` or `failedToPlayToEndTime` after that. The screen stays on a dead item with `error` still nil.

Fix: after `replaceCurrentItem`, wait until status is `.readyToPlay` or `.failed` (same 8 s budget as `reach`). Retry once on `.failed`. If the second item fails, set `error` and do not call `play()`.

### Unplugging headphones starts the phone speaker

`apple/App/Shared/PlayerScreen.swift` 786–808 and 815–817. `apple/Packages/BroadwaveKit/Sources/BroadwaveKit/SyncEngine.swift` 208–210 and 379–382. Do not edit `SyncEngine.swift` in this lane.

The route observer only records `routeLost`. For two seconds `pausedItself` stays false, which matches the comment at lines 87–89: that pause is the viewer's, because playing on would move the sound to the speaker. `SyncEngine` never reads `routeLost`. `pausedByCall` is set only for `.setRateCalled`. The next 250 ms tick calls `shouldKeepPlaying`, which is true whenever the room rate is non-zero and this player is paused or at rate 0, and then calls `play()`. Whole-Home Sync defaults on. iOS live chrome has no Play/Pause, so the viewer cannot choose.

Fix, in `SyncEngine` (reviewer-owned): a route loss must count as the viewer pausing, including when the buffer is under 1 s, so `shouldKeepPlaying` is false and `viewerPaused` is true. Call that from the route observer. Extend the existing `shouldKeepPlaying` / `viewerPaused` tests.

### Background and audio interruptions stay paused, and a failed resume retunes

`PlayerScreen.swift` 801–803. No `AVAudioSession.interruptionNotification` under `apple/`.

`.appBackgrounded` and `.audioSessionInterrupted` are excluded from `pausedItself`, so both look like a viewer pause. `noteFrozen` will not reload. `watchLifecycle` only logs, and only when `BroadwaveSyncLog` is set. Nothing calls `play()` on `didBecomeActive` or on interruption end with `shouldResume`. With sync off, the last frame stays up and there is no Play control. With sync on, `shouldKeepPlaying` calls `play()` during the interruption. If that posts `.setRateFailed` while rate stays 0, `pausedItself` becomes true and `noteFrozen` reloads about a second later, in the middle of the interruption.

Fix: observe `AVAudioSession.interruptionNotification`. While interrupted, do not count `stoppedItself` and do not let the engine call `play()`. On `.ended` with `shouldResume`, call `play()` once. The “do not resume while interrupted” predicate can sit next to `shouldKeepPlaying`. The session observer stays in the app target.

### Start over seeks even when sync did not let go

`PlayerScreen.swift` 329–346.

`jump` pauses, then waits at most 3 s for `sync == nil || detached`, then seeks anyway. The comment above says a seek before detach is undone. Detach needs a second of buffer, a second since the hold, and 3 s since the viewer became active (`SyncEngine.viewerPaused`). Right after foreground, or with under a second buffered, the room seek puts the picture back on live. If the seek’s `finished` is false, `play()` is skipped and the `pause()` already counted as `.setRateCalled`, so the engine can detach and leave the frame where it was.

Fix: seek only after `sync == nil || detached`. If `finished` is false, call `play()`.

### Skip break removes itself before the seek works

`PlayerScreen.swift` around 2686–2693.

`passed` and `inBreak` are cleared before `seek` returns. `followBreaks` then drops that marker, so the button and auto-skip stay off until the playhead leaves the break. A seek that does not finish leaves the commercial playing and the button gone. `skipIntro` does not do this: the next tick sets the intro from the playhead again.

Fix: set `passed` and clear `inBreak` only when `seek` returns true.

### The sound observer keeps the previous item alive

`PlayerScreen.swift` 599–617. `stop()` removes the token at 440–442. `reload()` does not.

The notification block captures `item` strongly, and NotificationCenter holds the block. `loadSounds` removes the previous token only after a group with two or more options loads. If that load throws, or the new item has one track, the guard returns and the previous `AVPlayerItem` stays until `stop()`.

Fix: remove and nil `soundObserver` at the start of `reload()` and at the start of `loadSounds`, including the early return.

### Rate and route observers are never removed

`PlayerScreen.swift` 784–808. `stop()` at 426–491 removes sound, lifecycle, tick, stall, and end observers. It does not remove `rateObserver` or `routeObserver`, and it does not reset `pausedItself`. There is no `deinit`.

`watchRate` adds each observer once (`guard rateObserver == nil`). `LivePlayer` is the single `NowPlaying.live` (`RootView.swift` 31), and both blocks use `[weak self]`, so they do not stack on the one instance. `routeObserver` is global (`object: nil`). A second `LivePlayer` would leak that global observer. A rate change after `stop()` can set `pausedItself`, so the next tune treats a system pause as a stopped picture.

Fix: remove both observers in `stop()` and in `deinit`, and set `pausedItself = false` in `stop()`.

### SyncEngine rate observer warning

`SyncEngine.swift` 249. Do not edit this file here.

`AVPlayer.rateDidChangeReasonKey` is read in the notification closure before `MainActor.assumeIsolated`. `swift test` warns that the main-actor-isolated key is used from a `Sendable` closure. Move the read inside `assumeIsolated`, matching `PlayerScreen.swift` 798–800.

## Leaving a server

### Forget leaves the previous channel playing on the next server

`AppStore.forget()` (`AppStore.swift` 230–242) nils the socket, API, and server. It does not know about `NowPlaying`. Settings “Use a different server” and “Leave the demo” (`SettingsView.swift` 192 and 204) call `store.forget()` and do not call `nowPlaying.stop()`.

`NowPlaying` lives on `RootView` (line 234) and keeps `channel`. Tabs unmount while `!store.connected`. On the next server, tvOS presents `fullScreenCover` when `nowPlaying.channel != nil` (`RootView.swift` 555–561). `.task(id: nowPlaying.watchKey)` (565–567) then calls `playLive` on the old channel id.

`tabs.onDisappear` (568–570) is `Task { await nowPlaying.live.stop() }`. That task is unstructured. A stop that starts as the tabs go away can finish after a reconnect has already started the next watch and cancel it.

Fix: call `nowPlaying.stop()` in the same action as `store.forget()`, before the tabs disappear. Make the disappear path cancel any previous stop task, and ignore a stop whose watch token is no longer current.

`AppStore.forget` and `connect` now bump a generation and drop a different server’s lineup. A relocate that finishes after Forget does not call `connect`. That does not clear `NowPlaying`.

## Network errors that look like success

### Home says there are no channels when the server did not answer

`HomeView.swift` 140–145. The empty state is `featured() == nil`, not loading, no saved sets, no recordings. `AppStore.refresh` sets `error` on failure (line 338). This branch does not read `error`, so a failed refresh of an empty cache shows “No channels yet” / “Add a tuner or scan for channels.”

Fix: when `store.error` is set, show that message and a way to try again. Keep the current copy for a refresh that succeeded with an empty lineup.

### The sports key and the saved secrets show success before the server accepts them

`SettingsView.swift` 317–325. Submit clears `sportsKey`, then `try?` ignores a failed `saveSettings`, then sets `sportsDB = true`. The header switches to “Scores from TheSportsDB” whether or not the key was stored. The typed key is already gone, so the viewer cannot retry it.

`saveSecret` (672–681) clears the field and calls `mark(true)` before `save`. The prompt becomes “Saved”. `save` (684–691) records `saveError` on failure, but the field stays empty and the prompt stays “Saved”.

The live-scores toggle (293–300) sets `liveScores` and then `try?` the save. A failure leaves the switch where the viewer put it and the server where it was.

Fix: await the save, set the success flag only in the `do` path, and put the previous value back in `catch`. Clear the field only after success, or keep the trimmed key until then.

### Diagnostics reports a clean bill of health when the fetch failed

`DiagnosticsView.swift` 175–187 and 32–33. `doctorNotes`, `deviceHealth`, and `signals` use `try?`. Any failure becomes an empty list. `loaded = true` then shows “Nothing needs attention.” Guide depth does say “not available yet” when the optional is nil (46–48). The antenna check (`checkAntenna`) already surfaces errors.

Fix: on a thrown doctor or health request, set `note` from the error and do not treat an empty list as a clean result. Leave the “Checking…” row up until one of those calls has succeeded.

### Setup finishes even when “setup is complete” did not save

`SetupWizard.swift` 261–267. `try?` ignores a failed `saveSettings(["setupComplete": "1"])`, then `onFinish()` runs and a channel plays. The next launch can show setup again, or the server can still consider setup open.

Fix: on failure, stay on the last step, show the error, and do not call `onFinish()`.

### A failed search keeps the previous hits, and a slow search wins

`SearchView.swift` 133–147. `run` sets `result` only on success. `catch` sets `note` and leaves `result`. There is no generation token. A search that finishes after a later one overwrites the hits the viewer is looking at.

Fix: capture the query (or a token) before the await, and write `result` and `note` only when it is still current. On failure, clear `result` when this search is the current one.

### A schedule refresh hides a new failure

`ScheduleView.swift` 177–180. On `catch`, `note` is set only when `plan == nil`. A later failure keeps the stale plan and clears nothing. `loadEvents` (185–188) is `try?`, so a failed events fetch is silent.

Fix: set `note` on every failure. Keep the last plan on screen if you want, and still show the error.

### The home scan retries after the view has gone

`HomeListView.swift` 115–119. A non-cancellation error sleeps 400 ms with `try?`, which swallows cancellation, then calls `api.home` again. Leaving the screen during that sleep still hits the network. “Scan again” can overlap the retry.

Fix: `try await Task.sleep`, return on `CancellationError`, and ignore the second result if the task is cancelled.

### A failed Bonjour browse is not started again until the view is recreated

`ConnectView.swift` 114–115 starts discovery in `onAppear` and stops it in `onDisappear`. Backgrounding does not call `onDisappear`. The kit now drops the browser on `.failed`, so a later `start()` browses again. Nothing on the connect screen calls `start()` when the scene becomes `.active`, so “No server answered on this network.” (`ConnectView.swift` 98) stays until the view is recreated.

Fix: from the connect screen, call `discovery.start()` when the scene becomes `.active`.

## Accessibility

### A guide cell speaks only the title and the start time

`GuideView.swift` 655–656. `.accessibilityElement(children: .combine)` plus an explicit label replaces the children. The recording dot (626–628), the score (630–632), and “NEW” (633–635) are visible and silent. The subtitle is also dropped when it differs from the start time.

Fix: build the label from title, subtitle or start time, and, when set, “Recording”, the score, and “New”.

### A favorite channel is not announced in the guide

`GuideView.swift` 490–495 and 525. The star is `accessibilityHidden(true)`, and the button label is number, name, and “ATSC 3.0”. VoiceOver never hears that the channel is a favorite.

Fix: append “, favorite” when `channel.favorite`.

### A game card does not speak the score or whether it is on

`HomeView.swift` 446–456. The explicit label is “A at B, number name” or “title, number name”. The score on the card (431–433) and the live state are dropped. The same card is used from Sports.

Fix: include “Live” when `airing.isOn(at: now)`, the score when it is non-empty, and the start time when the game is later.

### Later games do nothing when chosen

`HomeView.swift` 66–69 and 95–98. `SportsView.swift` 82–85. The button calls `nowPlaying.play` only when `airing.isOn(at: store.now)`. A later game is focusable and does nothing. The guide’s later airings have a real action.

Fix: for a later game, show the airing (or set a recording) instead of no-op. Keep play for a game that is on now.

### The on-now row does not say that it is recording

`apple/Packages/BroadwaveUI/Sources/BroadwaveUI/Components.swift` 241–257 (`OnNowRow`). Children are combined and the recording dot (242–244) has no label, so VoiceOver does not hear “Recording”. “Next …” is in the combined children and does get spoken.

Fix: set an accessibility label that adds “Recording” when `recording` is true.

### The portrait multiview divider is drag-only

`MultiviewScreen.swift` 1273–1277. The stacked divider is a `Rectangle` labeled “Divider” with a drag gesture. VoiceOver cannot change `session.split`.

Fix: `accessibilityAdjustableAction` that steps `split` between 0.25 and 0.75, plus a hint.

## Multiview

### Choosing another tile on side-by-side or quad goes silent

`MultiviewScreen.swift` 750–758.

Equal layouts share one rendition, so a focus change does not restart the tile. `setAudible(true)` sets the flag and returns. The old tile mutes and `clearRoute()` drops the route. The new tile stays muted. The speaker badge follows focus, not `canHear`. The same hole hits when the sound tile leaves without a retune: the survivor’s `setAudible(true)` does not take audio.

A second clobber: `start` assigns `audible` and `canHear` from the request captured when the task began (around 381 and 443). A tap during `stopWatching` is overwritten, and nothing calls `applyAudible()` again.

Fix: on `setAudible(true)`, call `applyAudible()` and set `canHear` when `currentItem != nil`. After the awaits in `start`, keep the flag `setAudible` owns and call `applyAudible()`.

### Adding or removing one channel stops every tile

`MultiviewScreen.swift` 1050–1056 and the gate around 1003.

Any `together` change with two or more channels sets `planReady = false` before the plan returns. The grid is destroyed. Each tile’s `onDisappear` calls `stop()`, which leaves the room and `stopWatching`. Those stops overlap the new watches. Every picture goes black, and the new tunes can fail because the old sessions still hold tuners.

Fix: leave `planReady` true once a grid is up. Refresh blocked and offered channels in place. Do not unmount a tile that is still allowed.

### A late “why did this die” read blanks a picture that already recovered

`MultiviewScreen.swift` around 607–612 and 675–679.

`askWhyDead` applies the reading when it returns, with no check that the tile is still dead. `replayIfStuck` then honors `namedCause` and can call `showOutage`, which pauses that tile and replaces its item with nil. `namedCause` is cleared only in `start` and `reloadItem`.

Fix: apply the reading only while `TilePlayback.isDead` is still true. Set `namingCause = false` on every exit, including a token mismatch. Clear `namedCause` when the item is healthy again.

### The ready flag is written off the main actor

`MultiviewScreen.swift` 1853–1912 and the read around 448.

`FrameOnScreen` is `@unchecked Sendable` with an unsynchronized `ready` Bool. The comment says KVO writes it off the main actor. The flag object is a stable `let` on the tile (line 344), so the coordinator capturing `flag` is fine. The data race is the bug. `ready` is also written from the main actor in `start` / `stop`.

Fix: store `ready` in a lock (or hop the KVO callback to the main actor) and read it the same way. Invalidate the observation in the coordinator’s `deinit` as well as when the layer object changes. The token already invalidates on deinit today.

### The picture-in-picture “come back” observer has no owner teardown

`PictureWindow.swift` 103–128.

`comeBack` is removed in `forgetAwayWindow`, which runs from a failed start and from `didStop`. `endAwayWindow` (116–121) clears `awayWindow` and does not remove the observer. The block uses `[weak self]`, so this is not a cycle. If the delegate is released while the away window is up and `didStop` does not run, the observer stays for the process.

Fix: remove `comeBack` in `deinit` as well as in `forgetAwayWindow`.

## Second pass

Verified by reading the code. No Xcode build.

### Turning one day off makes the pass record every day

`PassesView.swift` 324–335, 351–360, 386–394, and 555–564. Server: `server/internal/dvr/plan.go` 158–160 and `server/internal/httpapi/passes.go` 178–191.

`update` runs the same closure twice. The first call edits the pass on screen. The second builds the PATCH from `Pass(id:title)`, whose `days`, `timeEnd`, and `keepCount` are nil. `updatePass` encodes only the fields that are set.

A day toggle copies `p.days`. On the blank patch that is nil, so the set starts empty. Turning Wednesday off sends `"days": []`. `onDay` treats an empty list as every day, and the response replaces the toggles. Turning a day on sends only that day.

The From picker does the same with `timeEnd`. The blank patch looks empty, so an existing end is replaced with three hours after the new start. Choosing “The newest” sees `keepCount == nil` and sends `keepCount: 5`.

`save` also assigns `passes = list` when `mine != saves` (584–592). A slower PATCH then puts the list back to that response.

Fix: diff the edited pass against the pass from before this change and send those fields. Do not run the closure on a blank `Pass`. Apply `passes` and `pass` only when `mine == saves`.

### The one-channel player never uses the picture budget

`PlayerScreen.swift` 120 and 273–280. `PlaybackOutage.startAttempts` asks for a full picture budget ten times, about two seconds apart, and a refused tuner once more. The web player does that (`useLiveStream.ts` around 422). `LivePlayer.start` has no attempt loop.

First tune, watch returns `pictures_full`. `retry` is false, so the server sentence is shown at once. The same channel again (`Try again`, a frozen retune, a prefs change) has `retry` true. `holdPictureMessage` is true for that sentence, so it becomes “The picture stopped” and the slow quiet clock. `no_signal` is the only unnamed error that stays put.

Fix: after `decision.recovery == nil`, if this burst is still under `startAttempts(code:message:)`, sleep 2 seconds and call `start` again with the burst still counting as the first tune. On the last miss, show `decision.message`. Fold into `pictureStopped` only for a real `stream_down`, not for every `retry`. Use `try await Task.sleep` and return if that task is cancelled.

### A multiview tile gives up that budget on the second try

`MultiviewScreen.swift` 363, 380, and 486–501.

The tile does count attempts, but the recursive call never reaches them. `retry` is captured before `stop()`, and a failed attempt has already set `channelID` (line 380). The next `start` takes the “same channel” branch. `pictures_full` and `tuner_refused` hit line 491 and become `pictureStopped`. A tile whose watch returns `pictures_full` nine more times should keep asking. Today the second miss is the stopped-picture message.

The sleep is `try?`, and `stop()` (808–830) does not bump `startToken`. A cancelled retry continues. If `channelID` is still set, it calls `start()`, which bumps the token and `stop()`s whatever watch replaced it. `start()` does check cancellation after that `stop()`, so the cancelled task itself does not open a new watch.

Fix: pass a burst flag into the recursive `start` and treat `retry` as false while it is set. Reset `attempts` at the start of a non-burst `start`. Sleep with `try await` and return on cancel. Bump `startToken` at the top of `stop()`.

### A channel note is wiped when its timer is cancelled, and a new channel keeps the old one

`PlayerScreen.swift` 1098–1112. The recording note at 2330–2335 checks `Task.isCancelled`. This one does not.

```swift
.task(id: note) {
    try? await Task.sleep(for: .seconds(10))
    if nowPlaying.note == note {
        nowPlaying.note = nil
    }
}
```

`try?` treats cancel as a finished sleep. Minimizing destroys `PlayerScreen` and cancels the task, but `NowPlaying` lives on `RootView`, so the note is cleared at once. The same happens when `live.reconnecting` or an error replaces that branch.

`PlayerPanels.swift` 65–67 sets `nowPlaying.channel` and does not change `note`. `play` is what clears it. Open an encrypted 3.0 station, then choose another row in Channels within those 10 seconds. The new station plays under the old sentence until the sleep ends.

Fix: `try await Task.sleep`, and clear the note only when this task is not cancelled. Set `nowPlaying.note = nil` when the channels row changes the channel.

### Leaving the player puts the broadcast frame rate back

`PlayerScreen.swift` 1813–1815 and 1963–2006. tvOS only.

`dismantleUIViewController` calls `stop()` and then sets `preferredDisplayCriteria = nil`. `stop()` cancels the refresh loop and nils `task` while `refresh` can still be inside `videoPicture` (`try?` on the loads). That call does not look at cancellation, and it always `apply`s. The apply can write the broadcast mode back onto the window after dismantle cleared it.

`stop()` also nils `task` before the loop has exited. A later `start` sees `task == nil` and starts a second loop.

Fix: set a stopped flag in `stop()`, and return before `apply` when the task is cancelled or that flag is set. Do not start a new loop until the old one has exited.

### A deep link finishes after a newer one

`RootView.swift` 391–458.

`open` starts a `Task` and does not keep it. `watch`, `multiview`, `connect`, and the tvOS `recording` link all await the network or `coverGone` (700 ms), then write `NowPlaying` or `libraryFilter.recording` with no token. `coverGone` uses `try?`, so a later cancel would not stop the write either.

Open `broadwave://watch/4` while the lineup is empty, so the task is inside `refresh()` or `allChannels()`. A second link, `broadwave://watch/9`, starts. Whichever await returns last calls `play`. The same task still calls `play` after you switch tabs or after Forget. A failed `server()` or `allChannels()` is `try?`, so a connect link or a hidden channel does nothing and shows no error.

Fix: keep one task and cancel it at the start of `open`. Capture a token. After every await, including the sleep in `coverGone`, return if the task is cancelled or the token is stale. On a failed connect or lineup fetch, show the error.

### A settings edit is saved before the server accepts it

`SettingsView.swift` 544–553, 631–669, and 684–691. The sports key and the saved secrets are already filed. The Check for updates toggle is the same `try?`: it never sets `saveError` and does not put the switch back.

`flushServerText` copies the field into `loadedUser`, `loadedLineup`, `loadedGuide`, `loadedReserve`, or `loadedBuffer` before `save`. Each `save` is a new `Task`. Change Picture from Broadcast to Film, then to Smooth before the first PUT finishes. If the Film request completes last, the picker shows Smooth and `saveError` stays empty, while the server keeps Film. A failed guide-address save has already moved `loadedGuide`, so leaving the screen does not send it again. `store.api?.saveSettings` on a nil client does not throw.

Fix: one save task, cancelled and replaced by the next change. Apply `loaded*` and clear `saveError` only after a successful save. In `catch`, put the control back. Await Check for updates the same way.

### Settings you type before the load returns are thrown away

`SettingsView.swift` 578–610.

The guide fields are editable immediately. `load` is `.task { await load() }` and applies whatever `settings()` returns, with no check that the fields are still untouched. Open Settings, type a Schedules Direct user, and submit. `flushServerText` returns immediately because `guideKnown` is still false, so the submit does nothing. The load then assigns `sdUser` and replaces what you typed. If `settings()` throws, `try?` leaves Live scores, Check for updates, and Play the next episode showing their default On, disabled, with no error.

Fix: disable the fields until a load succeeds. Ignore a finished load if the task is cancelled or the user has edited. On failure, show the error and do not present the defaults as the current settings.

### A failed support download says nothing

`SettingsView.swift` 759–765 and 710–718.

`downloadBackup` sets `backupError` when `downloadFile` returns nil. `downloadSupport` does not. The button stops spinning. On Apple TV, `present` sets `savedNote = "backup"`. A later failed backup download sets `backupError` and leaves that line up, so the section shows both the error and “Saved on this Apple TV.” `downloadSupport` does clear `savedNote` first, and still shows no error.

Fix: give the support button the same error text as the backup download. Clear `savedNote` at the start of either download and set it only after the file is written.

### A failed schedule looks like the show is not set to record

`BroadwaveWidgets.swift` 49–67. `WidgetFeed.mark` treats an empty plan as not planned.

`api.schedule()` and `api.scoreboard()` use `try?`. The channels and airings calls do not. If the schedule request fails and the others succeed, `plan` is `[]` and the row shows a Record button for a show that is already set to record. A failed scoreboard shows the game with no score.

Fix: if `schedule()` throws, do not build rows with an empty plan. Surface “Can’t reach your server.” or keep the previous timeline. Do not call `mark` with a plan that never loaded.

### An On now card does not say how much is left

`apple/Packages/BroadwaveUI/Sources/BroadwaveUI/Components.swift` 177–206, used from `HomeView.swift`.

`minutesLeft` and `AiringProgress` are on the card. `.accessibilityLabel(Self.spoken(...))` replaces the children. `spoken` is the channel number, name, ATSC 3.0, and title. The time left and the percent aired are visible and silent. This is not the On now row already filed.

Fix: add the minutes-left string, and the percent aired when there is an airing, to `spoken`.

### Every iPhone is named “iPhone”

`ScreenIdentity.swift` 5–7. No entitlement file under `apple/` contains `com.apple.developer.device-information.user-assigned-device-name`. `RootView.swift` sends this name with `announce`.

On current iOS, `UIDevice.current.name` is a generic name (“iPhone”, “iPad”) without that entitlement. The home lists every phone as “iPhone”, and the arrival line is “New iPhone found: iPhone.” `AppStore.namesScreen` then hides any notice ending in `found: iPhone.` on every phone, because they all have that name.

Fix: add the entitlement if the home list should show the name from Settings. Until then, do not use the generic name as the only label for that screen.

### Search starts live TV for a listing that is not on

`SearchView.swift` 52–60 and 116–121. The guide sheet shows Watch only when the airing is on (`GuideView.swift` 791). Search shows Watch for every hit and `watch` always calls `nowPlaying.play`. A later listing tunes to whatever is on that channel now. The button label is only “Watch”, so the title and time beside it are not the focused control.

Recording hits (68–81) are an `HStack` with no button. The search stack has no recording destination. Select does nothing. The web row opens the recording.

Fix: Watch only when `airing.isOn(at: store.now)`. Otherwise open the airing, as the guide does. Put the title and start time in those button labels. Play a recording hit the same way `RecordingsView` does.

### A source error is only “Offline.”

`SourcesView.swift` 143–155 and 259–269. The web row (`web/src/features/settings/Sources.tsx` 211–214) says Offline and then the health sentence when it is not a URL.

`isOffline` is “`health` is non-empty”. The health sentence is never drawn. `healthWord`’s “Needs attention” branch sits inside `if !offline`, so it never runs. A disabled source with an empty health string shows “Off” in `Tokens.ColorToken.success`.

Fix: show the health string when it is non-empty and not a URL. Don’t use the success color for “Off”.

## Third pass

### Tonight and the day buttons open the start of the guide

`GuideView.swift` 403–407 and 459–463. The now anchor is a real layout width (`389–394`) because `scrollTo` ignores `offset`. Each hour marker is a 1×1 view at the leading edge, then `.offset`. Tap Tonight, Tomorrow, or a weekday. `jump` becomes that time and `scrollTo(hour)` runs. Every marker’s layout frame is still the origin, so the grid jumps to the start. Now does the same: it sets `jump`, not the `"guide-now"` id.

Fix: give each hour the same layout trick as `"guide-now"` (width `hour * 60 * perMinute`, id on the trailing point).

### The guide slides when the clock crosses a half hour

`Guide.swift` `guideOrigin` 224–231 floors to the half hour and steps back 30 minutes. `GuideView.swift` `origin` / `x` at 345–351 place every program and the now line from that date. The scroll offset (`450–453`) is not rebased, and nothing watches `origin`.

Leave the guide on a show. When `store.now` crosses :00 or :30, or the first tick after a long background, origin jumps by one or more half hours. The rows move left by `30 * perMinute` points per half hour and the time at the left edge moves with them. The now pill hides once it is left of the viewport.

Fix: keep one origin for the life of the grid, or on an origin change scroll by the pixel delta so the same instant stays at the leading edge.

### A failed scoreboard looks like a night with no scores

`GuideView.swift` 72–82. `try?` turns a failed or cancelled `scoreboard()` into `[]`, and that is stored. Cells show no score line. The task has no id, so it does not run again when the app becomes active.

Fix: `task(id: store.api?.base)`. On failure or cancel, leave `scores` as it was. Refresh when the scene becomes active.

### A slow pass reload puts Record back, and live Record hides the error

`GuideView.swift` 863 and 870–879, and the live button at 812–818. `refreshPasses` now ignores a list from a server you left. It still applies an older list from this server. Open an upcoming program (passes start empty, so the button says Record). Tap Record before the GET returns. `recordOnce` shows “Don’t record”. The GET then finishes with the list from before the tap and the button says Record again. `act` starts a new task per tap and writes `problem` with no token, so an older failure can replace a newer success.

The live Record button calls `toggleRecord`, which stores the failure on `store.error`. Nothing on the guide reads `store.error`. The button stays “Record”.

Fix: a pass epoch bumped by record and remove, checked before `refreshPasses` assigns. One `act` task, and drop a result that is not the latest. Route the live button through `act`.

### The portrait mini-guide does not say which channel is on

`PlayerScreen.swift` 1508–1519. The current row draws a checkmark and hides it from VoiceOver. Every row’s label is the number, name, and title.

Fix: when the row is the channel that is playing, add “, Playing” to its label.

### Home says “Recording” on the button that stops it

`HomeView.swift` 287–289. If the channel is already recording, the hero button title is “Recording” and the action is `toggleRecord`, which stops it. The portrait player uses the same word and then sets the label to “Stop recording” (`PlayerScreen.swift` 1557–1563). The hero does not.

Fix: keep the visible title, and set `.accessibilityLabel` to “Stop recording” while a recording is active.

### Start over can open a recording after you changed the channel

`PlayerScreen.swift` `beginStartOver` 1253–1263. The recording path is an unstructured task. `live.stop()` clears the session and then waits on `stopWatching`. Nothing after that wait checks the channel or whether the player is still up. Change the channel while the stop is in flight. The task still sets `startOverRecording`, and the cover opens that recording.

Fix: capture the channel id and a token before the wait. Set `startOverRecording` only if both still match.

### The sync pill says “Synced” while the rooms are still catching up

`PlayerScreen.swift` 1665–1678. The pill is shown for every state except `.off`, including waiting and syncing. The green tint is only when `locked`. The accessibility label is always “Synced” or “Synced with N screens”.

Fix: label waiting and syncing as “Syncing”, and “Synced” only when `locked`.

### A multiview tile’s Record stops a recording it does not name

`MultiviewScreen.swift` 1346–1348. The menu always says Record. `toggleRecord` stops the active recording on that channel. The one-channel player labels that action “Stop recording”.

Fix: “Stop recording” when `store.activeRecording(on: channel)` is set.

### The iPhone audio menu does not mark the current choice

`PlayerScreen.swift` 1568–1574 and 1619–1625. The current row draws a checkmark and does not set `.isSelected`. The Channels control on the same screen does (`1534`). The Apple TV audio actions set `UIAction` state `.on`.

Fix: `.accessibilityAddTraits(entry.current ? .isSelected : [])` on each audio and delay row, and hide the checkmark image from VoiceOver so it is not read twice.

### An old multiview plan opens the grid

`MultiviewScreen.swift` 1043–1056 and `refreshPlan` 1525–1528. After `await refreshPlan()` the task does not check `Task.isCancelled` or that the ids are still `nowPlaying.together`. Add or remove a channel while the plan is in flight. The new task hides the grid. The old task then sets `planReady` and the tiles call `live.start` under the old plan. A failed plan is `try?` nil, `refreshPlan` returns without filling `blocked`, and the caller still sets `planReady`, so the grid opens with nothing blocked.

Fix: return when the task is cancelled or the ids changed. On a failed plan, keep the spinner and show the error instead of opening the grid.

### Closing a tile’s system picture-in-picture leaves the watch up

`MultiviewScreen.swift` 1881–1883. The sound tile creates an `AVPictureInPictureController`, allows automatic start, and sets no delegate. `onDisappear` (1752–1754) does stop the watch when the tile goes away. Closing the system window while the tile is still on screen never calls `TilePlayer.stop()`, so `stopWatching` does not run. The one-channel player stops through `PictureHandoff.stopWhenClosed`.

Fix: a delegate `pictureInPictureControllerDidStopPictureInPicture` that stops that tile’s watch when the window was closed and the tile is not what the viewer is watching.

### A scan you just started is cancelled, and the button looks idle

`SourcesView.swift` `load` 298–305, `watch` 316–318, `scan` 322–325. `load` awaits the device list, then if `scanning` is still nil it may call `watch`. Tap Scan during that await. `watch` starts a poll. `load` can then call `watch` again, which cancels the poll. The cancelled `scan` always runs `defer { scanning = nil }`, even after the new poll has set `scanning`. The button returns to “Scan channels” while the tuner is still scanning. Another tap posts `scan=start` again.

Fix: a generation on `watch`. Clear `scanning` in `defer` only when it is still current. After the device await, do not start a second poll if one is already scheduled.

### A slow Sources reload puts back a tuner you removed

`SourcesView.swift` `load` 282–312. Nothing ties the result to the call that started it. Pull to refresh, then remove a tuner or add a playlist before that request finishes. The later `load` shows the right list. The first request lands after it and assigns `devices` and `sources`. The removed tuner is back, or the new source is gone. A late failure sets `error` over a list that already loaded.

Fix: a token at the start of `load`. Write `devices`, `sources`, `seenAt`, and `error` only while that token is current.

### The last channel edit wins

`ChannelsView.swift` `toggle` 226–232 and `save` 263–279. Each toggle starts its own save. `save` assigns `channel = updated` for the whole channel. Turn Favorite on, then Hide on, before the first PATCH returns. The favorite response still has Hide off. If it arrives last, the Hide switch snaps off. On failure, `channel = revert` restores the snapshot from before that toggle and wipes a newer one. Backing out calls `commitText` (`207`, `245–260`), and a failed rename is stored on the editor that is already gone.

Fix: one save at a time for that channel. Apply a response only when no newer save has started, and revert only the fields in the failed patch. If the editor is gone, show the error on the channel list.

### A failed library load looks like you have no recordings

`RecordingsView.swift` 94–98 and the task at 215–218. The empty copy is `listed.isEmpty`. `refreshRecordings` uses `try?` and sets nothing on failure, and the screen never reads `store.error`. After a server change the list is cleared. If that fetch fails, the screen says “No recordings yet”.

The kit now drops a recording list that started before a delete, a stop, a watch mark, or a bulk change on this server. This screen still needs the error and the empty copy.

Fix: show the error, and the empty copy only after a fetch has succeeded with no rows.
