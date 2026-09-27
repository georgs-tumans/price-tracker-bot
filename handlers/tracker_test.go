package handlers

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pricetrackerbot/config"

	"github.com/go-telegram/bot/models"
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
