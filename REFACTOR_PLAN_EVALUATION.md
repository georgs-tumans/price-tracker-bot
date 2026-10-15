# Refactoring plan evaluation

**Evaluator:** Claude Opus 5.5, 2026-10-15. **Plans compared:** the six `REFACTOR_PLAN_*_CONTEXT.md` files.
**Reference:** [`REFACTOR_PLAN_CLAUDE_OPUS_5_5.md`](REFACTOR_PLAN_CLAUDE_OPUS_5_5.md), written from the code before reading any other plan. Every claim marked as an error below was checked against the code (file and function given).

---

## 1. Ranking

| # | Plan | Key findings (of 18) | Factual errors | Fit with the codebase | Verdict |
|---|---|---|---|---|---|
| 1 | **Ornith 1.5 9B Q5 128K** | 11 | 4, minor; one leads to a pointless logging step | Good: stays in existing packages, builds on existing seams | **Best.** Found the issues that are hardest to spot (dead `Client`, clients writing HTML) with accurate locations. |
| 2 | Ornith 1.5 35B A3B 70K | 11 | 4, one of them underlies its top priority | Mixed: good risk notes, but splits code that is fine | Good diagnosis, wrong first step. |
| 3 | Qwen 3.6 35B A3B 70K | 10 | 6, several on central points | Poor: 6 new packages, interfaces with one implementation | Best coverage of `CommandHandler`, but the most errors and much over-engineering. |
| 4 | Ornith 1.5 9B Q6 96K | 5 | 5, including the file and role of `ProcessNotificationCriteria` | Good but timid | Accurate line numbers, little substance; dismisses a real problem. |
| 5 | Qwen 3.5 9B Q5 128K | 6 | 2, plus misused principles | Poor: 12-week plan, "DI framework", new GraphQL client | Generic SOLID text with almost no reference to the code. |
| 6 | Gemma 4 26B A4B 70K | 4 | 0 (says too little to be wrong) | Poor: proposes `pkg/` layout the repo doesn't use | Too shallow; half the table is "review" or "consider". |

"Key findings" = the coverage matrix in §2 (✔ = 2, ◐ = 1). The two Ornith plans in places 1 and 2 tie on coverage; accuracy and choice of first step separate them. Ornith 9B Q6 ranks above Qwen 3.5 9B despite one point less coverage: its findings come from the code and its proposals do no harm, while Qwen 3.5's are generic and far too big.

---

## 2. Coverage of the real problems

Reference findings, each verified in the code:

| | Finding | Where |
|---|---|---|
| K1 | `CommandHandler` does routing, tracker registry, lifecycle, persistence, navigation and view rendering | `handlers/command_handler.go`, `saveState`/`ResumeTrackers` as its methods |
| K2 | Clients evaluate criteria **and** write the Telegram HTML message | `clients.ProcessNotificationCriteria`, called by both clients |
| K3 | Tracker type hard-coded in 6 places, not just the `switch` | config fields + lookups, `DetermineTrackerType`, `CreateTracker`, loops in `startAllTrackers`/`handleStatus` |
| K4 | `clients.Client` has the wrong signature and nothing implements it; the two behaviors are copies | `clients/client.go`, `handlers/tracker_behavior.go` |
| K5 | `CreateTracker` builds concrete behaviors → no injection seam | `handlers/tracker.go` |
| K6 | Global `GetConfig()` + `log.Fatalf`; `main` is not the composition root | `config/config.go`, `NewBotFixer` |
| K7 | Navigation history reuses `Command`; request data (`CallbackMessageID`, `BackButtonEnabled`) passed through shared per-chat state, set by `botfixer` | `HandleCommand`, `botfixer/update.go` |
| K8 | ISP: `Tracker` uses 2 of 6 `Messenger` methods; `CreateTracker` takes the whole config for 3 values | `executeTrackerLogic`, `recordError` |
| K9 | Duplicated operator escaping; `helpers` mixes transport, menus, domain logic | `clients/client.go`, `helpers/string_formatting.go` |

