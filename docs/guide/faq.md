# FAQ

## Does it cost anything?

No. Broadwave is free and open source. There is no Broadwave account, no subscription, and no ads. Over-the-air TV is free too. An antenna and a network tuner are how you get broadcast channels. A playlist plays without one. Schedules Direct, if you add it, is that service's account. It is stored on your server.

## Where does the picture come from?

From your antenna. Broadwave does not provide, host, or stream any channels of its own. It plays the broadcasts your tuner receives, plus any playlist you add. It does not install tuner firmware.

## Which tuner should I get?

Any current HDHomeRun works, and Broadwave finds it on the network. A model number that contains 4K, including a FLEX 4K, receives ATSC 3.0 on its first two tuners. A FLEX 4K has four tuners. FLEX DUO, FLEX QUATRO, and a CONNECT whose model number does not contain 4K receive the regular broadcast. [Tuners](tuners.md) has the table.

Other network tuners and servers that speak HDHomeRun work by address. Leave the port off and Broadwave tries ports 80, 5004, 34400, 8409, and 9191. An address with no port that does not answer says "No HDHomeRun answered at that address. Add the port if it is not 80."

## What do I watch on?

A browser at `http://<your-server>:8477`, and the Broadwave apps on iPhone, iPad, and Apple TV, and a Cast television when the server has a Cast application id. The apps find the server on the same network. There is no Roku app. [Watching](watching.md) covers them.

The usual install is Docker. Building from source needs Go 1.26 or newer, Node 22 or newer, and ffmpeg. There is no separate download for a Mac or Linux. [Install](install.md) is the setup.

## Can I watch away from home?

Yes, over a VPN such as Tailscale. Do not count on the apps finding the server from there. Enter its Tailscale address and port 8477. Leave port 8477 and port 8478 off the public internet, and do not turn on Tailscale Funnel. There is no account password.

Device sign-in covers the guide, recordings, and settings inside Broadwave. The shared guide and the playlist stay open. A stream address opened directly can still start a tune with no code. A page on another site cannot. Port 8478 can too, while Act as an HDHomeRun is on. The iPhone, iPad, and Apple TV apps cannot enter a code. Leave "Require a device to sign in" off while you use them. With it on they still connect, but the server refuses the guide and playback with "Sign in from a paired device." [Away from home](remote.md) is the setup. [Security](security.md) is the name check.

## Can I watch more than one channel?

Yes. Side by side, one large picture with smaller ones beside it, or four at once, on the web, iPad, and Apple TV, when this server has room for that many pictures. The iPhone app shows two. One tile has the sound. [Multiview](multiview.md) is the layout list.

How many pictures can play at once comes from a short test encode at startup. It is not one number for every server. One picture says "This server can play 1 picture at once. Stop it to watch another." More than one names the count and says "Stop one."

## Do three rooms use three tuners?

No. Everyone on one channel shares one tune, including a recording of that channel. Subchannels of that station share it too. A second station takes another tuner. [Sync](sync.md) is how the rooms stay on the same moment.

When a recording is about to start, Broadwave holds a free tuner for it, so a new channel does not take the last one.

## How far behind live is the picture?

Settings, Live delay, is how far behind live a channel starts on that device. Lowest is about 6 seconds, Balanced is 16, and Stable is 20. iPhone, iPad, and Apple TV offer Balanced and Stable. A room with an Apple screen is not left on Lowest, and a channel whose pictures come in long groups uses Stable for that room. On the web, asking for Lowest while an Apple screen is in the room still shows "An Apple device is watching, so this channel stays at Balanced." On the web, sync is on unless you turn it off. On iPhone, iPad, and Apple TV, a channel starts right away, and Sync screens in the player lines it up. [Sync](sync.md) is the switch and Watch together.

## Where do recordings go?

In the recordings folder you mounted. In Docker that is `/config/work/recordings`, unless `BROADWAVE_RECORDINGS` points somewhere else. By default they are filed by show for Plex and Jellyfin. Settings, Recording folders, All in one folder, puts new files in one folder. Recordings already made stay where they are.

The picture and the first sound track are copied, not re-encoded. A tuner channel with AC-4 sound keeps the program's own packets, so every sound track and the captions stay as the tuner sent them. [Recordings and storage](recordings.md) covers passes and the disk reserve.

## Will a game record past the listing?

