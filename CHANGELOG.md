# Changelog

## Unreleased

### Added

- iPhone and iPad have four home screen widgets, in small, medium, and large: On now, Your teams, Recording now, and Up next. On now, Your teams, and Recording now open that channel. Up next opens Recordings. Record on an airing sets it to record once, and a game keeps going until it is final. A row that is already recording, or set to record, says so. The teams widget asks only for games, so a large guide does not make it say it cannot reach the server, and a game still on the scoreboard stays through overtime.
- On Apple TV, with Broadwave in the top row of the home screen, the Top Shelf lists games on now, favorite channels and what they are showing, and recordings you have started and not finished. Choosing one plays it. A recording made since the app last ran still opens, a player that is already up closes first, and a slow server holds the shelf for at most 10 seconds. A picture that fails while the recording is still being prepared is tried once more and then says why. If the place you left off is not in the file yet, it plays from the start.
- On the web, Add to other apps on a multiview shares those channels with Plex, Jellyfin, and Channels as one channel. They list it from 990.1 up, named Multiview plus the channel names, with the sound channel first. The guide fills it with two-hour blocks that say what it shows. Settings › Share with other apps can remove one, and the others keep their numbers. If one of its channels is hidden or gone, that shared channel is left out.
- On the web, a note appears when a team you follow starts playing, or a game on your channels comes down to the last minutes, with Watch and Not now. It waits while the player is up and then goes away. Settings › Game alerts is your teams and close games, your teams only, or off. A game you are recording, or a recording you have not marked watched, gets no score and no close-game note. Playing it does not show the score. Neither does any game while scores are hidden. A start note never includes a score. A team you followed from a listing counts.
- Recordings on the web, iPhone, iPad, and Apple TV open on Continue watching, then each show with how many recordings it has, how many are unwatched, and how much space it uses. A show's page groups episodes by season. Sort by newest, oldest, name, or size, and show only shows, movies, or sports. Select marks several watched or unwatched, or deletes them after you confirm, including with the remote. Select all takes only what the filters show. A file already in a library folder takes its show, season, and episode from its folders and its name.
- A pass can match a title, words in a title, or a category, on chosen days and in a time of day. A window that runs past midnight counts as the night it began. On the web, Schedule edits a pass and shows the next two weeks of what it would record, skip, and push aside. Passes on iPhone, iPad, and Apple TV do the same. The Apple screen is called Passes. Drag to reorder on iPhone and iPad; on Apple TV, move a pass up or down. When two passes need the same tuner, the higher one records.
- On the web, Rename file moves a recording inside the recordings folder and takes the files beside it. A finished recording you move by hand inside that folder is found again and plays from its new place.
- The recordings folder can be any folder you name with `BROADWAVE_RECORDINGS`, or with `-recordings` outside Docker. Recordings already made stay where they are.
- On the web, a recording can be marked Keep forever. Clean-up leaves it alone, and a pass's "keep the newest" does not count it. Settings can delete recordings you have watched after a number of days and, when space runs low, delete the oldest watched ones so a new recording still fits. Both are off until you turn them on. Played to the last 15 seconds, or marked watched, counts. Stopping earlier does not, even through 90% of a long recording. Playing it again starts the clock over. One you played in the last 6 hours stays, and so does anything outside the recordings folder.
- Skip intro and Up next on a recording, on the web, iPhone, iPad, and Apple TV, once that show's intro and end titles have been picked out from its other episodes. Skip intro is offered during the intro. From the end titles, or the last 10 seconds, Up next names the next episode in the order they aired, with Play now and Not now. With Play the next episode on, it plays after 10 seconds; off, it waits.
- A recording the signal ruined offers Record it again on the web, iPhone, iPad, and Apple TV, when that episode has a name. The row says how many seconds the signal dropped, or how many times it broke up. That records the episode the next time it airs. A series pass does the same on its own, and stops after a second ruined copy. Recordings from before this change are not marked.
- On the web, live TV has Start over. While the live window still holds the start of the show on now, it jumps there, and a group moves with you. Otherwise a recording of this showing that has the start opens, and it picks up where you left off.

