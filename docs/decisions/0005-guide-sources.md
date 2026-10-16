# 0005: Guide sources

Accepted, 2026-09-23.

## Goal

Listings for every channel the antenna can receive, for as many days as the source allows.

## Design

**SiliconDust first.** The tuner XMLTV feed is the default. It is two days on the free account and covers only the stations SiliconDust publishes. One lineup saw 9 of 27 channels in that feed. The JSON guide endpoint listed the same channels, so it is not a second listing source.

**Another source fills the gaps.** Schedules Direct, when an account and lineup are saved in Settings, asks for fourteen days and falls back to one day if the account refuses the longer request. An XMLTV link in Settings does the same job for channels that still have no listings. Neither source replaces a channel that already has listings from the tuner feed.

**Match, then pin.** A guide key wins. With none, a listing attaches by channel number, then by an exact name, then by call sign. A trailing DT, DT1, HD, TV, or LD still matches. A guide match on the channel pins a listing whose name does not.

**One write per refresh.** Every source is read first, merged in that order of preference (a channel takes all its listings from the first source that has any), and written in one transaction that leaves unchanged rows alone. A source that fails, or whose upcoming listings fall by more than half from at least 100, keeps the channels it listed until a later refresh brings them back. Schedules Direct is asked for 20 stations and 500 programs a request, and a response over 8 MB is an error rather than a truncated parse.

The Schedules Direct password is saved with the other settings and is not returned to the browser or written in the log.

## Consequences

Channels SiliconDust does not publish stay empty until an account or a guide address is added. Artwork and a lineup picker that talks to Schedules Direct come with the account, not before it.
