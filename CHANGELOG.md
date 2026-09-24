# Changelog

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
