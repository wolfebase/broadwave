# IPTV players

TiviMate, IPTV Smarters, Kodi, and VLC can take a Broadwave lineup without becoming a Broadwave client. They already speak one of two dialects: an M3U playlist plus an XMLTV file, or the Xtream Codes player API (`player_api.php`, `get.php`, `xmltv.php`, and `/live/…`). This design adds both, on top of the device tokens in `docs/design/pairing.md`.

Household profiles are not built. `GET /profile` is still the single Home row, and the `profiles` table is never read. Until a profile can own a lineup, an IPTV login is a paired device with a public username and a watch token. A later profile layer can filter that lineup. It does not replace the login.

Nothing here changes the HDHomeRun emulator, and nothing here requires a token on the shared playlist until a second switch is turned on. That switch defaults off.

## What stays

Plex, Jellyfin, and Channels have no field for a Broadwave token. These stay as they are:

- `GET /export/lineup.m3u`, `GET /export/guide.xml`, `GET /export/stream/{channels.id}`, and `GET /export/mosaic/{key}` stay on the main server. With `exportAuth` off they need no token. The M3U `url-tvg` value and each `tvg-id` keep matching the XMLTV `channel id` (`ota.` plus the display number, dots replaced with dashes; mosaics stay `mosaic.{key}`). Stream URLs stay `http://{Host}/export/stream/{id}` and `video/mp2t`, not HLS. The lineup filter stays the guide lineup: present, enabled, not hidden, `sameAs == 0`.
- Mosaic numbers stay `990.{slot}` with empty slots preserved, and only listed keys play.
- The emulator on port 8478, when `hdhrEmulate` is on, stays a separate listener with no token and no host check. Virtual channels stay on that lineup only.
- The main server's host check still wraps every new route. A public name works only when it is listed in `BROADWAVE_HOSTS`.
- `server/internal/source/xtream.go` is the client that fetches a remote panel. The new routes do not go through it.

`/export/stream/{id}` today does not check that the id is in the lineup. Leave that. The new play route does check.

## Logins

An IPTV login is a row in `client_devices` plus a username.

Migration `0037_client_login.sql` adds `login TEXT`. Normal devices leave it null. A partial unique index covers the rows where it is set. The password is the device token that already exists: `bw_` plus 32 random bytes, base64url, no padding. The server stores only the SHA-256 hex. The raw token is returned once, on the create response, and is not written to settings, the support zip, or a log line.

The username is eight characters from `a-z0-9`, generated, not typed. It is safe as a path segment. The token's alphabet (`A-Za-z0-9_-`) is safe too. Read both with path unescaping.

Create is `POST /api/v1/clients/iptv` with `{"name":"Living room"}`. The name follows the same rules as a paired device. Omitted, it is "IPTV app". The scope is watch only, so a leaked playlist cannot change settings or schedule a recording. Kind is `iptv`. While device sign-in is off, anyone on the allowed host can create one. While it is on, the route is admin.

The response is once:

```json
{
  "device": {},
  "username": "k3m9q2xa",
  "password": "bw_…",
  "server": "http://broadwave.local:8477",
  "m3u": "http://broadwave.local:8477/get.php?username=k3m9q2xa&password=bw_…&type=m3u_plus&output=ts",
  "xmltv": "http://broadwave.local:8477/xmltv.php?username=k3m9q2xa&password=bw_…"
}
```

`server` is the scheme and host of the request that created the login, with no path. `GET /clients` lists the device and the username, and never the password. Revoke is the existing `DELETE /clients/{id}`. A revoked login fails Xtream auth and stops playing. The same token also works as `Authorization: Bearer` on the Broadwave API, with watch scope. A browser admin token has no username, so it is not an Xtream password.

There is no second secret. Revoking the device revokes the playlist.

## The shared playlist switch

`exportAuth` is a setting, `"0"` or `"1"`, default `"0"`. `GET /settings` always includes it. `PUT /settings` accepts it. It is independent of `deviceAuth`.

While it is off, `/export/*` stays open. A token on those URLs is still recognized, and ignored for authorization.

While it is on, `/export/lineup.m3u`, `/export/guide.xml`, `/export/stream/`, and `/export/mosaic/` require a token whose scope includes watch. Bearer, the `bw_token` cookie, and `access_token` all count, the same way the rest of the API does. A missing token is 401 `unauthorized`. A watch token is enough. The emulator on 8478 stays open either way. Publishing the server still exposes that port; locking it is a later switch, as `docs/design/pairing.md` says.

Xtream and `get.php` always require the username and password, even when both switches are off. A playlist URL without a live login does not play.

## Addresses

