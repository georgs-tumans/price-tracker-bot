package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"pricetrackerbot/config"
	"pricetrackerbot/helpers"
	"pricetrackerbot/services"
	"pricetrackerbot/utilities"
)

const (
	API     = "api"
	Scraper = "scraper"

	// How many of the most recent execution errors are kept for a tracker.
	executionErrorHistory = 20

	// While a site keeps blocking requests, the run interval is doubled up to this many times the configured one.
	maxBackoffMultiplier = 8
	// Upper limit for a backed off delay and for a server provided Retry-After, unless the interval itself is longer.
	maxBackoffDelay = 24 * time.Hour
)

var errUnrecognizedTracker = errors.New("unrecognized tracker code")

type TrackerStatus struct {
	StartTimestamp    time.Time
	LastRunTimestamp  time.Time
	TotalRuns         int
	TotalErrors       int
	ConsecutiveErrors int
	LastRecordedValue string
	CurrentInterval   time.Duration
	// How many runs in a row the site responded with a blocking status code (403, 429, 503); 0 when not backing off
	BackoffLevel     int
	NextRunTimestamp time.Time
	ExecutionErrors  []*TrackerExecutionError
	// Only criteria-match notifications are paused; error alerts are still sent
	NotificationsPaused bool
}

type TrackerExecutionError struct {
	Error     error
	Timestamp time.Time
}

// Tracker represents a single URL that the bot will track - either through an API or by scraping a website.
type Tracker struct {
	Code        string
	Behavior    TrackerBehavior
	trackerData *config.Tracker
	chatID      int64
	messenger   helpers.Messenger
	errorLimit  int

	// Guards everything below; the tracker goroutine and the command handlers access these concurrently
	mu         sync.Mutex
	status     TrackerStatus
	retryAfter time.Duration      // Retry-After of the last blocking response
	cancel     context.CancelFunc // nil while the tracker is not running
}

func CreateTracker(messenger helpers.Messenger, code string, runInterval time.Duration, config *config.Configuration, chatID int64) (*Tracker, error) {
	var behavior TrackerBehavior
	trackerType := DetermineTrackerType(code, config)
	trackerData := config.GetTrackerData(code)

	if trackerData == nil {
		log.Printf("[Tracker] Failed to create a new tracker: %s; no such tracker found in configuration", code)

		return nil, errUnrecognizedTracker
	}

	switch trackerType {
	case API:
		behavior = NewAPITrackerBehavior()
	case Scraper:
		behavior = NewScraperTrackerBehavior()
	default:
		return nil, fmt.Errorf("unsupported client type for code: %s", code)
	}

	// If runInterval is not provided, use the default interval from the configuration
	runIntervalToUse := runInterval
	if runIntervalToUse == 0 {
		var err error
		runIntervalToUse, err = utilities.ParseDurationWithDays(trackerData.Interval)
		if err != nil {
			log.Printf("[Tracker] Error parsing default configured run interval for tracker '%s': %s", code, err.Error())
			return nil, err
		}
	}

	if runIntervalToUse < config.MinInterval {
		log.Printf("[Tracker] Run interval %s of tracker '%s' is below the minimum, using %s instead", runIntervalToUse, code, config.MinInterval)
		runIntervalToUse = config.MinInterval
	}

	return &Tracker{
		Code:        code,
		trackerData: trackerData,
		Behavior:    behavior,
		chatID:      chatID,
		messenger:   messenger,
		errorLimit:  config.ErrorNotifyLimit,
		status: TrackerStatus{
			CurrentInterval: runIntervalToUse,
		},
	}, nil
}

// Status returns a copy of the tracker status that is safe to read while the tracker keeps running.
func (t *Tracker) Status() TrackerStatus {
	t.mu.Lock()
	defer t.mu.Unlock()

	status := t.status
	status.ExecutionErrors = append([]*TrackerExecutionError(nil), t.status.ExecutionErrors...)

	return status
}

func (t *Tracker) ChatID() int64 {
	return t.chatID
}

func (t *Tracker) SetNotificationsPaused(paused bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.status.NotificationsPaused = paused
}

func (t *Tracker) executeTrackerLogic() {
	// A panic in a single run must not take down the whole bot
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Tracker] Recovered from panic in tracker '%s': %v", t.Code, r)
			t.recordError(fmt.Errorf("panic: %v", r))
		}
	}()

	value, notification, err := t.Behavior.Execute(t.trackerData)
	if err != nil {
		log.Printf("[Tracker] Error executing tracker '%s': %s", t.Code, err)
		t.recordError(err)

		return
	}

	t.mu.Lock()
	t.status.LastRunTimestamp = time.Now()
	t.status.TotalRuns++
	t.status.ConsecutiveErrors = 0
	t.status.BackoffLevel = 0
	t.retryAfter = 0
	t.status.LastRecordedValue = value
	paused := t.status.NotificationsPaused
	t.mu.Unlock()

	if notification == "" {
		return
	}

	if paused {
		log.Printf("[Tracker] Notifications paused for tracker '%s', not sending notification", t.Code)
		return
	}

	t.messenger.SendHTMLWithMenu(t.chatID, notification, helpers.GetNotificationMenu(t.Code, false))
}

