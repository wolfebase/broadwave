FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-bookworm AS build
ARG VERSION=dev
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /src/server/cmd/waveguide/assets/web ./cmd/waveguide/assets/web
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/waveguide ./cmd/waveguide

FROM debian:bookworm-slim
ARG TARGETARCH
RUN apt-get update \
  && apt-get install -y --no-install-recommends ffmpeg ca-certificates mesa-va-drivers \
     $(if [ "$TARGETARCH" = "amd64" ]; then echo intel-media-va-driver; fi) \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/waveguide /usr/local/bin/waveguide
EXPOSE 8477
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s CMD ["waveguide", "-healthcheck"]
VOLUME /config
ENTRYPOINT ["waveguide"]
CMD ["-config", "/config", "-addr", ":8477"]
