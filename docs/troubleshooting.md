# Troubleshooting

The line on the screen is the whole message. It names what failed and what to try. The same words are used on the web, iPhone, iPad, and Apple TV.

## Every tuner is busy

"Every tuner is busy. Stop a recording or watch something already on."

A recording or another channel already has each tuner. Stop one of those, or watch a channel that is already on. Subchannels of one station share a tuner, so a second subchannel of a station that is already tuned still plays.

A multiview tile that cannot start names what is already on:

- "The tuner is busy."
- "Both tuners are busy."
- "Every tuner is busy."
- "No tuner is available."

The rest of that line names the channels that are on, such as "Both tuners are busy. 4.1 and 5.1 are on."

A signal check uses that same sentence while a tuner is busy, while something is recording, or while a recording starts within 30 minutes.

## This channel isn't coming in

"This channel isn't coming in. Check the antenna."

The tuner answered and the channel did not lock. Check the antenna and the cabling, then press Try again. Broadwave has already freed the tuner.

## The server stopped

"The server stopped. It will try again when it's back."

The app can reach the network and cannot reach Broadwave. The picture comes back on its own once the server answers. Leave the player up.

"The connection dropped. It will try again when it's back."

The phone or the computer itself is offline. The picture comes back when that connection does.

## The picture stopped

"The picture stopped. Trying again usually fixes it."

The server is up, the tuner answers, and nothing else names a cause. A playlist whose stream died uses this same line. The player starts the channel again on its own, about every 10 seconds for two minutes. The message stays until the picture moves. After that, Try again is there.

"The picture stopped. Starting it again."

The server no longer has this picture, usually because it restarted. The player asks for it again as soon as the server answers.

A server that can only encode so many pictures at once says "This server can play 4 pictures at once. Stop one." The number is this server's limit. With room for one it says "This server can play 1 picture at once. Stop it to watch another." Stop another channel or another tile. Diagnostics shows the limit.

A playlist that has every stream it allows open says "All 2 streams from this playlist are in use. Stop one or raise the limit." The number is that playlist's limit.

## The file is gone

"The file is gone. It was moved or deleted outside Broadwave."

The recording finished, and the file is no longer in the recordings folder. It stays in the list so you can see the title. It does not play. On Apple the channel number comes first: "4.1 · The file is gone. It was moved or deleted outside Broadwave." Remove from the list drops the row. There is no file left to delete.

## An encrypted 3.0 station

"The 3.0 version is encrypted. Showing the regular broadcast."

The station's 3.0 channel is encrypted, so Broadwave plays and records the regular broadcast. Search, a link, and a multiview tile that name the encrypted channel all do this. The encrypted channel stays off the guide.

On the web, Settings reads "Encrypted (ATSC 3.0 DRM). Plays and records 5.1, the same station in ATSC 1.0." The number is that station's regular channel. On iPhone, iPad, and Apple TV the same row reads "Encrypted (ATSC 3.0 DRM). It plays and records the same station in ATSC 1.0." A station with no regular broadcast reads "Encrypted (ATSC 3.0 DRM). Only the tuner maker's app can play it." on every client.

## A tuner that stopped answering

While you are watching: "This tuner did not answer. Check that it is on."

The picture comes back on its own once the tuner answers. Check power and the network cable.

On its card in Settings › Tuners and channels: "Offline. Last seen 15 minutes ago." With no time to show, the card says "Offline." Remove is the first button. A tuner that still answers leaves this line off. A discovered tuner is marked offline after it misses three checks, about fifteen minutes. A playlist uses the same sentence when a refresh of that playlist fails.

Diagnostics › Fix these, when the check applies: "Your tuner stopped answering. Check that it is plugged in."

"The tuner would not start this channel. Try again." means the tuner answered and refused the channel, often because another app holds it.

## Setup finds nothing

Setup with an empty lineup says "No tuner answered yet." while it is idle, and "Searching…" while it looks. Settings with no tuner yet says "No HDHomeRun has answered yet."

An address that does not answer says "No HDHomeRun answered at that address. Add the port if it is not 80."

Look harder, when this network and SiliconDust's list both come back empty, says "Nothing else answered." Your home, before anything new answers, says "Nothing else answered yet."

Enter the tuner's address, or run Search the network from Settings › Tuners and channels. On Docker Desktop, set `HDHR_HOST` to that address. The README has the command.

## A few more lines

"This channel did not start. The server log says why." The start failed for a reason the log has. Diagnostics shows the recent lines, and the support bundle includes them.

"This channel did not start. Try again." The answer was a status page or a system message.

"That did not work. Try again." A button failed the same way.

"No source has this channel now. Check Sources in Settings." Nothing enabled still carries that channel.

"No listing for this channel." The guide has no row for what is on. After you check and it is still empty: "Still no listing for this channel."

"The recordings disk has 4.2 GB free, and Broadwave keeps 10 GB in reserve. Free some space or lower the reserve in Settings." Those two numbers are the free space and the reserve on this server. A show already recording is left alone.

"The recordings disk is full. Free some space, then try again."

"Broadwave can't save this recording. Check the recordings folder, then try again." The folder refused the write. On Docker and Unraid that folder is the one mounted at `/config/work/recordings`.

## Diagnostics

Open Diagnostics from Settings. The page is a snapshot of this server:

- The server name, version, and how many apps are connected.
- Tuners. Each one is Free, or it names the channel, who tuned it, the signal, and how many people are watching.
- Tuner health. Model, firmware, and whether each tuner is locked. "This app does not install firmware." When none answer: "No tuner answered."
- Relay. What is tuned right now, including "Nothing is tuned right now." A full buffer reads "Buffer paused: the disk is nearly full."
- Channel starts. How long the last channels took to reach a picture. "No channel has started since the server did."
- Encoding and storage. The encoder, ffmpeg, free space, and how much of the guide is filled. When this ffmpeg cannot decode AC-4: "This ffmpeg can't decode AC-4, so ATSC 3.0 channels play without sound."
- Fix these. One line per thing to change. It is absent when nothing needs a change. The lines include "Broadwave can't see your tuner from inside Docker. Switch the container to host networking." and "Less than 20 GB is free. Free some space before a long recording."
- Feeds. Channels on now, who is watching, and how many ffmpeg processes.
- Logs. The recent log lines. "No log lines yet." before the server has written any.
- Recent activity. Recordings, sources, and other events, with the time.

## Where the logs are

The server writes its log to the process's standard error. Diagnostics shows the recent lines. The support bundle carries the same lines.

- **Docker.** `docker logs broadwave`, using the container name from the install command. On Docker Desktop for Mac, that is the same command.
- **Unraid.** Open the Broadwave container from the Docker tab and choose Logs. That is the same stream.
- **A Mac running the server in Terminal.** The log is that terminal window.

## Filing a bug

In Settings, under Support, choose "Download a support bundle." The hint under the button is "Logs, versions, and settings. Passwords are left out." The same file is at `http://<your-server>:8477/api/v1/support`. The Apple apps use the same button.

The download is a zip with four files: `versions.json`, `doctor.json`, `config.json`, and `logs.txt`. Passwords and tuner credentials are left out. Attach it to an issue at [github.com/wolfebase/broadwave/issues](https://github.com/wolfebase/broadwave/issues).