### Changed

- The Docker image includes comskip. Find commercials and the scan after a recording use it when it is there, and comskip's own files stay out of the recordings folder. Without it, the server still finds breaks itself.
- A break is skipped on its own only when the server is sure. A less sure break still has Skip, and is not marked as a cut for other apps. On iPhone, iPad, and Apple TV, two quick jumps forward inside a break skip the rest of it, as on the web.
- The commercial scan waits until nobody is watching live TV and runs one recording at a time, or starts after three hours. A spot that plays again can mark a later break. The show, the station logo, a still picture, and black are not learned as ads.
- Find commercials stays disabled and says it is working until that scan finishes, on the web and on Apple, including if you leave and come back. Then it says how many breaks it found.
- New recordings are filed by show, the way Plex and Jellyfin read them: under TV, then the show, then a season folder when the guide has both a season and an episode number. Otherwise the file stays in the show folder, named with the date and the episode name, or the start time when it has none. Movies go under their title, with the year when the guide has it. Settings › Recording folders › All in one folder keeps every new file in one folder. Recordings already made stay put. An emptied season or show folder is removed.
- A pass's keep and limit rules touch the recordings that pass made, not every recording with a similar name. A title pass also covers older recordings of that same title. A keep rule does not delete a file outside the recordings folder. A recording a pass starts is tied to that pass, so those rules cover it. One you played to the last 15 seconds, or through 90% of it, now counts as watched for "keep unwatched", and so does one you marked watched.
- On the web, removing a pass asks first and says the recordings it made stay. Cancel, Escape, or Back keeps the pass.
- A quiet scoreboard goes stale when the next game starts, so the next look is within about a minute instead of up to two hours. The Sports page asks once when it opens. A game that started before midnight still matches its listing, so a recording of it can keep going.

### Fixed

- On iPhone, iPad, and Apple TV, the first play of a recording resumes where you left it once the file has reached that spot. It used to start over while the playlist was still being built.
- On the web, Up next follows the order episodes aired and stops after the last one. It used to wrap from the newest back to the oldest. On iPhone, iPad, and Apple TV, Play the next episode is honored. The player used to ignore it.
- A channel that two tuners carry is recorded once. An airing that will not be recorded no longer takes a tuner from one that will. Recording one showing ranks above every pass. Following a team again keeps that pass and its rules.
- Two recordings that start together no longer write the same file.
- On iPhone, iPad, and Apple TV, the time on a recording follows when it aired, not the moment you pressed play.
- Playing, downloading, or sending a recording to another app opens a file only from the recordings folder or a library folder. A file stored somewhere else is not served. One left in the default recordings folder still opens after that folder is pointed somewhere else.
- A channel name with a quotation mark no longer cuts the line short in an M3U shared with another app.
- Leaving a multiview while a tile is still tuning no longer drops the sound tile's picture, so coming back does not start that tile over and wait. On a server with room for only a couple of pictures, sharing a multiview no longer comes up empty while one you just closed is still winding down.
- On iPhone, iPad, and Apple TV, a tuner found on the network that has stopped answering says it is offline, with when it was last seen, and Remove is the first control.

## 0.12.20 — 2026-10-05

### Fixed

- A recording no longer skips past fades and scene changes. Without comskip, every short black stretch counted as a commercial break; now a break is a run of spot-length gaps.
- A channel with no signal says so in about 8 seconds instead of 17.
- Settings marks a tuner that has stopped answering as offline, with when it was last seen. A tuner added by its address keeps reading as online while it answers.
- On a Mac, or with an Intel, AMD, or NVIDIA GPU, an H.264 picture encode that fails as it starts falls back to the CPU instead of leaving the channel dark. One that dies after the picture is already going is tried once more on that GPU, and let go if it dies again. An HEVC encode is tried again on the GPU the same way, except a VAAPI start, which still tries the CPU.
- On iPhone and iPad, a recording has its scrubber, play and pause, and time again.
- On iPad, the "New … found" line no longer covers the tab bar, and a device is no longer told it found itself.
- On iPhone, the guide's filters and rows show while a channel is minimized.
- On Apple TV, Menu in multiview goes back to the channel you came from, or closes multiview if nothing was playing.
- On the web, Tab stays inside the full player and cycles its controls, and the first Tab while the controls are hidden shows them.
- On a TV browser, the player bar fades again after a button press. A focus ring used to hold it up.

