# Changelog

## 0.12.18 — 2026-10-04

### Fixed

- Search lists what is on now first, once per channel, and leaves out shows that have already ended and channels you hid. Shows with the search words in their title come before ones that only mention them. On the web, results follow your typing.
- An encrypted ATSC 3.0 station's shows can still be found in search, so they can be recorded from its regular broadcast.
- A link to a channel the guide doesn't show, such as the 1.0 half of a 3.0 pair, plays the channel the guide shows for it instead of an empty page.
- Channels found by a later scan take their place in the guide by number instead of sitting at the bottom.
- On the web, the mini-guide opens on the playing channel and its arrow keys move the selection you can see.
- Enter closes the player's keyboard help on the web, so a remote can close it.
- The web notice for a new device in the house goes away by itself after 12 seconds and waits until you leave the player.
- On a TV browser, the remote's focus ring stays where you can see it: Up from the mini player reaches the guide, and Back from the player lands on the channel that was playing.
- The guide's floating Now button no longer covers the grid when the guide opens at now.
- The player's keyboard shortcuts keep working after focus falls off the player.

## 0.12.17 — 2026-10-04

### Fixed

- Two tuners on one antenna no longer list every channel twice. The guide, Home, and channel up and down show each channel once, with its listings; the other tuner still carries it if the first is busy.
- Settings › Tuners and channels counts an ATSC 3.0 channel's tuner as Broadwave's while it plays, with its number of viewers. It used to look like another app holding the tuner, so multiview could count one tuner too few.
- Settings no longer says "All 4 tuners are free" on a server with 6 when 2 are off. It names the ones that don't answer.
- A multiview tile on a channel with no signal says to check the antenna, as a single channel does, and keeps saying it after the tuner is given back.
- After a server restart, a multiview with more tiles than the server has pictures for gives them back to the tiles that were playing, starting with the one with sound. A tile that had no picture could take one before.
- A recording whose file was moved or deleted outside Broadwave says the file is gone and offers only Delete. It used to offer Play, sit on Home, and show in search.

## 0.12.16 — 2026-10-04

### Fixed

- A recording, an export to another app, or a guide check on a channel with no signal no longer freezes live TV. Its tune takes about 17 seconds to fail, and every viewer's picture used to wait for it.
- A channel with no signal now says so ("isn't coming in") on tuners that answer "No Video Data", where it used to say the stream wasn't answering.
- On the web, a picture whose restart failed the same way is tried again, with a growing wait, until it plays. It used to say "Starting it again" for good.
- An iPhone, iPad, or Apple TV whose stream keeps failing at once now waits a little longer between tries instead of asking the server about 20 times a second.

### Changed

- Settings › Tuners and channels says on a tuner's or playlist's card when it is offline and when it was last seen, with Remove first. On iPhone, iPad, and Apple TV too, which can now remove a device.
- A playlist's card no longer shows an empty firmware line or Scan channels.
- A page for people installing Broadwave describes how tuners are found, paired, and removed.

## 0.12.15 — 2026-10-04

### Fixed

- A channel with no signal no longer stops every other channel. Its tune takes about 17 seconds to fail, and the server used to hold everything else while it tried, so a multiview with one dead channel froze the good tiles too, and other screens' pictures stalled. A viewer's tune now runs beside the others.
- The picture budget on a server without a GPU no longer changes from one restart to the next. The startup encode is now timed from its first frame, and the number of pictures at once is set from what real tile encodes cost on the processor. A six-core server holds four; three cores hold two at 540p60.

## 0.12.14 — 2026-10-04

### Added

- Settings › Tuners and channels can remove a tuner or playlist that is gone for good. Its channels leave the guide; a favorite, custom name, or pass moves to the same channel on another tuner when there is one. Recordings and their files stay. Broadwave refuses while something plays or records from it.
- A page for people installing Broadwave describes multiview: the layouts, which tile has the sound, picture sizes, and tuners.

### Fixed

- An iPhone, iPad, or Apple TV multiview link that names the hidden half of a 1.0/3.0 pair now plays the half on the guide, and an encrypted 3.0 station plays its regular broadcast with a note. That tile used to be dropped.

## 0.12.13 — 2026-10-04

### Fixed

- When two Broadwave servers run on the same computer, the apps' search now finds both of them. The one that started second used to be missed whenever Bonjour could not reach the app.
- An iPhone, iPad, or Apple TV joining a channel another screen is already watching, at a quality that needs its own encode, now shows a moving picture in about 2.5 seconds instead of up to 20. Its picture starts at the room's frame.
- An Apple screen that falls behind its room after a pause no longer stops trying to catch up for a minute when its player loses a moment while restarting.
- On Apple TV, Up and Down in the guide's channel column now step through the channels.
- A multiview page you return to with Back no longer sits paused on its pictures for 4 seconds or more before it plays.

