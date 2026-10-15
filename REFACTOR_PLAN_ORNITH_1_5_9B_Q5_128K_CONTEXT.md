# Refactoring Plan — SOLID alignment

**Model:** `ORNITH_1_5_9B_Q5_128K_CONTEXT` • **Target root:** `/var/home/georgs/Documents/Projekti/price-tracker-bot/`

## 0. Scope and how this plan was produced
- **Status:** *plan* only (read + analyze). No application code changed; changes are proposed for review.
- Produced by exploring the whole repo: `main.go`, `botfixer/`, `handlers/*`, `clients/*`, `config/*`, `services/*`, `helpers/*`, `utilities/*`, plus `.golangci.yml`, `go.mod` and existing tests. Baseline verified with `go build ./...` (exit 0); Go toolchain `1.27.1`.
- The codebase already has solid foundations to work *from*: an explicit `TrackerBehavior` interface, a `Messenger` interface with a fake (`recordingMessenger`) in tests, a `Client` abstraction concept, table-style unit tests, and a very strict linter set (golangci-lint v2). Refactors below should build on these rather than tear them down.

## 1. Current architecture map
```
main.go ─▶ botfixer.BotFixer ─▶ handlers.CommandHandler
                          │                    ▲
                     long polling            tracker goroutines (Tracker)
                          │              ◀── behaviors ── clients ── services.GetRequest/colly
```
- `botfixer.NewBotFixer()` wires the Telegram bot, config and a `CommandHandler`; `Run(ctx)` blocks on `b.Bot.Start`.
- `handlers.CommandHandler` owns: (a) the `commandMap`, dispatch (`HandleCommand`), tracker registry (`runningTrackers` + add/remove/get), *and* per-chat navigation state; it also formats help/status text and builds menus.
- `handlers.Tracker` is a long-lived goroutine owner: start/run/stop, error counting, exponential backoff on blocking HTTP codes, status snapshots (all under one `mu`). Its run loop delegates the data step to an injected `TrackerBehavior.Execute()`.
- `handlers.TRackerBehavior` has two implementations — `APITrackerBehavior`, `ScraperTrackerBehavior` — each wrapping a concrete client with an identical `Execute(*config.Tracker) (string, string, error)`.
- `clients.PublicAPIClient` / `ScraperClient` fetch the URL and extract a value; both then call package-level `ProcessNotificationCriteria(...)` before returning a bundled `DataResult{CurrentValue, NotificationMessage}`.
- `config.Configuration` is stored as a process-global singleton via `GetConfig()`.

## 2. SOLID analysis (evidence: file:line)
### 2.1 Single Responsibility Principle
| Location | Problem |
|---|---|
| `handlers/tracker_behavior.go:32-58` | Each behavior both **formats** the value (`fmt.Sprintf("%.2f", ...)`) *and* returns a pre-computed notification. Logic duplicated across both structs. |
| `clients/client.go:13-61` | Data layer also does **reporting**: `ProcessNotificationCriteria(...)` builds user text here instead of the handler/presentation layer. Fetch + notify together. |
| `handlers/command_handler.go:36-44, 525+` | Tracks trackers *and* navigation state *and* dispatches commands *and* formats help/status (`processTrackerStatus`, status builder). Pure formatting lives inside it. |
| `handlers/navigation_state.go:4-37` | Small aggregate overall, but two usages (stack ops + callback/back-button flags) live in one struct stored in `CommandHandler`, guarded by **two** different mutexes — blurred ownership. |

### 2.2 Open/Closed Principle
- Adding a **new data source** forces copy-paste of the whole `Execute` body plus editing package-level `ProcessNotificationCriteria`. The class's own comment (`tracker_behavior.go:10-16`) advertises easy extension that isn't achievable without touching existing code.
- Registering a **new tracker type** edits three near-duplicate lookups instead of one registration point: `config.GetAPITrackerData` (`config/config.go:201`), `config.GetScraperTrackerData` (`config/config.go:211`), `handlers.DetermineTrackerType` (`handlers/tracker.go:300`).

