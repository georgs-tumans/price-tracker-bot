# SOLID Analysis & Refactoring Plan for `price-tracker-bot`

## Current Architecture Summary

The project is a Telegram price-tracking bot with ~1500 lines across 14 Go files. It has a **single binary, no database**, uses long-polling, and stores state in JSON. The architecture already has some good abstractions (`TrackerBehavior`, `Messenger`) but several areas violate SOLID principles.

---

## S — Single Responsibility Principle (SRP)

| Component | Current Problem | What It Should Do Instead |
|---|---|---|
| **`config.Configuration`** | Loads env vars, parses tracker JSON files, validates config, checks chat permissions, AND provides lookup methods (`GetTrackerData`, `IsChatAllowed`) | Only hold configuration data. Defer loading, parsing, and validation to dedicated services. |
| **`handlers.CommandHandler`** | Handles `/run`, `/stop`, `/status`, `/interval`, `/mute`, `/list`, navigation state (menu history), button click routing — all in one massive `switch`. | Split into separate command handlers or a command router + individual handler structs. Navigation state belongs to its own service. |
| **`botfixer.BotFixer`** | Routes messages, dispatches commands, handles button clicks, builds menus, manages navigation state — it is the "god object" of message handling. | Separate message routing from business logic. The bot should only translate Telegram updates into domain events and pass them to a handler. |

---

## O — Open/Closed Principle (OCP)

| Component | Current Problem | Fix |
|---|---|---|
| **`CreateTracker`** (`handlers/tracker.go:69`) | Hardcoded `switch { case API, case Scraper }` to create behaviors. Adding a new tracker type (RSS, webhook, etc.) requires modifying this function. | Use a **registry/factory pattern**: register behavior constructors in a map keyed by tracker type. New types add themselves without touching existing code. |
| **`CommandHandler.handleUpdate`** (`handlers/command_handler.go`) | Massive `switch` on command names and button payloads. Every new command requires modifying the handler. | Register commands in a map of `string → HandlerFunc`. The switch becomes a lookup. New commands are added by registration, not modification. |
| **`config.Configuration.GetTrackerData`** | Linear scan through two slices with hardcoded logic for API vs scraper. | Store trackers in a `map[string]*Tracker` internally (or use a repository). Lookup is O(1) and adding new tracker categories does not change the lookup method. |

---

## L — Liskov Substitution Principle (LSP)

**No major violations found.** The `TrackerBehavior` interface is well-designed:
- Both `APITrackerBehavior` and `ScraperTrackerBehavior` implement `Execute(trackerData)` with the same signature and semantics.
- They can be substituted freely without breaking callers.

One minor note: `TrackerBehavior` implementations **create their own clients** internally (`NewPublicAPIClient()`, `NewScraperClient()`), which couples them to concrete types rather than interfaces (see DIP below).

---

## I — Interface Segregation Principle (ISP)

| Component | Current Problem | Fix |
|---|---|---|
| **`helpers.Messenger`** interface | Bundles `SendHTML`, `SendMessage`, `SendMarkdown` together. Some callers only need one method. | Split into focused interfaces: `HTMLMessenger`, `TextMessenger`, or use a single `Send(chatID, message)` with format hints. |
| **`config.Configuration`** exposed as a monolith | Every module that needs config gets the entire struct (chat IDs, tracker lists, state file path, etc.) | Provide focused interfaces: e.g., `TrackerProvider` (lookup trackers), `AccessControl` (check chat permissions). |

---

## D — Dependency Inversion Principle (DIP)

| Component | Current Problem | Fix |
|---|---|---|
| **Global config singleton** (`config.config`) | Every module calls `config.GetConfig()` directly. Tight coupling to a global accessor; hard to test. | Inject configuration through constructors or an application-level DI container / options struct. |
| **`APITrackerBehavior` creates its own client** (line 27) | Depends on concrete `clients.PublicAPIClient`, not an interface. Can not swap implementations or mock in tests. | Define a `DataFetcher` interface; inject it into behaviors via constructor. |
| **`CreateTracker` instantiates behaviors directly** (lines 81-84) | High-level code (`CreateTracker`) depends on low-level constructors (`NewAPITrackerBehavior`). | Use a factory/registry that returns `TrackerBehavior` instances. The factory itself is injected. |
| **`CommandHandler` creates trackers directly** | Depends on `config.GetConfig()` and `CreateTracker`. | Inject a `TrackerFactory` interface instead. |

