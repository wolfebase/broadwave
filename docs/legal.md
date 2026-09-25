# Licenses

Broadwave is licensed under the Apache License, Version 2.0. The text is `LICENSE`. Third-party names, versions, and licenses are in `NOTICE` at the repository root.

## ffmpeg

The container images install the Debian package `jellyfin-ffmpeg7` from <https://repo.jellyfin.org/debian> (suite bookworm). They do not install Debian's ffmpeg, and they do not pin a package version.

On 2026-09-25 that suite's candidate for amd64 and arm64 was `jellyfin-ffmpeg7` 7.1.4-3-bookworm. The corresponding source is tag `v7.1.4-3` of <https://github.com/jellyfin/jellyfin-ffmpeg> (<https://github.com/jellyfin/jellyfin-ffmpeg/archive/refs/tags/v7.1.4-3.tar.gz>). That tag's `debian/rules` passes `--enable-gpl` and `--enable-version3`, so this ffmpeg is GPL-3.0-or-later. It also links libfdk-aac, as the Jellyfin package ships it.

A later build may install a newer `jellyfin-ffmpeg7` from the same suite. Package version `X.Y.Z-N-bookworm` corresponds to tag `vX.Y.Z-N` in that repository. The image repeats this offer at `/usr/share/doc/broadwave/ffmpeg-source-offer.txt`.

## Blender

Store art and the demo use Big Buck Bunny, Sintel, Tears of Steel, and Elephants Dream. They are Blender Foundation films under CC BY. The same two sentences are on the About screen in the web app and the Apple apps.

## Trademarks

HDHomeRun, Apple TV, Plex, and Channels are trademarks of their owners. Broadwave names them only to describe compatibility.
