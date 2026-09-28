package handlers

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"pricetrackerbot/config"
	"pricetrackerbot/services"
)

type fakeBehavior struct {
	calls        atomic.Int32
	err          error
	doPanic      bool
	notification string
}

func (f *fakeBehavior) Execute(_ *config.Tracker) (string, string, error) {
	f.calls.Add(1)

	if f.doPanic {
		panic("boom")
	}

	return "1.00", f.notification, f.err
}

type sentMessage struct {
	text string
	menu *models.InlineKeyboardMarkup
}

type recordingMessenger struct {
	mu   sync.Mutex
	sent []sentMessage
}

func (m *recordingMessenger) record(text string, menu *models.InlineKeyboardMarkup) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sent = append(m.sent, sentMessage{text: text, menu: menu})
}

func (m *recordingMessenger) SendHTML(_ int64, text string) { m.record(text, nil) }
func (m *recordingMessenger) SendHTMLWithMenu(_ int64, text string, menu *models.InlineKeyboardMarkup) {
	m.record(text, menu)
}
func (m *recordingMessenger) SendHTMLWithKeyboard(_ int64, text string, _ *models.ReplyKeyboardMarkup) {
	m.record(text, nil)
}
func (m *recordingMessenger) EditHTMLWithMenu(_ int64, _ int, _ string, _ *models.InlineKeyboardMarkup) {
}
func (m *recordingMessenger) EditMenu(_ int64, _ int, _ *models.InlineKeyboardMarkup) {}
func (m *recordingMessenger) RemoveKeyboard(_ int64)                                  {}

// A high error limit keeps the tests from trying to send a Telegram notification.
func newTestTracker(behavior TrackerBehavior, interval time.Duration) *Tracker {
	return &Tracker{
		Code:       "test",
		Behavior:   behavior,
		errorLimit: 1000,
		status:     TrackerStatus{CurrentInterval: interval},
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}

		time.Sleep(time.Millisecond)
	}
}

func TestTrackerLifecycle(t *testing.T) {
	behavior := &fakeBehavior{}
	tracker := newTestTracker(behavior, 5*time.Millisecond)

	tracker.Start()
	tracker.Start() // Must not start a second goroutine

	// Read the status concurrently with the running tracker (caught by -race if unsafe)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			_ = tracker.Status()
		}
	}()

	waitFor(t, func() bool { return behavior.calls.Load() >= 3 })

	tracker.UpdateInterval(2 * time.Millisecond)
	if got := tracker.Status().CurrentInterval; got != 2*time.Millisecond {
		t.Errorf("CurrentInterval = %s, want 2ms", got)
	}

	callsBefore := behavior.calls.Load()
	waitFor(t, func() bool { return behavior.calls.Load() >= callsBefore+3 })

	tracker.Stop()
	tracker.Stop() // Stopping twice is a no-op
	wg.Wait()

	callsAtStop := behavior.calls.Load()
	time.Sleep(30 * time.Millisecond)
	// At most one run that was already in progress may finish after Stop
	if got := behavior.calls.Load(); got > callsAtStop+1 {
		t.Errorf("tracker kept running after Stop: %d calls at stop, %d later", callsAtStop, got)
	}

	if status := tracker.Status(); status.TotalRuns == 0 || status.LastRecordedValue != "1.00" {
		t.Errorf("unexpected status after runs: %+v", status)
	}
}

func TestTrackerErrorHistoryIsCapped(t *testing.T) {
	behavior := &fakeBehavior{err: errors.New("fetch failed")}
	tracker := newTestTracker(behavior, time.Hour)

	for range 30 {
		tracker.executeTrackerLogic()
	}

	status := tracker.Status()
	if status.TotalErrors != 30 || status.ConsecutiveErrors != 30 {
		t.Errorf("TotalErrors = %d, ConsecutiveErrors = %d, want 30 and 30", status.TotalErrors, status.ConsecutiveErrors)
	}

	if len(status.ExecutionErrors) != executionErrorHistory {
		t.Errorf("kept %d errors, want %d", len(status.ExecutionErrors), executionErrorHistory)
	}

	behavior.err = nil
	tracker.executeTrackerLogic()

	if status := tracker.Status(); status.ConsecutiveErrors != 0 || status.TotalErrors != 30 {
		t.Errorf("after a success: ConsecutiveErrors = %d, TotalErrors = %d, want 0 and 30", status.ConsecutiveErrors, status.TotalErrors)
	}
}

func TestTrackerRecoversFromPanic(t *testing.T) {
	tracker := newTestTracker(&fakeBehavior{doPanic: true}, time.Hour)

	tracker.executeTrackerLogic() // Must not panic

	if got := tracker.Status().TotalErrors; got != 1 {
		t.Errorf("TotalErrors = %d, want 1", got)
	}
}

