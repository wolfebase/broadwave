# FAQ

## Does it cost anything?

No. Broadwave is free and open source. There is no subscription, no account, and no ads. Over-the-air TV is free too. You need an antenna and a network tuner.

## Where does the picture come from?

From your antenna. Broadwave does not provide, host, or stream any channels of its own. It plays the broadcasts your tuner receives, plus any playlist you add.

## Which tuner should I get?

Any current HDHomeRun works, and Broadwave finds it on the network. A FLEX 4K receives ATSC 3.0 on two of its four tuners, and so does any other model whose name contains 4K. FLEX DUO, FLEX QUATRO, and a CONNECT whose name does not contain 4K receive the regular broadcast. [Tuners](tuners.md) has the table. Other network tuners and servers that speak HDHomeRun work by address.

## What do I watch on?

A browser at `http://<your-server>:8477`, and the Broadwave apps on iPhone, iPad, and Apple TV. The apps find the server on the same network. [Watching](watching.md) covers both.

## Can I watch away from home?

Yes, over a VPN such as Tailscale. Enter the server's address on that VPN if the app does not find it. Leave port 8477 off the public internet. [Security](security.md) says why.

## Can I watch more than one channel?

Yes. Side by side, one large picture with smaller ones beside it, or four at once, on the web, iPad, and Apple TV. The iPhone app shows two. One tile has the sound. [Multiview](multiview.md) is the layout list.

## Do three rooms use three tuners?

No. Everyone on one channel shares one tune, including a recording of that channel. A second station takes another tuner. [Sync](sync.md) is how the rooms stay on the same moment.

## Where do recordings go?

In the recordings folder you mounted. By default they are filed by show for Plex and Jellyfin, and Settings can put them all in one folder. Each one is the broadcast itself. [Recordings and storage](recordings.md) covers passes, games that run long, and the disk reserve.

## Some channels have no guide.

The tuner guide does not list every channel. Add Schedules Direct, or an XMLTV address, under Settings. The broadcast itself fills some of what is left. [Channels and the guide](channels.md) is the order Broadwave tries.

## Will it get along with Plex or Jellyfin?

Yes. Other media servers can use Broadwave as a tuner. Recordings are the original files, in folders those apps already understand. Broadwave keeps a tuner free for its own recordings when one is about to start.

## The picture stopped.

The line on the player says what failed and what to try. Diagnostics shows the tuners, the disk, and the recent log. [Troubleshooting](troubleshooting.md) lists the lines.

## Does an encrypted ATSC 3.0 channel play?

It plays as that station's regular broadcast, when there is one. The player says "The 3.0 version is encrypted. Showing the regular broadcast." Broadwave does not decrypt it. A station with no regular broadcast stays in Settings and does not play.
