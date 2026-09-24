package handlers

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pricetrackerbot/config"
)

type fakeBehavior struct {
	calls   atomic.Int32
	err     error
	doPanic bool
}

func (f *fakeBehavior) Execute(_ *config.Tracker, _ int64) (string, error) {
	f.calls.Add(1)

	if f.doPanic {
		panic("boom")
	}

	return "1.00", f.err
}

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