A game matched to a live score keeps recording until the game is over, in ten-minute steps, and stops eight minutes after it ends. The note is "The game is over. Stopping in 8 min." Live scores are on unless you turn them off. Off means a game is not extended as it runs. A sports listing that is not matched to a score, including when scores are off, records for an hour past the guide.

A show that is already on can still be recorded from its beginning, when the channel has been tuned and the buffer still holds the start. Settings, Keep for recording from the start, is Off, 30 minutes, 1 hour, 2 hours, or 4 hours. One hour is the usual choice.

## What if the disk is low?

Settings, Keep this much free, is the reserve. It is 10 GB unless you change it. Zero turns it off. A new recording that would cross it says how much is free and what the reserve is. A show already recording is left alone.

When space runs low is "Skip new recordings" unless you change it. "Delete the oldest watched" deletes watched recordings, the one watched longest ago first, when free space is under the reserve. It checks once an hour and before a new recording would be skipped. If deleting all of them would still leave the disk under the reserve, it deletes none. Delete watched recordings is off until you pick a number of days. Both count a recording as watched once it was played to the last 15 seconds or marked watched. Stopping earlier does not, even through 90% of a long recording. One you played in the last 6 hours stays, and so does one you marked Keep forever.

## Some channels have no guide.

The tuner guide does not list every channel. Broadwave refreshes it on its own, 20 to 28 hours after a successful pull. Add Schedules Direct, or paste an XMLTV address under Guide address. Schedules Direct asks for fourteen days, and tries today only if that request fails. The hint is "Fills channels the tuner guide skips. Fourteen days when the account allows it." While a channel is tuned, the broadcast's own listings fill times none of those already list. [Channels and the guide](channels.md) is the order.

## Will it get along with Plex, Jellyfin, or Channels?

Yes. Settings, Share with other apps, Act as an HDHomeRun opens port 8478. Type that address with the port. It does not answer tuner discovery. It calls itself an HDHR4-2US, a model that does not transcode, so Jellyfin does not ask for one. That port has no device code. Leave it off when you do not use those apps.

Recordings are the broadcast files. The usual folder layout, by show, is one those apps already understand. All in one folder is a single directory. Since 0.13.0, a channel's channel-id in the shared playlist is the id the XMLTV guide uses. Channels may ask you to match the guide once.

## Does it skip commercials?

The Docker image includes comskip. A server without it still looks for breaks. A break is skipped on its own only when the scan is sure. On the web, a less sure break shows Skip break. On iPhone, iPad, and Apple TV, Settings, Commercial breaks, is Skip them, Show a Skip button, or Play them. [Recordings and storage](recordings.md) is the rest.

## How do I update?

On Unraid, the optional updater checks at 03:30 and updates only the Broadwave container. It follows the tag that container already uses, so `latest` moves forward and a full version tag stays put. It skips a night while a tuner is in use, a recording is running, or a recording starts within two hours. If the server does not answer, the check allows the update. If it answers and cannot list the schedule or the recordings, it waits. To update by hand, pull the new image and recreate the container with the same folders.

Without an updater, Settings, Check for updates, asks once a day. The hint is "Once a day. Nothing else is sent." The page says when a release is out. It does not install it. [Install](install.md) has the updater.

## The guide is on UTC.

Set `TZ` to your time zone. Without it, a container runs on UTC, and Diagnostics says "The time zone is not set. Set TZ so the guide uses your local time." The guide on a phone or in a browser uses that device's clock. Pass times and the server's own clock messages use `TZ`.

## Is there a backup?

Settings, Download backup. The hint is "The backup is the catalog. Recordings stay put. Seven nightly copies and four weekly copies are kept. A copy is saved before an update." Restore puts the catalog back to that copy. The recordings stay.

## The picture stopped.

The line on the player says what failed and what to try. On the web, Diagnostics shows the tuners, the disk, and the recent log. On iPhone, iPad, and Apple TV it shows Fix these, guide depth, tuner health, and antenna readings. [Troubleshooting](troubleshooting.md) lists the lines. [Common problems](problems.md) is the cause, and the problems an update already fixed.

## Does an encrypted ATSC 3.0 channel play?

It plays as that station's regular broadcast, when there is one. The player says "The 3.0 version is encrypted. Showing the regular broadcast." Broadwave does not decrypt it. A station with no regular broadcast stays in Settings and does not play.