## 0.12.12 — 2026-10-03

### Fixed

- A browser joining a channel that an iPhone, iPad, or Apple TV is already watching now shows a moving picture in about 2 seconds instead of up to 20. Its picture starts at the room's frame, so it is in step with the other screens from the start.
- An Apple screen whose player gives up on the stream after a hiccup now reloads it by itself and comes back on the room's frame in a few seconds, where it used to sit on a still picture for up to a minute. A multiview tile does the same.
- A reloaded Apple picture is no longer named "stopped" a few seconds after it came back.

## 0.12.11 — 2026-10-03

### Fixed

- A multiview tile on a channel that sends a keyframe only every two seconds or more no longer drops out on an iPhone, iPad, or Apple TV right after it opens. The same went for the first watch of such a channel after the server restarted. The server now remembers which channels send long groups, so every encode of them starts with room for them.
- An Apple multiview tile on such a channel no longer drifts half a second behind and stays there. Its room now really plays 20 seconds behind live, as a single screen does.

## 0.12.10 — 2026-10-03

### Fixed

- An iPhone, iPad, or Apple TV now plays a channel whose station sends a keyframe only every two seconds or more. It used to stay on "Still tuning" and then say the picture stopped: the playlist's target duration grew as longer groups arrived, and Apple's player drops a stream when that changes. The target is now set once, with room for the longest group.
- An Apple screen on such a channel no longer drifts a few hundred milliseconds behind the other screens. Its room plays 20 seconds behind live instead of 16, where the player can still speed up to catch up.

## 0.12.9 — 2026-10-03

### Fixed

- A multiview tile no longer freezes for 2 seconds now and then when the broadcast sends two keyframes a few frames apart. The tile's segment now closes when its group is complete instead of a whole group later.
- A multiview address that names the hidden half of a 1.0/3.0 pair opens the half on the guide instead of dropping that tile. An encrypted 3.0 channel opens its regular broadcast, and the tile says why.

### Changed

- The server log notes a pause of 1.5 seconds or more in a tuner's stream, and an encode that stops producing segments while the tuner is still sending.

## 0.12.8 — 2026-10-03

### Fixed

- A multiview tile of a 60-frame channel no longer waits at the live edge every few minutes. The tile kept only every other frame and lost the broadcast's keyframes with them, so its segments ran up to 4 seconds long.
- A freshly tuned multiview tile no longer freezes for 2 seconds just after it starts. It now starts a little further back, so its first moving picture comes about 2 seconds later.
- Starring the hidden half of a 1.0/3.0 pair now stars the channel on the guide.
- An iPhone, iPad, or Apple TV no longer logs a bandwidth warning on every segment of a 1080 encode. Playlists declare each encode's peak rate and its average.

### Changed

- On Intel's low-power GPU encoder, the server runs six pictures at once instead of four, so a quad fits beside two full screens. Diagnostics says how many pictures fit.
- Diagnostics says the 720p picture goes to a large multiview tile. Quad tiles are 360p.

## 0.12.7 — 2026-10-03

### Fixed

- A second screen joining an ATSC 3.0 channel that is already playing no longer waits about 12 seconds now and then. ffmpeg held the stream while it worked out the channel's caption track; it now settles that on the first packet. The same wait could hit a 3.0 multiview tile and a 3.0 channel shared to Plex or Jellyfin.

### Changed

- Opening an encrypted ATSC 3.0 channel again shows the note that it plays the station's regular broadcast, not only the first time.
- On a TV browser, the 3.0 tag stays inside the channel name, the encrypted note sits below the tuning card, and Left and Right change a Settings menu.
- New page: [ATSC 3.0](docs/atsc3.md).

## 0.12.6 — 2026-10-03

### Changed

- On an Intel GPU, live TV now encodes on the chip's low-power encoder, which Plex and Jellyfin transcodes do not slow down. A 1080i channel kept 3.7 times real time beside three 4K transcodes, where it used to fall to 1.4 times. The server no longer moves live TV to the processor on such a machine.
- Multiview tiles decode on the GPU too. A 1080i tile takes a seventh of the processor time it did.

### Fixed

