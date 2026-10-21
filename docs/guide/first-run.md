# First run

Open `http://<your-server>:8477`. The first page is titled "Let's set up your TV." It has two steps, Sources and Ready.

## Sources

Setup looks for an HDHomeRun on the network. While it looks, the heading says "Looking for your tuner…" and the box under it says "Searching…". When a tuner answers, the box shows its name, how many tuners it has, how many channels it listed, and the word Found. The line under the heading says "Found it. Setup moves on in a few seconds. Tap anything here to stay." Tap or type anything on the page and it waits: "Take your time. Continue when you're ready."

If nothing answers, the heading says "No tuner found", and the page lists why a tuner might not show up: a different network, a Docker container without host networking, or a tuner that is off.

- **Not seeing your tuner?** Type its address and choose Add by address. The hint is "A tuner, or a server that speaks HDHomeRun. Add the port if it is not 80." On Docker Desktop, discovery cannot see the tuner. Set `HDHR_HOST` when you start the container, or type the address here. [Install](install.md) has that command.
- **Look harder** checks this network for tuners and other servers. Add on a result puts it in the lineup.
- **Free streaming channels (FastChannels)** looks for FastChannels, a separate free app that gathers internet channels. It can take about 15 seconds, and the page says it is looking. If FastChannels is not running, the page shows how to start it. The iPhone, iPad, and Apple TV apps call this Add free channels.
- **Other sources** adds a playlist, an Xtream server, or a file.

On iPhone, iPad, and Apple TV, these buttons say "Looking…" while they work, and adding a playlist says "Adding…".

You can skip all of these and come back to them in Settings, Tuners and lineup.

Continue stays off until a tuner or a channel is in the lineup. Then the page moves to Ready on its own after a few seconds, unless you have tapped or typed on it.

## Ready

Ready says "Setting up your TV" while it runs six checks, and marks each one Working, Done, Needs a look, or Skipped:

1. Channels. If that tuner has no lineup yet, it scans the antenna.
2. Guide. Listings are filled in.
3. Recordings. The recordings folder is checked.
4. Favorites.
5. Picture. The server times a 1080p60 encode and shows how fast it ran.
6. Signal.

Then the heading sums it up (the channels, the tuners, and the guide when some listings are in) and says "Ready to watch." Choose Watch. Below, "Watch on your other screens" says how to connect the rest of the house.

If you opened the page on the server itself, as localhost, it says "To connect your phone or TV, open this page from another device at this computer's network address, or open Broadwave there. It finds this server on its own." Opening the app lists the servers on your network. You pick one.

From any other address, the page shows a QR code and the words "Scan this with your iPhone. On Apple TV, open Broadwave. It finds this server on its own." The code opens the iPhone app on this server. On Apple TV you still pick the server from the list.

On iPhone, if the app cannot see the server, allow Local Network for Broadwave under Settings, Privacy and Security. You can also type the server's address in the app.

[Install](install.md) covers starting the server. When a tuner never appears, [Troubleshooting](troubleshooting.md) lists the lines the page uses.
