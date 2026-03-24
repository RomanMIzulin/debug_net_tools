# 13 — Hybrid CLI Design: wsproxy

## Philosophy

Лучшее от двух подходов:
- **websocat/wscat**: default action без subcommand, URL как главный аргумент, мгновенный старт
- **mitmproxy/gh**: subcommands для вторичных операций, graduated verbosity, `--json` для машин

Принцип: **самое частое действие = самое короткое**.

## Command Grammar

```
wsproxy [command] [args] [flags]
```

Нет command + есть URL → proxy mode (default action).
Нет command + нет URL → help.

---

## Commands

### Default: Proxy Mode

Самая частая операция — запустить прокси. Ноль subcommand-ов:

```bash
wsproxy ws://target:3000                      # proxy, listen on :8080
wsproxy ws://target:3000 -l :9090             # explicit listen address
wsproxy ws://target:3000 -w capture.wsar      # proxy + record to file
wsproxy ws://target:3000 -v                   # show each frame
wsproxy ws://target:3000 -vv                  # frames + payload preview
wsproxy ws://target:3000 -vvv                 # frames + full payload dump
wsproxy ws://target:3000 -q -w capture.wsar   # silent recording
```

### connect — Client Mode (wscat-style REPL)

Прямое подключение к серверу без прокси. Для быстрого тестирования:

```bash
wsproxy connect ws://localhost:3000           # interactive REPL
wsproxy connect ws://localhost:3000 -w out.wsar  # REPL + record
```

REPL-команды (slash, как в wscat):
- `/ping [data]` — send ping frame
- `/pong [data]` — send pong frame
- `/close [code [reason]]` — initiate close handshake
- `/binary <hex>` — send binary frame
- `/status` — connection info

### sessions — Manage Recordings

```bash
wsproxy sessions                              # list all from SQLite
wsproxy sessions show <id>                    # show frames of a session
wsproxy sessions show <id> -f text            # filter by frame type
wsproxy sessions show <id> --json             # machine-readable
wsproxy sessions delete <id>                  # delete recording
```

Alias: `sess`

### replay — Replay Sessions

```bash
wsproxy replay <id>                           # replay to original target
wsproxy replay <id> --target ws://other:3000  # different target
wsproxy replay <file.wsar>                    # replay from file
wsproxy replay <id> --speed 2x               # accelerated
wsproxy replay <id> --speed 0                 # send all at once (stress test)
```

### export — Export Sessions

```bash
wsproxy export <id> --format har              # HAR (Chrome-compatible)
wsproxy export <id> --format jsonl | jq '.'   # pipe-friendly
wsproxy export <id> --format wsar             # native archive
```

### diff — Compare Sessions

```bash
wsproxy diff <id1> <id2>                      # frame-by-frame diff
wsproxy diff <id1> <id2> --ignore-timing      # ignore timestamp differences
```

---

## Flags

### Global

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--config` | | | Config file path |
| `--output` | `-o` | `table` | Output format: table, json, jsonl |
| `--verbose` | `-v` | 0 | Verbosity: -v, -vv, -vvv |
| `--no-color` | | `false` | Disable colored output |
| `--quiet` | `-q` | `false` | Suppress all diagnostic output |

### Proxy / Connect

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--listen` | `-l` | `:8080` | Listen address (proxy only) |
| `--write` | `-w` | | Record session to file |
| `--filter` | `-f` | | Frame filter: text, binary, ping, pong, close |
| `--header` | `-H` | | Extra HTTP headers (repeatable) |
| `--origin` | | | Origin header for handshake |
| `--subprotocol` | `-s` | | WebSocket subprotocol |

---

## Verbosity Levels

Inspired by mitmdump `--flow-detail` (0-4):

| Level | Flag | Output |
|-------|------|--------|
| 0 | `-q` | Silent (recording only, no terminal output) |
| 1 | _(default)_ | Connection lifecycle events + periodic summary |
| 2 | `-v` | Each frame: direction, opcode, size |
| 3 | `-vv` | Frame + payload preview (truncated to terminal width) |
| 4 | `-vvv` | Frame + full payload (hex dump for binary) |

### Level 1 example (default)
```
── connected ws://target:3000
── 14 frames (8 text, 4 binary, 2 ping/pong) in 12.3s
── closed 1000 normal
```

### Level 2 example (-v)
```
── connected ws://target:3000
→ TEXT    13B
← TEXT    24B
→ PING    0B
← PONG    0B
→ BIN   4.2KB
← CLOSE 1000
── closed 1000 normal
```

### Level 3 example (-vv)
```
── connected ws://target:3000
→ TEXT    13B │ Hello, World!
← TEXT    24B │ {"status":"ok","ts":17...
→ PING    0B │
← PONG    0B │
→ BIN   4.2KB │ [binary 4,218 bytes]
← CLOSE 1000 │ normal closure
```

### Level 4 example (-vvv)
```
── connected ws://target:3000
→ TEXT 13B
  Hello, World!
← TEXT 24B
  {"status":"ok","ts":1709312400}
→ BIN 4218B
  00000000  89 50 4e 47 0d 0a 1a 0a  00 00 00 0d 49 48 44 52  |.PNG........IHDR|
  00000010  00 00 01 00 00 00 01 00  08 02 00 00 00 d3 10 3f  |...............?|
  ...
```

---

## Configuration

XDG-compliant: `~/.config/wsproxy/config.toml`

```toml
[proxy]
listen = ":8080"

[output]
format = "table"
color = "auto"
verbosity = 1

[recording]
auto_save = true
storage = "~/.local/share/wsproxy/sessions.db"
```

Priority: **CLI flags > env (WSPROXY_*) > config file > defaults**

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Connection refused / target unreachable |
| 3 | Listen port already in use |
| 4 | Invalid arguments / URL |

## Error Messages

Actionable, with suggestions:

```
Error: connection refused to ws://localhost:3000
  Is the target server running?
  Try: wsproxy ws://localhost:3001

Error: listen :8080: address already in use
  Try: wsproxy ws://target:3000 -l :8081
```

---

## Signals

- **Ctrl-C (first)**: graceful shutdown — send close frames, flush recording, print summary
- **Ctrl-C (second)**: immediate exit
- **SIGHUP**: reload config file without restart

---

## Aliases

Built-in abbreviations (cobra aliases):

| Alias | Full command |
|-------|-------------|
| `sess` | `sessions` |
| `conn` | `connect` |
| `re` | `replay` |
| `ex` | `export` |
