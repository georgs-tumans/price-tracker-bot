# SOLID Refactoring Plan for price-tracker-bot

**Model ID:** ORNITH_1_5_35B_A3B_70K_CONTEXT
**Scope:** Code-quality / architecture refactor to better match the five SOLID principles.
**Companion doc:** This plan is orthogonal to `MIGRATION_PLAN.md`, which covers the webhook→long-polling deployment migration. That infra work stays untouched; the items below are purely about internal structure and testability.

> This is a **plan**, not an implementation. No source files were changed — only this markdown file was added. When implementing, follow AGENTS.md: keep CRLF in edited Go files, run `golangci-lint` + `go test ./...`, and never touch deployment scripts unless asked.

---

## 1. How the code is structured today (quick map)

| Path | Current job |
|---|---|
| `main.go` | Entry point; calls `botfixer.NewBotFixer().Run(ctx)` with no composition root. |
| `botfixer/bot_fixer.go` | Long-polling wiring; constructs `TelegramMessenger` + `CommandHandler`. |
| `botfixer/update.go` | Routes an update to message vs. button handling via a `switch`. |
| `handlers/command_handler.go` (~570 lines) | Command dispatch, navigation stack, tracker lifecycle, state persistence, per-command logic, and message formatting/sending — all in one type. |
| `handlers/tracker.go` | Goroutine scheduler + status bookkeeping + error/backoff accounting + notification decisions. |
| `handlers/tracker_behavior.go` | `TrackerBehavior` interface + API/scraper implementations; factory returns concrete types. |
| `clients/client.go`, `api_client.go`, `scraper_client.go` | `Client` interface, two client impls, and `ProcessNotificationCriteria`. Clients call the global `services.GetRequest`. |
| `services/http_service.go` | Error type + request executor + Retry-After parsing in one file; package-global `httpClient`. |
| `config/config.go` | Global singleton that loads env + files, parses, validates and looks up. |
| `handlers/navigation_state.go`, `helpers/*`, `utilities/*` | Mostly well-factored; low priority for SOLID work. |

---

## 2. Findings by principle (with file references)

### S — Single Responsibility Principle

**S1 — `CommandHandler` is a God object.** (`handlers/command_handler.go`)
It carries at least six unrelated responsibilities in one type: command routing/dispatch, navigation history, tracker lifecycle management (`startAllTrackers`, `StopAllTrackers`), state persistence coordination (`saveState`), the individual command handlers themselves, and message building/selection (`handleCommandMessage`). It has ~60 methods. This is the single biggest SRP violation and the most impactful refactor.

**S2 — `Tracker` mixes scheduler, status store and backoff logic.** (`handlers/tracker.go`)
`run()` is a scheduling loop, `executeTrackerLogic()` runs behavior, `recordError()` updates error/backoff counters, and `scheduleNextRun()` computes the next delay. These are distinct concerns (lifecycle vs. accounting vs. timing policy).

**S3 — `Configuration`/`loadConfig` does everything.** (`config/config.go`)
Env loading, file loading, value parsing (`parseErrorNotifyLimit`, `parseMinInterval`, `parseChatIDs`, `loadTrackers`), validation, and lookups live in one type plus a package-global singleton.

**S4 — `services/http_service.go` bundles three responsibilities.** (file)
An error type (`HTTPStatusError` + `IsBlocking`), a request executor (`doRequest`/`GetRequest`), and header parsing (`parseRetryAfter`).

**S5 — `ProcessNotificationCriteria` couples comparison with message building.** (`clients/client.go`)
Parsing criteria values, comparing numbers, and formatting the notification string are three things in one function.

**S6 — `Command` mixes declarative metadata with executable behavior.** (`handlers/command_handler.go:25`)
The struct holds both config-like fields (`Type`, `DescriptionGeneral`, `Hidden`, `Params`) and a runnable `Handler CommandFunc`. (Minor; only refactor if command routing is rewritten.)

