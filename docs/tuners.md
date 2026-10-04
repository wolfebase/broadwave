# Tuners

Broadwave plays the channels on an HDHomeRun, or on a server that answers the same way. More than one can be in the lineup at once.

## How a tuner is found

Three ways. A broadcast on the local network, an address you type, and SiliconDust's discovery list when the network search finds nothing.

With no address, the server broadcasts for HDHomeRun devices and also listens for an HDHomeRun announcement and for the name `hdhomerun.local`. A reply counts only when it comes from this network. The address inside the packet is ignored. The address that answered is the one used.

The first time the lineup is empty, that search adopts one tuner. About every five minutes it refreshes tuners already in the lineup. It does not add a second one on its own.

Settings › Tuners and channels › Search the network runs that search and adds every tuner that answers. The search reads the lineup. It does not tune a channel.

Add by address takes a host, or a host and a port. A port you type is the only one tried. Leave it off and Broadwave checks port 80 and the ports other compatible servers use: 5004, 34400, 8409, and 9191. Any of those that answer are added. The field says "A tuner, or a server that speaks HDHomeRun. Add the port if it is not 80."

Look harder checks this network for tuners and other servers. If nothing answers, it asks SiliconDust's discovery list and keeps a device only when that device answers here with the same id. Add on a result is what puts it in the lineup.

## Several tuners

Each tuner keeps its own channels. One tune of a station is shared by everyone watching it, and by a recording of it. Subchannels of that station share the tune.

When the same channel number is on two tuners, Broadwave plays it from the tuner that channel belongs to. If that tuner does not answer, or has no free tuner that can take the channel, it tries the others that carry that number. A lower priority is tried first. A tuner you add starts at priority 0, the same as the others, so two at 0 follow device-id order. A regular channel would rather use a tuner that is not receiving ATSC 3.0, on another device, than take a 3.0 tuner on its own.

## A station in 1.0 and 3.0

A clear 3.0 channel is paired with the same station's regular channel, the lowest subchannel of that call sign. KBWV on 4.1 and on 104.1 is one station. The two share one guide.

On the web, the 3.0 row in Settings has Show:

- **3.0 only.** The usual choice. The regular channel stays off the guide.
- **1.0 only.** The regular channel stays.
- **Both.** The guide lists each.

The favorite follows the channel that stays. On iPhone, iPad, and Apple TV the same three choices are on the channel's page.

An encrypted 3.0 station does not offer the choice. WTST on 115.1 plays and records as 5.1, the regular broadcast, and the player says "The 3.0 version is encrypted. Showing the regular broadcast." The encrypted channel stays off the guide. A station with no regular broadcast stays in Settings and does not play.

[ATSC 3.0](atsc3.md) covers what plays, what does not, and which tuners receive 3.0.

## Removing a tuner

Settings › Tuners and channels › Remove forgets a tuner that is gone. The confirm is "Remove <name>? Its channels leave the lineup. Favorites and passes move to another tuner with the same channel, and the other passes go. Recordings stay."

The match is the channel number, on a tuner that still lists it. The favorite moves. A custom name and a custom number move when the channel that stays does not already have them. A pass moves with that channel. A pass for a channel no other tuner has is removed. Recordings keep their files.

A tuner that is playing or recording stays. The message is "Something is playing or recording from it. Stop that first."

Search the network, or Add by address, brings that tuner back when it answers. It comes back with the lineup the tuner reports now. Favorites and names that already moved stay on the other tuner.