func TestTrackerNotificationsCanBePaused(t *testing.T) {
	messenger := &recordingMessenger{}
	tracker := newTestTracker(&fakeBehavior{notification: "Good news"}, time.Hour)
	tracker.messenger = messenger

	tracker.executeTrackerLogic()

	if len(messenger.sent) != 1 || messenger.sent[0].text != "Good news" {
		t.Fatalf("expected one notification, got %+v", messenger.sent)
	}

	menu := messenger.sent[0].menu
	if menu == nil || menu.InlineKeyboard[0][0].CallbackData != "/mute test n" {
		t.Errorf("notification is missing the pause button: %+v", menu)
	}

	tracker.SetNotificationsPaused(true)
	tracker.executeTrackerLogic()

	if len(messenger.sent) != 1 {
		t.Errorf("notification sent while paused: %+v", messenger.sent)
	}

	if status := tracker.Status(); status.TotalRuns != 2 || !status.NotificationsPaused {
		t.Errorf("unexpected status: %+v", status)
	}
}

func TestBackoffDelay(t *testing.T) {
	tests := []struct {
		name       string
		interval   time.Duration
		level      int
		retryAfter time.Duration
		want       time.Duration
	}{
		{name: "not backing off", interval: time.Hour, level: 0, want: time.Hour},
		{name: "first block doubles", interval: time.Hour, level: 1, want: 2 * time.Hour},
		{name: "second block doubles again", interval: time.Hour, level: 2, want: 4 * time.Hour},
		{name: "capped at 8x", interval: 10 * time.Minute, level: 10, want: 80 * time.Minute},
		{name: "huge level does not overflow", interval: 10 * time.Minute, level: 1000, want: 80 * time.Minute},
		{name: "capped at 24h", interval: 6 * time.Hour, level: 3, want: 24 * time.Hour},
		{name: "interval longer than the cap is kept", interval: 48 * time.Hour, level: 3, want: 48 * time.Hour},
		{name: "longer Retry-After wins", interval: 10 * time.Minute, level: 1, retryAfter: time.Hour, want: time.Hour},
		{name: "shorter Retry-After is ignored", interval: time.Hour, level: 1, retryAfter: time.Minute, want: 2 * time.Hour},
		{name: "Retry-After is capped", interval: 10 * time.Minute, level: 1, retryAfter: 72 * time.Hour, want: 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backoffDelay(tt.interval, tt.level, tt.retryAfter); got != tt.want {
				t.Errorf("backoffDelay() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTrackerBacksOffWhenBlocked(t *testing.T) {
	behavior := &fakeBehavior{err: &services.HTTPStatusError{StatusCode: 429, RetryAfter: time.Hour}}
	tracker := newTestTracker(behavior, 10*time.Minute)

	tracker.executeTrackerLogic()
	tracker.executeTrackerLogic()

	if got := tracker.Status().BackoffLevel; got != 2 {
		t.Errorf("BackoffLevel = %d, want 2", got)
	}

	if got := tracker.scheduleNextRun(10 * time.Minute); got != time.Hour {
		t.Errorf("next delay = %s, want the server's Retry-After of 1h", got)
	}

	// Errors that don't come from a blocking status leave the backoff as is
	behavior.err = errors.New("network down")
	tracker.executeTrackerLogic()
	if got := tracker.Status().BackoffLevel; got != 2 {
		t.Errorf("BackoffLevel after a non-blocking error = %d, want 2", got)
	}

	behavior.err = nil
	tracker.executeTrackerLogic()
	if got := tracker.Status().BackoffLevel; got != 0 {
		t.Errorf("BackoffLevel after a successful run = %d, want 0", got)
	}

	if got := tracker.scheduleNextRun(10 * time.Minute); got != 10*time.Minute {
		t.Errorf("next delay after recovering = %s, want 10m", got)
	}
}

func TestCreateTrackerRaisesIntervalToMinimum(t *testing.T) {
	cfg := &config.Configuration{
		MinInterval: 10 * time.Minute,
		APITrackers: []*config.Tracker{{Code: "bonds", DataURL: "https://example.com", Interval: "1m", DataExtractionPath: "price"}},
	}

	// Once from the configured default, once from a restored state interval
	for _, interval := range []time.Duration{0, time.Minute} {
		tracker, err := CreateTracker(&recordingMessenger{}, "bonds", interval, cfg, 1)
		if err != nil {
			t.Fatalf("CreateTracker: %v", err)
		}

		if got := tracker.Status().CurrentInterval; got != 10*time.Minute {
			t.Errorf("CurrentInterval = %s, want the 10m minimum", got)
		}
	}
}

func TestSetIntervalRejectsValuesBelowMinimum(t *testing.T) {
	messenger := &recordingMessenger{}
	ch := NewCommandHandler(messenger, &config.Configuration{MinInterval: 10 * time.Minute})

	value := "5m"
	if err := ch.handleSetInterval("bonds", 1, &value); err == nil {
		t.Fatal("expected an error for an interval below the minimum")
	}

	messenger.mu.Lock()
	defer messenger.mu.Unlock()
	if len(messenger.sent) != 1 || !strings.Contains(messenger.sent[0].text, "at least 10m") {
		t.Errorf("unexpected messages: %+v", messenger.sent)
	}
}
