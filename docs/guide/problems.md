# Common problems

These are the situations people run into, with the cause and what to do. The sentence on the player is in [Troubleshooting](troubleshooting.md). Where this page says to update, that release already has the fix. A newer release has it too.

## Setup finds no tuner

**Docker Desktop.** Setup stays on "No tuner answered yet." The server finds a tuner by a broadcast, and Docker Desktop's host network is a virtual machine, so that broadcast never arrives. Publish port 8477 and set `HDHR_HOST` to the tuner's address. [Install](install.md) has the command. An address that does not answer says "No HDHomeRun answered at that address. Add the port if it is not 80."

**The container is on the bridge network.** Diagnostics says "Broadwave can't see your tuner from inside Docker. Switch the container to host networking." When a tuner is already stored, the line is "Phones and TVs can't find this server on their own from inside Docker. Switch the container to host networking." On Linux, use host networking. On Docker Desktop, set the tuner address instead.

**The browser says the server does not answer to its name.** The page names the host it saw and tells you to set `BROADWAVE_HOSTS` to that name. The status is 421. An address, a one-word name, and a home name work without that setting. A public name on port 8477 does not, until the setting lists it. A leading dot covers the subdomains of that name. [Security](security.md) is the rule, and [Away from home](remote.md) is the Tailscale case. The check has been there since 0.12.1. Since 0.13.0 it also covers port 8478, the HDHomeRun port this server can act as.

## A tuner stops answering

While you are watching, the line is "This tuner did not answer. Check that it is on." On the tuner's card, 15 minutes with no answer reads "Offline. Last seen" and then when. Remove is the first control. Diagnostics says "Your tuner stopped answering. Check that it is plugged in." after about three minutes when nothing is tuned. Check the power and the network cable. Remove the card if the tuner is gone.

A signal check uses the same busy sentence as a tune that cannot get a tuner, and it answers at once. It does not wait. It refuses while something is recording, while a recording starts within 30 minutes, or while a tuner is already in use.

Before 0.12.2, a channel whose tuner was unplugged said the tuner did not answer even when another tuner carried that channel. Update to 0.12.2 or later.

Before 0.12.17, two tuners on one antenna listed every channel twice. Update to 0.12.17 or later. The guide shows each channel once. The other tuner still carries it when the first is busy.

## No signal, or the tuner refuses the channel

"This channel isn't coming in. Check the antenna." The tuner answered and the channel did not lock. The tuner is released. Check the antenna and the cabling, then try again.

Before 0.12.16, one channel with no signal could stall live TV, a recording, or another screen for the length of that failure. A multiview tile on a dead channel could freeze the good tiles, which 0.12.15 stopped. Update to 0.12.16 or later. The wait before the message is shorter as of 0.12.20.

"The tuner would not start this channel. Try again." The tuner answered and rejected the tune, often because another app holds it. Stop that app, or watch a channel that is already on.

## The picture

**An ATSC 3.0 channel has no sound.** The player says "No sound: this server's ffmpeg can't decode ATSC 3.0 sound (AC-4)." On the web, Diagnostics says "This ffmpeg can't decode AC-4, so ATSC 3.0 channels play without sound." The Docker image includes that decoder. Before 0.12.6 the channel stopped after about a second and a half. Update to 0.12.6 or later, or run the Docker image.

**An ATSC 3.0 channel is encrypted.** The player says "The 3.0 version is encrypted. Showing the regular broadcast." Broadwave does not decrypt it. Since 0.13.0, Plex, Jellyfin, and Channels play that station's regular broadcast the same way, when there is one.

**The server will only play so many pictures.** The sentence uses this server's number. One picture says "This server can play 1 picture at once. Stop it to watch another." More than one says how many, and "Stop one." Stop another picture to free a slot. The number comes from a short test encode at startup.

**Live TV breaks up while Plex, Jellyfin, or Channels transcodes on the same Intel graphics.** Before 0.12.6, a 1080i channel could encode slower than real time, and Apple TVs drained their buffer. Update to 0.12.6 or later. On an Intel GPU, live TV uses that chip's low-power encoder, which those transcodes do not slow down. On other graphics, a processor that can keep up takes live TV off the shared GPU. Set `BROADWAVE_ENCODER` to `gpu` or `software` to choose yourself.

