# Cast

A television with Cast can play the channel that is on in the browser, on the same moment as the other screens in the house. The server serves the receiver page. The browser is the sender. Google's Cast SDK is loaded from Google when a session starts. It is not stored in this repo.

Registration of the Cast application stays with the person who runs the server. The steps are at the end.

## What plays

The sender is the web player. Cast is shown only when the server has an application id and there is a playlist to send. Pressing it asks the sender SDK for a session. A session already open on this page is used again, so the picker does not cover a television that is already playing. The sender then tells the receiver which channel to play. While that session is open, Cast stays in the player so it can be stopped, including while the next channel's playlist is not ready. A load is sent only for a playlist this server will play. When the television ends the session, the button returns to Cast.

The message is JSON on the namespace `urn:x-cast:app.broadwave.watch`:

| Field | Meaning |
| --- | --- |
| `type` | `load` or `stop` |
| `channelId` | The channel, a positive integer |
| `playlist` | A path on this server beginning with `/media/`. No host, no query |
| `room` | `channel:<id>`, or `group:ch<id>` when this screen is watching together |
| `latency` | `lowest`, `balanced`, or `stable`. The sender's live delay. It matters when this television is the first member of the room |
| `token` | The browser's device token, omitted when device sign-in is off |
| `title`, `subtitle` | The program and the channel, for the line on the television before the picture starts |

The receiver refuses any other playlist. A Cast message cannot make the television fetch an arbitrary URL. The token is not written into the playlist URL and it is not logged. The television sends it as `Authorization: Bearer` on media requests and on its watch request, and as `access_token` on the WebSocket, which is the same query the browser uses because a WebSocket cannot set that header.

Changing the channel, the playlist, or the room sends another `load`. The same load twice does not restart the picture. Stop leaves the sync room, releases the viewer, and ends the Cast session.

The television counts as a viewer. On a load it sends `POST /api/v1/watch` for the rendition in the playlist, with `confirmLive`, so the channel stays up when the browser leaves. The watch does not name the room. The socket join does, and the watch does not wait for the room's frame. A playlist that is not one rendition of this server is not asked for, so the message cannot start some other encode. A load of a different rendition releases the previous viewer. A `hello` whose boot changed means the server restarted, and the television watches again. The old boot does not release a viewer on the new process. A watch that gets no answer is asked again once that hello has named the process, and not again until the next hello.

Cast plays the room, not a private pause. A browser that has stepped off sync still sends the channel room. A browser that is watching together sends the group room, so pause and rewind move the television too.

Recordings and multiview are not sent. The Apple apps do not send in this version.

## Receiver

`GET /cast` is the receiver page. The HTML is embedded in the server. It loads the Cast Application Framework from `https://www.gstatic.com/cast/sdk/libs/caf_receiver/v3/cast_receiver_framework.js`, then `/cast/receiver.js`. That script is the Vite entry `web/src/cast/receiver.ts`, emitted at a stable path so the HTML does not need a new hash on every build. Hashed pieces it imports still live under `/assets/`.

The page is a GET that is not under `/api` or `/media`, so it stays open when device sign-in is on. The television has no token until the sender's message arrives. The page itself has no catalog data. `Cache-Control` is `no-cache`, same as the web app's HTML.

The receiver does not use the framework's player (`skipPlayersLoad`). Playback is hls.js on a video element, with the same live configuration the web player uses for a television, and `autoStartLoad` off so the first picture can be the room's frame. The framework would not see that picture as its own playback, so the receiver sets `disableIdleTimeout`. `maxInactivity` is 10 minutes, which turns on the sender heartbeat.

Until a load arrives the page says "This page plays on a Cast device." The line after a load is the program and the channel. The channel is left off when it repeats the program. That line hides once the picture is playing.

Starting the picture calls play. A Cast device can leave the video at readyState 0 until then, and the corrections never run without a frame. Play is tried again until a frame is ready, including when the room is paused. Once that frame is on screen a paused room holds it, and can move to the anchor. A pause that lines the picture up is not played through when the frame drops. Stop leaves the room, and a later load opens a new session. A tick after stop does not start the picture again.

## Sync

The receiver opens `ws://<this-server>/api/v1/ws` (or `wss:` on HTTPS) and speaks the same socket the web player speaks. On open it sends `here` (name `Cast`, kind `web`), `sync.join` for the room in the load, and a clock sample. It repeats the clock a few times, then every 15 seconds, and keeps the sample with the shortest round trip. The offset is `t1 - (t0 + t2) / 2`.

The target media time is the room's anchor, advanced by the room's rate. The same function the web player uses (`roomTarget`) is the check for that math. A media playlist that already has fragments starts at the room's frame (half a second ahead, so the first paint is not late). A multivariant playlist does not list fragments until playback starts, so the receiver starts at the live edge and then comes back to the room. The correction after that is the web player's Chromium path:

- Under 25 ms of drift, play at the room's rate.
- Under 400 ms, trim `playbackRate` by at most 3 percent, in steps of 0.5 percent, and hold that rate until the drift has crossed back through 5 ms. Speeding up with less than 1.5 seconds buffered holds the room's rate instead.
- Ahead by more than 400 ms, pause for the drift. A live buffer is not seeked backward.
- Behind by more than 400 ms, with the frame in the playlist and 1.5 seconds buffered, seek forward. Otherwise hold the room's rate. A seek is not repeated within 2 seconds.
- Ahead by more than 2 seconds on a `group:` room is a rewind by another screen, and the receiver seeks to that frame.
- A paused room pauses, and seeks to the anchor when it is more than 40 ms off.

The receiver does not copy the web player's Safari stall detector. A Cast device runs Chromium. The pure decisions live in `web/src/cast/protocol.ts` and are what the unit tests cover. The page wires them to the video element.

## Application id

The id is the environment variable `BROADWAVE_CAST_APP_ID`. The process reads it at startup. `GET /api/v1/server` includes `castAppId` when the value is 4 to 32 letters and digits. Anything else is omitted, and the player hides Cast. An empty server is unchanged.

The id is not a secret. It is the public identifier of a Cast application.

## Registering with Google

Do this once, on the account that should own the application. Google's console is [cast.google.com/publish](https://cast.google.com/publish). Their registration guide is [developers.google.com/cast/docs/registration](https://developers.google.com/cast/docs/registration). The console moves; those two are the places to start.

1. Add an application and choose Custom Receiver.
2. Name it Broadwave. That name is what the television shows while the page loads.
3. Set the receiver URL to this server's page, for example `http://192.0.2.10:8477/cast`. Use the address the television can reach. `192.0.2.10` here is a documentation address, not a server you have.
4. Leave the application unpublished. A published application has to be served over HTTPS from a public name. This server is for the home network and does not open that.
5. Register the television: in the console, add its serial number under Cast Receiver Devices. An unpublished application loads only on a registered device. The serial is in the Google Home app, in the device's settings, or on the television's Cast screen.
6. Copy the application id.
7. Set `BROADWAVE_CAST_APP_ID` to that id and start the server again. In Docker it is an environment variable on the container, the same list as the other `BROADWAVE_*` settings.
8. Open the web player on a computer on the same network. Cast is in the player. Choose the television. The first load can take a minute while the television fetches the page and the SDK.

The television loads the page, the socket, and the segments from the server itself. The computer only tells it which channel. Both have to be on a network that can reach the server.

Device sign-in does not change those steps. The browser sends its own token in the load message. Revoking that browser stops the television on the next media request.
