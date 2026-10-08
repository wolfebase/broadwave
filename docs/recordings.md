# Recordings

A recording is the broadcast itself, saved on your server. Watching it later does not change that file.

## Where they go

Recordings live in the recordings folder. In Docker and on Unraid that folder is the one you mount at `/config/work/recordings`. Settings › Recordings folder shows the path. To keep them on another disk, mount that disk there, or point `BROADWAVE_RECORDINGS` (or `-recordings` on a Mac or Linux install) at it. Recordings already made stay where they are; move them yourself if you change it.

Each recording is one MPEG-TS file:

```text
20261005_193000_4.1_KBWV.ts
```

The name is the time it started, the channel number, and the channel's name. A character a file name cannot hold becomes a hyphen. Two recordings that start in the same second get `-2`, `-3`, and so on.

The file is the station's program, copied, not re-encoded. A clear ATSC 3.0 channel with AC-4 sound is the program's own packets, so the picture, every sound track, and the captions stay as the tuner sent them. Any other channel is that program's picture and its first sound track, still the original encoding.

When the recording ends, a `.json` file with the same name sits beside it. It holds the id, title, channel number, status, and the start and end times.

Commercial breaks, when any are found, go in a matching `.edl`. Each line is a start and an end, in seconds. Broadwave looks after the file is closed, with comskip, which the Docker image includes. A server without comskip, such as one on a Mac, looks for black frames between ads instead, which finds far fewer breaks. A series pass looks unless Commercials is off on that pass. A recording with no pass looks too. comskip's own notes stay in a temporary folder. The recordings folder gets the `.edl` only.

Turn on "Write .nfo files for Plex, Jellyfin, and Kodi" and a finished recording also gets an `.nfo` in that same folder, with the title and description. The switch is off until you turn it on. Those files are never written outside the recordings folder.

A poster, when there is one, is kept with the server's other artwork, not beside the recording.

## Passes

Record this airing, record the series, or record every game.

A series pass records the show whose title matches, on one channel or any. A pass can also match words anywhere in a title ("Chiefs") or a guide category ("Sports"). It can be limited to new episodes, to days of the week, and to a time of day; a window that runs past midnight counts as the night it began. While you set a pass up, the schedule shows what it would record in the next two weeks and anything it would stop from recording. Padding is in minutes before the listing and after it. A new pass keeps every episode. You can keep only the ones you have not watched, or the last few: when a new episode finishes, older finished ones of that show are removed. The last few, with no number set, keeps the newest one. A recording that is still in progress is left alone. An episode already in the library is not recorded again, unless you deleted it and the pass is set to record it again. A limit stops new ones once that many are unwatched.

Record this airing is a one-time pass for that channel and that start time. It wins over a series pass for the same showing, and it records even when that episode is already in the library. The one-time pass is removed a day after the showing ends.

Record every game follows a team on any channel. The listing matches when the title or the episode name mentions the team. It starts one minute early and runs two minutes past the listing. The other channel of a simulcast is not recorded again.

When two shows overlap and there are not enough tuners, the pass lower in the list is skipped. Drag passes, or use Up and Down, to change the order. The schedule offers "Record the later airing" when the same show has a later showing that fits. "Skipped once" is a showing you already moved. "Already recorded" and "Limit reached" are the library rules.

A sports listing that is not matched to a live game records an hour past the guide. While live scores are on, a matched game keeps going in ten-minute steps for as long as the game is on, or while it is late to start, and stops eight minutes after it is over. The time chosen at the start is at most eight hours. The extra steps are what carry a long game. Turning live scores off leaves the recording at the time it was given.

A show that is already on can still be recorded from its beginning, when the channel has been tuned and the buffer still holds the start. Settings › Keep for recording from the start is how long that buffer is. The choices are Off, 30 minutes, 1 hour, 2 hours, and 4 hours. One hour is the usual choice.

## When every tuner is busy

A recording of a channel shares that channel's tune with everyone watching it. Subchannels of the same station share it too.