## 0.12.19 — 2026-10-04

### Fixed

- On iPhone, a minimized player keeps playing in the mini player, and opening it again picks up at once instead of starting the channel over.
- On iPad, Picture in Picture that started when you left the app ends when you come back, and the picture returns to the player.
- Skipping back 10 seconds from Picture in Picture on live TV no longer freezes the picture for about 15 seconds.
- On the web, a paused live player keeps counting how far behind live it is.
- On Apple TV, a channel that starts can no longer sit on a still picture after the transport bar hides.
- On Apple TV, Up and Down change the channel again after the Channels, Stream, or Audio page was opened.
- On Apple TV, the guide no longer puts focus on a show that has already ended.
- On Apple TV, the player's Channels page opens on the channel that is playing, and channel numbers stay on one line.
- On Apple TV, Settings opens at the top.
- Closing Picture in Picture closes the player, instead of leaving a stopped mini player.

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
- The picture budget on a server without a GPU no longer changes from one restart to the next. The startup encode is timed from its first frame, and the number of pictures follows how fast that 1080p60 encode ran. At about 5.5 times real time or faster, four pictures fit. From about 3.8 times up to that, two 1080p pictures fit. From about 2.7 times up to 3.8, two pictures fit and the large tile is 540p.
- An encode from about 1.9 times real time up to 2.7 keeps one 720p picture. Below that, down to about 1.4 times, the picture is 540p.

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
- In Watch together, a seek toward live stops at the room. It used to aim a few seconds from live, which an Apple screen cannot reach.

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
- On iPhone, iPad, and Apple TV, search and a link to an encrypted 3.0 station play the regular broadcast and say why. The 3.0 mark shows on Home, the guide, and search, and you can put only the regular half on the guide.
- A link to a channel the guide already shows opens without waiting for the whole lineup.

### Changed

- On Intel's low-power GPU encoder, the server runs six pictures at once instead of four, so a quad fits beside two full screens. Diagnostics says how many pictures fit.
- Diagnostics says the 720p picture goes to a large multiview tile. Quad tiles are 360p.

## 0.12.7 — 2026-10-03

### Fixed

- A second screen joining an ATSC 3.0 channel that is already playing no longer waits about 12 seconds now and then. ffmpeg held the stream while it worked out the channel's caption track; it now settles that on the first packet. The same wait could hit a 3.0 multiview tile and a 3.0 channel shared to Plex or Jellyfin.
- Search, or a link, to an encrypted 3.0 station plays the regular broadcast and says so. The note shows again the next time, not only the first open from the guide.

### Changed

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
- On iPhone, iPad, and Apple TV, a copied channel no longer stops after its first frame when a group opens on a tiny fragment with no sound.
- Those channels no longer pause themselves a few seconds before one of those fragments.
- On a copied 3.0 channel, Apple players no longer pause a few minutes in because a playlist reload answered before the next segment existed.
- A second screen on a 3.0 channel that is already playing keeps that channel's other sound track.
- Two screens that open the same 3.0 channel at once share one tuner, and a warm 3.0 channel gives its tuner to a new one.
- A tuner set to none is freed at once. An unplugged tuner no longer slows the tuner list or every channel start.
- Guide access is asked of every tuner, not only the first one, and program times stay right after a tuner goes quiet.
- A web page can no longer reach the server by pointing its own name at your server's address, and a page on another site can no longer join a room's live updates. A browser that reaches the server by HTTPS on port 443 needs nothing extra. Set `BROADWAVE_HOSTS` for any other public name or port.

## 0.12.0 — 2026-09-29

