# Parity

What each client can do. A cell that only the web has says why.

iPad runs the iPhone app with the iPad's own layout. Where a column matches iPhone, the control is the same screen. Multiview is the exception: iPad shows four pictures, and iPhone shows two.

## Multiview

Five layouts: Side by side and Small over big (2 pictures), One big and two (3), One big and three and Quad (4).

Captured 2026-09-25 on staging `v0.8.0-33-g8ccb3e1-dirty`, encoder `h264_vaapi`. Four pictures on two tuners, with two subchannels sharing each frequency. Shots and clips are in `.evidence/mv6/` (not committed).

| Layout | Web phone | Web desktop | Web TV | iPhone | iPad | Apple TV |
| --- | --- | --- | --- | --- | --- | --- |
| Side by side | yes | yes | yes | yes | yes | yes |
| Small over big | yes | yes | yes | yes | yes | yes |
| One big and two | yes | yes | yes | not offered | yes | yes |
| One big and three | yes | yes | yes | not offered | yes | yes |
| Quad | yes | yes | yes | not offered | yes | yes |

The iPhone app keeps two tiles (`cap` is 2, layouts are side by side and small over big). The web phone shows all five, and each tile measured 1.778.

Web, Chrome, every tile: ratio 1.778, `readyState` 4. Fifteen clips, 10.9–11.0 s. Dropped frames in the first ~15 s, startup included: phone side by side 26/805, the other fourteen layouts 0–8. Opening Home logged four poster 404s. The multiview pages did not add any.

- Phone 390×844: `web-phone-2up.jpg`, `web-phone-pip.jpg`, `web-phone-1plus2.jpg`, `web-phone-1plus3.jpg`, `web-phone-quad.jpg`, and the matching `.webm`.
- Desktop 1440×900: `web-desktop-2up.jpg`, `web-desktop-pip.jpg`, `web-desktop-1plus2.jpg`, `web-desktop-1plus3.jpg`, `web-desktop-quad.jpg`, and the matching `.webm`.
- TV 1920×1080: `web-tv-2up.jpg`, `web-tv-pip.jpg`, `web-tv-1plus2.jpg`, `web-tv-1plus3.jpg`, `web-tv-quad.jpg`, and the matching `.webm`.

Apple clips are 9.7–9.9 s. The focused tile reads 720p60 HEVC.

- iPhone: `iphone-2up.jpg`, `iphone-pip.jpg`, and the matching `.mp4`.
- iPad: `ipad-2up.jpg`, `ipad-pip.jpg`, `ipad-1p2.jpg`, `ipad-1p3.jpg`, `ipad-quad.jpg`, and the matching `.mp4`.
- Apple TV: `tv-2up.jpg`, `tv-pip.jpg`, `tv-1p2.jpg`, `tv-1p3.jpg`, `tv-quad.jpg`, and the matching `.mp4`.

iPhone landscape is a separate line. The focused Channels button on Apple TV is a separate line.

## Settings

Server choices. The iPhone, iPad, and Apple TV controls are the shared settings screen, and they save the same fields as the web.

| Choice | Web | iPhone | iPad | Apple TV |
| --- | --- | --- | --- | --- |
| Picture (Broadcast, Smooth, Film) | yes | yes | yes | yes |
| Schedules Direct account, lineup, guide address, movie artwork | yes | yes | yes | yes |
| Hide scores | yes | yes | yes | yes |
| Play the next episode | yes | yes | yes | yes |
| Offer this server as an HDHomeRun | yes | yes | yes | yes |
| Recordings folder and free-space reserve | yes | yes | yes | yes |
| Catalog backups: list, download, restore a saved copy | yes | yes | yes | yes |
| Restore a catalog file from this device | yes | yes | yes | Apple TV has no file picker. It restores a copy the server already kept. |
| Layout (Auto, Desktop, TV, Phone) | yes | The app uses this device's layout. | The app uses this device's layout. | The app uses this device's layout. |
| Diagnostics: doctor, tuner health, guide depth, last antenna reading, check the antenna | yes | yes | yes | yes |

The demo does not share a tuner, and it does not change a guide account, a folder, or a backup. Passwords are sent once and are not shown again. Layout stays on the web because each Apple app already has its own screen.

## Channels

Settings, Channels on Apple; the lineup under Sources on the web. Every channel is listed, including hidden, off-guide, and off-air ones.

| Choice | Web | iPhone | iPad | Apple TV |
| --- | --- | --- | --- | --- |
| Favorite | yes | yes | yes | yes |
| On the guide | yes | yes | yes | yes |
| Hide | yes | yes, also from a channel's menu on Home, Guide, and Sports | yes, also from a channel's menu on Home, Guide, and Sports | yes, also from a channel's menu on Home, Guide, and Sports |
| Name, number, and guide match | yes | yes | yes | yes |

The demo keeps favorites and hidden channels for the session. It does not rename a channel or take one off the guide.

## Passes

Settings, Passes on Apple; Schedule on the web. A pass is set from the guide on every client.

