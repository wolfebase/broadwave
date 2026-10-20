# 0001: Monorepo and Apple-first clients

Accepted, 2026-09-22.

## Context

The prototype was a Go server with an embedded web app and guidance that deferred native apps until the website proved itself. The goal is now a public product whose flagship experience is on iPhone and Apple TV.

## Decision

- One repository: `server/` (Go), `web/`, `apple/`, `design/`, `api/`, `deploy/`, `docs/`. A root `go.work` and `Makefile` let everything build from the root.
- Native iOS and tvOS apps are built now, in parallel with the web app. The iOS app runs on iPhone and iPad. Apple Watch is a remote for the other screens and plays no video. Android and other platforms are out of scope.
- The OpenAPI document is the contract between the server and all clients.

## Update, 2026-10-20

The iOS and tvOS apps moved to a private repository and are not open source. This repository holds the server, web app, API, design tokens, and docs.

## Consequences

Changes that touch the API, web, and apps can land together and stay consistent. The server stays a single binary with the web app embedded.
