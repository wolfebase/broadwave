# Parity

What each client can do. A cell that only the web has says why. P5 fills the rest of this table.

## Multiview

Five layouts: Side by side and Small over big (2 pictures), One big and two (3), One big and three and Quad (4).

Captured 2026-09-25 on staging `v0.8.0-33-g8ccb3e1-dirty`, encoder `h264_vaapi`. 5.1 and 62.1 share 533 MHz. 38.1 and 41.1 share 605 MHz. Four pictures, two tuners. Shots and clips are in `.evidence/mv6/` (not committed).

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
