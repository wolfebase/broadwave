# Channels and the guide

The guide is the list of what is on. Home opens on what is playing now. Search covers the guide and your recordings.

## Where listings come from

Broadwave tries listings in this order:

- The tuner's own guide. For an HDHomeRun that is SiliconDust's feed: about two days for everyone, and about fourteen days with an HDHomeRun DVR subscription. The server refreshes it on its own, somewhere between 20 and 28 hours after the last pull.
- Schedules Direct, if you add an account under Settings. The hint there is "Fills channels the tuner guide skips. Fourteen days when the account allows it."
- An XMLTV address you paste under Guide address. The hint is "An XMLTV link for channels the tuner guide skips."
- The broadcast itself fills times none of those already list.

With no guide key, a channel matches on its number, then its name, then its call sign. A trailing DT, DT1, HD, TV, or LD still counts. Guide match, on the channel in Settings, pins a listing when the names differ. Leave it blank to use the number, the name, and the call sign.

Logos and episode art are kept on the server. Movie artwork, in Settings, fills posters the guide left blank.

## On the guide, or hidden

Settings, Tuners and channels, lists every channel the tuner reported. On guide shows it in the guide. Hidden takes it off. A custom name and a custom number are per channel. Two rows for the same station, one on each tuner, share the favorite, the hide, and a custom name.

A clear 3.0 station can be shown as the 3.0 channel, the regular channel, or both. The usual choice is 3.0 only. [Tuners](tuners.md) and [ATSC 3.0](../atsc3.md) cover that choice. An encrypted 3.0 station does not offer it. It plays as the regular broadcast, and search can still find its shows so you can record that broadcast.

Channels found by a later scan take their place by number.

## Signal

A signal check reports Great, OK, Weak, or Lost. It waits while a tuner is busy, while something is recording, or while a recording starts within 30 minutes, and it says so in the same words as a busy tuner.

## Favorites and Home

Star a channel to keep it on Home. The favorite follows the channel that stays when you choose 3.0 or the regular broadcast for a station that has both.

Setup's Favorites step can star channels for you. You can change them later.
