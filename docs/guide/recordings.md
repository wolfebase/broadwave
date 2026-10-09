# Recordings and storage

A recording is the broadcast itself, saved on your server. Watching it later does not change that file.

## Where they go

In Docker and on Unraid the folder is the one you mount at `/config/work/recordings`. Settings, Recordings folder, shows the path. To keep them on another disk, mount that disk there, or point `BROADWAVE_RECORDINGS` at it. Outside Docker, `-recordings` does the same. Recordings already made stay where they are.

Each recording is one MPEG-TS file, filed by show in the layout Plex and Jellyfin read:

```text
TV/Harbor Watch/Season 01/Harbor Watch - S01E02 - The Lighthouse.ts
Movies/Night Flight (1999)/Night Flight (1999).ts
```

The file is the station's program, copied, not re-encoded. A clear ATSC 3.0 channel with AC-4 sound keeps the program's own packets, so every sound track and the captions stay as the tuner sent them.

When the recording ends, a `.json` file with the same name sits beside it, with the title and the times. Commercial breaks, when any are found, go in a matching `.edl`. The Docker image includes comskip, which looks after the file is closed. A server without comskip, such as one on a Mac, looks for black frames instead, and finds fewer breaks.

Turn on "Write .nfo files for Plex, Jellyfin, and Kodi" and a finished recording also gets an `.nfo` beside it. The switch is off until you turn it on.

## What you can record

Record this airing, record the series, or record every game for a team you follow.

A series pass can be one channel or any channel, new episodes only, certain days, and a time of day. Padding is minutes before the listing and after it. While you set a pass up, the schedule shows what it would record in the next two weeks. Passes higher in the list win when two shows overlap and a tuner is short. The schedule offers a later airing when one fits.

A game that is matched to a live score keeps recording until the game is over, in short steps, and stops a few minutes after it ends. Turning live scores off leaves the recording at the time the guide gave it.

A show that is already on can still be recorded from its beginning, when the channel has been tuned and the buffer still holds the start. Settings, Keep for recording from the start, is how long that buffer is. The choices are Off, 30 minutes, 1 hour, 2 hours, and 4 hours. One hour is the usual choice.

A pass claims its tuner 90 seconds before the listing, or earlier when the pass has padding. Watching inside that window asks first. The recording still takes the tuner when it starts, unless that tuner is already recording something else.

## Disk space

Settings, Keep this much free, is the reserve. It is 10 GB unless you change it. Zero turns it off.

A new recording is refused while free space is under that reserve. The message names the free space and the reserve. A show that is already recording keeps going.

Settings, When space runs low, is "Skip new recordings" unless you change it. "Delete the oldest watched" deletes watched recordings, the one watched longest ago first, before a new recording would be skipped. It does not delete an unwatched recording, one still recording, or one you marked Keep forever.

Settings, Delete watched recordings, deletes a recording a number of days after it was watched. It is off unless you pick a number. Played to the end, or marked watched, counts. Stopping a few minutes early does not.

## Playing and deleting

Open a recording from Recordings or from Home. It starts again where you left off. While it is still recording, you watch the file as it grows. Download, on the web, appears after the recording has finished and saves the MPEG-TS.

Delete removes the recording and the files beside it inside the recordings folder. Stop it first if it is still recording. A file that was moved or deleted outside Broadwave stays in the list with the line "The file is gone. It was moved or deleted outside Broadwave." Remove from the list drops that row. There is no file left to delete.

The folder layout, pass rules, rename, and the `.edl` format are in [How recordings are stored](../recordings.md).