| | Gemma 26B | Qwen 3.5 9B | Ornith 9B Q5 | Ornith 9B Q6 | Ornith 35B | Qwen 3.6 35B |
|---|---|---|---|---|---|---|
| K1 CommandHandler | ◐ registry only | ✔ | ◐ keeps lifecycle in place | ◐ registry, "optional" | ✔ | ✔ incl. persistence |
| K2 clients write HTML | ✘ | ◐ vague, as ISP | ✔ | ✘ **dismissed** | ◐ function only | ◐ right diagnosis, contradictory fix |
| K3 type hard-coded | ◐ switch | ◐ switch | ◐ 3 lookups + type | ◐ 3 lookups + type | ◐ switch | ◐ switch |
| K4 dead `Client` | ✘ | ✘ | ✔ grep-verified | ✔ | ◐ "overlapping" | ✘ **claims it is implemented** |
| K5 concrete behaviors | ◐ | ◐ | ✔ | ✘ claims clients are injected | ✔ | ✔ |
| K6 config / composition root | ◐ vague | ✘ | ◐ flagged, kept | ◐ flagged, deferred | ✔ | ✔ with false details |
| K7 navigation / request state | ✘ | ✘ | ◐ flags vs stack | ✘ | ◐ `isReturn` flag | ✘ |
| K8 ISP | ✘ | ◐ generic | ✘ | ✘ | ◐ with wrong detail | ◐ |
| K9 duplication / `helpers` | ✘ | ✘ | ◐ | ✘ | ✘ | ◐ |
| **Score** | **4** | **6** | **11** | **5** | **11** | **10** |

No plan found K7 (the `Command` struct doing double duty and the request data passed through shared state), and none counted all 6 places where tracker types are hard-coded (K3).

---

## 3. Per plan

### 1. Ornith 1.5 9B Q5 128K — best

**Strong:**
- The only plan that names both "hidden" defects with correct evidence: `clients.Client` doesn't match its implementers and is unused (§2.3), and clients write user text (§2.1).
- The line numbers are right (`config.go:201`, `:211`, `tracker.go:300`).
- Its proposals fit the code: "smaller methods, not necessarily new packages", keep API-before-scraper precedence, keep `HTTPStatusError` semantics for backoff.
- Its conventions section matches `AGENTS.md`.

**Errors:**
- "`NavigationState` … guarded by **two** different mutexes": only `navigationMu` guards the map; the fields themselves aren't locked.
- "tests share state via the package var": no test calls `GetConfig()`; they build `&config.Configuration{}` or call `loadConfig()`.
- Says the string-concatenated logs trip `perfsprint`. They merged through CI lint, and `perfsprint` flags the reverse pattern (`Sprintf` where concatenation would do), so its fix of "route through `fmt.Sprintf`" would be the change that gets flagged. *(Reasoned from the linter's rules; I didn't run v2.13.2 locally — the installed 2.11.3 is built with Go 1.26.)*
- "clients can't be unit-tested without hitting the network": `clients_test.go` uses a local `httptest` server.

**Weak:**
- Keeps tracker lifecycle and persistence in `CommandHandler`, which is the biggest SRP problem.
- Rates the HTTP seam "highest-value", but at least puts it last as optional.

### 2. Ornith 1.5 35B A3B 70K

**Strong:**
- Broadest SRP list for `CommandHandler`, persistence included.
- Finds the singleton and the missing composition root (D3/D4) and the `isReturn` boolean trap.
- Its risk section is accurate: `MIN_INTERVAL` enforced in two places, `savedTracker` format, one poller per key.

**Errors:**
- "`CommandHandler` has ~60 methods": it has 22.
- "A tracker only needs `SendHTMLWithMenu`": `recordError` also calls `SendHTML`.
- "Introduce a tiny registry of `*Command` entries": `commandMap` already is one.
- "tests must stand up a real local server … removes the reliance on live servers": they're `httptest` servers, the idiomatic Go approach.

**Wrong first step:** Phase 0 injects an `HTTPRequester.Get(url) ([]byte, error)` into both clients and calls it "highest value, lowest risk". `ScraperClient` fetches through colly's own collector, so that interface doesn't fit it without rewriting the scraper.

**Over-splitting:** it splits `Tracker` (backoff is already a pure, tested function) and splits the 109-line `http_service.go` into three.

### 3. Qwen 3.6 35B A3B 70K

**Strong:**
- Best coverage of `CommandHandler`: Phase 4 moves persistence out.
- Places `ProcessNotificationCriteria` correctly as business logic that doesn't belong in `clients`.
- Its "What stays the same" table is accurate: colly per run, `savedTracker`, mutex strategy, CRLF.

**Errors (several on central points):**
- §6 "`Client` stays; both API and scraper implement it": neither does (wrong parameter type). Following the plan would not compile.
- "All packages import `config` and call `GetConfig()` … no way to inject test configs": there is one caller (`botfixer/bot_fixer.go:38`), and tests inject configs. The next table row even says "Tests use `newBotFixer(cfg, ...)`".
- Phase 1 says to "add `*config.Configuration` parameter to `NewCommandHandler()`, `CreateTracker()`": both already take it.
- "`commandMap` … No registry/command-dispatcher pattern": it is a registry. Its own "after" state describes the same thing.
- Contradicts itself on criteria evaluation: §3 moves it "from `clients/` … to its own package", while Phase 0 and §5 move it to `clients/notification_engine.go`. The new `DataFetcher.Fetch(trackerCode string) (value, notification, err)` keeps the notification inside the fetcher and repeats the dead interface's `string` parameter.
- "Phase 0–1: All existing tests should pass without modification", yet those phases move `NotifyCriteria` to `domain/` and change constructors.

