package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"pricetrackerbot/config"
	"pricetrackerbot/helpers"
	"pricetrackerbot/utilities"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	API     = "api"
	Scraper = "scraper"
)

type TrackerStatus struct {
	StartTimestamp    time.Time
	LastRunTimestamp  time.Time
	TotalRuns         int
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
	Ticker      *time.Ticker
	Context     context.Context
	Cancel      context.CancelFunc
	Behavior    TrackerBehavior
	trackerData *config.Tracker
	Status      TrackerStatus
	running     bool
	chatID      int64
	bot         *tgbotapi.BotAPI
	errorLimit  int
}

func CreateTracker(bot *tgbotapi.BotAPI, code string, runInterval time.Duration, config *config.Configuration, chatID int64) (*Tracker, error) {
	var behavior TrackerBehavior
	trackerType := DetermineTrackerType(code, config)
	trackerData := config.GetTrackerData(code)

	if trackerData == nil {
		log.Printf("[Tracker] Failed to create a new tracker: %s; no such tracker found in configuration", code)

		return nil, errors.New("uncregonzied tracker code")
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

	ctx, cancel := context.WithCancel(context.Background())

	return &Tracker{
		Code:        code,
		Ticker:      time.NewTicker(runIntervalToUse),
		trackerData: trackerData,
		Context:     ctx,
		Cancel:      cancel,
		Behavior:    behavior,
		chatID:      chatID,
		bot:         bot,
		errorLimit:  config.ErrorNotifyLimit,
		Status: TrackerStatus{
			CurrentInterval: runIntervalToUse,
		},
	}, nil
}

func (t *Tracker) executeTrackerLogic() {
	t.Status.LastRunTimestamp = time.Now()
	t.Status.TotalRuns++

	if value, err := t.Behavior.Execute(t.trackerData, t.chatID); err != nil {
		log.Printf("[Tracker] Error executing tracker '%s': %s", t.Code, err)
		t.Status.ExecutionErrors = append(t.Status.ExecutionErrors, &TrackerExecutionError{Error: err, Timestamp: time.Now()})

		// Notify the user about error accumulation
		if len(t.Status.ExecutionErrors) >= t.errorLimit {
			notificationMessage := fmt.Sprintf("More than %d execution errors registered for tracker <b>%s</b>, you should probably take a look at the logs :(", t.errorLimit, t.Code)
			helpers.SendMessageHTML(t.bot, t.chatID, notificationMessage, nil)
		}
	} else {
		t.Status.LastRecordedValue = value
	}
}

func (t *Tracker) Start() {
	if t.running {
		return
	}

	// Recreate context for when the tracker is being restarted after interval update
	if t.Context.Err() != nil {
		t.Context, t.Cancel = context.WithCancel(context.Background())
	} else {
		t.Status.StartTimestamp = time.Now() // Set the start timestamp only when the tracker is started for the first time
	}

	t.running = true

	go func() {
		defer func() { t.running = false }()

		// Execute immediately on start
		t.executeTrackerLogic()

		for {
			select {
			case <-t.Ticker.C:
				t.executeTrackerLogic()
			case <-t.Context.Done():
				log.Printf("[Tracker] Stopping tracker '%s'", t.Code)
				return
			}
		}
	}()
}

func (t *Tracker) Stop() {
	if !t.running {
		return
	}

	t.Ticker.Stop()
	t.Cancel()
	t.running = false
}

func (t *Tracker) UpdateInterval(newInterval time.Duration) {
	t.Stop()
	time.Sleep(1 * time.Second)
	t.Ticker = time.NewTicker(newInterval)
	t.Status.CurrentInterval = newInterval
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
