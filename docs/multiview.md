# Multiview

Several channels at once. One tile has the sound. The others stay quiet.

## Layouts

The web app, iPad, and Apple TV share these layouts.

| Layout | On screen | In a link |
| --- | --- | --- |
| Side by side | Two pictures, the same size | `layout=2up` |
| One big and two | One large picture and two smaller ones | `layout=1+2` |
| One big and three | One large picture and three smaller ones | `layout=1+3` |
| Quad | Four pictures, the same size | `layout=quad` |
| Small over big | A small picture over a large one | `layout=pip` |

iPhone offers Side by side and Small over big, two pictures. A link that asks for a quad opens side by side there. In the browser on a phone, Side by side stacks the two pictures, and the other layouts stay available.

A page link looks like `/multiview?ch=12,34&layout=2up`. The apps use `broadwave://multiview?ch=12,34&layout=2up`. `focus` names the tile with the sound. Write `1%2B2` in an address for One big and two: a bare plus sign is read as a space.

## Sound

Click or tap a tile to hear it. With a remote, move to the tile and press Enter or the clickpad. The page says "Select a tile to hear it" until you do.

In One big and two, One big and three, and Small over big, the tile you hear is the large one. Side by side and Quad stay the same size when the sound moves.

## Picture size

A quad is 360p on every tile.

Side by side uses the large-tile size for both pictures. In the other layouts, the tile with the sound is that size and the quiet tiles beside it are 540p, or 360p when the large tile is 360p. Small over big uses 360p for the small picture.

The large-tile size comes from a short encode the server runs at startup. Diagnostics prints the result, for example "720p60 on a large tile, 4 pictures at once."

| Startup encode of 1080p60 | Large tile | Pictures at once |
| --- | --- | --- |
| 4 times real time, or faster | 720p60 | 4 |
| 2 times real time | 720p60 | 2 |
| Real time | 540p60 | 2 |
| Half of real time | 360p60 | 1 |
| Slower than that | 360p | 1 |
| The encode ran out of time | 360p | 1 |
| The encode failed, and a GPU is present | 720p60 | 2 |
| The encode failed, software only | 360p | 1 |

On an Intel GPU using its low-power encoder, a startup encode of 4 times real time or faster allows six pictures: a quad and two full screens. AMD, NVIDIA, Apple, and a CPU encode stay at four in that row. A slower result stays on the table above, including on that Intel encoder.

Side by side, and a quiet tile beside a large one, can keep a clear 1080 HEVC picture as the station sent it. A quad stays 360p.

Another picture past the limit stays off. The player says "This server can play 4 pictures at once. Stop one." With room for one picture, it says "This server can play 1 picture at once. Stop it to watch another."

## Tuners

One tuner plays a whole station. KBWV 4.1 and 4.2 share that tuner. WTST 5.1 takes another tuner when one is free. A channel already on, for someone else in the house or for a recording, shares the tuner that is already tuned to it.

When every tuner is busy, the extra channel stays off the grid. Two tuners say "Both tuners are busy." One tuner says "The tuner is busy." A playlist channel takes no antenna tuner.

## A 3.0 channel in the link

A link can name the half of a station that is off the guide. With Show set to 3.0 only, KBWV 4.1 opens as 104.1. An encrypted station such as WTST 115.1 opens as 5.1, and that tile says "The 3.0 version is encrypted. Showing the regular broadcast."

[ATSC 3.0](atsc3.md) covers the Show choice and which tuners receive 3.0.
