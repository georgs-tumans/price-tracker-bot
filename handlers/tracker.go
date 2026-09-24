package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"pricetrackerbot/config"
	"pricetrackerbot/helpers"
	"pricetrackerbot/utilities"
)

const (
	API     = "api"
	Scraper = "scraper"

	// How many of the most recent execution errors are kept for a tracker
	executionErrorHistory = 20
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
	ExecutionErrors   []*TrackerExecutionError
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
	bot         *tgbotapi.BotAPI
	errorLimit  int

	// Guards everything below; the tracker goroutine and the command handlers access these concurrently
	mu     sync.Mutex
	status TrackerStatus
	cancel context.CancelFunc // nil while the tracker is not running
}

func CreateTracker(bot *tgbotapi.BotAPI, code string, runInterval time.Duration, config *config.Configuration, chatID int64) (*Tracker, error) {
	var behavior TrackerBehavior
	trackerType := DetermineTrackerType(code, config)
	trackerData := config.GetTrackerData(code)

	if trackerData == nil {
		log.Printf("[Tracker] Failed to create a new tracker: %s; no such tracker found in configuration", code)

		return nil, errUnrecognizedTracker
	}

	switch trackerType {
	case API:
		behavior = NewAPITrackerBehavior(bot)
	case Scraper:
		behavior = NewScraperTrackerBehavior(bot)
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

	return &Tracker{
		Code:        code,
		trackerData: trackerData,
		Behavior:    behavior,
		chatID:      chatID,
		bot:         bot,
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

func (t *Tracker) executeTrackerLogic() {
	// A panic in a single run must not take down the whole bot
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Tracker] Recovered from panic in tracker '%s': %v", t.Code, r)
			t.recordError(fmt.Errorf("panic: %v", r))
		}
	}()

	value, err := t.Behavior.Execute(t.trackerData, t.chatID)
	if err != nil {
		log.Printf("[Tracker] Error executing tracker '%s': %s", t.Code, err)
		t.recordError(err)

		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.status.LastRunTimestamp = time.Now()
	t.status.TotalRuns++
	t.status.ConsecutiveErrors = 0
	t.status.LastRecordedValue = value
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
	// Notify only once when the limit is reached, not on every following failed run
	notify := t.status.ConsecutiveErrors == t.errorLimit
	t.mu.Unlock()

	if notify {
		notificationMessage := fmt.Sprintf("Tracker <b>%s</b> has failed %d times in a row, you should probably take a look at the logs :(", t.Code, t.errorLimit)
		helpers.SendMessageHTML(t.bot, t.chatID, notificationMessage, nil)
	}
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
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Execute immediately on start
	t.executeTrackerLogic()

	for {
		select {
		case <-ticker.C:
			t.executeTrackerLogic()
		case <-ctx.Done():
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