A pass claims its tuner 90 seconds before the listing, or earlier when the pass has padding. While that claim is waiting, the same number of free tuners are held back from a new channel, so the recording still has one.

Watching inside that window asks first. The line is "<title> starts at <time>. Watching stops when that recording starts." Several recordings at once name the count. Watch anyway plays the channel now. The recording still takes the tuner when it starts.

If every tuner is in use when the recording starts, it takes one that only has people watching, and those channels stop. The schedule records "<number> stops at <time>. <title> is recording." Multiview shows that same line on the tile that will be taken. A tuner that is already recording is not taken.

When nothing can be taken, the message is "Every tuner is busy. Stop a recording or watch something already on."

## Disk space

Settings › Keep this much free is the reserve, 10 GB unless you change it. Zero turns it off.

A new recording is refused while free space is under that reserve. The message is "The recordings disk has <free> free, and Broadwave keeps <reserve> in reserve. Free some space or lower the reserve in Settings." A disk with no room left says "The recordings disk is full. Free some space, then try again." A show that is already recording keeps going.

The live buffer is what uses space first, and what gives it up first. It may hold at most half the free space. On the same disk as the recordings it also leaves the reserve, and another 4 GB. Under that floor it drops its oldest pieces and waits until there is room. Recordings are not deleted to make that room. A keep rule removes older episodes of one show only after a new episode of that show finishes.

Settings › When space runs low is "Skip new recordings" unless you change it. "Delete the oldest watched" deletes watched recordings, the one watched longest ago first, before a new recording would be skipped, and checks once an hour too. It stops once the reserve is back. If deleting every watched recording would not be enough, it deletes none and the recording is skipped.

Settings › Delete watched recordings deletes a recording a number of days after it was watched. It is off unless you pick a number.

Both count a recording as watched once it was played to its last seconds or marked watched. Stopping a few minutes early does not count. A recording played in the last 6 hours stays, and playing one again starts its days over. Neither touches an unwatched recording, one still recording, a file outside the recordings folder, or a recording you chose to keep forever (Keep forever on its row). A keep rule skips a kept recording too, and doesn't count it among the episodes it keeps. Each deletion is in Activity.

## Playing one

On the web, open it from Recordings or from Home. It starts again where you left off, once that spot is a couple of seconds in and already in the file. While it is still recording, you watch the file as it grows and can move inside the part already saved. With skip set to automatic, a commercial mark is jumped. Download appears after the recording has finished and saves the MPEG-TS. The browser writes it straight to disk.

On iPhone, iPad, and Apple TV the same lists open the player. It resumes when you were more than five seconds in. Skip break is on the player, and the place you reached is saved.

Playback is a stream the device can play. The file in the recordings folder stays the broadcast.

A finished recording whose file was moved or deleted outside Broadwave stays in the list. The line is "The file is gone. It was moved or deleted outside Broadwave." Remove from the list drops that row. There is no file left to delete.

When the picture broke up, the row says how many times. A clean recording says nothing.

## ATSC 3.0

A clear 3.0 recording is the program's own packets when the sound is AC-4, as [ATSC 3.0](atsc3.md) describes. The Docker image includes the decoder playback needs. A server built without it still shows the picture, with no sound.

An encrypted 3.0 station records the regular broadcast. Scheduling it, or opening one of its shows, records and plays that broadcast. The player says "The 3.0 version is encrypted. Showing the regular broadcast."

## Deleting

Stop a recording before you delete it. Delete, then "Delete this file", removes that recording. Inside the recordings folder it removes the `.ts`, the `.json`, the `.edl`, and the `.nfo`, plus the poster and the temporary files playback made. A file that is not in the recordings folder is left where it is. A keep rule removes the same files when it drops an older episode.

Remove on a tuner never does this. The confirm is "Remove <name>? Its channels leave the lineup. Favorites and passes move to another tuner with the same channel, and the other passes go. Recordings stay." The files stay. A tuner that is playing or recording stays too, until you stop that. [Tuners](tuners.md) covers bringing it back.
