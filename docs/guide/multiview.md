# Multiview

Several channels at once. One tile has the sound. The others stay quiet.

## Layouts

The web app, iPad, and Apple TV share these layouts.

| Layout | On screen |
| --- | --- |
| Side by side | Two pictures, the same size |
| One big and two | One large picture and two smaller ones |
| One big and three | One large picture and three smaller ones |
| Quad | Four pictures, the same size |
| Small over big | A small picture over a large one |

iPhone offers Side by side and Small over big. A link that asks for a quad opens side by side there. In a phone browser, Side by side stacks the two pictures, and the other layouts stay available.

Click or tap a tile to hear it. With a remote, move to the tile and press Enter or the clickpad. The page says "Select a tile to hear it." until you do. In the layouts with one large picture, the tile you hear is the large one.

## How big the pictures are

A quad is 360p on every tile. The large-tile size comes from a short encode the server runs at startup. Diagnostics prints the result, for example "Intel GPU found: 1080p60 at 5.4x real time. 720p60 on a large tile, 4 pictures at once."

On an Intel GPU using its low-power encoder, a fast startup encode allows six pictures, so a quad fits beside two full screens. A fast GPU stays at four. A software encoder needs a faster result than that to reach four.

Another picture past the limit stays off. The player says "This server can play 4 pictures at once. Stop one." With room for one picture, it says "This server can play 1 picture at once. Stop it to watch another."

## Tuners

One tuner plays a whole station. KBWV 4.1 and 4.2 share that tuner. WTST 5.1 takes another tuner when one is free. A channel someone else is already watching, or a recording, shares the tuner that is already on it.

When every tuner is busy, the extra tile stays off and names what is on: "The tuner is busy", "Both tuners are busy", or "Every tuner is busy." A playlist channel takes no antenna tuner.

A tile on a channel with no signal says to check the antenna, the same way a single channel does.

## Watch together on Home

When two or more games are on, Home and Sports offer Watch together. That opens those games in multiview. On the guide, Watch together puts the show beside the channel already playing. On an iPhone a quad from that button opens side by side.

That button is several channels. The web player's Watch together, which pauses every screen on one channel, is in [Sync](sync.md).

The picture sizes, the link format, and the startup table are in [Multiview layouts](../multiview.md).