| Choice | Web | iPhone | iPad | Apple TV |
| --- | --- | --- | --- | --- |
| List in priority order, with the web's labels (title, words, category, team, one airing) and rules | yes | yes | yes | yes |
| New pass from the list | yes | yes, + in the toolbar | yes, + in the toolbar | yes, New pass at the top |
| Match: title is, title contains, or category, and edit the text | yes | yes | yes | yes |
| Channel: any or one | yes | yes | yes | yes |
| Days of the week | yes | yes, a switch per day | yes, a switch per day | yes, a switch per day |
| Time window (start between two times, past midnight allowed) | yes, any minute | yes, half-hour menus and any value the web set | yes, half-hour menus and any value the web set | yes, half-hour menus and any value the web set |
| Early and after (0–30 min) | yes | yes | yes | yes |
| Episodes: all or new only | yes | yes | yes | yes |
| Keep: all, unwatched, or the newest few | yes | yes, with how many | yes, with how many | yes, with how many |
| Stop at N unwatched | yes, 0–99 | yes, no limit to 20 and any value the web set | yes, no limit to 20 and any value the web set | yes, no limit to 20 and any value the web set |
| Record again after a delete | yes | yes | yes | yes |
| Find commercials | yes | yes | yes | yes |
| Order: the higher pass gets the tuner | yes, drag or Up and Down | yes, Edit and drag | yes, Edit and drag | yes, Move up and Move down in the pass |
| Preview: what records in the next 2 weeks, what is skipped, and what it would stop | yes | yes | yes | yes |
| Delete a pass | yes | yes | yes | yes |

A team pass keeps its team: no match, title, episodes, or record again. A pass for one airing takes only early, after, and commercials. Apple uses menus for minutes, counts, and times because tvOS has no stepper, number field, or time picker. The demo answers the preview with nothing to record.

## Recordings

Recordings on every client. On Apple the actions are in each recording's menu (press and hold on iPhone, hold Select on Apple TV), and iPhone also swipes to delete or mark watched.

| Choice | Web | iPhone | iPad | Apple TV |
| --- | --- | --- | --- | --- |
| List by show, then Movies, with status, size, length, and resume point | yes | yes | yes | yes |
| All or Unwatched | yes | yes | yes | yes |
| Continue watching: started and not finished, last played first | yes, on Home and in Recordings | in Recordings | in Recordings | in Recordings |
| A show's page: every recording by season, with episode numbers | yes | yes | yes | yes |
| Count, unwatched, and space per show | yes | yes | yes | yes |
| Sort (newest, oldest, name, size) and show only shows, movies, or sports | yes | yes | yes | yes |
| Select several to mark watched, unwatched, or delete | yes | yes | yes | yes |
| Mark watched or unwatched | yes | yes | yes | yes |
| Stop a recording in progress | yes | yes | yes | yes |
| Delete, with a confirmation | yes | yes | yes | yes |
| Play while it records | yes | yes | yes | yes |
| Find commercials | yes, in the player | yes, from the menu | yes, from the menu | yes, from the menu |
| Commercial breaks: skip, offer a Skip button, or play | yes, in the player | yes, in Settings, DVR | yes, in Settings, DVR | yes, in Settings, DVR; Skip break is the player's own button |
| Add or remove a break by hand | yes | yes, from Breaks in the player | yes, from Breaks in the player | yes, in the player's transport bar |
| Make a channel from a recording | yes | yes, from the menu | yes, from the menu | yes, from the menu |
| Library channels: list and play, with Next and break skipping, without a tuner | yes, in the guide | yes, in Recordings | yes, in Recordings | yes, in Recordings |
| Coming up: what passes record next, skipped airings and why, and record the later airing | yes | yes, from Recordings | yes, from Recordings | yes, from Recordings |
| Recent activity | yes | yes, in Coming up | yes, in Coming up | yes, in Coming up |
| Download the original file | yes | not yet (offline downloads are a later phase) | not yet (offline downloads are a later phase) | no (Apple TV keeps no files) |

The demo has no recordings or library channels.

## Sources

Settings, Tuners and playlists on Apple; Sources on the web. Setup uses the same finder and form.

| Choice | Web | iPhone | iPad | Apple TV |
| --- | --- | --- | --- | --- |
| Tuners with model, tuner count, and firmware | yes | yes | yes | yes |
| Scan a tuner for channels | yes | yes, with a running count | yes, with a running count | yes, with a running count |
| Playlists and links with health, stream use, and last update | yes | yes | yes | yes |
| Look harder, free channels, and add a tuner by address | yes | yes | yes | yes |
| Add a playlist, Xtream Codes, tvheadend, Channels DVR, Threadfin, xTeVe, ErsatzTV, Dispatcharr, a stream link, or a media folder | yes | yes | yes | yes |
| Group filter, and choosing groups or channels from a long playlist | yes | yes, channels kept by id | yes, channels kept by id | yes, channels kept by id |
| Add a playlist file from this device | yes | yes | yes | Apple TV has no file picker. Add the playlist by address. |
| Signal check | yes | yes, in Diagnostics | yes, in Diagnostics | yes, in Diagnostics |

The demo has no tuners or sources, so Tuners and playlists is not shown there.
