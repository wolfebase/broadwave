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
  && apt-get install -y --no-install-recommends ffmpeg xz-utils ca-certificates mesa-va-drivers \
     $(if [ "$TARGETARCH" = "amd64" ]; then echo intel-media-va-driver; fi) \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/broadwave /usr/local/bin/broadwave
EXPOSE 8477
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s CMD ["broadwave", "-healthcheck"]
VOLUME /config
ENTRYPOINT ["broadwave"]
CMD ["-config", "/config", "-addr", ":8477"]
