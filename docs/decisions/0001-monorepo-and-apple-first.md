# 0001: Monorepo and Apple-first clients

Accepted, 2026-09-22.

## Context

The prototype was a Go server with an embedded web app and guidance that deferred native apps until the website proved itself. The goal is now a public product whose flagship experience is on iPhone and Apple TV.

## Decision

- One repository: `server/` (Go), `web/`, `apple/`, `design/`, `api/`, `deploy/`, `docs/`. A root `go.work` and `Makefile` let everything build from the root.
- Native iOS and tvOS apps are built now, in parallel with the web app. iPad and Apple Watch come after the iPhone and Apple TV apps ship. Android and other platforms are out of scope.
- The OpenAPI document is the contract between the server and all clients.

## Consequences

Changes that touch the API, web, and apps can land together and stay consistent. The server stays a single binary with the web app embedded.
