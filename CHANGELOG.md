# Changelog

## 0.11.6 — 2026-09-27

Apple TVs keep playing through a garbled broadcast, and stay together from the start.

### Fixed

- One damaged frame from the antenna could stop every Apple TV and iPhone on that channel until someone changed channels. The server now drops a frame whose timing is far off and keeps the rest.
- Apple TVs that joined a channel a browser had just started sat about a tenth of a second apart for the first few minutes. They now line up right away (with the next app update).
- The player shows a clear message when a channel loses its signal, the recordings folder can't be written, or a phone loses its connection, and plays again on its own when that's fixed.
- A recording that would cross the free-space reserve says how much space is left and how much Broadwave keeps.

## 0.11.5 — 2026-09-27

Optional automatic updates that wait until nobody is watching or recording.

### Added

- An optional updater for Docker Compose and Unraid keeps Broadwave on the newest release. It checks nightly at 03:30 and skips a night while someone is watching, a recording is running, or a recording starts within two hours. See "Automatic updates" in the README.
- `broadwave -update-check` reports whether the running server is busy, for any updater that can run a check first.

### Changed

- The install section covers Docker Desktop, where the tuner address has to be entered by hand.

## 0.11.4 — 2026-09-27

Multiview and single screens stop stalling at the live edge.

### Fixed

- A multiview tile of a 60 frames-a-second channel could freeze for 15-20 seconds at a time. Its segments now stay short.
- A browser alone on a channel whose picture arrives late or unevenly stalled again and again near live. After a stall, that screen and multiview now step back from live, a little at a time, until the picture holds.

## 0.11.3 — 2026-09-27

Several screens on one channel stay smooth and in step, on Apple TV and in the browser.

### Fixed

- With several screens or a multiview on one broadcast, a live encode could decode damaged data for as long as it ran, so every screen on it stuttered. Each encode now starts on a whole picture, recovers when it falls behind, and the server logs when a reader lags.
- Apple TV playback hitched several times a second while it kept in step with other screens. It now changes speed rarely and by a fixed amount, and Apple TVs in a room lock together within seconds of joining.
- Screens now share a delay of 13 seconds instead of 10, the closest Apple TV can reliably play to live.
- On Apple TV the playback controls hide again, and the notice about a new device no longer covers the picture.
- The guide no longer lists a show twice.
- A playlist channel learns its real codecs from the stream and has a preview picture.
- The web player says when live TV cannot keep going, and setup actions stay on screen on a TV.

## 0.11.2 — 2026-09-27

Sound and picture line up in every browser, and Safari plays live TV again.

### Fixed

- In Chrome and other browsers the sound could play up to a second late. The server now places each track's start inside the stream itself, not only in a table that some browsers skip.
- Safari showed no picture on live TV. It plays now, and the sync between screens corrects Safari by seeking instead of changing its speed, which made it stall.
- A new screen no longer pauses and then jumps when it joins a channel.

## 0.11.1 — 2026-09-26

Live TV no longer freezes a few seconds after it starts.

### Fixed

- A few seconds into a channel the picture froze for about five seconds while the server moved everyone to the shared delay. Screens now ease onto that delay by playing a little slower for a few minutes, so nothing pauses.
- A channel starts with a short buffer instead of at the very edge of the broadcast, which stopped the brief stalls in the first minute.

## 0.11.0 — 2026-09-26

Live playback keeps one timeline across renditions, rewinds about ninety minutes, and continues when the broadcast clock jumps.

### Fixed

- A channel the server already knows opens on the frequency it stored, and playback can start on the first segment that is ready.
- A copied broadcast and a transcode of it cut on the same pictures.
- When the broadcast clock jumps, that segment closes and the next one continues. The time the player shows stays on the wall clock.
- Rewind reaches about ninety minutes of the broadcast.
- A long live session no longer keeps every earlier playlist in memory.
- The software encoder no longer holds the first picture for one frame per processor core.
- Each encode follows one channel, so another channel on the same broadcast cannot delay the start.
- An export to another app reads the whole channel before it copies, the same way a live tune does.
- On now lists every channel, and the Apple guide opens on that list.
- The tuning screen names the channel. A setup step says when it still needs a look.

## 0.10.0 — 2026-09-26

The server times its own picture and sizes the stream to match. The apps can still find it when Bonjour is quiet.

### Added

