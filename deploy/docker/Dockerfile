FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-bookworm AS build
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /src/server/cmd/ota-viewer/assets/web ./cmd/ota-viewer/assets/web
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ota-viewer ./cmd/ota-viewer

FROM debian:bookworm-slim
ARG TARGETARCH
RUN apt-get update \
  && apt-get install -y --no-install-recommends ffmpeg ca-certificates mesa-va-drivers \
     $(if [ "$TARGETARCH" = "amd64" ]; then echo intel-media-va-driver; fi) \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/ota-viewer /usr/local/bin/ota-viewer
EXPOSE 8477
VOLUME /config
ENTRYPOINT ["ota-viewer"]
CMD ["-config", "/config", "-addr", ":8477"]