**Apple stays on "Still tuning."** A station that sends a keyframe only every two seconds or more used to leave iPhone, iPad, and Apple TV on "Still tuning. A weak signal can take longer", and then say the picture stopped. Update to 0.12.10 or later. A room with an Apple screen on such a channel plays at Stable, 20 seconds behind live.

**One damaged frame stopped every Apple screen.** Before 0.11.6, one bad frame from the antenna stopped every Apple TV and iPhone on that channel until someone changed the channel. Update to 0.11.6 or later.

**Live TV never started.** Before 0.13.0, a channel that sent a run of short keyframes never began. Update to 0.13.0 or later.

## Recordings

**The disk is low, or the folder will not take a file.** A recording that would cross the reserve names the free space and the reserve. Settings calls the reserve "Keep this much free". The default is 10 GB. Set it to 0 to turn it off. Diagnostics warns earlier, with "Less than 20 GB is free. Free some space before a long recording." A disk with no room left says "The recordings disk is full. Free some space, then try again." A folder that refuses the write says "Broadwave can't save this recording. Check the recordings folder, then try again."

**A recording file was moved.** On the web the line is "The file is gone. It was moved or deleted outside Broadwave." On iPhone, iPad, and Apple TV the channel number comes first. A finished recording you move inside the recordings folder is found again when the notes file beside it moves with it. The server looks about a minute after it starts, then about once an hour. The same set of missing files is left until the next day. A file moved outside that folder stays on the list and does not play.

**The next half-hour of the same show did not record.** Before 0.13.0, a finished episode blocked the next one with the same title for an hour. Update to 0.13.0 or later.

**A recording skips fades, or finds almost no breaks.** Before 0.12.20, and without comskip, every short black stretch counted as a commercial. Update to 0.12.20 or later. The Docker image includes comskip as of 0.13.0.

**Two recordings that started together wrote one file.** Update to 0.13.0 or later.

**A recording would not delete.** The web says "The recording could not be deleted. It is still in your library." The file is still there, so the row stays and a later delete can try again.

**Updating the catalog stopped the server.** Before 0.13.0, a database update stopped the server when something else was writing the catalog. Update to 0.13.0 or later.

## Other apps

**Jellyfin asks this server to transcode.** The port Broadwave opens as an HDHomeRun calls itself an HDHR4-2US, a model that does not transcode. Update to 0.13.0 or later if Jellyfin still asks for a transcode. Turn on Act as an HDHomeRun under Settings, Share with other apps, and add the address with port 8478. That port does not answer tuner discovery, so type it. Leave it off when you do not use those apps.

**Channels asks you to match the guide after an update.** Since 0.13.0, a channel's channel-id in the shared playlist is the id the XMLTV guide uses, not the channel number. Match the channels once.

**Plex or Channels would not play a channel.** Before 0.13.0 they asked for it by number on paths the server did not answer. Update to 0.13.0 or later, and type the address with port 8478.

**A channel name with a quotation mark cut the shared playlist short.** Update to 0.13.0 or later.

## The apps

**The app found the server and could not open it.** Type the address. On iPhone and iPad the heading is "Or enter an address". On Apple TV the button is "Enter an address". Then Connect. With no port, the app uses 8477. If device sign-in is on, a new browser needs a code, and the iPhone, iPad, and Apple TV apps cannot enter one. Leave "Require a device to sign in" off while you use the apps. [Away from home](remote.md) is the rest.

**Only one of two servers on the same computer showed up.** Before 0.12.13, the one that started second was missed when Bonjour could not reach the app. Update the apps and the server to 0.12.13 or later.

**The first play of a recording started over.** Before 0.13.0, on iPhone, iPad, and Apple TV, the first play started over while its playlist was still being built. Update to 0.13.0 or later.

## Watching together

Screens that sit well behind the broadcast are on the room's delay, not a stuck buffer. Lowest is 6 seconds, Balanced is 16, and Stable is 20. Pick it for the room from the player's Options, or set a default per device in Settings. An Apple screen is not left on Lowest. In a room with an Apple screen, a channel whose pictures come in long groups uses Stable. [Sync](sync.md) is the rest.

Before 0.11.9, Apple TVs in one room could sit a third of a second apart and never meet. Update the app and the server to 0.11.9 or later.

Before 0.11.8, an Apple TV that fell half a second behind seeked every two seconds and froze each time. Update the app and the server to 0.11.8 or later.

Before 0.13.0, on the web, a screen that was behind could jump past what had loaded, a paused group could start when you came back to the tab, and pausing could turn sync off. Update to 0.13.0 or later.