func (t *Tracker) recordError(err error) {
	t.mu.Lock()
	t.status.LastRunTimestamp = time.Now()
	t.status.TotalRuns++
	t.status.TotalErrors++
	t.status.ConsecutiveErrors++
	t.status.ExecutionErrors = append(t.status.ExecutionErrors, &TrackerExecutionError{Error: err, Timestamp: time.Now()})
	if len(t.status.ExecutionErrors) > executionErrorHistory {
		t.status.ExecutionErrors = t.status.ExecutionErrors[len(t.status.ExecutionErrors)-executionErrorHistory:]
	}

	var statusErr *services.HTTPStatusError
	blocked := errors.As(err, &statusErr) && statusErr.IsBlocking()
	if blocked {
		t.status.BackoffLevel++
		t.retryAfter = statusErr.RetryAfter
	}
	nextDelay := backoffDelay(t.status.CurrentInterval, t.status.BackoffLevel, t.retryAfter)

	// Notify only once when the limit is reached, not on every following failed run
	notify := t.status.ConsecutiveErrors == t.errorLimit
	t.mu.Unlock()

	if blocked {
		log.Printf("[Tracker] Tracker '%s' got HTTP %d, backing off; next run in %s", t.Code, statusErr.StatusCode, nextDelay)
	}

	if notify {
		notificationMessage := fmt.Sprintf("Tracker <b>%s</b> has failed %d times in a row, you should probably take a look at the logs :(", t.Code, t.errorLimit)
		if blocked {
			notificationMessage += fmt.Sprintf("\n\nThe site seems to be blocking or rate limiting requests (HTTP %d), so the tracker now runs less often: next run in %s",
				statusErr.StatusCode, utilities.DurationToString(nextDelay))
		}
		t.messenger.SendHTML(t.chatID, notificationMessage)
	}
}

// Returns how long to wait until the next run: the interval, doubled for every blocked run in a row (up to
// maxBackoffMultiplier), and never shorter than what the server asked for with Retry-After.
func backoffDelay(interval time.Duration, backoffLevel int, retryAfter time.Duration) time.Duration {
	if backoffLevel <= 0 {
		return interval
	}

	multiplier := min(1<<min(backoffLevel, 30), maxBackoffMultiplier)
	delay := min(interval*time.Duration(multiplier), max(interval, maxBackoffDelay))

	return max(delay, min(retryAfter, maxBackoffDelay))
}

// Works out when the next run happens, records it in the status and returns the time to wait.
func (t *Tracker) scheduleNextRun(interval time.Duration) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	delay := backoffDelay(interval, t.status.BackoffLevel, t.retryAfter)
	t.status.NextRunTimestamp = time.Now().Add(delay)

	return delay
}

func (t *Tracker) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel != nil {
		return
	}

	if t.status.StartTimestamp.IsZero() {
		t.status.StartTimestamp = time.Now() // Set the start timestamp only when the tracker is started for the first time
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel

	go t.run(ctx, t.status.CurrentInterval)
}

func (t *Tracker) run(ctx context.Context, interval time.Duration) {
	// Execute immediately on start, then wait between runs; a timer instead of a ticker lets the delay grow
	// while the site is blocking requests
	for {
		t.executeTrackerLogic()

		timer := time.NewTimer(t.scheduleNextRun(interval))
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			log.Printf("[Tracker] Stopping tracker '%s'", t.Code)
			return
		}
	}
}

// Stop signals the tracker goroutine to exit. It does not wait for a run that is already in progress;
// the clients are stateless, so a finishing run cannot interfere with a restarted tracker.
func (t *Tracker) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel == nil {
		return
	}

	t.cancel()
	t.cancel = nil
}

func (t *Tracker) UpdateInterval(newInterval time.Duration) {
	t.Stop()

	t.mu.Lock()
	t.status.CurrentInterval = newInterval
	t.mu.Unlock()

	t.Start()
}

func DetermineTrackerType(trackerCode string, config *config.Configuration) string {
	for _, tracker := range config.APITrackers {
		if tracker.Code == trackerCode {
			return API
		}
	}

	for _, tracker := range config.ScraperTrackers {
		if tracker.Code == trackerCode {
			return Scraper
		}
	}

	return ""
}
