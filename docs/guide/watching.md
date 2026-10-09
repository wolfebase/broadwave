# Watching

The same server plays on a browser, an iPhone, an iPad, and an Apple TV. Nothing in the apps is a separate lineup. They all ask this server what is on.

## The web

Open `http://<your-server>:8477`. There is nothing to install. The page is the guide, live TV, recordings, and multiview.

The player picks a picture the browser can play. A broadcast the browser already understands is passed through. Anything else is encoded for that browser. Settings, Live delay, is how far behind live a channel starts: Lowest is about 6 seconds, Balanced is 16, Stable is 20.

While Whole-Home Sync is on, the player shows Synced when this screen is the only one, the number of screens on that channel when others are there, and Together while you are watching together. [Sync](sync.md) covers that control and Watch together.

## iPhone, iPad, and Apple TV

The apps look for a Broadwave server on the local network and connect when they find one. You can type an address instead.

On iPhone and iPad, Local Network access has to be on for Broadwave (Settings, Privacy and Security, Local Network). With no server around, the apps play a built-in demo so you can try the screens first. The demo does not contact your server or the internet.

Apple TV opens Broadwave and finds the server the same way. The first-run page on the web can show a QR code for the iPhone. [First run](first-run.md) describes it.

Whole-Home Sync is on unless you turn it off. The switch is in Settings, and the line under it is "Every screen on the same channel shows the same moment, so nobody hears the next room cheer first." Apple devices start a channel at Balanced or Stable, not at Lowest.

A recording remembers where you stopped, once you were more than a few seconds in. On the web it resumes a couple of seconds in. On the Apple apps it resumes past five seconds.

## Picture and sound

The server decides direct play, bitrate, and audio for each device. Quality and sound are saved on that device. Languages and described video, when the broadcast carries them, are a picker on the player. A full-size encode keeps the other languages so the switch does not restart the picture. Set `BROADWAVE_ALTERNATES` to `0` to send only the chosen track.

Auto passes through a 5.1 mix the device can play. A 5.1 AC-4 mix is sent as Dolby Digital where the device plays it, on Auto and on Surround, and as stereo AAC on Auto when it does not. Data saver on Auto is stereo. Surround sends 5.1 AAC when the device cannot play the station's mix or Dolby Digital.

Diagnostics names the encoder and how many pictures this server can play at once. A server that is at that limit says "This server can play 4 pictures at once. Stop one." The number is this server's limit.

## One tune

Three screens on KBWV 4.1 use one tuner, not three. A recording of that channel uses the same tune. The subchannel 4.2 shares it too. A different station takes another tuner when one is free.