### 2.3 Liskov Substitution Principle
- **The `clients.Client` interface lies about its contract** (`clients/client.go:19-21`): declares `FetchAndExtractData(trackerCode string)` while both implementers take `*config.Tracker`. It's also dead code — no caller uses the `Client` type directly (grep-verified) and nothing calls a client through it. Fixing or deleting is trivial and safe.
- **Behaviors depend on concrete clients, not an injected seam.** They build `NewPublicAPIClient()` (`tracker_behavior.go:23`) / `NewScraperClient()` (`tracker_behavior.go:42`), bypass the abstraction (no substitution/DI benefit) and can't be tested with an alternate source; the unexported field also makes testing copy-heavy.

### 2.4 Interface Segregation / Abstraction Clustering
- `Client.FetchAndExtractData` bundles **two** concerns in one method/return: extraction + notification evaluation, returned as a single `DataResult{CurrentValue, NotificationMessage}`. Splitting into "extract float64" + "evaluate criteria" makes each smaller and independently testable.
- The behaviors are fat: client creation + execution + value formatting in one method.

### 2.5 Dependency Inversion Principle
- `config.Configuration` is a **process-global singleton** (`var config *Configuration`, `GetConfig()`, `config/config.go:50-75`). Everything pulls the global; parallel tests and explicit DI are awkward (tests share state via the package var). Intentional for this app, but a DIP-level concern.
- Clients reach **package-level shared HTTP code** (`services.GetRequest`, `scraper_client.go:31`) with no injected seam; `PublicAPIClient` has no fields at all — neither can be unit-tested without hitting the network, pushing behavior-dependent tests into integration tests.

### 2.6 DRY / maintainability (secondary)
- Three identical "find-by-code" loops (`config.go:201`, `config.go:211`, `tracker.go:300`).
- Two identical `Execute` bodies; value formatting duplicated (`%.2f`) in both behaviors.
- Very verbose, string-concatenated logging throughout the clients (e.g. `"... for tracker: " + code, err.Error()`), hurting readability and tripping several linters (`perfsprint`, etc.). Tangential to SOLID but worth cleaning while touching those files.

## 3. Proposed refactor plan
Goal / principle(s) · **Order: structural correctness first (safe), then slimming, then testability.**

### 3.1 Fix or remove the dead `clients.Client` interface — *LSP*
- Change: delete `Client`, or redefine it correctly as `FetchAndExtractData(*config.Tracker) (*DataResult, error)` and wire real DI through it (§3.4/§3.6). Given current callers use concrete types directly, deletion is lowest-risk.
- Risk: **Low** (verified unused). Tests/lint: ensure `clients_test.go` still compiles; table-test if you keep/rewrite it.

### 3.2 Split extraction from criteria evaluation — *ISOC, DRY*
- Change: an `Extractor` seam returning just a float64 (`FetchCurrentValue`); move `ProcessNotificationCriteria` out of `clients/` into a small dedicated place (e.g. `handlers/notify.go`). Client returns the raw value; evaluation happens once in one testable spot.
- Risk: Low/Med. Tests/lint: relocate coverage from `clients_test.go`; table-test both extractor and evaluator per existing conventions.

### 3.3 Extract shared formatting / notification building — *SRP, DRY*
- Change: put small pure helpers in `helpers/` (e.g. `FormatRecordedValue(float64) string`) used by both behaviors; keep the single "Good news…" builder parameterized by code + value; route log strings through `fmt.Sprintf`.
- Risk: Low. Tests/lint: unit-test the helpers; fixes `perfsprint`/string-concat hits while you're there.

### 3.4 Replace three duplicate lookups with one registry — *OCP, DRY* (biggest structural win)
- Change: a small `TrackerRegistry` (slice/map keyed by type `"api"`/`"scraper"`) with `GetData(code)` / `GetType(code)`; both lookups and `DetermineTrackerType` delegate to it. Template-method seam so behaviors get client + type from one place — the real OCP payoff for §3.1/§3.2 extensions.
- Risk: Med — preserve precedence (API before scraper) and ordering semantics. Tests/lint: replace coverage of the three lookups; table-test the registry; run with `-race`.

