# Broadwave privacy policy

Effective October 9, 2026. Broadwave is made by Wolfe Up LLC.

## The short version

Broadwave doesn't collect, sell, or share your data. The apps talk only to the Broadwave server you run at home, or, if you choose Try the demo, to a player on the device itself. Nothing about what you watch is sent to Wolfe Up.

## What the apps do

- **Find your server.** The iPhone, iPad, and Apple TV apps look for a Broadwave server on your local network. They try Bonjour, then send a short UDP probe on the local network. A dropped connection looks again the same way. Before the app follows a server to a new address, it checks a signature only that server can make, using a key it learned from that server. The check stays on your network. They also connect to an address you enter or a link you open. That's why they ask for Local Network access.
- **Store settings on the device.** The servers you have used, your playback preferences, and a cached copy of the guide stay on your device so the app opens quickly and can follow a server that moves. Deleting the app deletes them. The apps also keep the address of the server you use in the device's keychain, so the Apple TV Top Shelf and the iPhone and iPad widgets can show what's on; the app clears it when you leave that server, and the first time it opens after a reinstall. The widgets ask that server what's on when iOS refreshes them, and their Record button asks it to record.
- **Device sign-in.** When you turn it on, each phone, TV, and browser gets its own token. The web app keeps that token in the browser. The server stores only a hash of it. Revoking the device in Settings makes the token stop working. The token is not sent to Wolfe Up.
- **No accounts, ads, analytics, or tracking.** The apps contain no advertising or analytics SDKs, and they don't use the advertising identifier.
- **Cast.** Cast is off until the server is given a Google Cast application id. With one, pressing Cast in the web player loads Google's Cast script into the browser, and the television loads Google's Cast receiver script along with your server's page. Those scripts come from Google and fall under Google's privacy policy. The picture and the guide still come from your server.
- **Try the demo.** Sample films bundled in the app play on the device. That choice does not contact a server, Wolfe Up, or the internet.

## What your server does

The Broadwave server runs on hardware you own. Your channels, guide, recordings, and watch history are stored in its database on that machine. To do its job, the server contacts:

- your tuner (for example an HDHomeRun) on your network;
- guide providers you enable, such as the SiliconDust guide for HDHomeRun owners, Schedules Direct, or an XMLTV address you add;
- public sports scoreboards, to show scores and extend recordings of games. The apps never ask for those scores. The server does, and it caches the answer. Settings, Live scores, stops every scoreboard request. A TheSportsDB key, if you add one, stays on your server. Nothing is sent to Wolfe Up;
- GitHub, once a day, to see if a newer Broadwave release exists. That request carries no account, no id, and nothing about what you watch. Turn it off under Settings, Check for updates;
- any playlist or channel sources you add;
- only if you turn on the secure name (`BROADWAVE_DIRECT=1`, off by default): the Broadwave direct service at api.direct.wolfeup.com, which Wolfe Up runs, and Let's Encrypt, to give the server a trusted HTTPS name on your home network. The server sends a public key it made, the private home network addresses the name should point at (only private ranges are accepted), and the value Let's Encrypt asks for. The service keeps the key, a random label, those addresses as DNS records, and a hash of your network's public address for rate limits. Nothing about what you watch is sent. Setting `BROADWAVE_DIRECT=0` on the server turns it off.

Apart from the direct service, those requests come from your server, not from Wolfe Up LLC, and we receive none of the data.

## Children

Broadwave isn't directed to children and doesn't knowingly collect data from anyone.

## Changes and contact

If this policy changes, the new version will be posted at this address with a new effective date. Questions: open an issue at https://github.com/wolfebase/broadwave/issues or email twolfekc@gmail.com.
