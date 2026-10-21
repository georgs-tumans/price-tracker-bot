# Benchmark tasks

Your prompt names one task: T1, T2 or T3. Do only that task. The three tasks are independent and each one starts from this commit.

## Rules

These rules override `AGENTS.md` where the two differ.

1. **No visible behavior change** unless the task says so: same commands, bot messages, buttons, env vars, tracker JSON format and state file format.
2. **Required API:** every name and signature listed under "Required API" must exist exactly as written, because other code depends on it. Existing exported types, functions and methods keep their names and signatures unless the task removes them. Organize everything else however you think is best.
3. **Existing tests:** don't change them, except where the task lists an allowed change. Add tests for the code you write, next to it (table tests, see `AGENTS.md`).
4. **Before finishing,** run these and fix what they report:
   ```bash
   go build ./...
   go vet ./...
   go test ./...                          # add -race if your machine supports it
   golangci-lint run --timeout=5m ./...
   ```
5. **When done:**
   - Write `BENCH_REPORT.md` in the repository root with:
     - the files you changed and why;
     - each command from rule 4 with its real result (copy the last lines of the output; if you couldn't run a command, say so);
     - anything you left undone, decided on your own, or are unsure about.
   - Commit all your changes on the current branch. Start each commit message with `bench: T<n>`. This overrides "Don't commit" in `AGENTS.md`. Don't push, and don't create or switch branches.
6. **Nobody can answer questions** during the task. If something is unclear, choose what keeps existing behavior and note it in the report.
7. **Don't edit** `TASKS.md`, `AGENTS.md`, `deployment/` or `.github/`.

---

## T1 — One tracker behavior over a working `clients.Client`

**Today:**
- `clients.Client` declares `FetchAndExtractData(trackerCode string)`, but both clients take `*config.Tracker`, so nothing implements the interface.
- `APITrackerBehavior` and `ScraperTrackerBehavior` are copies of each other that differ only in which client they create.

**Do:**
- Change `clients.Client` so that `*clients.PublicAPIClient` and `*clients.ScraperClient` both implement it, and add compile-time checks for that.
- Replace both behavior types and their constructors with one `ClientTrackerBehavior` that works with any `clients.Client`.
- In `CreateTracker`, pick the client for the tracker's type and wrap it in a `ClientTrackerBehavior`.
- `Execute` returns:
  - the client's value formatted with two decimals (`3.14159` → `"3.14"`), and the client's notification message;
  - on a client error, an empty value, an empty notification and that same error.

**Required API:**
```go
// package clients
type Client interface {
	FetchAndExtractData(trackerData *config.Tracker) (*DataResult, error)
}

// package handlers
type ClientTrackerBehavior struct{ /* unexported fields */ }

func NewClientTrackerBehavior(client clients.Client) *ClientTrackerBehavior
func (b *ClientTrackerBehavior) Execute(trackerData *config.Tracker) (string, string, error)
```

**Must no longer exist:** `APITrackerBehavior`, `ScraperTrackerBehavior`, `NewAPITrackerBehavior`, `NewScraperTrackerBehavior`.

**Allowed changes to existing tests:** none.

---

## T2 — Register tracker types in one place

**Today:** the two tracker types (API and scraper) are hard-coded in several places, and a new type would need edits in all of them:
- `Configuration.APITrackers` / `ScraperTrackers`
- `GetAPITrackerData` / `GetScraperTrackerData`
- `DetermineTrackerType`
- the `switch` in `CreateTracker`
- two loops each in `startAllTrackers` and `handleStatus`

**Do:**
- **Type on each tracker.** Every `config.Tracker` gets a `Type`, set while loading from the file it came from: `API_TRACKERS_FILE` → `api`, `SCRAPER_TRACKERS_FILE` → `scraper`. It is never read from JSON: a `"type"` key in a tracker file is ignored.
- **One list.** `Configuration` holds a single `Trackers` list: the API file's trackers in file order, then the scraper file's.
- **Duplicate codes.** `loadConfig()` returns an error when a tracker code appears more than once, in one file or across both. Today the API tracker silently wins. This is the one intended behavior change: document it in `tracker_configs/README.md`.
- **Behavior map.** `handlers` keeps a map from tracker type to behavior constructor. `CreateTracker` uses it and returns an error for a type that has no entry. After this task, adding a type means adding a map entry (plus its client and its loading) and nothing else in `handlers`.
- **Single loops.** The status overview and `/run` (all) loop over `Trackers` once. Each status line shows the tracker's type; for API and scraper trackers the text stays identical to today's.
- **Remove:** `Configuration.APITrackers`, `Configuration.ScraperTrackers`, `GetAPITrackerData`, `GetScraperTrackerData`, `handlers.DetermineTrackerType`, and the `handlers` constants `API` and `Scraper`.
- **Keep:** `GetTrackerData(code)` (with a single loop), the env vars, the tracker JSON field names, and the `loadConfig()` signature.

**Required API:**
```go
// package config
type TrackerType string

const (
	TrackerTypeAPI     TrackerType = "api"
	TrackerTypeScraper TrackerType = "scraper"
)

type Tracker struct {
	// ...existing fields unchanged, plus:
	Type TrackerType `json:"-"`
}

type Configuration struct {
	// ...existing fields, with APITrackers and ScraperTrackers replaced by:
	Trackers []*Tracker `validate:"dive"`
}

func (c *Configuration) GetTrackerData(code string) *Tracker

// package handlers
var trackerBehaviors map[config.TrackerType]func() TrackerBehavior
```

**Allowed changes to existing tests:** replace `APITrackers:` / `ScraperTrackers:` with `Trackers:` and set `Type` where needed, and make assertions that read `cfg.APITrackers` read `cfg.Trackers` instead. Nothing else.

---

## T3 — Move the running trackers out of `CommandHandler`

**Today:** on top of handling commands, `CommandHandler` keeps the running-tracker list and its mutex, starts, stops and resumes trackers, and saves the state file through 7 separate `saveState()` calls.

**Do:**
- **New type.** `TrackerManager` owns the running trackers, their lifecycle and the state file. `NewCommandHandler` creates one, and `CommandHandler` uses it for every tracker operation, keeping only command handling, navigation and messages.
- **Saving.** Every manager method that changes the running trackers saves the state file itself; `CommandHandler` no longer calls any save function.
- **Concurrency.**
  - All manager methods are safe to call concurrently.
  - Starting the same code from many goroutines at once starts it exactly once.
  - After concurrent calls finish, the state file matches the running trackers (no lost updates).
  - The manager never calls the messenger while holding its lock. `Resume` sends the "bot was restarted" messages after releasing it.
- **Interval check.** `SetInterval` checks the minimum interval before it checks whether the tracker is running.
- **Bot messages stay the same.**

**Required API** (package `handlers`):
```go
var (
	ErrTrackerAlreadyRunning = errors.New("...")
	ErrTrackerNotRunning     = errors.New("...")
	ErrIntervalBelowMinimum  = errors.New("...")
)

type TrackerManager struct{ /* unexported fields */ }

func NewTrackerManager(messenger helpers.Messenger, config *config.Configuration) *TrackerManager

// Get returns the running tracker with this code, or nil.
func (m *TrackerManager) Get(code string) *Tracker
// Start creates and starts a tracker for the chat. Errors: ErrTrackerAlreadyRunning; for an unknown
// code, an error for which errors.Is(err, errUnrecognizedTracker) is true.
func (m *TrackerManager) Start(code string, chatID int64) error
// StartAll starts every configured tracker that isn't running. Returns the errors by tracker code;
// nil or empty when all started.
func (m *TrackerManager) StartAll(chatID int64) map[string]error
// Stop stops a running tracker. Returns false if it wasn't running.
func (m *TrackerManager) Stop(code string) bool
func (m *TrackerManager) StopAll()
// Errors: ErrIntervalBelowMinimum, ErrTrackerNotRunning.
func (m *TrackerManager) SetInterval(code string, interval time.Duration) error
// Errors: ErrTrackerNotRunning.
func (m *TrackerManager) SetNotificationsPaused(code string, paused bool) error
// Resume does what CommandHandler.ResumeTrackers does today.
func (m *TrackerManager) Resume()

func (ch *CommandHandler) Trackers() *TrackerManager
```

**Keep:** `CreateTracker`, `errUnrecognizedTracker`, `readStateFile`, `writeStateFile`, `savedTracker` and the state file format.

**Must no longer exist on `CommandHandler`:** `ResumeTrackers`, `StopAllTrackers`, `GetActiveTracker`, `AddRunningTracker`, `RemoveRunningTracker`, `saveState`, and the `runningTrackers` field.

**Allowed changes to existing tests:** in `botfixer/bot_fixer_test.go`, replace `CommandHandler.StopAllTrackers()` with `CommandHandler.Trackers().StopAll()` and `CommandHandler.GetActiveTracker(` with `CommandHandler.Trackers().Get(`. Nothing else.