### O — Open/Closed Principle

**O1 — Behavior selection in `CreateTracker` is an un-extendable switch.** (`handlers/tracker.go:80`)
```go
switch trackerType {
case API: behavior = NewAPITrackerBehavior()
case Scraper: behavior = NewScraperTrackerBehavior()
}
```
Adding a client type means editing `CreateTracker`. A keyed registry/factory would make this open for extension, closed for modification.

**O2 — `handleUpdate` dispatch switch.** (`botfixer/update.go:32`)
Message vs. callback-query routing; new update kinds require touching this function.

**O3 — `CompareNumbers` operator switch.** (`helpers/logical_operations.go:5`)
Adding an operator means editing the function. (Validator already validates operators at config time via `oneof`, so the runtime comparison is isolated; make it strategy-mapped only if operators grow.)

**O4 — `isReturn bool` flag branches behavior.** (`handlers/command_handler.go`)
A boolean parameter that flips control flow ("boolean trap"). Prefer separate entry points or an enum.

### I — Interface Segregation Principle

**I1 — `Messenger` is one big interface for trackers and handlers alike.** (`helpers/messages.go`)
A tracker only needs `SendHTMLWithMenu`; navigation/button code needs edits/keyboard removal. Splitting into a small `Notifier` (sends) and an `Editor` (edits + keyboard) lets each dependent depend on only what it uses. All members are used somewhere today, so this is optional polish — do it if you extract sub-handlers anyway.

**I2 — Overlapping abstractions.** (`TrackerBehavior` vs `Client`)
Both exist to "fetch and extract data"; the split adds coupling rather than removing it (see DIP for the fix).

### L — Liskov Substitution Principle

**L1 — The two behaviors currently satisfy the contract consistently** (`handlers/tracker_behavior.go`). Both `APITrackerBehavior` and `ScraperTrackerBehavior` return `(value, notification string, err error)` where "empty notification = no alert." No substitution surprise today. **Keep this contract explicit** so future behaviors (e.g. a webhook client) don't silently break it — e.g. add a small contract test or document the invariants on the interface.

### D — Dependency Inversion Principle

**D1 — THE BIG ONE: clients depend on a global, concrete HTTP layer.** (`clients/*.go`, `services/http_service.go`)
`PublicAPIClient` and `ScraperClient` call `services.GetRequest` / the package-global `httpClient`. No transport interface exists, so the dependency can't be injected or faked — tests must stand up a real local server (see `botfixer/bot_fixer_test.go`). Introduce an `HTTPTransport`/`HTTPRequester` interface and inject it into both clients. This is the highest-value change: it's a clean DIP win *and* removes the reliance on live servers in tests.

**D2 — Concrete behavior construction.** (`handlers/tracker.go`)
`CreateTracker` builds concrete `NewAPITrackerBehavior()`/`NewScraperTrackerBehavior()`. The tracker already stores the `TrackerBehavior` interface (good), but creation consumes concretes. A small factory / registry (composition root) inverts this (see O1).

**D3 — Global config singleton.** (`config/config.go`)
`var config *Configuration` + `GetConfig()` is module-level mutable state, not injected. Hard to unit-test and impossible to run in parallel cleanly. Provide a constructor that returns a fresh `*Configuration` for injection (the tests already build `&config.Configuration{...}` directly — mirror that).

**D4 — No composition root.** (`botfixer/bot_fixer.go`)
Wiring is ad-hoc inside `newBotFixer`. Centralize assembly in one place so high-level modules are built from abstractions, not concretes.

---

## 3. Prioritized, phased rollout

Each phase preserves behavior and the existing test suite (all changes are internal refactorings). Verify after **every** phase with:

```bash
golangci-lint run --timeout=5m ./...   # CI's golangci-lint version (see .github/workflows/golangci-lint.yml)
go test ./... -race
```