Live captions, every sound track in one stream, Watch together, a choice of live delay, and the last hour of each tuned channel kept for Start over and recordings.

### Added

- Live captions in the web player and in the iPhone and Apple TV players. Roll-up captions appear as they are typed, in step with the picture, and stay on through a break in the broadcast. `c` turns them on and off on the web.
- Switch between a channel's sound tracks (English, Spanish, described video) without restarting the picture, on the web, iPhone, iPad, and Apple TV. Every sound track rides in the one stream; set `BROADWAVE_ALTERNATES=0` to carry only the main one.
- Watch together in the browser: see who is watching the same channel, join or leave, and a pause, rewind, or seek moves everyone in the group.
- Live delay: Lowest, Balanced, or Stable, per room from the player's Options, with a default for each device in Settings. On iPhone, iPad, and Apple TV too.
- The server keeps the last hour of each tuned channel on disk while it is tuned, using at most half the free space and always leaving 4 GB. Record a show you are already watching and the recording starts from the beginning of the show. A series pass that finds its show already on, on a channel someone has been watching since it began, records it from the beginning. Settings can make it 30 minutes, 2 or 4 hours, or turn it off, and Diagnostics shows what each tuned channel holds.
- Channel changes are faster: the channel you just left stays warm for 20 seconds, and on Apple TV the channel the remote rests on in the Channels panel starts before you press. On the web, resting on a row in the player's Channels list starts that picture when its frequency is already tuned and a picture slot is free. That guess never tunes a new frequency, never stops another picture, and it stops after 20 seconds. Diagnostics lists the last channel starts and how long each step took.
- Web player extras: a stats overlay (`i`), keyboard help (`?`), last channel (`L`), a sleep timer, volume memory, and theater mode (`t`). A typed channel number can skip the dot, so 51 is 5.1. A TV remote can walk the guide, the player, and setup.
- Apple TV info panels (Info, Channels, Stream), Record, Start over, and Multiview in the player's menu, and clickpad up and down to change channel. On iPhone, swipe to change channel and pinch to fill the screen.
- On iPhone, Previous and Next change the channel, and a tap that shows the controls does not pause. Settings opens from the gear on Home, so Recordings stays on the tab bar.
- On iPhone and iPad, leaving the app starts Picture in Picture, and the player has AirPlay. A tap on the small window comes back to the same channel.
- On iPhone, iPad, and Apple TV, a recording can become a library channel, and a break can be marked by hand.
- A public page describes Broadwave and copies the install steps.
- Download a finished recording from the web. A `.nfo` file can be written beside each recording for Plex, Jellyfin, and Kodi.
- Settings lists how much space each show uses. Recordings lists what records next, including on iPhone and Apple TV, and a conflict can record the later airing instead.
- A recording that finishes counts the signal damage it carries.
- Manage recordings, skip commercial breaks, and record one airing from the guide in the Apple apps. On the web, the program sheet offers Record and Don't record for one upcoming airing, and a search result shows the day and opens that sheet.
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
- In side by side and quad, moving the sound to another tile no longer restarts the pictures.
- Rewinding live TV stays rewound. Go to live returns to the room.
- Playing a recording shows the show's length, and a seek past what is ready lands at the end. On the web, Space pauses and the arrow keys skip.
- Multiview offers another channel only when a picture is free. A tile that cannot start can be removed, and the sound moves to a tile that plays.
- An iPhone or Apple TV found over Bonjour connects on IPv4. A link-local address used to leave that server unable to connect.
- Home hides a shelf that has nothing on it. On Apple TV, focus no longer jumps back to Watch on every refresh.
- A lost server shows on the picture, not only behind the player.
- Opening the full player again keeps the picture that is already playing.
- On Apple TV, Down from Now in the guide reaches the first channel.
- Setup's signal check no longer says the tuners are busy while it only borrowed one for the guide.
- Diagnostics no longer says the time zone is unset on a normal host clock, shows the last failed guide pull, and no longer prints a channel number twice.
- A picture nobody is fetching stops holding a slot, so the next layout does not wait about a minute for it.
- VoiceOver reads a channel, a recording, and the mini player as one sentence.
- On the web, a click on Mute or Info no longer leaves the player controls up. Focus inside fading controls returns to the picture, and Tab on a faded player still brings the controls back.
- The guide's Now button scrolls to half an hour before the current show, then puts the cursor on the show that is on.
- Start over, skip back, and the slider are no longer undone by the saved resume point. A seek you just made wins.
- Right after setup, Back from the player opens the guide, because the page under the player is still the wizard.
- On Apple TV, Diagnostics no longer leaves the sidebar open. The first Fix these row takes focus, including after the notes load.

