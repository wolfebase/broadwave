# Watching

The same server plays on a browser, an iPhone, an iPad, and an Apple TV. Nothing in the apps is a separate lineup. They all ask this server what is on.

## The web

Open `http://<your-server>:8477`. There is nothing to install. The page is the guide, live TV, recordings, and multiview.

The player picks a picture the browser can play. It copies the broadcast when the browser lists that codec and the picture is progressive or HEVC. A picture that has not been measured as progressive is encoded, and so is interlaced H.264. Open Stats. The choice is under Playing. For interlaced H.264 that line starts with "Deinterlaced for smooth motion." A cellular connection or Save-Data asks for 720p before that choice. Settings, Live delay, is how far behind live a channel starts: Lowest is about 6 seconds, Balanced is 16, Stable is 20.

While Whole-Home Sync is on, the player shows Synced when this screen is the only one, the number of screens on that channel when others are there, and Together while you are watching together. [Sync](sync.md) covers that control and Watch together.

## Cast

A television with Cast can play the channel that is on in the browser. Cast is in the player when the server has a Cast application id (`BROADWAVE_CAST_APP_ID`). The television plays the same moment as the other screens. Watch together includes it. Leaving the browser does not stop the television. Stop casting does.

The id comes from a Cast application you register with Google. The steps are in [the Cast design note](../design/cast.md). The application stays unpublished, and the television has to be registered by its serial number.

## iPhone, iPad, and Apple TV

The apps look on your home network and list the servers they find. You pick one. The next time you open the app, it connects to the one you saved. You can type an address instead. The first screen says "Find your server," and the button is "Look for my server." The list is headed "On your network." A server you have used is under "Saved."

On iPhone and iPad, Local Network access has to be on for Broadwave (Settings, Privacy and Security, Local Network). "Try the demo" plays sample films on this device. The line under that button is "Sample films on this device." The demo does not contact your server or the internet. It starts when you tap the button.

Apple TV uses the same list. The first-run page on the web can show a QR code for the iPhone. That code opens the app on this server. [First run](first-run.md) describes it.

Whole-Home Sync is on unless you turn it off. The switch is in Settings, and the line under it is "Every screen on the same channel shows the same moment, so nobody hears the next room cheer first." Apple devices start a channel at Balanced or Stable, not at Lowest.

A recording remembers where you stopped, once you were more than a few seconds in. On the web it resumes a couple of seconds in. On the Apple apps it resumes past five seconds.

## Picture and sound

The server decides direct play, bitrate, and audio for each device. Quality and sound are saved on that device. Languages and described video, when the broadcast carries them, are a picker on the player. A full-size encode keeps the other languages so the switch does not restart the picture. Set `BROADWAVE_ALTERNATES` to `0` to send only the chosen track.

Auto passes through a 5.1 mix the device can play. A 5.1 AC-4 mix is sent as Dolby Digital where the device plays it, on Auto and on Surround, and as stereo AAC on Auto when it does not. Data saver on Auto is stereo. Surround sends 5.1 AAC when the device cannot play the station's mix or Dolby Digital.

Diagnostics names the encoder and how many pictures this server can play at once. A server that is at that limit says "This server can play 4 pictures at once. Stop one." The number is this server's limit.

## One tune

Three screens on KBWV 4.1 use one tuner, not three. A recording of that channel uses the same tune. The subchannel 4.2 shares it too. A different station takes another tuner when one is free.
