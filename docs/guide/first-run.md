# First run

Open `http://<your-server>:8477`. The first page is titled "Let's set up your TV." It has two steps, Sources and Ready.

## Sources

Setup looks for an HDHomeRun on the network and moves on by itself once one answers. The line under the heading says so: "Setup adds an HDHomeRun on this network and moves on by itself. Start adding something else and it waits for you."

While it is looking and nothing has answered, the page says "No tuner answered yet." While a search is running it says "Searching…". A tuner that answered shows its name, how many tuners it has, how many channels it listed, and the word Found.

If nothing answers, type the tuner's address and choose Add by address. The hint under the field is "A tuner, or a server that speaks HDHomeRun. Add the port if it is not 80." On Docker Desktop, discovery cannot see the tuner. Set `HDHR_HOST` when you start the container, or type the tuner's address here. [Install](install.md) has that command.

Look harder checks this network for tuners and other servers. Add on a result is what puts it in the lineup. Add free channels looks for a free-channel server that is already running. A playlist or an Xtream server can be added on this page too. You can skip both and come back to them in Settings.

Continue stays off until a tuner or a channel is in the lineup. After a tuner or a channel is in the lineup, the page moves to Ready on its own after a few seconds, unless you have clicked or typed on that page.

## Ready

Ready runs six checks and marks each one Working, Done, Needs a look, or Skipped:

1. Channels. If that tuner has no lineup yet, it scans the antenna.
2. Guide. Listings are filled in.
3. Recordings. The recordings folder is checked.
4. Favorites.
5. Picture. The server times a 1080p60 encode and shows how fast it ran.
6. Signal.

The heading becomes a Ready line, with the channel count, whether the guide is filled, the tuner count, and the encoder. Choose Watch.

If you opened the page on the server itself, as localhost, it tells you to open it from another device at that computer's network address, or to open the Broadwave app. The app finds the server on its own.

From any other address, the page shows a QR code and the words "Scan this with your iPhone. On Apple TV, open Broadwave. It finds this server on its own."

On iPhone, if the app cannot see the server, allow Local Network for Broadwave under Settings, Privacy and Security. You can also type the server's address in the app.

[Install](install.md) covers starting the server. When a tuner never appears, [Troubleshooting](troubleshooting.md) lists the lines the page uses.