### Changed

- Settings shows the folder recordings actually use, and how to mount another one. A path typed in the app was ignored.

## 0.11.9 — 2026-09-27

Apple TVs in one room play in step.

### Fixed

- Apple TVs in one room now land within a few hundredths of a second of each other. Whole-Home Sync plays 16 seconds behind the broadcast instead of 13: at 13 an Apple TV that started behind the room could not catch up and sat a third of a second off the others.
- Typing a channel number waits until only one channel matches, you press Enter, or you pause. It used to tune on the first digit.
- When one guide source does not answer, listings still come in from the others you set.
- An Apple TV that could not speed up to catch the room tries again after a minute, not five.

## 0.11.8 — 2026-09-27

Live TV keeps playing when Plex or another app is busy on the same GPU.

### Fixed

- On a server where Plex, Jellyfin, or Channels DVR transcodes on the same Intel GPU, a 1080i channel could encode slower than real time. Apple TVs drained their buffer, paused under the progress bar, and the broadcast broke up. Broadwave now measures the CPU at startup and runs live TV on it when a 1080p60 encode runs at about 2.5 times real time or faster, leaving the GPU to the other apps. Set `BROADWAVE_ENCODER=gpu` or `software` to choose yourself.
- An Apple TV that fell half a second behind the others seeked every two seconds and froze each time. A seek that doesn't land now waits a minute and the TV plays on (with the next app update).
- An Apple TV that sat a quarter second behind the others jumped every 15 seconds trying to close a gap it could not. It now plays on, and seeks only when it is much further behind.
- A channel with no signal says it isn't coming in, in about 6 seconds when the channel is already known and about 10 the first time, and gives the tuner back. It used to spin for about 17 seconds and then say the picture stopped.
- On iPhone and Apple TV, a busy tuner, a tuner that does not answer, a dark channel, and a server that has stopped use the same words as the web, and the picture comes back when that clears.
- With only playlists, a stall no longer says the tuner did not answer and then never tries again.
- Diagnostics says when a container on the bridge network has a tuner the apps still cannot find.
- A signal check no longer says both tuners are busy when the device has more than two.

## 0.11.7 — 2026-09-27

Apple TVs play smoothly: no freeze a few seconds into a channel, and no catch in voices.

### Fixed

- An Apple TV that started a channel played a few seconds, then froze for about five while the room settled. It now waits once on its first picture and plays straight through.
- Apple TVs nudged their speed to hold sync over differences nobody could hear, and each nudge held a frame and clipped speech. They now leave anything under 60 ms alone, wait longer between nudges, and stretch sound smoothly when they do (with the next app update).
- Apple TVs report their playback health to the server log every 10 seconds, so a stutter on a real TV can be traced without a Mac attached (with the next app update).
- When the broadcast clock steps backward, the picture keeps going. It used to freeze every screen until a channel change.
- A short gap where a break restarted the encode no longer freezes the web picture for about 2 seconds.
- A browser tab that was frozen or hidden comes back on the room's frame.
- Changing channels quickly no longer waits out the tune you already left.
- An Apple screen leaves the room only when you pause. A pause the player makes on its own no longer sticks until Back in sync.

### Changed

- The install notes explain the latest image tag, an Unraid install, and that an update keeps your folders.

## 0.11.6 — 2026-09-27

