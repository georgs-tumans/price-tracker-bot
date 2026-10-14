# SOLID Analysis & Refactoring Plan — `price-tracker-bot`

**Model:** Ornit 1.5 9B (100K context) · **Date:** 2026-10-14

## 1. SOLID assessment (with concrete evidence)

### S — Single Responsibility Principle — 4 real violations (+ 1 worth noting)

| Component | Problem | Evidence |
|---|---|---|
| `config.Configuration` | Loads env vars, parses tracker files, validates, checks chat permissions, **and** serves data lookups. Four+ reasons to change in one type. | `loadConfig()` L78, `ValidateConfig()` L194, `IsChatAllowed()` L186, `GetTrackerData()` L221 — all on one struct with no methods separated from the data. |
| `handlers.CommandHandler` | Owns 8 command handlers **plus** navigation state (`navigation`, `navigationMu`) **plus** tracker lifecycle (Add/Remove/StopAllTrackers, saveState). It's the project hub. | Fields L36–44; `HandleCommand` dispatches + pushes nav stack L89; `handleStatus` builds HTML *and* menus *and* queries config. |
| `botfixer.BotFixer` | Routes messages, dispatches commands, handles buttons, and reaches into navigation state directly (`b.CommandHandler.GetUserNavigationState(chatID).BackButtonEnabled = …`). God object of message handling. | `update.go` `handleMessage`/`handleButton` L83–133. |
| `handlers.Tracker` *(under-emphasized elsewhere)* | Coordinates lifecycle (Start/Stop), scheduling/timer loop, backoff math, error accounting + history, status snapshot, **and** notification sending. Several reasons to change. | `tracker.go`: `run` L258, `scheduleNextRun` L230, `recordError` L179, `executeTrackerLogic` L140, `Start/Stop` L240/278. |

### O — Open/Closed Principle — 2 violations

- **`CreateTracker` factory** (`tracker.go:80–87`) — hardcoded `switch { case API; case Scraper }`. Adding an RSS/webhook tracker means editing this function. **This is the single clearest OCP violation.**
- **Button routing** in `botfixer` (`update.go:123`) — a `if command == "back" {…} else {…}` branch; every new button payload edits one place. (Minor, since there are only two branches today.)

> Command dispatch itself is already registry-based, so skip that part of OCP. Also note the tracker lookup path (`DetermineTrackerType` L300 and `GetTrackerData` L221) does a linear scan over two slices with hardcoded API-vs-scraper logic — adding a new tracker category changes the lookup method too.

### L — Liskov Substitution Principle — clean (0 violations)

`TrackerBehavior` and `Client` interfaces are well-formed; both API/Scraper impls share identical signatures/semantics and substitute freely. No action needed. The one caveat is DIP-related (below).

### I — Interface Segregation Principle — 2 issues

- **`helpers.Messenger`** bundles `SendHTML`, `SendHTMLWithMenu`, `SendHTMLWithKeyboard`. Callers that only need HTML pay for keyboard methods. Split into focused interfaces (`HTMLMessenger`) or unify with an explicit format parameter.
- **`config.Configuration` exposed as a monolith** — every module gets the whole struct (chat IDs, tracker lists, state path). Provide small caller-facing interfaces instead: `TrackerProvider` (lookup) and `AccessChecker` (chat permissions).

### D — Dependency Inversion Principle — 3 issues

