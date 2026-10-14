# SOLID Analysis & Refactoring Plan — `price-tracker-bot`

## 1. What I found (architecture snapshot)

A Telegram price-tracking bot, ~1500 LOC across 14 Go files, single binary, no DB, JSON-persisted state. Long-polling loop in `botfixer.BotFixer`. The code already has **some solid abstractions**:

- `handlers/tracker_behavior.go` — a clean `TrackerBehavior` interface with two interchangeable implementations (API/Scraper).
- `clients/client.go` — a `Client` interface + shared `ProcessNotificationCriteria`.
- `handlers/command_handler.go:39` — commands are **already** registered in a `commandMap map[string]*Command` registry rather than a giant switch.
- `handlers/tracker_state.go` — atomic (temp-file + rename) state writes, backoff logic in `tracker.go`.

So the foundation is decent; the violations are concentrated and fixable without rewriting anything user-facing.

## 2. SOLID assessment (with concrete evidence)

### SRP — Single Responsibility — **3 real violations**

| Component | Problem | Evidence |
|---|---|---|
| `config.Configuration` | Loads env vars, parses tracker files, validates, checks chat permissions, **and** serves data lookups. Four reasons to change in one type. | `loadConfig()` (L78), `ValidateConfig()` (L194), `IsChatAllowed()` (L186), `GetTrackerData()` (L221) all on one struct. |
| `handlers.CommandHandler` | Owns 8 command handlers **plus** navigation state (`navigation map`, `navigationMu`) plus tracker lifecycle. It's the project's hub. | Fields at L36-44; `HandleCommand` dispatches + pushes nav stack (L89). |
| `botfixer.BotFixer` | Routes messages, dispatches commands, handles buttons, builds menus — "god object" of message handling. | (per your repo) |

### OCP — Open/Closed — **2 violations**

- **`CreateTracker` factory** (`handlers/tracker.go:80`) — hardcoded `switch { case API: NewAPITrackerBehavior(); case Scraper: ... }`. Adding an RSS/webhook tracker means editing this function. This is the single clearest OCP violation.
- **Button/payload routing** in `botfixer` — a big switch on callback payloads; every new button edits one place.

> Note: command dispatch is already registry-based, so that part of OCP is done — good.

### LSP — Liskov Substitution — **clean (0 violations)**

`TrackerBehavior` and `Client` interfaces are well-formed; both API/Scraper impls share identical signatures/semantics and substitute freely. One caveat feeds into DIP below: the behaviors construct their own clients internally, so they're not truly substitutable at the dependency level.

### ISP — Interface Segregation — **2 minor violations**

- `helpers.Messenger` bundles `SendHTML`/`SendMessage`/`SendMarkdown`. Callers that only need one still depend on all three.
- `config.Configuration` is passed whole to every module even though they only need a slice of it (e.g., handlers only need tracker lookup + chat allow-list).

### DIP — Dependency Inversion — **2 violations**

- **Global singleton**: `var config *Configuration` + lazy `GetConfig()` in `config.go:50`. Callers depend on a concrete global, not an abstraction; untestable and hard to swap.
- **Behaviors own their clients**: `NewAPITrackerBehavior()` calls `clients.NewPublicAPIClient()` internally (L26-29), so high-level behavior depends directly on low-level client constructors instead of injecting them.

</new_text>
<parameter=path>
/home/georgs/Documents/Projekti/price-tracker-bot/REFACTORING_PLAN_ORNITH_1_5_35B_A3B.md

## 3. Prioritized refactoring plan (impact ÷ risk)

The existing plans agree the sweet spot is **Phase 1 first**. I concur, and I'd add a "do-nothing-yet" gate: keep all Telegram behavior byte-identical; this is pure internal restructuring.

### Phase 1 — Dependency Inversion + Factory (highest impact, lowest risk) ⭐ start here

1. **Kill the singleton.** Replace `var config *Configuration` / `GetConfig()` with an explicit constructor `config.Load() (*Configuration, error)` called once in `main`. Inject the instance everywhere via struct fields (already threaded through most handlers).
2. **Registry/factory for behaviors.** Turn `CreateTracker`'s switch into a registry:

   ```go
   var behaviorFactory = map[string]func() TrackerBehavior{
       API:     NewAPITrackerBehavior,
       Scraper: NewScraperTrackerBehavior,
   }
   ```

   Adding a new tracker type = register one line; no edits to `CreateTracker`.
3. **Inject clients into behaviors.** Change `NewAPITrackerBehavior()` → accept a `clients.Client` (or the `Client` interface) instead of constructing it internally. Same for scraper.

### Phase 2 — Split `config.Configuration` (SRP + ISP)

- Extract a plain data struct (`ConfigData`, no methods, no loading).
- Move `loadConfig()` → a `ConfigLoader`; move validation into its own step; keep only lookups on the data type.
- Introduce small caller-facing interfaces: `TrackerProvider` (GetTrackerData), `AccessChecker` (IsChatAllowed) so handlers depend on what they need, not the whole struct.

### Phase 3 — Navigation state service (SRP)

- Extract `navigation map[int64]*NavigationState` + its mutex out of `CommandHandler` into a dedicated `NavigationService`. `CommandHandler` delegates to it; `botfixer` never touches navigation directly.

### Phase 4 — Thin message router (OCP for buttons)

- Make `botfixer` translate Telegram updates → domain events only. Route button callbacks through a dispatch table (`map[payload]handler`) instead of a switch, mirroring the already-good command registry.

### Phase 5 — Messenger segregation (ISP)

- Split `helpers.Messenger` into focused interfaces (`HTMLMessenger`, etc.) or unify with an explicit format param — pick based on how many callers actually use all three methods.

## 4. Target sketch

```
main.go                              # calls config.Load(), wires the DI container
├── config.ConfigLoader              # env + tracker files → plain ConfigData (SRP)
│   └── Configuration                # data only; TrackerProvider / AccessChecker interfaces
├── botfixer.BotFixer                # THIN: TG update → event
│   └── CommandRouter                # dispatch tables, no switches (OCP)
│       ├── RunHandler ...
├── handlers.TrackerFactory          # registry of behavior constructors (OCP)
│   └── TrackerBehavior iface        # API/Scraper/future — clients injected (DIP)
└── services.NavigationService       # per-chat menu history (SRP)
```

## 5. Key trade-offs to keep in mind

- **Don't over-engineer a ~1500 LOC project.** Phases 2–5 add files/interfaces; only do them if the pain is real. Phase 1 alone delivers most of the SOLID benefit and makes testing dramatically easier (mock injected clients instead of fake messengers).
- **No user-facing changes** at any phase — commands, menus, and messages stay identical.
- **LSP stays clean throughout** — the interfaces are already good; don't touch them beyond making dependencies injectable.

## 6. Recommendation

Start with **Phase 1 (DI + factory)** — it's the lowest-risk, highest-value change and unblocks everything else. The two existing plans in this repo (`REFACTORING_PLAN_GEMMA_26B.md` and `REFACTORING_PLAN_QWEN3_6_35B.md`) cover the same ground; this version differs by emphasizing that command dispatch is *already* registry-based (so skip that part of OCP) and by pinning every finding to a specific line.

</new_text>
<parameter=path>
/home/georgs/Documents/Projekti/price-tracker-bot/REFACTORING_PLAN_ORNITH_1_5_35B_A3B.md