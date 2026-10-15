# SOLID Refactoring Plan

**Target model:** `ORNITH_1_5_9B_Q6_96K_CONTEXT`
**File written to:** project root, as `REFACTOR_PLAN_<MODEL_ID>.md`.
**Status of this document:** plan only. No code has been changed yet — see §8 for the rollout approach.

---

## 1. Executive summary

The price-tracker-bot is already well-engineered and largely follows SOLID: there is a `Messenger` interface that decouples handlers from Telegram, a `Client`/`TrackerBehavior` abstraction for data sources (open for extension), per-field mutexes with safe copy-on-read status accessors, and a small, cohesive helper/utility layer. The code compiles cleanly, passes `go vet`, and all existing tests pass.

The main SOLID gaps are **not structural bloat** but three smaller issues: one genuine LSP/dead-abstraction problem, some duplicated "find the tracker" logic (DRY), and a little duplicated value-formatting inside the two behaviors (SRP/DRY). These are low-risk to fix and each strengthens a different principle.

The plan is deliberately incremental. Nothing here requires ripping out working code; every change preserves existing behavior and can be validated with the project's own lint + test suite (§5).

---

## 2. How this was analyzed

- Read all production `.go` files under `main.go`, `botfixer/`, `handlers/`, `clients/`, `config/`, `helpers/`, `services/`, `utilities/`.
- Verified the baseline is green: `go build ./...`, `go vet ./...`, and `go test ./...` all pass (Go 1.27; golangci-lint v2.13.2 per CI).
- Confirmed real call sites with greps before concluding whether an interface/type is used or dead.

---

## 3. Issues by SOLID principle, prioritized

Issues are ordered by severity and effort. Each has the principle violated, the evidence (file:line), impact, and a concrete fix.

### P1 — `Client` interface is mis-declared / never fulfilled (Liskov + dead abstraction)
- **Evidence:** `clients/client.go:19-21` declares
  ```go
  type Client interface {
      FetchAndExtractData(trackerCode string) (*DataResult, error)
  }
  ```
  But the two concrete clients implement a different signature — `PublicAPIClient.FetchAndExtractData(trackerData *config.Tracker)` (`clients/api_client.go:21`) and `ScraperClient.FetchAndExtractData(...)` (`clients/scraper_client.go:25`). A search for usages of the interface as a type finds **none** (the behaviors hold concrete client pointers, not the interface), so the abstraction is dead documentation that does *not* match reality.
- **Impact:** LSP conceptually — an "interface" claims a contract no implementer satisfies; it also misleads future readers into thinking clients can be injected by this type. It's pure noise in the data layer.

- **Fix (small, low risk):** Delete the `Client` interface entirely and replace its one-line comment with a pointer to where the abstraction really lives — i.e. the `TrackerBehavior.Execute` seam (`handlers/tracker_behavior.go:17`) which is what callers already depend on via an interface. This removes a false abstraction and keeps the real one truthful.

### P2 — Duplicated "find a tracker by code" logic (DRY → SRP)
- **Evidence:** `config/config.go` has three near-duplicate lookups — `GetAPITrackerData` (`:201`), `GetScraperTrackerData` (`:211`) and the delegating `GetTrackerData` (`:221`, which re-searches both lists) — while `handlers/tracker.go:DetermineTrackerType(:300-314)` implements a **second** two-pass "which list contains this code" routine. Then `CreateTracker` (`:69-72, :80-87`) calls *both* `DetermineTrackerType` and `config.GetTrackerData`, doing the membership check twice over different slices.
- **Impact:** The same "does tracker X exist in the API list / scraper list" logic lives in two packages. A new tracker source (e.g. a third category) would have to be updated in both places — OCP/DRY risk.
- **Fix (small, low risk):** Replace `GetAPITrackerData`/`GetScraperTrackerData` and `DetermineTrackerType` with **one** method that resolves a code once, returning the tracker *and* whether it is an API or scraper tracker (e.g. `(tracker *config.Tracker, kind string)`). Keep the single source of truth in `config`, delete the duplicate iteration from `handlers/tracker.go`. Update `CreateTracker` to use it and drop `DetermineTrackerType`.

### P3 — Duplicate value-formatting logic inside both behaviors (SRP / DRY)
- **Evidence:** `APITrackerBehavior.Execute` (`handlers/tracker_behavior.go:32-39`) and `ScraperTrackerBehavior.Execute` (`:51-58`) contain byte-for-byte identical post-processing after the fetch:
  ```go
  if err != nil { return "", "", err }
  return fmt.Sprintf("%.2f", result.CurrentValue), result.NotificationMessage, nil
  ```
- **Impact:** The formatting responsibility is duplicated. If "how to format a raw extracted value" ever changes (e.g. currency-aware rounding for scrapers vs APIs), both bodies must change together — SRP/DRY smell.
- **Fix (small):** Extract the tail into a shared helper used by both `Execute` methods, e.g. in the behaviors file or a tiny `helpers/format_util.go`:
  ```go
  func formatDataResult(result *clients.DataResult) (string /*formatted value*/, string /*notification message*/, error) { ... }
  ```
  Both behaviors then call it. Behavior types still keep their single data source each, so OCP for adding new sources is preserved; only the common post-fetch step is shared.