- **Global singleton config** (`config.go:50`, lazy `GetConfig()` var) — the biggest DIP win. High-level modules depend on a concrete global instead of an abstraction.
- **Concrete constructors everywhere** — `NewPublicAPIClient()`, `NewScraperClient()` create their own dependencies; high-level code depends on concretes (`*Tracker`) rather than abstractions.
- **Under-emphasized (the one I'd add):** the behaviors create their clients internally and the `Client` interface is *not used by them*. `APITrackerBehavior.Execute` calls a concrete `*PublicAPIClient`, so you cannot inject a fake/stub client to test the Tracker's error-handling path without real network calls. The abstraction exists but isn't wired in — that defeats its purpose for testing and DIP.

---

## 2. Target architecture sketch

```
main.go                              # calls config.Load(), wires DI once
├── config.ConfigLoader              # env + tracker files → plain ConfigData (SRP)
│   └── Configuration                # data only; TrackerProvider / AccessChecker interfaces
├── botfixer.BotFixer                # THIN: TG update → event
│   └── CommandRouter                # dispatch tables, no switches (OCP)
│       ├── RunHandler …
├── handlers.TrackerFactory          # registry of behavior constructors (OCP)
│   └── TrackerBehavior iface        # API/Scraper/future — clients injected (DIP)
│       └── Tracker                  # lifecycle/scheduling coordinator only
└── services.NavigationService       # per-chat menu history (SRP)
    services.HTTPService             # shared HTTP (already well-separated)
```

---

## 3. Phased roadmap

**Phase 1 — DI + config split (highest value, lowest risk).** Replace the global singleton with an injected `*Configuration`; extract a plain data struct (`ConfigData`, no methods, no loading) from loading/validation/lookups → `ConfigLoader` + small interfaces (`TrackerProvider`, `AccessChecker`). Unblocks everything else and makes the `CreateTracker` switch extensible.

**Phase 2 — Navigation service (SRP).** Extract `navigation map[int64]*NavigationState` + its mutex into a dedicated `NavigationService`. `CommandHandler` delegates to it; `botfixer` never touches navigation directly anymore.

**Phase 3 — Command router + button dispatch table (OCP/SRP).** Split `CommandHandler`'s tracker-lifecycle and state-save concerns into a `TrackerManager`; route button callbacks through a dispatch table (`map[payload]handler`) mirroring the already-good command registry instead of the `if "back"` branch.

**Phase 4 — Behavior factory registry (OCP + DIP).** Replace the `CreateTracker` switch with a registry of behavior constructors; **wire the existing `Client` interface into the behaviors via constructor injection** so a test client can be supplied (fixes the DIP gap). This also lets you unify the API/scraper fetch paths.

**Phase 5 — ISP refinement + Tracker split (optional).** Split `Messenger` into focused interfaces only if callers actually use all three; extract the Tracker's run/error-accounting/backoff logic out of its lifecycle coordinator **only if a concrete pain appears**. For ~1500 LOC this is likely over-engineering; defer unless justified.

---

## 4. Trade-offs & risks

1. **Don't over-engineer ~1500 LOC.** Phases 2–5 add files/interfaces; only do them if the pain is real. Phase 1 alone delivers most of the SOLID benefit.
2. **No user-facing changes** at any phase — commands, menus, and messages stay byte-identical. This is purely internal restructuring.
3. **LSP stays clean throughout** — don't touch the interfaces beyond making dependencies injectable.
4. **Keep CRLF / lint discipline** per `AGENTS.md` before reporting anything done (`golangci-lint run --timeout=5m ./...` + `go test ./...`).

---

## 5. My recommendation

Start with **Phase 1 (DI + factory injection)** — lowest risk, highest value, and it unblocks everything else. It removes the global singleton (biggest DIP win), makes the `CreateTracker` switch extensible (OCP), and lets tests mock injected clients instead of relying on fake messengers.

**Two things I'd add beyond the two existing plans in this repo**, since they under-emphasize them:

- The **`handlers.Tracker` god-object** — it mixes lifecycle, scheduling, backoff, error accounting, and notifications; worth a Phase 5 split if pain shows.
- An **inconsistent fetch path**: the API client goes through `services.GetRequest`, but the scraper bypasses it with its own colly collector and HTML headers (`scraper_client.go:31–62`). Not strictly SOLID, but a cohesion smell worth fixing when you touch HTTP anyway.
---
