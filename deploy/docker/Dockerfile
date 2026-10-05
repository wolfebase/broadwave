FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
# Browser tests use the machine's Chrome. The image only builds the site.
ENV PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
RUN npm ci
COPY web/ ./
# The typecheck decodes the golden API responses (web/src/api/contract.ts).
COPY api/fixtures /src/api/fixtures
RUN npm run build

FROM golang:1.26-bookworm AS build
ARG VERSION=dev
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /src/server/cmd/broadwave/assets/web ./cmd/broadwave/assets/web
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/broadwave ./cmd/broadwave

# comskip (GPL-2.0) finds commercial breaks in recordings. It runs as its own
# program, statically linked to a small LGPL ffmpeg (decoders and the TS
# demuxer only), so it does not mix with the GPL-3.0 ffmpeg below.
FROM debian:bookworm-slim AS comskip
ARG FFMPEG_VERSION=7.1.4
ARG COMSKIP_COMMIT=a140b6ac8bc8f596729e9052819affc779c3b377
RUN apt-get update \
  && apt-get install -y --no-install-recommends build-essential autoconf automake libtool pkg-config curl ca-certificates xz-utils libargtable2-dev
WORKDIR /src
RUN curl -fsSL https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz | tar xJ \
  && cd ffmpeg-${FFMPEG_VERSION} \
  && ./configure --prefix=/opt/ff --enable-static --disable-shared --disable-programs --disable-doc \
       --disable-x86asm --disable-autodetect --disable-network --disable-everything \
       --enable-protocol=file --enable-demuxer=mpegts \
       --enable-decoder=mpeg2video,h264,hevc,ac3,eac3,aac,aac_latm,mp2 \
       --enable-parser=mpegvideo,h264,hevc,ac3,aac,aac_latm,mpegaudio >/dev/null \
  && make -j"$(nproc)" >/dev/null \
  && make install >/dev/null
RUN mkdir -p /out \
  && curl -fsSL -o /out/comskip-source.tar.gz https://github.com/erikkaashoek/Comskip/archive/${COMSKIP_COMMIT}.tar.gz \
  && tar xzf /out/comskip-source.tar.gz \
  && cd Comskip-${COMSKIP_COMMIT} \
  && ./autogen.sh >/dev/null \
  && ./configure \
       ffmpeg_CFLAGS="-I/opt/ff/include" \
       ffmpeg_LIBS="-L/opt/ff/lib -lavformat -lavcodec -lswscale -lavutil -lm -lpthread" >/dev/null \
  && make -j"$(nproc)" >/dev/null \
  && install -s -m 755 comskip /out/comskip \
  && cp LICENSE /out/comskip-LICENSE \
  && ! ldd /out/comskip | grep -q 'not found'

FROM debian:bookworm-slim
ARG TARGETARCH
# Broadwave is Apache-2.0. jellyfin-ffmpeg7 at tag v7.1.4-3 is built with
# --enable-gpl and --enable-version3, so the bundled ffmpeg is GPL-3.0-or-later.
LABEL org.opencontainers.image.source="https://github.com/wolfebase/broadwave" \
      org.opencontainers.image.licenses="Apache-2.0 AND GPL-3.0-or-later"
RUN apt-get update \
  && apt-get install -y --no-install-recommends curl ca-certificates gnupg xz-utils libargtable2-0 mesa-va-drivers \
     $(if [ "$TARGETARCH" = "amd64" ]; then echo intel-media-va-driver; fi) \
  && mkdir -p /etc/apt/keyrings \
  && curl -fsSL https://repo.jellyfin.org/jellyfin_team.gpg.key | gpg --dearmor -o /etc/apt/keyrings/jellyfin.gpg \
  && printf '%s\n' 'Types: deb' 'URIs: https://repo.jellyfin.org/debian' 'Suites: bookworm' 'Components: main' "Architectures: ${TARGETARCH}" 'Signed-By: /etc/apt/keyrings/jellyfin.gpg' > /etc/apt/sources.list.d/jellyfin.sources \
  && apt-get update \
  && apt-get install -y --no-install-recommends jellyfin-ffmpeg7 \
  && ln -sf /usr/lib/jellyfin-ffmpeg/ffmpeg /usr/local/bin/ffmpeg \
  && ln -sf /usr/lib/jellyfin-ffmpeg/ffprobe /usr/local/bin/ffprobe \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/broadwave /usr/local/bin/broadwave
COPY --from=comskip /out/comskip /usr/local/bin/comskip
COPY --from=comskip /out/comskip-LICENSE /out/comskip-source.tar.gz /usr/share/doc/broadwave/
COPY LICENSE /usr/share/doc/broadwave/LICENSE
COPY NOTICE /usr/share/doc/broadwave/NOTICE
COPY docs/legal.md /usr/share/doc/broadwave/legal.md
RUN mkdir -p /usr/share/doc/broadwave \
 && printf '%s\n' \
    'Broadwave is Apache-2.0. See NOTICE in this directory and https://github.com/wolfebase/broadwave.' \
    'This image installs the Debian package jellyfin-ffmpeg7 from https://repo.jellyfin.org/debian suite bookworm. It does not install Debian ffmpeg. The package version is not pinned.' \
    'On 2026-09-25 the bookworm candidate for amd64 and arm64 was jellyfin-ffmpeg7 7.1.4-3-bookworm.' \
    'Corresponding source: https://github.com/jellyfin/jellyfin-ffmpeg/archive/refs/tags/v7.1.4-3.tar.gz (tag v7.1.4-3).' \
    'That tag is built with --enable-gpl and --enable-version3, so this ffmpeg is GPL-3.0-or-later. The Jellyfin package also links libfdk-aac.' \
    'A later build may install a newer jellyfin-ffmpeg7 from the same suite. Package version X.Y.Z-N-bookworm corresponds to tag vX.Y.Z-N in https://github.com/jellyfin/jellyfin-ffmpeg.' \
    'comskip is GPL-2.0 (comskip-LICENSE in this directory), built from https://github.com/erikkaashoek/Comskip at commit a140b6ac8bc8f596729e9052819affc779c3b377; its source is comskip-source.tar.gz in this directory.' \
    'comskip is statically linked to ffmpeg 7.1.4 built without --enable-gpl (LGPL-2.1-or-later): https://ffmpeg.org/releases/ffmpeg-7.1.4.tar.xz. It runs as its own program and uses the Debian package libargtable2-0 (LGPL-2.0-or-later).' \
    > /usr/share/doc/broadwave/ffmpeg-source-offer.txt
# NVENC needs the video capability; the NVIDIA runtime grants only compute and utility by default.
ENV NVIDIA_DRIVER_CAPABILITIES=compute,video,utility
EXPOSE 8477
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s CMD ["broadwave", "-healthcheck"]
VOLUME /config
ENTRYPOINT ["broadwave"]
CMD ["-config", "/config", "-addr", ":8477"]