- An ATSC 3.0 channel plays its picture, without sound, on an ffmpeg that has no AC-4 decoder. It used to stop after a second and a half. Diagnostics says why.
- A playlist link that carries AC-4 sound without declaring it plays on its first watch on such an ffmpeg.
- The preview pictures for a channel group with several programs no longer time out every minute.

## 0.12.5 — 2026-10-03

### Fixed

- On an ATSC 3.0 channel, an iPhone, iPad, or Apple TV could sit a tenth of a second or more off the other screens in the room and never catch up. The channel's first segment ran four seconds long while the sound started, which kept Apple players far from live for as long as it was listed.
- The preview picture for an ATSC 3.0 channel timed out every minute and logged an error.
- Switching the sound track in a browser no longer counts the cancelled download as a player error.

## 0.12.4 — 2026-10-02

### Fixed

- After a tuner went quiet and carried on, the program clock kept the quiet time for a moment and then lost it again, so screens chased a live edge that was not there.

## 0.12.3 — 2026-10-02

### Fixed

- Live playlists keep a closed segment's parts listed for as long as low-latency HLS asks. iPhone, iPad, and Apple TV no longer lose their place in a 3.0 channel's picture when a segment closes.
- The captions playlist holds a low-latency reload until the segment it asks for is out. Apple players logged an error on every 3.0 channel and fell back to slower reloads.
- A low-latency reload that arrives before a channel's first playlist waits the full time instead of 1.5 s.

## 0.12.2 — 2026-10-02

### Fixed

- A channel whose tuner was unplugged plays from another tuner that carries it, also when that tuner's copy is hidden behind its 3.0 version. It said "This tuner did not answer."

## 0.12.1 — 2026-10-02

ATSC 3.0 channels, and a picture that never stays frozen.

### Added

- ATSC 3.0 (NextGen TV) channels on tuners that receive them. Apple TV, iPhone, iPad, and Safari get the broadcast's own HEVC picture, untouched; other browsers get H.264. AC-4 sound arrives as 5.1 Dolby Digital where the device takes it and as stereo AAC elsewhere, and a second sound track (often Spanish) can be picked like any other.
- A 3.0 channel is paired with its station's 1.0 channel and shows the same guide listings. An encrypted 3.0 channel stays hidden and opens the station's clear 1.0 channel instead; Settings lists it as encrypted rather than broken.
- 3.0 recordings keep the broadcast's own packets. A 3.0 multiview tile is sent as broadcast instead of encoded again.
- A 2160p HEVC picture passes through as sent; a picture taller than the screen it plays on is scaled down.

### Fixed

- A picture that stops moving reloads itself, then tunes again, on the web. On the server, an encode that stops writing starts again, a tuner stream that goes silent is opened again, and a recording keeps going when the tuner refuses to reopen.
- A 5.1 channel with more than one sound track no longer stops after its first frame on iPhone, iPad, and Apple TV.
- Two screens that open the same 3.0 channel at once share one tuner, and a warm 3.0 channel gives its tuner to a new one.
- A tuner set to none is freed at once. An unplugged tuner no longer slows the tuner list or every channel start.
- Guide access is asked of every tuner, not only the first one, and program times stay right after a tuner goes quiet.
- A web page can no longer reach the server by pointing its own name at your server's address, and a page on another site can no longer join a room's live updates. A browser behind an HTTPS reverse proxy needs nothing; set `BROADWAVE_HOSTS` for any other public name you reach the server by.

## 0.12.0 — 2026-09-29

Live captions, every sound track in one stream, Watch together, a choice of live delay, and the last hour of each tuned channel kept for Start over and recordings.

### Added

- Live captions in the web player and in the iPhone and Apple TV players. Roll-up captions appear as they are typed, in step with the picture, and stay on through a break in the broadcast. `c` turns them on and off on the web.
- Switch between a channel's sound tracks (English, Spanish, described video) without restarting the picture, on the web, iPhone, iPad, and Apple TV. Every sound track rides in the one stream; set `BROADWAVE_ALTERNATES=0` to carry only the main one.
- Watch together in the browser: see who is watching the same channel, join or leave, and a pause, rewind, or seek moves everyone in the group.
- Live delay: Lowest, Balanced, or Stable, per room from the player's Options, with a default for each device in Settings. On iPhone, iPad, and Apple TV too.
- The server keeps the last hour of each tuned channel on disk while it is tuned, using at most half the free space and always leaving 4 GB. Record a show you are already watching and the recording starts from the beginning of the show. A series pass that finds its show already on, on a channel someone has been watching since it began, records it from the beginning. Settings can make it 30 minutes, 2 or 4 hours, or turn it off, and Diagnostics shows what each tuned channel holds.
- Channel changes are faster: the channel you just left stays warm for 20 seconds, and on Apple TV the channel the remote rests on in the Channels panel starts before you press. Diagnostics lists the last channel starts and how long each step took.
- Web player extras: a stats overlay (`i`), keyboard help (`?`), last channel (`L`), typing a channel number, a sleep timer, volume memory, and theater mode (`t`). A TV remote can walk the guide, the player, and setup.
- Apple TV info panels (Info, Channels, Stream), Record, Start over, and Multiview in the player's menu, and clickpad up and down to change channel. On iPhone, swipe to change channel and pinch to fill the screen.
- Download a finished recording from the web. A `.nfo` file can be written beside each recording for Plex, Jellyfin, and Kodi.
- Settings list how much space each show uses and what records next, and a conflict can record the later airing instead.
- A recording that finishes counts the signal damage it carries.
- Manage recordings, skip commercial breaks, and record one airing from the guide in the Apple apps.
- The web app installs as a standalone app.

