# Security

Broadwave runs on your network and serves the apps in the house. Anyone who can reach the server's address can watch, record, and change settings. Keep that address on the home network. Accounts come later.

## Requests

A browser page from another site cannot change the server. A request that changes something has to come from this server's own pages, or from an app that is not a browser. The host name on the request has to be this server.

The tuner interface this server offers to other apps uses that same host check.

## Fetches

Playlists, guides, artwork, scoreboards, the release check, and tuner requests use http or https. Other schemes are refused. A path to a file on this machine is not a playlist address; upload the file instead. Link-local addresses, and the well-known cloud metadata names and addresses, are refused. A hex spelling of an address is refused too. A tuner on a private network address still works. A tuner that answers only on a link-local address does not.

When ffmpeg or ffprobe opens a URL, the input is limited to http and https. A playlist cannot hand either program a file on this machine. A recording on disk is still opened as a file.

## Files

A channel number from a playlist is cleaned before it becomes a recording's file name, so the file stays in the recordings folder. Show and season folders are cleaned the same way. Artwork, backups, and the library are served from ids and from names that stay inside their own folders.

## Size

A playlist upload, a picture, and a guide each stop at a fixed size. An app copying the broadcast stops at 32 streams at once. A picture whose header claims a huge width or height is not decoded. A compressed guide that expands past the cap is refused, and xz is held to a memory limit while it expands. An event socket keeps its message cap. The server holds a fixed number of those sockets and closes the next one.

Playlist text, guide XML, and the tables inside a broadcast are read with their own caps. A fragment that claims a huge sample count and no per-sample fields is dropped. A guide request longer than 15 days is refused.

## Secrets

A password, token, secret, or key in a playlist or guide address, and a tuner DeviceAuth, are masked before they are shown or written into diagnostics and the support bundle. The field names are password, token, secret, api_key, apikey, access_token, passwd, and pwd, matched either way. A password in an Xtream-style stream path (`/live/<user>/<password>/<id>`) is masked too. The settings page shows the guide address with the password removed. Saving that page again keeps the password that was already stored. The process log drops those values, and ffmpeg progress goes to that log. A tuner request that fails does not repeat the address. An artwork lookup that fails does not repeat the key.

## Limits to know

Anyone on the network can use the server until accounts exist.

On plain http, a page on another site can still open an export or a tuner URL. A cookie on those addresses would stop other players that fetch them with no browser cookie of their own.

The encoder's command line still shows the stream address it was given. The log of its progress does not.

An HLS name is resolved before the tune. If any answer is link-local or a metadata address, the tune is refused. The encoder resolves that name again, and a later segment or key can still ask for a blocked address.

A tuner that uses only a link-local address is refused.

Recordings stay the original broadcast. Transcoding is only for playback.
