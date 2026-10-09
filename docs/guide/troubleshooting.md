# Troubleshooting

The line on the screen is the whole message. It names what failed and what to try. The player lines below are the same on the web, iPhone, iPad, and Apple TV. The missing-file line is not: on the Apple apps the channel number comes first.

## Lines you will see

**Every tuner is busy.** "Every tuner is busy. Stop a recording or watch something already on." Stop one of those, or watch a channel that is already tuned. Subchannels of one station share a tuner. A multiview tile names the channels that are on, such as "Both tuners are busy. 4.1 and 5.1 are on."

**No signal.** "This channel isn't coming in. Check the antenna." The tuner answered and the channel did not lock. Check the antenna and the cabling, then press Try again. The tuner is already free.

**The server.** "The server stopped. It will try again when it's back." The app can reach the network and cannot reach Broadwave. Leave the player up. "The connection dropped. It will try again when it's back." means the phone or the computer itself is offline.

**The picture.** "The picture stopped. Trying again usually fixes it." The player starts the channel again on its own. "The picture stopped. Starting it again." means the server no longer has this picture, usually because it restarted.

**The file.** On the web the line is "The file is gone. It was moved or deleted outside Broadwave." On iPhone, iPad, and Apple TV the channel number comes first. The row stays so you can see the title. It does not play. Remove from the list drops it.

**Encrypted 3.0.** "The 3.0 version is encrypted. Showing the regular broadcast." Broadwave plays and records the regular channel. [Tuners](tuners.md) explains why.

**A tuner that stopped answering.** While you are watching: "This tuner did not answer. Check that it is on." On its card in Settings: "Offline. Last seen 15 minutes ago." Remove is the first button. A tuner added by its address keeps reading as online while it answers.

**Setup finds nothing.** "No tuner answered yet" while it is idle, and "Searching…" while it looks. An address that does not answer says "No HDHomeRun answered at that address. Add the port if it is not 80." On Docker Desktop, set `HDHR_HOST`. [Install](install.md) has the command.

**The disk.** "The recordings disk has 4.2 GB free, and Broadwave keeps 10 GB in reserve. Free some space or lower the reserve in Settings." Those two numbers are this server's. A show already recording is left alone.

## Diagnostics

Open Diagnostics from Settings. It is a snapshot of this server: the version, the tuners and who holds them, whether each tuner is locked, what is tuned right now, how long the last channels took to start, the encoder, free space, how much of the guide is filled, and the recent log.

Fix these is one line per thing to change, and it is absent when nothing needs a change. One of those lines is "Broadwave can't see your tuner from inside Docker. Switch the container to host networking." Another is "Less than 20 GB is free. Free some space before a long recording."

When this ffmpeg cannot decode AC-4, Diagnostics says "This ffmpeg can't decode AC-4, so ATSC 3.0 channels play without sound." The Docker image includes that decoder.

## Logs and a bug report

The server writes its log to the process's standard error. Diagnostics shows the recent lines.

- **Docker.** `docker logs broadwave`, using the container name from the install command.
- **Unraid.** Open the Broadwave container from the Docker tab and choose Logs.
- **A Mac running the server in Terminal.** The log is that window.

In Settings, under Support, choose "Download a support bundle". The hint is "Logs, versions, and settings. Passwords are left out." The same file is at `http://<your-server>:8477/api/v1/support`. Attach it to an issue at [github.com/wolfebase/broadwave/issues](https://github.com/wolfebase/broadwave/issues).

Every line, including the ones only Diagnostics uses, is in [Messages](../troubleshooting.md).
