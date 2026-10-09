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

### A failed Bonjour browse is not replaced

`ConnectView.swift` 114–115 starts discovery in `onAppear` and stops it in `onDisappear`. `Discovery.start()` (`Discovery.swift` 67–68) returns immediately when `browser != nil`. The state handler (line 81) sets `searching = false` on `.failed` and does not release `browser`. Backgrounding does not call `onDisappear`. After a failed browse, “No server answered on this network.” (`ConnectView.swift` 98) stays until the view is recreated, and a later `start()` does nothing.

Fix: on `.failed`, cancel and nil `browser` so the next `start()` browses again. From the connect screen, call `start()` when the scene becomes `.active`. A kit test must not call `Discovery.start()` or `LANProbe.collect()`; those probe the LAN.

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