- At startup the server times a short 1080p60 encode. Diagnostics reports that speed and what this machine will send: the tallest picture, whether the selected tile stays at 60, and how many tiles fit. A slow machine gets a smaller picture. A copied broadcast is unchanged.
- The iPhone and Apple TV look for the server on the local network when Bonjour finds nothing. They follow a move to a new address only when the reply proves it is the same server.
- Diagnostics lists the channels that are on, and the recent log with passwords left out.
- The web home leads with program artwork.

### Fixed

- A startup encode that runs past its deadline is stopped, including when a script keeps running under it.

## 0.9.1 — 2026-09-25

### Fixed

- Restoring a backup keeps playlist passwords.
- A sports key stays out of the log when a score request fails.

## 0.9.0 — 2026-09-25

Multiview fills the tile, and the apps can be tried with no server.

### Added

- Tiles on the web, iPhone, iPad, and Apple TV are 16:9. The one you are listening to stays at 60 frames. The others stay at 30.
- Adding a channel says whether it shares a tuner, takes one, or has to wait.
- The iPhone portrait player shows the channel, the program, and labeled controls.
- Program pictures show on the Apple guide, search, sports, and recordings, and on web search and sports.
- Apple Home shows a live frame when the listing has no picture and that channel is already on.
- Try the demo plays four short films on the phone or Apple TV when no server is set up. About credits the Blender Foundation.
- The server checks once a day for a newer Broadwave. You can turn that off.
- Settings keeps nightly catalog backups and can restore one. Recordings stay where they are.
- Settings can download a support bundle. Passwords are left out.
- Sports scores come from ESPN's public scoreboard, and only from your own server. A switch turns them off.
- Diagnostics shows the tuner model, firmware, and whether it is locked. This app does not install firmware.
- When a recording loses the tuner, the schedule names a later airing and says what a one-time skip will miss.
- In multiview, the focused tile can move to the game that matters.

### Fixed

- Apple TV keeps the channel name visible when a button is focused, and the sidebar steps aside on Home, Sports, and Recordings.
- A guide title no longer slides under the channel name, and the time labels stay clear of that column.
- The phone tab bar no longer covers the next row.
- A numbered subchannel is not starred as the main network.
- A tuner that is playing a channel is not reported as gone.
- A second picture stays on the same clock as the first.
- An older app is told when the server has moved on.

## 0.8.0 — 2026-09-25

Setup finds the house and finishes itself.

### Added

- Your home lists the tuners, servers, and screens on the network. Nothing is added until you tap, except the first HDHomeRun on a new install.
- After that tuner is in, setup scans the channels, fills the guide, checks the recordings folder, stars ABC, CBS, FOX, and NBC, and tests the picture. It ends on a Ready line.
- A tuner or screen that shows up later raises one banner.
- A station title that arrives compressed still shows in the guide.

### Fixed

- A progressive recording plays at its own 60 frames instead of being doubled.
- A scan that arrives late corrects the picture already on screen, including a movie that should play at 24 frames.
- An H.264 channel that has not been scanned keeps the frame rate it was sent at.
- A program map that spans packets keeps every audio track.
- Apple TV matches the size and frame rate of the picture on screen.
- If the graphics encode dies, it starts once more. A second death frees the tuner.
- A second full English mix stays the main track, and surround sound is passed through.
- A second tuner waits for you to add it. The server uses the address that answered, not a link the device sends.

## 0.7.1 — 2026-09-25

### Fixed

- Captions no longer knock an H.264 stream off the graphics chip. The picture stays there instead of being rebuilt in software.

## 0.7.0 — 2026-09-24

Live TV keeps the motion the station sent, and Apple devices get the surround mix.

### Added

- A 720p channel is recognized before the picture starts, so the first tune is already 60 frames.
- A movie shot on film plays at 24 frames, including when the station wove it into 60.
- iPhone and Apple TV play the station's surround sound. The player can pick the main mix, another language, or described video. Even volume stays off unless you turn it on.
- Apple TV matches the station's frame rate.
- The player can show what the stream is doing: the picture coming in, the picture going out, and how far this screen is from the others.

### Fixed

- The graphics chip decodes the broadcast, so a live channel uses less of the server.
- Invented frames on a 30-frame show looked ghosted, so that path is gone. A 30-frame show stays 30.
- Closed captions no longer knock an Apple stream off the graphics chip.
- A movie no longer leaves the channel at 24 frames. The next show is judged on its own.
- The iPad guide shows the channel number and name beside each row. A short lineup fills the screen.