### 3.5 Slim `CommandHandler` — *SRP*
- Change: keep tracker lifecycle where it is, but route help/status text building, menu construction and navigation handling toward dedicated helpers/small types (`helpers/`, a focused `Navigator`). Goal is smaller methods, not necessarily new packages.
- Risk: Med/Low (only if menus/navigation are mis-handled). Tests/lint: cover command → menu → click round-trips in tests.

### 3.6 Add an HTTP-fetch seam for testability — *DIP*
- Change: inject an HTTP fetcher into `PublicAPIClient`; give `ScraperClient` analogous injectable request/parse/error hooks so clients can be unit-tested without the network. Highest-value testability win but riskiest, so last.
- Risk: High/Med — touches real fetching paths; must preserve 403/429/503 → `HTTPStatusError.IsBlocking()` semantics exactly (the backoff logic depends on it). Tests/lint: full table tests against fakes; `-race`.

### 3.7 (optional, low priority) Clean up client logging
- Route the verbose string-concatenated logs through `fmt.Sprintf` helpers for readability and to keep linters happy without adding new `//nolint`.

## 4. Rollout phases
1. **Phase 0 — Baseline** (done): confirm `go build ./...`, `go vet ./...`, `go test ./...` pass; note the pinned golangci-lint version from `.github/workflows/golangci-lint.yml` for local linting.
2. **Phase 1 — Structural correctness (§3.1–§3.4):** fix/remove dead `Client`; extract evaluation (§3.2); shared formatting (§3.3); single registry (§3.4). Highest ROI, lowest risk; clears most violations at once.
3. **Phase 2 — Slim handlers (§3.5) + logging cleanup (§3.7).**
4. **Phase 3 — Testability seam (§3.6), as a follow-up** if network-dependent coverage is painful.

## 5. Conventions to honor while implementing
- **Concurrency:** `Tracker.mu` guards shared status; always read via locked accessors / existing methods (`Status()`, etc.). Preserve goroutine-safety and keep `-race` green (CI runs it, needs cgo locally). No blocking work in command handlers.
- **Reuse existing patterns:** fake messenger (`recordingMessenger`) and `fakeTelegram` server; table tests next to code.
- **Linting:** honor the strict set (`durationcheck`, `nlreturn`, `mnd`, `perfsprint`, `godot`, non-predeclared) — add tests, don't add unexplained `//nolint`; never multiply a `time.Duration` by another `time.Duration`.
- **Config singleton:** keep the global unless injecting top-level DI; if you do, update `README.md` / `.env.example` per project rules.

## 6. Verification commands (after each phase)
```bash
cd /var/home/georgs/Documents/Projekti/price-tracker-bot
go build ./... && go vet ./... && go test ./...
golangci-lint run --timeout=5m ./...
```

## 7. Risk register
| Item | Why it matters | Mitigation |
|---|---|---|
| Changing `FetchAndExtractData` signature | Could ripple to callers if used elsewhere | Verified unused; grep all call sites before touching. |
| Behavior/formatting changes altering output | Status/notification text is asserted in tests | Re-run and update expectations only where behavior genuinely improves; keep value formatting identical unless intentional. |
| Registry precedence/ordering | Lookup prefers API over scraper | Preserve exact order (API first) in the registry. |
| §3.6 seam touching backoff semantics | 403/429/503 → `HTTPStatusError.IsBlocking()` drives retry delay | Keep returning that error type with identical fields; table-test blocking paths against fakes. |

## 8. Boundaries (not doing this)
- No changes to deployment scripts, Dockerfile, or tracker config formats/field names unless explicitly asked.
- No production-key moves; keep a separate dev bot for any hands-on testing.
- No large DI re-architecture beyond what's needed — the config singleton is intentional and low-cost here, so §2.5/DIP is flagged but not forced in Phase 1.

