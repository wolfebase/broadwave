# Changelog

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
