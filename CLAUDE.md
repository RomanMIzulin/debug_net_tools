# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

WebSocket debugging proxy (`net_proxy_tools`) — a CLI tool that acts as a man-in-the-middle proxy for WebSocket connections, recording frames for inspection, replay, and editing. Written in Go 1.25.5.

The tool sits between a WS client and server: client connects to the proxy, proxy connects to the target server, and all frames are captured bidirectionally.

## Build & Development Commands

```bash
go build ./...          # build all packages
go test ./...           # run all tests
go test -race ./...     # run tests with race detector (preferred)
go vet ./...            # static analysis
```

No Makefile or task runner exists yet. Use standard Go toolchain.

## Architecture

The project follows Go's `internal/` convention with two packages so far:

- **`internal/core`** — Domain types: `Frame` (a single WS frame with opcode, payload, direction, timestamp), `Session` (a collection of frames with lifecycle states), `Direction` enum (ClientToServer/ServerToClient), `SessionState` enum (Created→Connecting→Active→Paused→Closing→Closed/Error).

- **`internal/storage`** — SQLite-backed persistence using `modernc.org/sqlite` (pure Go, no CGO) and `squirrel` as SQL builder.

### Design Decisions (from docs/Intro.md)

- **Proxy approach** over pcap/eBPF — works at application layer, avoids raw packet parsing and TLS key extraction.
- **Fan-out pub/sub via Go channels** for distributing events to consumers (TUI, file writer, etc.). Each subscriber gets its own channel. File writer is part of core (not a subscriber) to guarantee persistence.
- **SQLite** as primary storage (not JSONL) — better for multi-session queries and historical data. JSONL may be used for export.
- **Two proxy modes planned**: passive capture (record only) and interactive proxy (can inject/edit frames).

### Planned Components (implementation order)

1. Minimal proxy with stdout logging
2. Event type + JSONL writer
3. Session management (list, inspect, frame ranges)
4. Replay with timing
5. REPL for interactive attach
6. Edit & resend (interactive proxy mode)
7. HAR export
8. TUI via bubbletea

## Key Dependencies

- `modernc.org/sqlite` — pure-Go SQLite (no CGO required)
- `github.com/Masterminds/squirrel` — SQL query builder
- `github.com/google/uuid` — session IDs

## Linting

Use `golangci-lint` with `exhaustive` and `gochecksumtype` enabled — critical because Go lacks real enums, so the linter catches unhandled iota cases.

## Code Conventions

- Use English for all code comments and docstrings.
- `SessionState` and `Direction` use iota enums — always handle all cases exhaustively in switches.
