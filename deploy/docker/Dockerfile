FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/ota-viewer ./cmd/ota-viewer

FROM debian:bookworm-slim
RUN apt-get update \
  && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/ota-viewer /usr/local/bin/ota-viewer
EXPOSE 8477
VOLUME /config
ENTRYPOINT ["ota-viewer"]
CMD ["-config", "/config", "-addr", ":8477"]
