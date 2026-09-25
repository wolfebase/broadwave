FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
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

FROM debian:bookworm-slim
ARG TARGETARCH
RUN apt-get update \
  && apt-get install -y --no-install-recommends curl ca-certificates gnupg xz-utils mesa-va-drivers \
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
EXPOSE 8477
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s CMD ["broadwave", "-healthcheck"]
VOLUME /config
ENTRYPOINT ["broadwave"]
CMD ["-config", "/config", "-addr", ":8477"]