---

## Proposed Refactoring Plan (Phased)

### Phase 1: Dependency Injection & Factories (DIP + OCP)
- [ ] Define interfaces for clients (`DataFetcher`, `HTMLScraper`) in the `clients` package
- [ ] Create a `TrackerBehaviorFactory` registry — map of type name → constructor function
- [ ] Inject config into `CreateTracker` via an options struct instead of global accessor
- [ ] Make `APITrackerBehavior` and `ScraperTrackerBehavior` accept their clients as constructor parameters

### Phase 2: Configuration Service (SRP + DIP)
- [ ] Split `config.Configuration` into:
  - `ConfigData` — plain data struct (no methods, no loading logic)
  - `ConfigLoader` — reads env vars, parses tracker files, validates
  - `TrackerRepository` — provides O(1) lookup by code, supports multiple types
- [ ] Replace global singleton with an injected config instance

### Phase 3: Command Router (OCP + SRP)
- [ ] Extract command handling into a map-based router: `map[string]func(CommandContext) error`
- [ ] Each command (`/run`, `/stop`, etc.) gets its own handler function or struct
- [ ] Button click routing similarly becomes a dispatch table

### Phase 4: Navigation State Service (SRP)
- [ ] Extract navigation state from `CommandHandler` into a dedicated `NavigationService`
- [ ] `BotFixer` delegates to this service instead of managing it inline

### Phase 5: Interface Segregation (ISP)
- [ ] Split `Messenger` into focused interfaces or use a single unified interface with format parameter
- [ ] Create small interfaces for config access (`TrackerProvider`, `AccessChecker`)

---

## Target Architecture Sketch

```
main.go
├── app.App                    # wires everything together, holds DI container
│   ├── Config                 # injected config data (plain struct)
│   ├── TrackerRepository      # O(1) lookup by code
│   ├── Messenger              # focused interface
│   └── NavigationService      # per-chat menu history
│
├── botfixer.BotFixer          # thin: translates TG updates → events
│   └── CommandRouter           # dispatch table, no switch statements
│       ├── RunHandler
│       ├── StopHandler
│       ├── StatusHandler
│       └── ...
│
├── handlers.TrackerFactory     # registry of behavior constructors
│   └── Tracker                 # orchestrator (scheduling, backoff, status)
│       └── TrackerBehavior     # interface (API, Scraper, future types)
│           ├── APIClient      # implements DataFetcher
│           └── ScraperClient  # implements HTMLScraper
│
├── config.ConfigLoader         # reads env + tracker files → ConfigData
└── services.HTTPService        # shared HTTP (already well-separated)
```

---

## What This Achieves

| Principle | Before | After |
|---|---|---|
| **SRP** | `Configuration` loads, validates, and serves data; `CommandHandler` handles 8+ commands + navigation | Each type has one reason to change |
| **OCP** | Adding a tracker type or command requires modifying existing code | New types/commands register themselves |
| **LSP** | Already good (no violations) | Maintained — interfaces stay clean |
| **ISP** | `Messenger` and `Configuration` are fat interfaces | Small, focused interfaces per caller |
| **DIP** | Global singleton, concrete constructors everywhere | Interfaces injected from top level |

---

## Risks & Trade-offs

1. **More files/interfaces**: The project is small (~1500 LOC). Over-engineering could hurt readability. Start with Phase 1 (DI + factory) since it gives the most value for least complexity.
2. **No breaking changes to Telegram commands**: All user-facing behavior stays identical; this is purely internal restructuring.
3. **Tests improve naturally**: With injected interfaces, existing tests can mock dependencies instead of using workarounds like fake messengers.

---

## Recommended Starting Point

**Phase 1 (DI + Factory)** is the lowest-risk and highest-impact change:
- It decouples high-level logic from low-level implementations
- It makes testing easier without refactoring everything at once
- It sets the foundation for all subsequent phases