Clients append paths to a server URL the user typed. Some keep a directory prefix (TiviMate does, for a URL such as `http://host:8477/xtream`). Others rebuild `/live/…` from `server_info`, which has a host and a port and no path. Both have to work, so each route is registered twice: at the root, and under `/xtream`.

| Root | Also |
| --- | --- |
| `GET /player_api.php` | `GET /xtream/player_api.php` |
| `GET /get.php` | `GET /xtream/get.php` |
| `GET /xmltv.php` | `GET /xtream/xmltv.php` |
| `GET /live/{login}/{password}/{stream}` | `GET /xtream/live/{login}/{password}/{stream}` |
| `GET /live/{login}/{password}/icon/{channelID}` | `GET /xtream/live/{login}/{password}/icon/{channelID}` |
| `GET /live/{login}/{password}/mosaic/{key}` | `GET /xtream/live/{login}/{password}/mosaic/{key}` |

`{stream}` is `{channelID}.ts` or `{channelID}.m3u8`, one path segment. The icon and the mosaic are their own patterns. A mosaic key contains dashes (`4-12`) and no slash, so it is one segment. Register the longer patterns as well as the one-segment pattern. Go's ServeMux picks the more specific one.

These sit beside the other routes, so `GET /` still serves the web app. `/live/` does not collide with `/media/live/`.

Settings tells the user to type `http://broadwave.local:8477` as the server, with no trailing slash and no `player_api.php`. The `/xtream` prefix is for a player that was given that longer URL. Both hit the same handler.

`panel_api.php`, `api.php`, `portal.php`, and `enigma2.php` are not implemented. They are reseller and set-top APIs. A request for one falls through to the web app and is not a login.

## player_api.php

Credentials are the query `username` and `password`. These clients do not send `Authorization`. A wrong password, an unknown username, a revoked device, and a device with no watch scope all look the same.

Success is HTTP 200 and this JSON. Types matter. `auth` and `timestamp_now` and `stream_id` and `tv_archive` are numbers. The account counters are strings, because that is what the panels these apps were written against return.

```json
{
  "user_info": {
    "username": "k3m9q2xa",
    "password": "bw_…",
    "message": "",
    "auth": 1,
    "status": "Active",
    "exp_date": "2082801600",
    "is_trial": "0",
    "active_cons": "0",
    "created_at": "1760000000",
    "max_connections": "0",
    "allowed_output_formats": ["ts"]
  },
  "server_info": {
    "url": "broadwave.local",
    "port": "8477",
    "https_port": "",
    "server_protocol": "http",
    "rtmp_port": "",
    "timezone": "UTC",
    "timestamp_now": 1760000000,
    "time_now": "2026-10-09 18:00:00"
  }
}
```

`exp_date` is a unix timestamp about ten years ahead, as a string. There is no expiry. JSON `null` is not what these panels send, and some players treat a missing date as expired. `max_connections` `"0"` means the login is not counted against a seat cap. `allowed_output_formats` is only `ts`. The play path is the MPEG-TS export, not a stable HLS playlist. `server_info.url` is the hostname only, no scheme and no path. `port` is the port the request arrived on, as a string. `server_protocol` is `https` when the request is TLS, otherwise `http`. `timezone` is the server's local IANA name, or `UTC` when it has none. `time_now` is the server's local clock as `YYYY-MM-DD HH:MM:SS`.

The success body echoes the password because the client sent it and the usual panel response includes it. The failure body does not.

Failure is HTTP 200, not 401:

```json
{"user_info": {"auth": 0, "status": "Disabled", "username": "", "password": "", "message": ""}}
```

A player that only reads `auth` then says the sign-in failed. A connection that never arrives is a different error, and that is the player's to report. Once the per-address failure cap is tripped, the response is 429 with no JSON body.

Actions, all `GET` on the same script:

| `action` | Extra query | Body |
| --- | --- | --- |
| (none) | | the object above |
| `get_live_categories` | | a JSON array |
| `get_live_streams` | optional `category_id` | a JSON array |
| `get_vod_categories`, `get_vod_streams`, `get_series_categories`, `get_series` | | `[]` |
| `get_vod_info`, `get_series_info` | | `{}` |
| `get_short_epg` | `stream_id`, optional `limit` (default 4, cap 20) | `{"epg_listings":[…]}` |
| `get_simple_data_table` | `stream_id` | the same wrapper, up to 50 listings |

An unknown action is `[]`, so a client that probes an action we do not have still finishes its refresh.

Categories come from the channel's network field when it is set. Each distinct network is one category: `category_id` is a short stable slug of that name, `category_name` is the name, `parent_id` is `0`. Channels with no network share one category, `category_id` `"channels"`, `category_name` `"Channels"`. `get_live_streams` without `category_id` returns every channel in the guide lineup. With `category_id`, it returns that group. Order is the guide order. `num` is 1-based in that order.