Apple TVs keep playing through a garbled broadcast, and stay together from the start.

### Fixed

- One damaged frame from the antenna could stop every Apple TV and iPhone on that channel until someone changed channels. The server now drops a frame whose timing is far off and keeps the rest.
- Apple TVs that joined a channel a browser had just started sat about a tenth of a second apart for the first few minutes. They now line up right away (with the next app update).
- The player shows a clear message when a channel loses its signal, the recordings folder can't be written, or a phone loses its connection, and plays again on its own when that's fixed.
- A recording that would cross the free-space reserve says how much space is left and how much Broadwave keeps.
- Pause on iPhone and Apple TV pauses live TV. It used to start again within a quarter second. Back in sync returns that screen to the room.
- A guide, search, sports, or recordings link while a channel is up closes the player on Apple TV and shrinks it on iPhone, so the page opens.

## 0.11.5 — 2026-09-27

Optional automatic updates that wait until nobody is watching or recording.

### Added

- An optional updater for Docker Compose and Unraid keeps Broadwave on the newest release. It checks nightly at 03:30 and skips a night while someone is watching, a recording is running, or a recording starts within two hours. See "Automatic updates" in the README.
- `broadwave -update-check` reports whether the running server is busy, for any updater that can run a check first.

## 0.11.4 — 2026-09-27

Multiview and single screens stop stalling at the live edge.

### Fixed

- A multiview tile of a 60 frames-a-second channel could freeze for 15-20 seconds at a time. Its segments now stay short.
- A browser alone on a channel whose picture arrives late or unevenly stalled again and again near live. After a stall, that screen and multiview now step back from live, a little at a time, until the picture holds.

### Changed

- The install notes cover Docker Desktop, where the tuner address has to be entered by hand. Setup asks for that address above the playlist form.

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
- On Apple TV, Guide, Search, and Settings no longer leave the sidebar open over the page.
- A browser alone on a channel stays at normal speed. Easing it back from live was dropping frames in Chrome.
- On the phone, the guide can jump between now and tonight. Arrow keys move to the nearest control.

## 0.11.2 — 2026-09-27

Sound and picture line up in every browser, and Safari plays live TV again.

### Fixed

- In Chrome and other browsers the sound could play up to a second late. The server now places each track's start inside the stream itself, not only in a table that some browsers skip.
- Safari showed no picture on live TV. It plays now, and the sync between screens corrects Safari by seeking instead of changing its speed, which made it stall.
- A new screen no longer pauses and then jumps when it joins a channel.

### Added

- On iPhone and Apple TV, Settings can add and scan tuners and playlists, and add a link, a folder, or a playlist file.
- Channels can be favorited, renamed, renumbered, matched to the guide, or hidden. A series pass can change its padding, priority, which episodes, and how long to keep it.

## 0.11.1 — 2026-09-26

Live TV no longer freezes a few seconds after it starts.

### Fixed

- A few seconds into a channel the picture froze for about five seconds while the server moved everyone to the shared delay. Screens now ease onto that delay by playing a little slower for a few minutes, so nothing pauses.
- A channel starts with a short buffer instead of at the very edge of the broadcast, which stopped the brief stalls in the first minute.

### Added

- On iPhone and Apple TV, Settings can save a guide account and the free-space reserve, list and restore a catalog backup, and open Diagnostics (tuner health, guide depth, and the last antenna reading). Picture, hiding scores, autoplay, and tuner sharing are there too.

## 0.11.0 — 2026-09-26

Live playback keeps one timeline across renditions, rewinds about ninety minutes, and continues when the broadcast clock jumps.

### Fixed

- A channel the server already knows opens on the frequency it stored, and playback can start on the first segment that is ready.
- A copied broadcast and a transcode of it cut on the same pictures.
- When the broadcast clock jumps, that segment closes and the next one continues. The time the player shows stays on the wall clock.
- Rewind reaches about ninety minutes of the broadcast.
- A multiview tile shows a picture as soon as it has one, instead of staying black.
- A channel the server already knows starts before its sound scan finishes.
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
