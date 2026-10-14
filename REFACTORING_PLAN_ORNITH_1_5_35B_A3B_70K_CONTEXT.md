# SOLID Analysis & Refactoring Plan — `price-tracker-bot`

## 0. Architecture snapshot (what I found)

A Telegram price-tracking bot: single Go binary, no database, JSON-persisted state (`data/state.json`), long-polling loop. The data flow is:

```
botfixer.BotFixer   →  CommandHandler (commandMap registry)
                         → Tracker (goroutine + timer) → TrackerBehavior (API/Scraper)
                                                            → Client (PublicAPIClient / ScraperClient)
                                                              → services.GetRequest / colly
                         → NavigationState (per-chat menu history, in-memory)
config.Configuration  ← global singleton (lazy GetConfig())
helpers.Messenger     ← Telegram wiring behind an interface
```

**What's already solid** — worth preserving so the refactor doesn't throw good abstractions away:

- `handlers/tracker_behavior.go` — a clean `TrackerBehavior` interface with two interchangeable impls.
- `clients/client.go` — a `Client` interface + shared `ProcessNotificationCriteria`.
- `handlers/command_handler.go` — commands are **already** registry-driven (`commandMap map[string]*Command`), not a giant switch. That part of OCP is done.
- `helpers.Messenger` — Telegram is behind an interface, so it's testable with a fake.
- `handlers/tracker_state.go` — atomic state writes (temp file + rename).

The violations are **concentrated and fixable** without touching any user-facing behavior.

---

## 1. SOLID assessment (with concrete evidence)

### S — Single Responsibility — 3 real violations (+ 1 worth noting)

| Component | Problem | Evidence |
|---|---|---|
| `config.Configuration` | Loads env vars, parses tracker files, validates, checks chat permissions, **and** serves data lookups. Four+ reasons to change in one type. | `loadConfig()` L78, `ValidateConfig()` L194, `IsChatAllowed()` L186, `GetTrackerData()` L221 — all on one struct with no methods separated from the data. |
| `handlers.CommandHandler` | Owns 8 command handlers **plus** navigation state (`navigation`, `navigationMu`) **plus** tracker lifecycle (Add/Remove/StopAllTrackers, saveState). It's the project hub. | Fields L36–44; `HandleCommand` dispatches + pushes nav stack L89; `handleStatus` builds HTML *and* menus *and* queries config. |
| `botfixer.BotFixer` | Routes messages, dispatches commands, handles buttons, and reaches into navigation state directly (`b.CommandHandler.GetUserNavigationState(chatID).BackButtonEnabled = …`). God object of message handling. | `update.go` `handleMessage`/`handleButton` L83–133. |
| `handlers.Tracker` *(under-emphasized elsewhere)* | Coordinates lifecycle (Start/Stop), scheduling/timer loop, backoff math, error accounting + history, status snapshot, **and** notification sending. Several reasons to change. | `tracker.go`: `run` L258, `scheduleNextRun` L230, `recordError` L179, `executeTrackerLogic` L140, `Start/Stop` L240/278. |

### O — Open/Closed — 2 violations

- **`CreateTracker` factory** (`tracker.go:80–87`) — hardcoded `switch { case API: NewAPITrackerBehavior(); case Scraper: … }`. Adding an RSS/webhook tracker means editing this function. **This is the single clearest OCP violation.**
- **Button routing** in `botfixer` (`update.go:123`) — a `if command == "back" {…} else {…}` branch; every new button payload edits one place. (Minor, since there are only two branches today.)

> Command dispatch itself is already registry-based, so skip that part of OCP.

### L — Liskov Substitution — clean (0 violations)

`TrackerBehavior` and `Client` interfaces are well-formed; both API/Scraper impls share identical signatures/semantics and substitute freely. One caveat feeds into DIP: behaviors construct their own clients internally, so they aren't truly substitutable at the dependency level.

### I — Interface Segregation — 2 minor violations

- **`helpers.Messenger`** bundles 6 methods (`SendHTML`, `SendHTMLWithMenu`, `SendHTMLWithKeyboard`, `EditHTMLWithMenu`, `EditMenu`, `RemoveKeyboard`). Callers that need one still depend on all. | `messages.go:17–24`.
- **`config.Configuration`** passed whole to every module even though they only need a slice of it (handlers need tracker lookup + chat allow-list; trackers need nothing).

### D — Dependency Inversion — 2 violations

- **Global singleton** `var config *Configuration` + lazy `GetConfig()` (`config.go:50`). Callers depend on a concrete global, not an abstraction; mutable, untestable, hard to swap. This is the highest-impact DIP issue.

- **Behaviors own their clients**: `NewAPITrackerBehavior()` calls `clients.NewPublicAPIClient()` internally (`tracker_behavior.go:26–29`), so a high-level behavior depends directly on a low-level constructor instead of an injected dependency.

---

## 2. Prioritized refactoring plan (impact ÷ risk)

The guiding rule from the repo's own docs and both existing plans: **no user-facing changes**, keep it ~1500 LOC, don't over-engineer. I've ordered phases by value/risk.

### Phase 1 — Inject config + behaviors (DIP + OCP). *Highest value, lowest risk.* Do this first.

1. **Kill the global singleton.** Replace `var config *Configuration` / `GetConfig()` with an explicit constructor `config.Load() (*Configuration, error)` called once in `main.go`. Thread the instance through struct fields (it's already threaded through most handlers). This alone makes testing dramatically easier and removes the biggest DIP violation.
2. **Turn `CreateTracker`'s switch into a registry** so new tracker types register without editing existing code:

   ```go
   var behaviorFactory = map[string]func() TrackerBehavior{
       API:     NewAPITrackerBehavior,
       Scraper: NewScraperTrackerBehavior,
   }
   ```

3. **Inject clients into behaviors.** Change `NewAPITrackerBehavior()` to accept a `clients.Client` (the interface) instead of constructing it internally; same for scraper. This makes LSP real at the dependency level and unblocks mocking in tests.

### Phase 2 — Split `config.Configuration` (SRP + ISP).

- Extract a plain data struct (`ConfigData`, no methods, no loading).
- Move `loadConfig()` → a dedicated `ConfigLoader`; keep validation as its own step; leave only lookups on the data type.
- Introduce small caller-facing interfaces — `TrackerProvider` (GetTrackerData) and `AccessChecker` (IsChatAllowed) — so handlers depend on what they need, not the whole struct.

### Phase 3 — Navigation state service (SRP).

- Extract `navigation map[int64]*NavigationState` + its mutex out of `CommandHandler` into a dedicated `NavigationService`. `CommandHandler` delegates to it; `botfixer` never touches navigation directly anymore.

### Phase 4 — Thin message router (OCP for buttons).

- Make `botfixer` translate Telegram updates → domain events only. Route button callbacks through a dispatch table (`map[payload]handler`) instead of the `if "back" {…}` branch, mirroring the already-good command registry.

### Phase 5 — Messenger segregation (ISP) + Tracker split (SRP). *Optional.*

- Split `helpers.Messenger` into focused interfaces (`HTMLMessenger`, etc.) or unify with an explicit format param — pick based on how many callers actually use all three.
- If the `Tracker` god-object pain becomes real, extract its run/error-accounting/backoff logic out of the lifecycle coordinator. **Defer** — for 1500 LOC this is likely over-engineering; only do it if a concrete problem appears.


---

## 3. Target architecture sketch

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