**Over-engineering:**
- New `app/`, `domain/` (`TrackerCode`, `Interval` value objects), `messaging/`, `handlers/command/`, `handlers/tracker/`, a `ConfigLoader` interface, a `TrackerRegistry` interface, and an event bus floated for `handleUpdate`.
- Splitting `handlers` into sub-packages would force unexported tracker fields (e.g. `trackerData`, read by `handleStatus`) to be exported. The plan doesn't mention it.

### 4. Ornith 1.5 9B Q6 96K

**Strong:**
- Correct and well evidenced on the dead `Client` interface and the duplicate lookups.
- Keeps the new registry inside `handlers` to avoid import cycles.
- Rightly leaves `Tracker` alone.

**Errors:**
- §7 lists "`helpers/messages.go` formatting helpers (`ProcessNotificationCriteria`, menu builders)" as cohesive presentation. That function is in `clients/client.go`, the menus are in `helpers/ui_helper.go`, and `messages.go` is the Telegram messenger. So it dismisses K2 using the wrong file.
- "the code already injects its seams (messenger, clients)": clients are created inside the behavior constructors.
- "`botfixer` imports `handlers`, `config`, `helpers` (`main.go:11-15`)": `main.go` imports only `botfixer`; those imports are in `botfixer/bot_fixer.go`.
- "per-field mutexes": each tracker has one mutex.
- Wants the shared `%.2f` formatter tested with `1 234,56 €` → `1234.56`. That cleanup happens in `ScraperClient`; the behavior only formats a float.

**Weak:**
- "The main SOLID gaps are not structural bloat" underrates a 575-line type with 6+ jobs.
- P3 adds a helper instead of merging the two identical behaviors.

### 5. Qwen 3.5 9B Q5 128K

**Strong:** a correct responsibility list for `CommandHandler`. The `ValueExtractor` / `NotificationBuilder` split points the right way, though it never says the clients are where it happens.

**Weak:**
- No model header and almost no file or function references.
- LSP is misused ("Tracker mixes config with runtime state" is not a substitution problem).
- The `HTTPClient` interface is justified by "preventing easy mocking" when `httptest` already covers it.
- Proposes a 12-week schedule, a "dependency injection framework" and a new GraphQL client (a feature, not a refactor). Its success metric of "< 5% increase in cold start time" is labelled "Build Time".
- Wants `Tracker` split into Coordinator, StateManager, BackoffPolicy and ErrorNotifier.

### 6. Gemma 4 26B A4B 70K

**Strong:** the two main ideas are correct: a `TrackerManager` and the switch in `CreateTracker`.

**Weak:**
- 35 lines. The LSP and ISP rows are "conduct a review" / "consider splitting", which aren't findings.
- The phases are numbered 1, 4, 3.
- Proposes `pkg/tracker/manager.go`, a layout the repo doesn't use. Putting the manager there while `Tracker` stays in `handlers` would create an import cycle.
- No tests, concurrency or `AGENTS.md` constraints.

---

## 4. Patterns across the models

- **The same two false "wins" come up repeatedly:** an `HTTPClient` interface in 4 of 6 plans, and splitting `Tracker` in 3. Both ignore that `httptest` already tests the clients and that `backoffDelay` is already pure and tested.
- **The bigger models covered more but made more confident factual errors**, such as Qwen 3.6 35B on `Client` and `GetConfig`, and Ornith 35B with "~60 methods". The best-grounded plan came from a 9B model.
- **Quantization didn't decide it.** The two Ornith 9B runs (Q5 / 128K vs Q6 / 96K) disagree on the most important finding (K2). Run-to-run variance is probably at least as large as the Q5/Q6 difference, so one run per configuration isn't enough to rank the configurations.

---

## 5. Note on `REFACTORING_PLAN_CLAUDE_OPUS_5_5.md`

The untracked file from 2026-10-14 is a previous Claude Opus 5.5 plan. It isn't ranked, because it was written while the earlier plan files were in the repo (its §4 table is titled "Idea (seen in other plans)"). Its findings F1–F5 match the blind reference plan written today almost one to one, so the reference list in §2 holds up across two runs.