## 0.6.0 — 2026-09-24

The product is now Broadwave, and 720p channels play at a true 60 frames.

### Changed

- The app, server, image, and repo are named Broadwave: `ghcr.io/wolfebase/broadwave`, `github.com/wolfebase/broadwave`, Bonjour `_broadwave._tcp`, and links `broadwave://`. The catalog file is `broadwave.db`.
- The iPhone, iPad, and Apple TV Home screen shows each program's picture.

### Fixed

- A 720p station (most ABC and FOX affiliates) played at 120 frames per second on Intel graphics and 30 without them. It now keeps its own 60 frames and is never deinterlaced or upscaled.
- A playlist or Xtream source with a password went offline at its first daily refresh, and an Xtream guide lost its password. Both refresh now, and a guide that fails says so in the activity log.

### Added

- The apps and web client check every server response against recorded examples, so a server change can't quietly break them.
- `-staging` runs a test copy next to a real server without recording, scanning, or pulling the guide.

## 0.5.0 — 2026-09-24

The guide fills in from the broadcast, and Settings says how the antenna is doing.

### Added

- A channel with no listings gets them from the broadcast when that channel is tuned, and an idle tuner checks the ones that are still empty.
- The guide runs as far as the listings go. A row says which day they run through, and a program says where its listing came from.
- Settings shows each channel as Great, OK, Weak, or Lost. Check all channels uses a free tuner and stops when you start watching.

## 0.4.0 — 2026-09-24

Broadwave can take a tuner, a playlist, or a free-channel server, and a new install walks you there.

### Added

- The server finds an HDHomeRun on the network, and looks harder for the other servers people already run.
- A playlist keeps the channel details Channels uses, including artwork and its own guide. A big list asks what to keep.
- Xtream Codes, tvheadend, Channels DVR, and the usual emulator playlists can be added. Passwords stay masked.
- Add free channels finds a FastChannels, Pluto, or Samsung server that is already running. Streams that need DRM are left out.
- Setup is five steps: sources, channels, guide, recordings, and the apps. The same steps are on iPhone and Apple TV.
- Diagnostics says what to fix, in one line, when the network, disk, clock, or tuner is in the way.
- A source says when it goes offline, when it comes back, and when every stream from it is in use.

## 0.3.3 — 2026-09-23

### Fixed

- A channel that is just starting keeps enough of the broadcast for the picture to come out. The playlist was staying empty.

## 0.3.2 — 2026-09-23

### Fixed

- A preview still is saved. The temporary file name was not one ffmpeg would write.

## 0.3.1 — 2026-09-23

### Fixed

- A preview still waits long enough for the next keyframe, so a tuned channel actually gets one.

## 0.3.0 — 2026-09-23

The guide is there when you open the app, artwork stays sharp, and a channel that is already on can show a still.

### Added

- The web app and the Apple apps open on the last guide, then refresh. The first hours load first.
- A still from a channel that is already tuned, for listings and On now cards that have no artwork. Taking one never starts a tune.

### Fixed

- A small poster stays its real size instead of being stretched across the hero.
- Hiding a score no longer changes that score for the rest of the house.
- Side by side checks that the channels fit on the tuners before it starts them.
- The multiview title matches the layout.

## 0.2.0 — 2026-09-23

Everything since the first public image: multiview, a fuller guide, and sports that follow the game.

### Added

- Watch two or more channels at once. The web app has 2-up, 1+2, 1+3, quad, and picture-in-picture. iPhone, iPad, and Apple TV have the same idea, sized for each screen. Sound follows the tile you pick.
- The guide matches listings by call sign, takes a Schedules Direct account or an XMLTV link for channels the tuner guide skips, and keeps season, episode, and artwork with each program. Search covers titles, descriptions, and recordings.
- Sports reads a public scoreboard, links a listing to the game, and keeps recording until the game is over. Follow a team to record every game. Scores stay hidden on a recording until you have watched it.
- The server recovers after a crash or a stop: leftover transcodes are cleared, a cut-off recording is marked, and a show still on the air resumes.

### Fixed

- The Apple apps build on the Xcode version CI uses, as well as newer Xcode.
- The web app talks only to the versioned API.