### Phase 0 — DIP transport interface (highest value, lowest risk, no behavior change)
- Define `services.HTTPRequester`/`HTTPTransport` (`Get(url string) ([]byte, error)`), keep the global as one default implementation.
- Inject it into `PublicAPIClient` and `ScraperClient` via constructor parameters instead of calling the package-global.
- **Reward:** clients become unit-testable without a live server (fake transport that returns canned bytes / an `HTTPStatusError`). Reduces reliance on external servers across tests.
- Follows DIP directly: high-level (`clients`) no longer depends on low-level (`services`) concretely.

### Phase 1 — SRP: split the God objects (biggest structural win)
- **`CommandHandler`:** extract each command handler into its own small type + file, e.g. `startStop.go`, `interval.go`, `notifications.go`, `status.go`, `help.go`. Introduce a tiny registry of `*Command` entries pointing at per-handler methods (this also helps O1/O4 by removing the boolean flag). Keep `command_handler.go` for dispatch + navigation only.
- **`Tracker`:** pull error/backoff accounting into an `errorStore`/backoff helper (`backoffDelay`, `recordError`) and split status bookkeeping from the scheduler loop so each unit has one reason to change.
- Split `services/http_service.go` per S4 once Phase 0 is in (the executor moves out anyway).

### Phase 2 — OCP + DIP factory for behaviors (extend without editing)
- Replace the switch in `CreateTracker` with a registry keyed by tracker type (`map[trackerType]func() TrackerBehavior`) or a dedicated factory. New client types = new registry entry, no edits to `CreateTracker`.
- Make the comparison operators strategy-mapped only if more than the current five appear (defer for now).

### Phase 3 — DIP polish: config injection + composition root (+ optional ISP)
- Replace the global singleton with an injectable constructor returning a fresh `*Configuration`; wire it through rather than reading `config.GetConfig()` inside the bot.
- Centralize assembly in `botfixer`/`main.go` so every high-level object is built from its interface (D4).
- If sub-handlers were extracted, split `Messenger` into `Notifier` + `Editor` for real (I1); otherwise keep the existing one `Messenger` to limit churn.

### Recommended minimum viable scope
**Phase 0 alone** already delivers most of the value (testability + a clean DIP win) with minimal risk. Phase 1 is the structural heart. Phases 2–3 are nice-to-have once the codebase feels comfortable with the change cadence.

---

## 4. Risk & safety notes

- **Concurrency model must survive.** Tracker fields are shared between the tracker goroutine and command handlers; lock `t.mu` when touching `t.status`, read via `Status()`. Splitting `Tracker` into helpers means those helpers must either be immutable value types or clearly own their locking — don't move a mutex onto a type that's copied.
- **Keep one poller per key.** Telegram allows only one long-poller; never introduce concurrent polling when refactoring wiring (D4).
- **Minimum interval in two places.** `MIN_INTERVAL` is enforced in both `handleSetInterval` and `CreateTracker`; keep them in sync if that logic moves.
- **State file format is stable.** `savedTracker` JSON (`code`, `chatId`, `interval`, `notificationsPaused`) must not change — it's persisted across restarts; only its *serialization* layer may move with SRP.
- **Conventions when implementing (AGENTS.md):** keep CRLF in edited `.go` files, match the `[Component]` log prefix style, add table tests next to code (`*_test.go`), run `golangci-lint` + `go test ./... -race` before finishing, and don't modify deployment scripts unless asked.

---

## 5. Verification checklist (after implementing each phase)

- [ ] `golangci-lint run --timeout=5m ./...` passes with CI's linter version.
- [ ] `go test ./... -race` passes (tracker/command concurrency is covered by existing tests).
- [ ] No behavior change: `/run`, `/stop`, `/status`, `/interval`, `/mute`, resume-on-restart still work identically; state file format unchanged.
- [ ] New injected fakes cover the client transport path without a live server.
- [ ] CRLF preserved in any edited `.go` files; shell scripts stay LF.