### P4 — `CommandHandler` bundles dispatch + tracker registry together (SRP / OCP)
- **Evidence:** `CommandHandler` (`handlers/command_handler.go:36-44`) holds both command routing (`commandMap`, `HandleCommand`, `:25-109`) and the running-tracker registry (`runningTrackers`, plus `AddRunningTracker :493`, `GetActiveTracker :480`, `RemoveRunningTracker :500`, `startAllTrackers :170`).
- **Impact:** The type has two independent reasons to change — command additions and tracker-lifecycle changes. Extracting the registry improves cohesion, but this is a larger refactor with constraints (see §6). Recommended as Phase 3 / optional; P1–P3 give most of the SOLID value for far less risk.

> Note on `Tracker` itself (`handlers/tracker.go:53-116`): it mixes scheduling (`scheduleNextRun`, `backoffDelay :218`, `run :258`) with execution orchestration (`executeTrackerLogic :140`). Cohesion here is acceptable — the pieces share a single reason to change (tracker runtime behavior) and are already split into helper functions. I do **not** recommend splitting it unless profiling shows otherwise; doing so would be churn for little gain.

---

## 4. Proposed changes (incremental, each independently reviewable)

| Phase | Issue | Files changed | Risk | Effort |
|---|---|---|---|---|
| P1 | Dead/mis-declared `Client` interface | `clients/client.go` (delete + comment pointer to the real seam) | Very low | 5 min |
| P3 | Duplicated formatting in behaviors | `handlers/tracker_behavior.go` (+ new shared helper, e.g. `helpers/format_util.go`) | Very low | 15 min |
| P2 | Duplicate tracker lookup | `config/config.go`, `handlers/tracker.go`, `CreateTracker` | Low | 30 min |
| P4 (optional) | Split registry from dispatch | New type inside `handlers/`; wire into existing handlers | Medium — see §6 | 2-3 days |

P1→P3 are quick wins that touch no hot path. P2 touches config resolution used at startup and on resume, so it is validated first against the full test suite. P4 is optional polish; skip if not desired.


---

## 5. Validation strategy (must stay green after every change)

Run this exact sequence before considering a change complete:

```bash
cd /var/home/georgs/Documents/Projekti/price-tracker-bot
go build ./... && go vet ./...
golangci-lint run --timeout=5m ./...        # v2.13.2, as in .github/workflows/golangci-lint.yml
go test -race ./...                          # race is enforced in CI (needs cgo locally)
```

Because the codebase uses white-box tests (`package handlers`, `package botfixer` — see existing `*_test.go`) and table-driven tests, new/changed logic should be covered by matching that convention: add a focused table test next to each changed function. In particular:
- **P3** needs a unit test for the shared formatter covering API-style floats and scraped currency strings (e.g. `1 234,56 €` → `1234.56`).
- **P2** needs tests that a code resolves once from each category and that an unknown code returns no tracker — reuse/extend `config/config_test.go`.

---

## 6. Constraints & risks for any larger refactor (esp. P4)

- **No import cycles.** `botfixer` imports `handlers`, `config`, `helpers` (`main.go:11-15`). If the tracker registry were moved to a *new* package, that package must not be imported back into `handlers`/`botfixer` in a way that closes a loop. The safest place is still inside `handlers` (same package), which avoids all cycle risk — P4's suggested split should therefore keep the new type within `handlers`.
- **CRLF vs LF.** The working copy uses CRLF; git stores LF (`AGENTS.md`). Edit tools preserve CRLF automatically, but shell scripts that write files must stay LF. Keep this in mind when adding/rewriting files.
- **`scratch` Docker image** has no shell/tools — debug via logs only after refactoring. Tracker configs are mounted at runtime, not baked into the image, so P2's config changes don't require image rebuilds for existing trackers (only `.example`/README docs).
- **Don't change deployment scripts** (`deployment/*.sh`, `docker-compose.yml`) without being asked; this plan touches none of them.

---

## 7. Out of scope / intentionally not refactored

To avoid unrequested churn on an already-solid design:
- The global config singleton (`config.go:50` `var config *Configuration`). It improves testability/DIP in theory, but the code already injects its seams (messenger, clients) and tests pass; changing it is higher risk than benefit. Flagged only as a future consideration.
- `helpers/messages.go` formatting helpers (`ProcessNotificationCriteria`, menu builders). They are cohesive presentation logic with no duplication worth splitting.
- The overall command-dispatch architecture in `CommandHandler`. It works and is tested; P4 only *optionally* improves cohesion, so it's the least-priority item.

---

## 8. Rollout approach (recommended)

1. **P1** — delete the false `Client` interface. Re-run validation (§5).
2. **P3** — extract shared value formatting; add formatter tests. Validate. *(quick win, no hot path)*
3. **P2** — unify tracker resolution in config, remove duplicate lookup from handlers; extend config tests to cover the new single resolver and unknown codes. Validate (this touches startup/resume paths).
4. **(Optional) P4** — extract a `TrackerRegistry` inside `handlers/`; migrate registry methods; keep dispatch thin. Validate.
5. Leave changes uncommitted for review per project workflow rules (`develop` is main; releases/builds are CI-driven, deployment is manual).

Starting with **P1 → P3 → P2** gives the best SOLID-to-risk ratio: fix the genuine LSP/dead-abstraction issue and remove duplications first (cheap), then tackle the optional cohesion refactor last.
