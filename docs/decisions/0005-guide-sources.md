# 0005: Guide sources

Accepted, 2026-09-23.

## Goal

Listings for every channel the antenna can receive, for as many days as the source allows.

## Design

**SiliconDust first.** The tuner XMLTV feed is the default. It is two days on the free account and covers only the stations SiliconDust publishes. On this lineup that is 9 of 27 channels. The JSON guide endpoint lists the same nine, so it is not a second listing source.

**Another source fills the gaps.** Schedules Direct, when an account and lineup are saved in Settings, asks for fourteen days and falls back to one day if the account refuses the longer request. An XMLTV link in Settings does the same job for channels that still have no listings. Neither source replaces a channel that already has listings from the tuner feed.

**Match, then pin.** A listing attaches by channel number, then by call sign. A trailing DT or HD still matches. A guide match on the channel pins a listing whose name does not.

The Schedules Direct password is saved with the other settings and is not returned to the browser or written in the log.

## Consequences

Channels SiliconDust does not publish stay empty until an account or a guide address is added. Artwork and a lineup picker that talks to Schedules Direct come with the account, not before it.
