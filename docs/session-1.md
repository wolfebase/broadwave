# Session 1 gate

The guide is served by the Go process on port 8477. Startup discovery found the CONNECT DUO and stored 27 channels. `go test ./...` passed. The browser pass covered:

- Home shows 4.1 as the hero and the four device favorites.
- The guide orders 14.2 before 14.10, draws the now line, and moves with the arrow keys.
- Enter opens the channel sheet. Watch and Record are disabled and say why.
- Favorites and HD filters reduce the grid to 4 and 8 channels.
- Hiding Nosey removes it from the guide. Restoring it brings the lineup back to 27.
- Phone width switches to the vertical list. Desktop width keeps the grid.
- Sources names the CONNECT DUO, two tuners, and the published firmware without offering to install it.
- Settings saves layout and the recordings path as local settings. There is no password field.
- The browser console had no errors.

No stream was opened on port 5004. Both tuners were left with whoever already held them.

## Later sessions, from the cross-check

These were confirmed after the guide was already up. They do not change Session 1.

- This CONNECT DUO is not an EXTEND, so its HTTP `transcode=` profiles do nothing. The browser picture has to be made by ffmpeg on the server.
- SiliconDust’s XMLTV feed, updated 20 May 2026, no longer needs their DVR subscription. Each request can use DeviceAuth read from the tuner at that moment, or an email plus device id. DeviceAuth changes and is only good for about 16–24 hours, so it is read per request and never written to the database. Try this feed in the listings session before Schedules Direct.
- Busy tuners come back as HTTP 503 with `X-HDHomeRun-Error: 805`. Signal fields to show later are strength (`ss`), quality (`snq`), and symbol quality (`seq`).
- NextPVR already shares one tuner across channels on the same frequency. Jellyfin shares one MPEG-TS body and counts viewers. The channel 14 test is the same idea. A shared rewind buffer follows that one tune. Separate rewind files per viewer would copy the same broadcast.
- Channels commercial skip has three modes: jump automatically, show a Skip button, or leave it manual. A double seek-forward inside a break also skips it. Match those when detection exists.
- A FLEX 4K, if one is ever added, has four tuners and only two of them do ATSC 3.0. Where AC-4 gets decoded is still undocumented.
- Android, Android TV, and Fire TV are Channels clients. This project’s native apps stay Apple-only until the site is the player people actually use.
