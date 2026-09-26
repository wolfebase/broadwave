# Broadwave privacy policy

Effective September 25, 2026. Broadwave is made by Wolfe Up LLC.

## The short version

Broadwave doesn't collect, sell, or share your data. The apps talk only to the Broadwave server you run at home, or, if you choose Try the demo, to a player on the device itself. Nothing about what you watch is sent to Wolfe Up.

## What the apps do

- **Find your server.** The iPhone, iPad, and Apple TV apps look for a Broadwave server on your local network. They try Bonjour, then send a short UDP probe on the local network. A dropped connection looks again the same way. Before the app follows a server to a new address, it checks a signature only that server can make, using a key it learned from that server. The check stays on your network. They also connect to an address you enter or a link you open. That's why they ask for Local Network access.
- **Store settings on the device.** The servers you have used, your playback preferences, and a cached copy of the guide stay on your device so the app opens quickly and can follow a server that moves. Deleting the app deletes them.
- **No accounts, ads, analytics, or tracking.** The apps contain no advertising or analytics SDKs, and they don't use the advertising identifier.
- **Try the demo.** Sample films bundled in the app play on the device. That choice does not contact a server, Wolfe Up, or the internet.

## What your server does

The Broadwave server runs on hardware you own. Your channels, guide, recordings, and watch history are stored in its database on that machine. To do its job, the server contacts:

- your tuner (for example an HDHomeRun) on your network;
- guide providers you enable, such as the SiliconDust guide for HDHomeRun owners, Schedules Direct, or an XMLTV address you add;
- public sports scoreboards, to show scores and extend recordings of games. The apps never ask for those scores. The server does, and it caches the answer. Settings, Live scores, stops every scoreboard request. A TheSportsDB key, if you add one, stays on your server. Nothing is sent to Wolfe Up;
- GitHub, once a day, to see if a newer Broadwave release exists. That request carries no account, no id, and nothing about what you watch. Turn it off under Settings, Check for updates;
- any playlist or channel sources you add.

Those requests come from your server, not from Wolfe Up LLC, and we receive none of the data.

## Children

Broadwave isn't directed to children and doesn't knowingly collect data from anyone.

## Changes and contact

If this policy changes, the new version will be posted at this address with a new effective date. Questions: open an issue at https://github.com/wolfebase/broadwave/issues or email twolfekc@gmail.com.
