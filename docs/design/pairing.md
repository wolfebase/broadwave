# Device pairing

A Broadwave server on a home network has no accounts. Phones, TVs, browsers, and scripts on that network call the API directly. That stays the default. Device sign-in is an opt-in setting for a server that can be reached past the house (a forwarded port, a public name in `BROADWAVE_HOSTS`). Each phone, TV, or browser then holds its own token. Revoking that device stops the token. The LAN behavior of an untouched server does not change.

This is the design for that switch, the tokens, and the pairing codes. Accounts and profiles can sit on top of it later. They are not in this version.

## Default

The setting is `deviceAuth`. Missing or `0` means off. `1` means on.

While it is off:

- Every existing client keeps working. No header, cookie, or query is required.
- A valid token is still recognized, so a paired device can be named, and its last-seen time updates.
- A missing or unknown token is ignored. A watch token does not reduce what the home network can do.
- Pairing still works, so devices can be ready before the switch is turned on.

While it is on, `/api`, `/api/v1`, and `/media` require a token whose scope covers the route. These stay open, with no token:

- `GET /health` and `GET /server` (discovery and liveness).
- `GET /profile` (says whether sign-in is on).
- `POST /pair` and `GET /pair/{id}` (a new device asks and waits).
- `POST /pair/claim` (a new device enters a code).
- `GET /clients/me` (this token, or an empty seat).
- The web app's files (`GET /` and hashed assets), so the pairing page can load.
- `OPTIONS`.
- `/export/*` (M3U, XMLTV, and the exported streams). Plex, Jellyfin, and Channels have no field for a Broadwave token. The host check still refuses a public name the server was not given.
- The HDHomeRun emulator on port 8478. It is a separate listener with the SiliconDust routes, and those clients cannot send this token.

Turning sign-in on does not lock the emulator or the exports. Publishing the server on a public address still exposes those. A later version can add a second switch for them.

A route under `/api` that the scope table does not name requires admin. A new route is closed until it is classified.

## Scopes

A token has one or more of `watch`, `record`, and `admin`. A higher scope includes the lower ones: admin may record and watch, record may watch.

| Scope | What it may do |
| --- | --- |
| watch | Guide, playback, the realtime socket, `/media`, progress, and marking a recording watched. |
| record | Watch, plus recordings, passes, markers, teams, and schedule changes. |
| admin | Record, plus settings, sources, tuners, backups, setup, channel edits, and pairing approval. |

Storage, diagnostics, metrics, and the support bundle are admin. They name paths and versions.

`GET /passes` is watch (the schedule is on screen while you watch). Creating or deleting a pass is record.

## Tokens

A token is `bw_` plus 32 random bytes, encoded base64url with no padding. The server stores only the SHA-256 hex of the whole string. The raw token is returned once, to the device that will keep it. It is not written to the settings table, the support zip, or a log line.

The client may send it three ways. The first match wins:

1. `Authorization: Bearer <token>`
2. Cookie `bw_token` (HttpOnly, `SameSite=Strict`, `Path=/`, no `Secure` flag, because a home server is often plain HTTP on the LAN)
3. Query `access_token`

The query form exists because a player cannot always set a header. AVPlayer and a browser WebSocket are the two cases. The server removes `access_token` from the request URL before the handler runs, so a later log of that URL does not include the token. Web media requests send the `Authorization` header from hls.js instead of putting the token in the playlist URL.

Last seen is updated at most once a minute per device. A revoked token does not authenticate. While sign-in is off, a revoked token is ignored and the cookie is cleared.

### Recovery

Enabling sign-in from a request that is not already an admin mints an admin token for that browser, named "This browser", and returns it once as `deviceToken` on that settings response, with the cookie set. The web app stores it and does not send it back as a setting. Echoing `deviceToken` on a later save is ignored.

If every admin token is lost, the server stays locked. Recovery is on the machine that holds the catalog: set `deviceAuth` to `0` in the `settings` table, or delete the `client_devices` rows, and start the server again. There is no network backdoor. Loopback is not treated as an exception, because a reverse proxy on the same computer would look like loopback and would reopen the API.

## Pairing

Codes are 6 digits, shown as `482 913`. Spaces and dashes are ignored on entry. A code lasts 10 minutes. At most 20 pairings can be waiting. The code and the poll secret are stored as SHA-256 only.

Two directions:

**The new device shows the code.** `POST /pair` with a name, a kind (`phone`, `tv`, `web`, or `other`), and scopes. The response is `{id, code, expiresAt, pollSecret}`. The device shows the code and polls `GET /pair/{id}?secret=`. An admin (or anyone, while sign-in is off) calls `POST /pair/approve` with the code and optional scopes. The raw token is held in process memory under the pairing id, not in SQLite, so a catalog backup cannot contain a live token. The token is stored before the approved row is committed, and that lock is held across the commit, so a poll cannot see `approved` before the token is waiting. Poll responses send `Cache-Control: no-store`. `HEAD` does not take the token. The token stays until a `GET` writes its body; a failed write leaves it for the next poll. That poll receives the token once and the cookie. A later poll gets the device and no token. If the process restarts before a successful poll, the token is gone. The poll still says `approved`, without a token, and the device row stays in the list until someone revokes it. The device shows a new code. The screen waits through one approved poll that has no token, then says the token was missed.

**An admin shows the code.** `POST /pair/code` with scopes and an optional kind. The response is `{id, code, expiresAt}` and no poll secret. The new device calls `POST /pair/claim` with the code, its name, and its kind, and receives the token in that response.

Approve matches only a code the device is showing. Claim matches only a code an admin created. Scopes on approve replace the scopes the device asked for. Scopes on a claim are the ones the admin chose when the code was created. Omitted scopes mean `watch`. An unknown kind is stored as `other`.

### Rate limits

Limits are in memory, per server process, keyed by the TCP peer address. `X-Forwarded-For` is not trusted. The per-address cap is counted under one lock. The server-wide failure cap is checked before the handler runs and recorded after a bad code, so overlapping calls from many addresses can pass that check together.

- 10 pairing calls per address per 10 minutes (`POST /pair`, `/pair/code`, `/pair/approve`, `/pair/claim`).
- 30 failed code submissions per 10 minutes on the whole server, and 10 per address.
- 5 wrong poll secrets for one pairing deny that pairing.
- A full waiting list or a tripped limit returns 429 with code `rate_limited`.

A 6-digit code plus those caps is the whole guessing budget. Codes are not written down in the support bundle.

## Web

The token lives in `localStorage` under `bw.deviceToken`. API fetches send `Authorization`. The socket URL adds `access_token` when a token is stored, because the browser WebSocket constructor cannot set that header. hls.js sends `Authorization` on playlist and segment requests. The cookie covers clients that send cookies and no header.

Settings has a Devices section: the sign-in switch, the paired list with Revoke, a code to type on another device, and a field to approve a code a TV is showing. `/pair` is the screen a new browser opens: enter a code, or show one and wait. A 401 from the API opens that screen.

Tuners stay on `/devices`. Paired screens are `/clients`. The settings heading is Devices because that is the word on the screen. The tuner list remains "Tuners and channels".

## Apple

Not built in this version. The apps already share the server URL through the keychain item `SharedServer` (`AppStore.swift`, read by the Top Shelf and the widgets). A device token belongs in that same keychain group, not in UserDefaults, so the Top Shelf, the widgets, and `RecordIntent` can send it. `APIClient` should set `Authorization: Bearer`. HLS requests from AVPlayer should use `access_token` on the URL, since those requests do not go through `APIClient`.

A TV pairs by calling `POST /api/v1/pair`, showing the code, and polling `GET /api/v1/pair/{id}?secret=` until the token arrives, then saving it. The Apple settings sentence that still says a password is required is unchanged until that screen exists.

## What a later accounts pass adds

- A person, with more than one device, and a way to tell two people in one house apart. Tokens stay per device. An account would own a set of tokens, not replace them.
- A switch that also requires a token for `/export` and port 8478, for a server that is deliberately published. Default remains open, so Plex on the LAN does not break when device sign-in is turned on.
- The Apple pairing screen and keychain item described above.
- Profiles (whose recordings, whose watch history) as a separate layer. Nothing in the token table is a profile.

## API

All of these are under `/api/v1` and `/api`.

| Method and path | Auth when sign-in is on | Body / result |
| --- | --- | --- |
| `POST /pair` | public | `{name, kind, scopes?}` → `{id, code, expiresAt, pollSecret}` |
| `GET /pair/{id}?secret=` | public | `{state, expiresAt?, token?, device?}` |
| `POST /pair/approve` | admin | `{code, scopes?}` → `{device}` |
| `POST /pair/code` | admin | `{kind?, scopes?}` → `{id, code, expiresAt}` |
| `POST /pair/claim` | public | `{code, name, kind}` → `{token, device}` and `Set-Cookie` |
| `GET /clients` | admin | `{devices}` unrevoked, no tokens |
| `GET /clients/me` | public | `{device: ClientDevice \| null, auth: "local-open" \| "device"}` |
| `DELETE /clients/{id}` | admin | `{devices}` |

`PUT /settings` accepts `deviceAuth` of `0` or `1`. The response that turns it on may include `deviceToken` once. `GET /settings` always includes `deviceAuth`, and never includes `deviceToken`.

Errors use the usual envelope. 401 is `unauthorized` ("Sign in from a paired device."). 403 is `forbidden` ("This device can't do that."). 429 is `rate_limited`.