A live item:

```json
{
  "num": 1,
  "name": "Valley News",
  "stream_type": "live",
  "stream_id": 12,
  "stream_icon": "http://broadwave.local:8477/live/k3m9q2xa/bw_…/icon/12",
  "epg_channel_id": "ota.12-1",
  "added": "1760000000",
  "category_id": "channels",
  "custom_sid": "",
  "tv_archive": 0,
  "direct_source": "",
  "tv_archive_duration": 0
}
```

`stream_id` is `channels.id`, the same integer `/export/stream/` already uses. `epg_channel_id` is the XMLTV id, which is not that integer. `direct_source` stays empty so the player does not skip our play URL. `tv_archive` is `0`. Catch-up is not this version.

`name` uses the same quote and control-character cleanup as the M3U writer.

Mosaics are not in `get_live_streams`. They have no integer id that is stable and clear of `channels.id`. They stay in the M3U, below.

## get.php and xmltv.php

`GET /get.php?username=&password=&type=m3u_plus&output=ts` is the guide lineup as M3U, with the same channels, the same `tvg-id`, the same `tvg-chno`, and the same mosaic rows as `/export/lineup.m3u`. Differences:

- Each stream URL is `{scheme}://{host}/live/{login}/{password}/{channels.id}.ts`.
- Each mosaic URL is `{scheme}://{host}/live/{login}/{password}/mosaic/{key}`.
- `url-tvg` points at this login's `xmltv.php`, with the username and password in the query.
- Channel rows gain `tvg-logo` (the icon URL below) and `group-title` (the category name). Mosaic rows keep `group-title="Multiview"`.
- The scheme follows the request: `https` when TLS, otherwise `http`. The open export's hardcoded `http://` is unchanged.

`type=m3u` is the same list without the `tvg-` attributes. `output=m3u8` and a missing `output` still emit `.ts` URLs. We do not have a stable HLS export, and `allowed_output_formats` does not advertise `m3u8`. Kodi's IPTV Simple Client and VLC are pointed at this URL with `output=ts`.

`GET /xmltv.php?username=&password=` is the same document as `GET /export/guide.xml`: the same channel ids, the same 14-day window, the same mosaic blocks. Optional `prev_days` and `next_days` clamp that window. Each is an integer from 0 to 14. Omitted, the window stays two hours back and 14 days forward. Titles in XMLTV are plain text.

`get_short_epg` returns the next listings for that `stream_id` (the channel id), `limit` items, default 4. `get_simple_data_table` returns the same shape, up to 50. Each listing has `id`, `epg_id`, `title`, `lang`, `start`, `end`, `description`, `channel_id`, `start_timestamp`, and `stop_timestamp`. `start` and `end` are `YYYY-MM-DD HH:MM:SS` in the server's local zone. The timestamps are unix strings. `title` and `description` are standard base64. XMLTV stays plain text; the player API is the one that encodes. `channel_id` is the XMLTV id. `stream_id` on the query is `channels.id`, not that XMLTV id. Listing ids and the two timestamps are strings. The listing that is on now adds `"now_playing": 1` and `"has_archive": 0`, both numbers. An unknown `stream_id` is `{"epg_listings":[]}`.

## Playback and logos

`GET /live/{login}/{password}/{channelID}.ts` checks the login, then checks that the channel is in the guide lineup, then calls the same `exportChannel` path as `/export/stream/{id}`. Content type is `video/mp2t`. A hidden, disabled, or unknown id is 404. A busy tuner is the same 503 and `X-HDHomeRun-Error` the export already returns. The password is not copied into that error body.

`GET /live/{login}/{password}/mosaic/{key}` does the same for a key that `exportMosaics` currently lists. Anything else is 404.

`GET /live/{login}/{password}/{channelID}.m3u8` is 406, plain text `This server plays MPEG-TS.` A player whose account only lists `ts` should not ask. Returning the transport stream under an HLS name would hand it a file it cannot parse.

`GET /live/{login}/{password}/icon/{channelID}` serves the same bytes as `GET /media/art/channel/{channelID}`, after the login checks out and the channel is in the lineup. IPTV apps fetch a logo with no `Authorization` header, and `/media/art/` is watch-scoped while device sign-in is on, so the icon cannot be that URL. The open M3U gains no `tvg-logo` in this version. A 401 logo is worse than a missing one.

`/timeshift/…` is not registered. Catch-up stays off (`tv_archive` 0). The live ring is not a wall-clock archive.

VOD from recordings is not this version. The empty VOD and series arrays are what a refresh expects when the account has no movies.

## Rate limits and logs

