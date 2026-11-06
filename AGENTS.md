# Agent instructions

Telegram bot written in Go that checks prices (or any number) on websites and APIs on a schedule and messages the user when a value meets their criteria. Single binary, no database. Runs in Docker on a home server.

## Project layout

| Path | What it does |
|---|---|
| `main.go` | Entry point. Starts `botfixer` and stops on Ctrl+C / SIGTERM. |
| `botfixer/` | Telegram wiring: long polling, chat allow-list, routes messages and button clicks to `CommandHandler`. |
| `handlers/command_handler.go` | All bot commands (`/run`, `/stop`, `/status`, `/interval`, `/mute`, ...), in `commandMap`. |
| `handlers/tracker.go` | One `Tracker` = one goroutine that runs its behavior on a timer. Status, errors, backoff. |
| `handlers/tracker_behavior.go` | Connects a tracker to its client (API or scraper). |
| `handlers/tracker_state.go` | Saves running trackers to `STATE_FILE` (JSON) and resumes them after a restart. |
| `handlers/navigation_state.go` | Per-chat menu history for the "Return" button. In memory only. |
| `clients/` | `api_client.go` reads JSON with gjson paths; `scraper_client.go` reads HTML with colly/CSS selectors. `client.go` checks notify criteria. |
| `services/http_service.go` | Shared HTTP code: timeout, browser-like headers, `HTTPStatusError`. |
| `config/` | Reads env vars (`.env`) and the tracker JSON files. Field names are in the `Tracker` struct. |
| `helpers/` | Telegram messenger interface, menus/buttons, message formatting. |
| `utilities/` | Interval parsing (`ParseDurationWithDays` supports `m`, `h`, `d`), small string helpers. |
| `tracker_configs/` | Tracker JSON files (private, not in git; only `*.example` files are). |
| `deployment/` | Docker Compose file and start/stop/update scripts for the server. |

## How a tracker run works

1. A user sends `/run <code>` → `CommandHandler` calls `CreateTracker` → `Tracker.Start()` starts a goroutine.
2. The goroutine runs right away, then waits `CurrentInterval` between runs.
3. Each run: client fetches the URL → extracts the value → compares it to `notifyCriteria` → sends a Telegram message if matched (unless muted).
4. On errors: counted; after `ERROR_NOTIFY_LIMIT` in a row the user gets one warning. On HTTP 403/429/503 the wait doubles (max 8x, max 24h, respects `Retry-After`) until a run succeeds.
5. Starting, stopping, muting or changing the interval saves the state file.

## Gotchas

- **Concurrency:** tracker fields are shared between the tracker goroutine and command handlers. Lock `t.mu` when touching `t.status`; read status via `Status()` (returns a copy). `CommandHandler.mu` guards `runningTrackers`. Tests should pass with `-race` (runs in CI; needs cgo locally).
- **Minimum interval:** `MIN_INTERVAL` (default 10m) is enforced in `handleSetInterval` (rejects) and `CreateTracker` (raises). Keep both in sync.
- **Tracker config field names:** the JSON names come from the `config.Tracker` struct tags (`dataUrl`, `dataExtractionPath`, ...). If you change them, update `tracker_configs/README.md` and the `*.example` files too.
- **Scraper:** a new colly collector per run (callbacks pile up otherwise). `<meta>` tags have their value in the `content` attribute. Prices like `1 234,56 €` are cleaned to `1234.56`.
- **One bot instance per API key:** Telegram allows only one poller. Use a separate dev bot locally, never the production key.
- **Updates are handled one at a time:** `botfixer` handles Telegram updates one by one (`updateMu`). Don't add blocking work in command handlers.
- **Tests with Telegram:** use the fake messenger in `handlers/tracker_test.go` or the fake API server in `botfixer/bot_fixer_test.go`. Never call the real Telegram API in tests.
- **Docker image is `scratch`:** no shell, no tools. Debug with logs only. Tracker configs are mounted, not built in.
- **Windows line endings:** the working copy uses CRLF, git stores LF. Keep CRLF in files you edit (Edit tools do this; scripts that write files may not). Shell scripts (`*.sh`) must stay LF.

## Code style

- Match the surrounding code. Log lines start with a `[Component]` prefix, e.g. `log.Printf("[Tracker] ...")`.
- Comments are full sentences ending with a period when they document a declaration (`godot` linter).
- No magic numbers: use named constants (`mnd` linter).
- Blank line before `return` after other statements in a block (`nlreturn` linter).
- Don't multiply two `time.Duration` values; multiply by `time.Duration(intValue)` (`durationcheck` linter).
- Add or update tests next to the code (`*_test.go` in the same package). Table tests are the norm.
- Update `README.md` / `.env.example` when adding env vars, commands or user-visible behavior.

## Before finishing a task

Always lint and test the changes before reporting a task as done:

```bash
golangci-lint run --timeout=5m ./...
go test ./...
```

- Use the same golangci-lint version as CI (see `.github/workflows/golangci-lint.yml`), built with the Go version from `go.mod`. An older build refuses to run ("the Go language version used to build golangci-lint is lower than the targeted Go version"). Install the matching one with `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@<version>`.
- On Windows the CRLF working copy makes `gci` report "File is not properly formatted" for random unchanged files. CI checks out LF, so either ignore `gci` hits on files you didn't touch or lint an LF-normalised copy of the project.
- Fix every reported issue in the changed code; don't add `//nolint` unless there's a real reason, and say why in the comment.

## Workflow

- Don't commit or push unless asked; leave changes for review.
- The main branch is `develop`. Releases are built by `.github/workflows/release.yml`.
- Deployment is manual on the server (`git pull` + `deployment/update.sh`); don't change deployment scripts without being asked.
