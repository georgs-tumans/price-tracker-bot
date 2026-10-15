# SOLID Refactoring Plan — Price Tracker Bot

> **Date:** 2026-10-15  
> **Current codebase:** Telegram price-tracking bot in Go, single binary, no database. ~25 `.go` files across 8 packages.  
> **Goal:** Restructure the project to better adhere to SOLID principles while preserving all existing behavior, API contracts, and Docker deployment.

---

## Table of Contents

1. [Current Architecture Summary](#1-current-architecture-summary)
2. [SOLID Violation Audit](#2-solid-violation-audit)
3. [Proposed Target Architecture](#3-proposed-target-architecture)
4. [Refactoring Phases](#4-refactoring-phases)
5. [Package Restructuring Map](#5-package-restructuring-map)
6. [Interface Definitions](#6-interface-definitions)
7. [Migration Strategy & Risk Mitigation](#7-migration-strategy--risk-mitigation)
8. [What Stays the Same](#8-what-stays-the-same)

---

## 1. Current Architecture Summary

### Existing package layout

```
main.go                              ← entry point, calls NewBotFixer().Run(ctx)
botfixer/
  bot_fixer.go                       ← Telegram long-polling setup, wiring
  update.go                          ← update dispatch (messages + callbacks)
handlers/
  command_handler.go                 ← all /commands, tracker lifecycle, navigation state (~570 lines)
  tracker.go                         ← Tracker struct, goroutine loop, backoff logic
  tracker_behavior.go                ← APITrackerBehavior, ScraperTrackerBehavior
  tracker_state.go                   ← JSON persistence (save/load/resume)
  navigation_state.go                ← per-chat menu history stack
clients/
  client.go                          ← Client interface + ProcessNotificationCriteria()
  api_client.go                      ← PublicAPIClient (gjson extraction)
  scraper_client.go                  ← ScraperClient (colly scraping)
config/
  config.go                          ← singleton GetConfig(), env parsing, tracker loading
services/
  http_service.go                    ← shared HTTP client, Retry-After parsing
helpers/
  messages.go                        ← Messenger interface + TelegramMessenger impl
  ui_helper.go                       ← keyboard/button builders
  logical_operations.go              ← CompareNumbers()
  string_formatting.go               ← FormatNotificationCriteriaString()
utilities/
  interval_util.go                   ← ParseDurationWithDays(), DurationToString()
  string_util.go                     ← GetStringPointerValue()
```

### How data flows today

1. `main()` → `NewBotFixer().Run(ctx)`
2. `BotFixer` creates a `CommandHandler`, loads config via singleton, starts tracker resume loop
3. Telegram updates arrive in `handleUpdate()` → routed to `CommandHandler.HandleCommand()` or `HandleUserInput()`
4. Commands create/stop/update `Tracker` instances (goroutines) stored in `CommandHandler.runningTrackers`
5. Each tracker calls its `TrackerBehavior.Execute()`, which delegates to a client (`PublicAPIClient` / `ScraperClient`)
---

## 2. SOLID Violation Audit

### 2.1 Single Responsibility Principle (SRP) — **HIGH PRIORITY**

| Component | Violation | Details |
|-----------|-----------|---------|
| `CommandHandler` | **Massive god object** (~570 lines). Handles: command dispatch, tracker lifecycle management, navigation state, status formatting, help messages, interval setting, mute/unmute logic, state persistence calls. | Should be split into at least 4–5 focused handlers or a handler + coordinator pattern. |
| `Tracker` struct | Does too much: goroutine scheduling, backoff math, error counting, messenger notifications, interval management, context cancellation. | Scheduling/backoff should be separate from execution/notification. |
| `config.Configuration` | Loads env vars, parses tracker JSON files, validates config, manages chat allow-lists — all in one struct with a singleton pattern. | Config loading/parsing is a different responsibility than the config data model itself. |
| `ProcessNotificationCriteria()` (in `clients/`) | Lives in the clients package but has nothing to do with fetching data; it's business logic for criteria evaluation. | Should be its own service or part of a notification engine. |
| `helpers.FormatNotificationCriteriaString()` | Formatting logic mixed into helpers alongside UI builders and number comparison. | Formatting belongs near the domain model (NotifyCriteria). |

### 2.2 Open/Closed Principle (OCP) — **MEDIUM PRIORITY**

| Area | Violation | Details |
|------|-----------|---------|
| `DetermineTrackerType()` in `tracker.go` | Switch/if-else on tracker type; adding a new data source requires modifying this function. | The TrackerBehavior interface exists but the factory logic is scattered and not extensible without code changes. |
| `CommandHandler.commandMap` | Hardcoded map of command strings to handlers. Adding commands means editing the struct literal in `NewCommandHandler()`. | No registry/command-dispatcher pattern; every new command requires modifying CommandHandler. |
| `handleUpdate()` in `update.go` | Switch on update type (message vs callback). Extensible but not cleanly — adding a new update kind requires touching this function. | Acceptable for Telegram-specific dispatch, but could be cleaner with an event bus. |

### 2.3 Liskov Substitution Principle (LSP) — **LOW PRIORITY**

| Area | Observation |
|------|-------------|
| `APITrackerBehavior` / `ScraperTrackerBehavior` | Both implement `TrackerBehavior.Execute()` correctly with the same signature and semantics. No violations detected here. |
| `Messenger` interface | `TelegramMessenger` implements all methods faithfully. No violations. |

### 2.4 Interface Segregation Principle (ISP) — **MEDIUM PRIORITY**

| Area | Violation | Details |
|------|-----------|---------|
| `Messenger` interface | Has 6 methods (`SendHTML`, `SendHTMLWithMenu`, `SendHTMLWithKeyboard`, `EditHTMLWithMenu`, `EditMenu`, `RemoveKeyboard`). Some callers only need send; others need edit. A tracker goroutine that only sends notifications doesn't need the full interface. | Split into `MessageSender` and `MessageEditor` interfaces, or use functional options / method signatures per caller. |
| `Client` interface | Only one method (`FetchAndExtractData`). Not a real problem, but the interface lives in `client.go` alongside `ProcessNotificationCriteria()` which doesn't implement it — confusing cohesion. |

### 2.5 Dependency Inversion Principle (DIP) — **HIGH PRIORITY**

| Area | Violation | Details |
|------|-----------|---------|
| `config.GetConfig()` is a **singleton** | All packages import `config` and call `GetConfig()`, creating hard compile-time dependencies on the singleton. No interface, no way to inject test configs. | Should be replaced with an explicit dependency injection pattern: pass `*Configuration` (or a `ConfigReader` interface) through constructors. |
| `NewBotFixer()` calls `config.GetConfig()` internally | The bot's top-level constructor hides its dependencies. Tests use `newBotFixer(cfg, ...)` but production code bypasses this. | All constructors should accept their dependencies explicitly. |
| `APITrackerBehavior` creates `clients.NewPublicAPIClient()` internally | Hard-coded dependency on the concrete client type. The behavior layer knows about clients directly instead of depending on an abstraction. | Should accept a `Client` via constructor or factory. |
| `ScraperTrackerBehavior` same issue | Creates `NewScraperClient()` directly. | Same fix needed. |

---

## 3. Proposed Target Architecture

```
main.go                              ← minimal: wire everything, call App.Run(ctx)

app/                                 ← NEW: top-level orchestrator
  app.go                             ← App struct holds all dependencies, Run() method

config/                              ← RESTRUCTURED
  config.go                          ← Configuration struct + methods (data model only)
  loader.go                          ← NEW: env parsing, file loading, validation logic
  errors.go                          ← NEW: typed config errors

botfixer/                            ← SIMPLIFIED
  bot_fixer.go                       ← BotFixer accepts explicit dependencies via constructor
  update.go                          ← unchanged (Telegram-specific dispatch)

handlers/                            ← SPLIT INTO SUB-PACKAGES
  command/                           ← NEW sub-package
    dispatcher.go                    ← CommandDispatcher: registry, routing, navigation state
    start_handler.go                 ← handleStart / handleStop / startAllTrackers
    status_handler.go                ← handleStatus
    interval_handler.go              ← handleSetInterval
    mute_handler.go                  ← handleMute / handleUnmute
    help_handler.go                  ← handleHelp
  tracker/                           ← NEW sub-package (extracted from handlers/)
    tracker.go                       ← Tracker struct + lifecycle (Start, Stop)
    runner.go                        ← NEW: goroutine loop, scheduling, backoff logic
    error_handling.go                ← NEW: error counting, notification on limit
    state.go                         ← saveState / resumeTrackers (moved here)

clients/                             ← RESTRUCTURED
  client.go                          ← Client interface + DataResult only
  api_client.go                      ← PublicAPIClient (depends on HTTPService interface)
  scraper_client.go                  ← ScraperClient (depends on HTTPService interface)
  notification_engine.go             ← NEW: ProcessNotificationCriteria moved here

services/                            ← RESTRUCTURED
  http_service.go                    ← HTTPService interface + implementation
  errors.go                          ← NEW: HTTPStatusError, typed error helpers

messaging/                           ← NEW package (extracted from helpers/)
  messenger.go                       ← MessageSender, MessageEditor interfaces + TelegramMessenger
  keyboards.go                       ← keyboard/button builders (from ui_helper.go)

domain/                              ← NEW package
  criteria.go                        ← NotifyCriteria with Format() method
  tracker_code.go                    ← TrackerCode type with validation
  interval.go                        ← Interval value object with Parse(), Min() methods

utilities/                           ← KEPT, slimmed down
  duration.go                        ← DurationToString (kept)
  string_helpers.go                  ← GetStringPointerValue (kept if still needed)
```

### Key architectural changes

1. **Explicit dependency injection** — every struct receives its dependencies via constructor parameters instead of calling singletons or creating concrete types internally.

2. **Command handler split** — the monolithic `CommandHandler` becomes a `CommandDispatcher` that delegates to focused handler structs, each responsible for one command family.

3. **Tracker separation** — the Tracker's goroutine loop and backoff logic move into a separate `Runner` component; the Tracker struct becomes a lightweight coordinator.

4. **Notification engine extraction** — criteria evaluation moves from `clients/` (where it doesn't belong) to its own package, making it testable independently of HTTP fetching.

5. **Config singleton removal** — config loading is separated into a `Loader` type; the resulting `Configuration` struct is passed explicitly through constructors.

---

## 4. Refactoring Phases

### Phase 0: Foundation — Interfaces & Domain Types (No behavior change)

**Goal:** Introduce abstractions and value objects without changing any runtime behavior.

| Task | Details | Files affected |
|------|---------|----------------|
| Create `domain/criteria.go` | Move `NotifyCriteria` struct here; add `Format() string` method | config → domain |
| Create `domain/tracker_code.go` | New `TrackerCode` type wrapping `string` with validation | new file |
| Create `domain/interval.go` | Value object for intervals: `Parse(s string) (Interval, error)`, `IsBelow(Interval) bool` | utilities → domain |
| Create `messaging/messenger.go` | Split `Messenger` into `MessageSender` and `MessageEditor` interfaces; move `TelegramMessenger` here | helpers/messages.go → messaging/ |
| Create `services/errors.go` | Move `HTTPStatusError`, `NewHTTPStatusError`, `IsBlocking()` here | services/http_service.go |
| Create `clients/notification_engine.go` | Move `ProcessNotificationCriteria()` here, depend on `domain.NotifyCriteria` | clients/client.go → clients/notification_engine.go |

**Verification:** All existing tests pass. No behavioral changes.

---

### Phase 1: Dependency Injection — Remove Singleton (No behavior change)

**Goal:** Replace the `config.GetConfig()` singleton with explicit constructor injection.

| Task | Details | Files affected |
|------|---------|----------------|
| Create `config/loader.go` | Extract `loadConfig()`, `parseErrorNotifyLimit()`, `parseMinInterval()`, `loadTrackers()`, `parseChatIDs()` into a `ConfigLoader` type with `Load() (*Configuration, error)` method | config/config.go → config/loader.go |
| Update all constructors | Add `*config.Configuration` parameter to: `NewCommandHandler()`, `NewBotFixer()`, `CreateTracker()`, etc. | handlers/, botfixer/, handlers/tracker.go |
| Update `main.go` | Call `loader.Load()` explicitly, pass result to `App.Run(cfg)` | main.go |
| Remove global `var config *Configuration` and `GetConfig()` | Delete from config package | config/config.go |

**Verification:** All existing tests pass. CI still passes with `-race`.

---

### Phase 2: Split CommandHandler (Behavior-preserving refactor)

**Goal:** Break the ~570-line `CommandHandler` into focused handlers + a dispatcher. This is the largest structural change.

| Task | Details | Files affected |
|------|---------|----------------|
| Create `handlers/command/dispatcher.go` | New `Dispatcher` struct holding: config, messenger, tracker registry, navigation state, command registry. Replaces `CommandHandler.commandMap`. | handlers/command_handler.go → handlers/command/dispatcher.go |
| Extract `StartStopHandler` | `handleStart`, `handleStop`, `startAllTrackers`, `stopAllTrackers`, lifecycle helpers (`GetActiveTracker`, `AddRunningTracker`, `RemoveRunningTracker`) | handlers/command/start_stop_handler.go |
| Extract `StatusHandler` | `handleStatus`, `processTrackerStatus`, `handleCommandMessage` (message sending helper) | handlers/command/status_handler.go |
| Extract `IntervalHandler` | `handleSetInterval`, interval validation logic | handlers/command/interval_handler.go |
| Extract `MuteHandler` | `handleMute`, `handleUnmute`, `setNotificationsPaused` | handlers/command/mute_handler.go |
| Extract `HelpHandler` | `handleHelp`, `formatCommandWithParams` | handlers/command/help_handler.go |
| Keep navigation in dispatcher | `HandleCommand`, `HandleReturn`, `HandleUserInput`, `GetUserNavigationState` remain in the dispatcher as they orchestrate across command families. | handlers/command/dispatcher.go |

**Key design decisions:**
- Each handler receives its dependencies (messenger, config, tracker registry) via constructor — no shared state between handlers except what's passed in.
- The `TrackerRegistry` interface abstracts `runningTrackers` operations:
  ```go
  type TrackerRegistry interface {
      GetActive(code string) *handlers.Tracker
      Add(tracker *handlers.Tracker)
      Remove(code string)
      StopAll()
  }
  ```
- The dispatcher holds the registry, not individual handlers.

**Verification:** All existing tests pass. Command behavior identical (same messages, same buttons).

---

### Phase 3: Split Tracker into Coordinator + Runner (Behavior-preserving refactor)

**Goal:** Separate the Tracker's goroutine scheduling/backoff logic from its lifecycle management.

| Task | Details | Files affected |
|------|---------|----------------|
| Create `handlers/tracker/runner.go` | New `Runner` struct: owns the `run()` loop, `scheduleNextRun()`, `backoffDelay()`, timer management. Accepts a `DataFetcher` interface for execution. | handlers/tracker.go → handlers/tracker/runner.go |
| Create `handlers/tracker/error_handler.go` | New `ErrorHandler`: tracks consecutive errors, counts them, sends notification when limit is reached. | handlers/tracker.go (error parts) → handlers/tracker/error_handler.go |
| Slim down `Tracker` struct | Keeps: Code, Behavior, chatID, messenger, status fields, mutex. Methods: Start(), Stop(), UpdateInterval(), Status(). Delegates execution to Runner. | handlers/tracker.go |

**Key design decisions:**
- `Runner` is created when `Tracker.Start()` is called and runs until `Tracker.Stop()` cancels its context. This matches current behavior (one goroutine per tracker).
- The `TrackerBehavior` interface stays but becomes a thin adapter: it implements the `DataFetcher` interface by delegating to the underlying client.

```go
// Runner depends on this abstraction, not on TrackerBehavior directly
type DataFetcher interface {
    Fetch(trackerCode string) (value string, notification string, err error)
}
```

**Verification:** All existing tests pass. Race detector clean. Backoff behavior identical.

---

### Phase 4: Extract State Management (Behavior-preserving refactor)

**Goal:** Move state persistence out of `CommandHandler` into its own component.

| Task | Details | Files affected |
|------|---------|----------------|
| Create `handlers/tracker/state.go` | New `StateManager` type with `Save(trackers []*Tracker) error`, `Load() ([]savedTracker, error)`, `Resume(cfg *config.Configuration, registry TrackerRegistry, messenger messaging.Messenger) error`. | handlers/tracker_state.go → handlers/tracker/state.go |
| Remove state methods from CommandHandler | Delete `saveState()` and `ResumeTrackers()` from dispatcher/command_handler. Dispatcher calls `StateManager` instead. | handlers/command/dispatcher.go |

**Verification:** All existing tests pass. State file format unchanged (backward compatible).

---

### Phase 5: Clean up helpers & utilities (Low-risk cleanup)

| Task | Details | Files affected |
|------|---------|----------------|
| Move keyboard builders to `messaging/keyboards.go` | `InlineButton()`, `GetReturnButtonMenu()`, `GetIntervalCustomMenu()`, `GetNotificationMenu()`, `GetStatusInlineKeyboard()` | helpers/ui_helper.go → messaging/keyboards.go |
| Keep `CompareNumbers` in helpers or move to domain | Since it's only used by the notification engine, consider moving to `domain/criteria.go` as a method on `NotifyCriteria`. | helpers/logical_operations.go → domain/ |
| Slim down `helpers/string_formatting.go` | Remove `FormatNotificationCriteriaString()` (moved to domain). Keep any remaining utility functions. | helpers/string_formatting.go |

---

### Phase 6: Final cleanup & documentation

| Task | Details |
|------|---------|
| Update README.md | Document new package structure, explain DI pattern, update command reference if needed |
| Add `.env.example` | If not already present, document all env vars with defaults |
| Review linter config | Ensure golangci-lint rules are appropriate for the new structure |
| Run full test suite + race detector | `go test ./... -race` |
| Run golangci-lint | `golangci-lint run --timeout=5m ./...` |

---

## 5. Package Restructuring Map

### Files that move (content mostly unchanged)

| From | To | Reason |
|------|-----|--------|
| `config.NotifyCriteria` struct | `domain/criteria.go` | Domain model belongs in its own package |
| `ProcessNotificationCriteria()` | `clients/notification_engine.go` | Business logic, not client logic |
| `HTTPStatusError`, `NewHTTPStatusError`, `IsBlocking()`, `parseRetryAfter()` | `services/errors.go` | Separate errors from HTTP service |
| `Messenger` interface + `TelegramMessenger` | `messaging/messenger.go` | Messaging is a cross-cutting concern |
| Keyboard builders (`InlineButton`, etc.) | `messaging/keyboards.go` | Group with messaging layer |
| `FormatNotificationCriteriaString()` | `domain/criteria.go` as method | Formatting belongs with the data it formats |

### Files that split (content redistributed)

| From | To | Reason |
|------|-----|--------|
| `handlers/command_handler.go` (~570 lines) | `handlers/command/dispatcher.go`, `start_stop_handler.go`, `status_handler.go`, `interval_handler.go`, `mute_handler.go`, `help_handler.go` | SRP: each file handles one command family |
| `handlers/tracker.go` (~314 lines) | `handlers/tracker/tracker.go`, `runner.go`, `error_handler.go` | SRP: lifecycle vs scheduling vs error handling |
| `config/config.go` (singleton + data model) | `config/config.go` (data only), `config/loader.go` (loading logic) | SRP: data model vs loading/parsing |

### Files that stay (minor changes)

| File | Change |
|------|--------|
| `botfixer/bot_fixer.go` | Constructor accepts explicit dependencies; no more `config.GetConfig()` call |
| `botfixer/update.go` | No changes needed |
| `clients/api_client.go` | Depends on `services.HTTPService` interface instead of `services.GetRequest()` function |
| `clients/scraper_client.go` | Same — depends on HTTPService interface |
| `utilities/duration.go` (was interval_util.go) | Minor: may gain a `Min()` method if domain.Interval uses it |

---

## 6. Interface Definitions

### New interfaces to introduce

```go
// config/loader.go
type ConfigLoader interface {
    Load() (*config.Configuration, error)
}

// handlers/tracker/runner.go — what the Runner needs from clients
type DataFetcher interface {
    Fetch(trackerCode string) (value string, notification string, err error)
}

// handlers/command/start_stop_handler.go — tracker registry abstraction
type TrackerRegistry interface {
    GetActive(code string) *Tracker
    Add(tracker *Tracker)
    Remove(code string)
    StopAll()
}

// messaging/messenger.go — split Messenger
type MessageSender interface {
    SendHTML(chatID int64, text string)
    SendHTMLWithMenu(chatID int64, text string, menu *models.InlineKeyboardMarkup)
    SendHTMLWithKeyboard(chatID int64, text string, keyboard *models.ReplyKeyboardMarkup)
}

type MessageEditor interface {
    EditHTMLWithMenu(chatID int64, messageID int, text string, menu *models.InlineKeyboardMarkup)
    EditMenu(chatID int64, messageID int, menu *models.InlineKeyboardMarkup)
    RemoveKeyboard(chatID int64)
}

// services/http_service.go — HTTP abstraction for clients
type HTTPService interface {
    Get(url string) ([]byte, error)
}
```

### Existing interfaces that stay (already well-designed)

| Interface | Location | Notes |
|-----------|----------|-------|
| `Messenger` (current combined) | helpers/messages.go | Will be split into MessageSender + MessageEditor |
| `Client` | clients/client.go | Stays; both API and scraper implement it |
| `TrackerBehavior` | handlers/tracker_behavior.go | Stays; already a clean abstraction for data fetching strategies |

---

## 7. Migration Strategy & Risk Mitigation

### Incremental approach — no big-bang rewrite

Each phase is independently testable. After every phase:
1. Run `go build ./...` — must compile
2. Run `go test ./... -race` — all tests pass, no data races
3. Run `golangci-lint run --timeout=5m ./...` — clean or only pre-existing issues

### Risk matrix

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Breaking existing tracker config format | Low | High | State file format never changes; `savedTracker` struct stays identical |
| Backoff behavior change | Medium | High | Extract backoff tests from existing tracker_test.go into dedicated table tests before refactoring |
| Navigation state regression | Medium | Medium | Keep navigation logic in the dispatcher (Phase 2); test with existing button-click scenarios |
| Telegram message format change | Low | Medium | Message formatting functions move but don't change content; compare output strings in tests |
| Concurrency race conditions | High during Phase 3 | Critical | Run `go test -race` after every commit; keep the Tracker.mu lock strategy identical |

### Testing strategy for each phase

- **Phase 0–1:** All existing tests should pass without modification. These phases only move code and add interfaces.
- **Phase 2 (CommandHandler split):** Write integration-style tests that send a command string and verify the output message + buttons. This ensures the dispatcher routes correctly after splitting.
- **Phase 3 (Tracker split):** The existing `tracker_test.go` already uses a fake messenger. Extend it to test the Runner independently from the Tracker coordinator.
- **Phase 4–6:** Existing tests cover state persistence; no new tests needed unless behavior changes.

### Rollback plan

Since each phase is a pure refactor (no behavior change), any phase can be rolled back by reverting that phase's commits. The git history will show clean, atomic changes per phase.

---

## 8. What Stays the Same

The following are **intentionally preserved** to avoid scope creep and maintain compatibility:

| Aspect | Why it stays |
|--------|-------------|
| Telegram long-polling approach | Works well; no need for webhook migration |
| Docker `scratch` image deployment | No new dependencies that would bloat the binary |
| JSON state file format | Backward compatible; mounted as volume |
| Tracker config JSON structure (field names) | Documented in README, used by users |
| Colly collector per-run pattern | Necessary due to callback accumulation |
| `sync.Mutex` concurrency strategy | Proven working; no need for channels-only approach |
| `golangci-lint` configuration | Existing rules are appropriate |
| CRLF line endings in Go files | Per project convention |

---

## Summary of SOLID Improvements

| Principle | Before | After |
|-----------|--------|-------|
| **SRP** | CommandHandler: 570 lines, 12+ responsibilities. Tracker: goroutine + backoff + notifications + lifecycle. Config: singleton with loading logic. | Dispatcher delegates to focused handlers (StartStop, Status, Interval, Mute, Help). Runner handles scheduling separately from Tracker coordinator. ConfigLoader separates loading from data model. |
| **OCP** | Adding a new tracker type requires modifying `DetermineTrackerType()`. Adding commands requires editing CommandHandler's commandMap literal. | Factory pattern for behaviors (extensible via registry). CommandDispatcher uses a registry that can be extended without modifying the dispatcher itself. |
| **LSP** | No violations found. | No changes needed — existing abstractions already comply. |
| **ISP** | Messenger interface has 6 methods; callers get more than they need. | Split into `MessageSender` (3 send methods) and `MessageEditor` (3 edit/remove methods). Callers depend only on what they use. |
| **DIP** | Singleton `config.GetConfig()` creates hard dependencies everywhere. Concrete clients created inside behaviors. | All constructors accept dependencies explicitly. `HTTPService` interface abstracts HTTP calls. Behaviors accept `DataFetcher` via constructor or factory. |

---

## Estimated Effort

| Phase | Estimated complexity | Risk level |
|-------|---------------------|------------|
| 0: Foundation interfaces & domain types | Low | Very low |
| 1: Dependency injection (remove singleton) | Medium | Low |
| 2: Split CommandHandler | High | Medium |
| 3: Split Tracker into Coordinator + Runner | High | Medium-High (concurrency) |
| 4: Extract state management | Low | Very low |
| 5: Clean up helpers & utilities | Low | Very low |
| 6: Final cleanup & documentation | Low | Very low |

**Total:** ~7 phases, all behavior-preserving refactors. No new features, no breaking changes to external contracts (Telegram commands, config format, Docker deployment).