Limits match the pairing style: in memory, per process, keyed by the TCP peer, `X-Forwarded-For` ignored.

- 30 failed Xtream authentications per address per 10 minutes. The next one is 429 until the window moves. A good login does not count.
- The cap is checked and recorded under one lock.

A `bw_` token is not guessable the way a 6-digit code is. The cap is there so a broken client cannot write the failure path forever.

The password must not land in a log line. Query keys `password` and `access_token` are removed from the request URL before a handler runs, the same way `access_token` already is. The `/live/…` handler reads the password from the path and does not log the path. A test captures `slog` during a failed login and a successful playlist build and fails if the password appears. The create response is the one place the raw password is written, and that response is `Cache-Control: no-store`.

## Web

The block is Settings › Share with other apps, under the M3U and XMLTV lines that are already there.

A switch, "Require a login for the shared playlist", bound to `exportAuth`. Off: "The M3U and XMLTV addresses stay open on your home network." On: "Those addresses need a device login. The HDHomeRun tuner port stays open."

A button, "Add an app", asks for a name and then shows the server, username, password, M3U URL, and XMLTV URL once, with copy buttons and the line "Copy these now. The password is not shown again." Leaving the page drops them from memory. The new row shows in Devices as kind App, and Revoke there revokes the login. The password is not in `localStorage`.

The existing M3U and XMLTV lines stay. They are the open addresses. When `exportAuth` is on, the hint under them says they need a login, and points at Add an app.

## Apple

Not this version. The apps do not add an Xtream login. The server URL and the one-time password are copied from the web settings page.

## Broadwave API

These are the only additions to `api/openapi.yaml`. Regenerate the TypeScript and Swift schema types. Do not add `player_api.php` to the generated client. That contract is the Go tests below.

| Method and path | Auth when sign-in is on | Body |
| --- | --- | --- |
| `POST /clients/iptv` | admin | `{name?}` → `{device, username, password, server, m3u, xmltv}` |
| `PUT /settings` | admin | accepts `exportAuth` of `0` or `1` |
| `GET /settings` | admin | always includes `exportAuth`, never a password |

`GET /clients` gains `login` on the device, omitted when empty.

## Tests

Go, in `httpapi`, against the fake store. No tuner, no Docker.

- A good login returns `auth` 1 as a number, `status` `"Active"`, and `allowed_output_formats` `["ts"]`. `server_info.url` has no scheme. A wrong password, an unknown username, and a revoked login return the same `auth` 0 object, without the password, HTTP 200.
- `get_live_streams` uses `channels.id` as `stream_id` and the XMLTV id as `epg_channel_id`. A hidden channel is absent. `get_vod_categories` and `get_series` are `[]`.
- `xmltv.php` channel ids equal the guide export's channel ids for the same store. A programme title is plain text. `get_short_epg` base64-decodes to that title.
- `get.php` with `output=ts` contains `/live/{login}/{password}/{id}.ts`, the `tvg-id`, and the mosaic URL for a listed mosaic. `output=m3u8` still ends the stream lines in `.ts`.
- `GET /live/…/{id}.ts` for a lineup channel is `video/mp2t` through the existing export hook the stream tests already use. A hidden id is 404. `.m3u8` is 406.
- The same handler answers under `/xtream/…`. `GET /` is still the web app.
- `exportAuth` off: `/export/lineup.m3u` is 200 with no token. `exportAuth` on: 401 with no token, 200 with a watch token's `access_token`. The emulator's routes, when the test harness mounts them, do not read the setting.
- A failed login and a playlist response do not contain the password in captured `slog` output.
- The 31st failed authentication from one address in the window is 429.
- A public `Host` that is not in `Hosts` is still 421.

Web: a component test that Add an app renders the password once and clears it when the section unmounts, and that the open M3U line is still the untokened `/export/lineup.m3u`.

Examples in tests use fictional display numbers. No real call signs and no one's home address.

The accept for the implementation is those tests. A curl against the local fake-tuner harness (`BROADWAVE_E2E=1`) checks the same routes. A session in TiviMate or IPTV Smarters is a later check, on a machine that has one.

## Later

- A profile owns a login and filters the lineup, the favorites, and the guide. Tokens stay per device.
- Stable HLS at `/live/…/{id}.m3u8`, advertised in `allowed_output_formats`, for a player that will not open MPEG-TS.
- Catch-up from the ring buffer, with `tv_archive` and `/timeshift/…`, once the window is long enough to be honest.
- Recordings as VOD. Empty arrays stay correct until that exists.
- Mosaics in `get_live_streams`, once they have an integer id that cannot collide with `channels.id`.
- A switch that also requires a token on port 8478. Default remains open.