### Fixed

- Screens come back on their own after the server restarts, and the pictures that were playing come back first. A multiview tile keeps its picture across a restart, the sound tile gets its picture first, and a tile the picture budget turned away asks again.
- A channel that will not start says why in plain words, and a picture that stops tries again by itself.
- A web screen joining a room starts on the room's frame instead of catching up. Every encode of a channel dates the same broadcast frame the same way, so a TV and a browser on different streams stay together.
- The web player no longer trims its speed forever when it sits just outside the sync band, which dropped frames at Lowest delay.
- The player's Options panel no longer drops half the frames on a browser drawing without a graphics chip.
- A server with a GPU and a fast processor now checks the processor with the encode live TV actually runs. The old check timed a different encode, counted process startup, and read 2.3x on a machine that runs live TV at 3.1x, so live TV stayed on a GPU that Plex was saturating.
- An Apple TV that fell a second or more behind the others skipped every minute trying to catch up: a forward jump on Apple TV lands short. It now jumps past the others and pauses the exact difference, once, and if that misses twice it stops correcting and offers "Back in sync" instead of skipping.
- Apple TVs wait for the TV to finish switching to the channel's frame rate before lining up again.
- A channel's other sound tracks are found even when the first read of the broadcast starts in the middle of a packet.
- A page kept for Back no longer holds its tuner, and a tuner held for a flip back goes to the next channel when it is needed.
- HD encodes are tagged BT.709, so small tiles keep their colors.
- The container switches to `PUID` and `PGID` safely.
- A page on another website can no longer change the server or replace its catalog through a browser open on your network. The web app and the Apple apps work as before.
- The server log no longer repeats a look around the house every 45 seconds.

## 0.11.9 — 2026-09-27

Apple TVs in one room play in step.

### Fixed

- Apple TVs in one room now land within a few hundredths of a second of each other. Whole-Home Sync plays 16 seconds behind the broadcast instead of 13: at 13 an Apple TV that started behind the room could not catch up and sat a third of a second off the others.

## 0.11.8 — 2026-09-27

Live TV keeps playing when Plex or another app is busy on the same GPU.

### Fixed

- On a server where Plex, Jellyfin, or Channels DVR transcodes on the same Intel GPU, a 1080i channel could encode slower than real time. Apple TVs drained their buffer, paused under the progress bar, and the broadcast broke up. Broadwave now measures the CPU at startup and runs live TV on it when it has room to spare (about 2.5 cores for 1080p60), leaving the GPU to the other apps. Set `BROADWAVE_ENCODER=gpu` or `software` to choose yourself.
- An Apple TV that fell half a second behind the others seeked every two seconds and froze each time. A seek that doesn't land now waits a minute and the TV plays on (with the next app update).

## 0.11.7 — 2026-09-27

Apple TVs play smoothly: no freeze a few seconds into a channel, and no catch in voices.

### Fixed

- An Apple TV that started a channel played a few seconds, then froze for about five while the room settled. It now waits once on its first picture and plays straight through.
- Apple TVs nudged their speed to hold sync over differences nobody could hear, and each nudge held a frame and clipped speech. They now leave anything under 60 ms alone, wait longer between nudges, and stretch sound smoothly when they do (with the next app update).
- An Apple TV that sat a quarter second behind the others jumped every 15 seconds trying to close a gap it could not, and each jump hitched the picture and cut the sound. It now plays on (with the next app update).
- Apple TVs report their playback health to the server log every 10 seconds, so a stutter on a real TV can be traced without a Mac attached (with the next app update).

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
