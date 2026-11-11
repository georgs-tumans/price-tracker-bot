package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"pricetrackerbot/config"
)

const testChatID = int64(1)

// Returns a command handler with one tracker, "bonds" (interval 1h, notify when < 10), that reads a value of 5
// from a local test server. Trackers are stopped when the test ends.
func newTestCommandHandler(t *testing.T) (*CommandHandler, *recordingMessenger) {
	t.Helper()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"price": 5}`))
	}))
	t.Cleanup(api.Close)

	cfg := &config.Configuration{
		MinInterval: 10 * time.Minute,
		StateFile:   filepath.Join(t.TempDir(), "state.json"),
		APITrackers: []*config.Tracker{{
			Code:               "bonds",
			DataURL:            api.URL,
			Interval:           "1h",
			DataExtractionPath: "price",
			NotifyCriteria:     []config.NotifyCriteria{{Operator: "<", Value: "10"}},
		}},
	}

	messenger := &recordingMessenger{}
	ch := NewCommandHandler(messenger, cfg)
	t.Cleanup(ch.StopAllTrackers)

	return ch, messenger
}

func hasButton(menu *models.InlineKeyboardMarkup, callbackData string) bool {
	if menu == nil {
		return false
	}

	for _, row := range menu.InlineKeyboard {
		for _, button := range row {
			if button.CallbackData == callbackData {
				return true
			}
		}
	}

	return false
}

func TestHandleCommandParsesParams(t *testing.T) {
	tests := []struct {
		command   string
		wantCode  string
		wantParam *string
	}{
		{command: "/probe", wantCode: ""},
		{command: "/probe bonds", wantCode: "bonds"},
		{command: "/probe bonds ", wantCode: "bonds"}, // Commands rebuilt from the navigation history end with a space
		{command: "/probe bonds 2h", wantCode: "bonds", wantParam: new("2h")},
		{command: "/probe  bonds   1 <= 45 ", wantCode: "bonds", wantParam: new("1 <= 45")},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			ch, _ := newTestCommandHandler(t)

			var gotCode string
			var gotParam *string
			ch.commandMap["probe"] = &Command{Handler: func(code string, _ int64, param *string) error {
				gotCode, gotParam = code, param

				return nil
			}}

			if err := ch.HandleCommand(testChatID, tt.command, nil, false); err != nil {
				t.Fatalf("HandleCommand: %v", err)
			}

			if gotCode != tt.wantCode {
				t.Errorf("code = %q, want %q", gotCode, tt.wantCode)
			}

			if (gotParam == nil) != (tt.wantParam == nil) || (gotParam != nil && *gotParam != *tt.wantParam) {
				t.Errorf("param = %v, want %v", ptrString(gotParam), ptrString(tt.wantParam))
			}
		})
	}
}

func ptrString(s *string) string {
	if s == nil {
		return "<nil>"
	}

	return `"` + *s + `"`
}

// A prompt that the user leaves by clicking something else must not catch the next text they send.
func TestCommandCancelsPendingInput(t *testing.T) {
	ch, messenger := newTestCommandHandler(t)
	messageID := 42

	if err := ch.HandleCommand(testChatID, "/interval bonds", &messageID, false); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	state := ch.GetUserNavigationState(testChatID)
	if !state.AwaitingUserInput() || !state.CustomKeyboardActive {
		t.Fatalf("the interval button did not ask for input: %+v", state)
	}

	if err := ch.HandleCommand(testChatID, "/status", &messageID, false); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if state.AwaitingUserInput() || state.CustomKeyboardActive {
		t.Errorf("another command left the prompt waiting for input: %+v", state)
	}

	if got := messenger.removedKeyboardCount(); got != 1 {
		t.Errorf("reply keyboard removed %d times, want 1", got)
	}
}

// "Return" must not run a change such as starting a tracker again when going back past it.
func TestReturnSkipsActions(t *testing.T) {
	ch, messenger := newTestCommandHandler(t)
	messageID := 42

	state := ch.GetUserNavigationState(testChatID)
	state.BackButtonEnabled = true
	state.Push(&Command{Command: "status", Params: []string{"", ""}})
	state.Push(&Command{Command: "run", Params: []string{"bonds", ""}})
	state.Push(&Command{Command: "status", Params: []string{"bonds", ""}})

	if err := ch.HandleReturn(testChatID, &messageID); err != nil {
		t.Fatalf("HandleReturn: %v", err)
	}

	if ch.GetActiveTracker("bonds") != nil {
		t.Error("Return started the tracker again")
	}

	if last := messenger.last(); !strings.Contains(last.text, "All available trackers") {
		t.Errorf("Return showed %q, want the list of all trackers", last.text)
	}
}

func TestSettingsSurviveStopAndRun(t *testing.T) {
	ch, _ := newTestCommandHandler(t)

	// Changing a stopped tracker is allowed and saved
	if err := ch.handleSetInterval("bonds", testChatID, new("2h")); err != nil {
		t.Fatalf("handleSetInterval on a stopped tracker: %v", err)
	}

	data, _ := os.ReadFile(ch.config.StateFile)
	if !strings.Contains(string(data), `"interval": "2h0m0s"`) || !strings.Contains(string(data), `"running": false`) {
		t.Errorf("state file does not contain the stopped tracker's interval: %s", data)
	}

	for range 2 {
		if err := ch.handleStart("bonds", testChatID, nil); err != nil {
			t.Fatalf("handleStart: %v", err)
		}

		tracker := ch.GetActiveTracker("bonds")
		if got := tracker.Status().CurrentInterval; got != 2*time.Hour {
			t.Errorf("CurrentInterval after run = %s, want the changed 2h", got)
		}

		if tracker.Status().NotificationsPaused {
			t.Error("a newly started tracker must have its notifications on")
		}

		tracker.SetNotificationsPaused(true)
		if err := ch.handleStop("bonds", testChatID, nil); err != nil {
			t.Fatalf("handleStop: %v", err)
		}
	}

	// Setting the configured value again drops the change
	if err := ch.handleSetInterval("bonds", testChatID, new("1h")); err != nil {
		t.Fatalf("handleSetInterval: %v", err)
	}

	if settings := ch.trackerSettings("bonds"); settings.Interval != 0 {
		t.Errorf("interval equal to the config is still saved as a change: %+v", settings)
	}

	data, _ = os.ReadFile(ch.config.StateFile)
	if strings.Contains(string(data), "bonds") {
		t.Errorf("state file still contains a stopped tracker without changes: %s", data)
	}
}

func TestSetIntervalUnknownTracker(t *testing.T) {
	ch, messenger := newTestCommandHandler(t)

	if err := ch.handleSetInterval("unknown<b>", testChatID, new("2h")); err == nil {
		t.Error("expected an error for a tracker that is not in the configuration")
	}

	if last := messenger.last(); !strings.Contains(last.text, "unknown&lt;b&gt;") {
		t.Errorf("unexpected message: %q", last.text)
	}

	ch.mu.Lock()
	defer ch.mu.Unlock()
	if len(ch.settings) != 0 {
		t.Errorf("settings saved for an unknown tracker: %+v", ch.settings)
	}
}

func TestResetRestoresConfig(t *testing.T) {
	ch, messenger := newTestCommandHandler(t)
	changedCriteria := []config.NotifyCriteria{{Operator: ">", Value: "100"}}
	ch.setSettings("bonds", TrackerSettings{Interval: 2 * time.Hour, NotifyCriteria: &changedCriteria})

	if err := ch.handleStart("bonds", testChatID, nil); err != nil {
		t.Fatalf("handleStart: %v", err)
	}

	tracker := ch.GetActiveTracker("bonds")
	if got := tracker.Status().NotifyCriteria; !slices.Equal(got, changedCriteria) {
		t.Errorf("started tracker criteria = %+v, want the changed %+v", got, changedCriteria)
	}

	_ = ch.handleStatus("bonds", testChatID, nil)
	status := messenger.last()
	if !strings.Contains(status.text, "Current run interval: 2h 0m 0s (changed, config: 1h 0m 0s)") || !hasButton(status.menu, "/reset bonds") {
		t.Errorf("status does not show the changes: %q %+v", status.text, status.menu)
	}

	if err := ch.handleReset("bonds", testChatID, nil); err != nil {
		t.Fatalf("handleReset: %v", err)
	}

	if settings := ch.trackerSettings("bonds"); settings.Interval != 0 || settings.NotifyCriteria != nil {
		t.Errorf("settings after reset = %+v, want none", settings)
	}

	status = messenger.last()
	if strings.Contains(strings.ToLower(status.text), "changed") || hasButton(status.menu, "/reset bonds") {
		t.Errorf("status after reset still shows changes: %q %+v", status.text, status.menu)
	}

	got := tracker.Status()
	if got.CurrentInterval != time.Hour || !slices.Equal(got.NotifyCriteria, ch.config.APITrackers[0].NotifyCriteria) {
		t.Errorf("running tracker after reset: interval %s, criteria %+v; want the config values", got.CurrentInterval, got.NotifyCriteria)
	}
}

func TestStatusOfStoppedTrackerOffersReset(t *testing.T) {
	ch, messenger := newTestCommandHandler(t)
	ch.setSettings("bonds", TrackerSettings{Interval: 2 * time.Hour})

	if err := ch.handleStatus("bonds", testChatID, nil); err != nil {
		t.Errorf("showing a stopped tracker is not an error, got %v", err)
	}

	if status := messenger.last(); !hasButton(status.menu, "/reset bonds") || !hasButton(status.menu, "/run bonds") {
		t.Errorf("stopped tracker status is missing the run or reset button: %+v", status.menu)
	}
}

func TestStatusOfUnknownTracker(t *testing.T) {
	ch, messenger := newTestCommandHandler(t)

	if err := ch.handleStatus("unknown<b>", testChatID, nil); err == nil {
		t.Error("expected an error for a tracker that is not in the configuration")
	}

	status := messenger.last()
	if !strings.Contains(status.text, "unknown&lt;b&gt;</b> not found") || hasButton(status.menu, "/run unknown<b>") {
		t.Errorf("unexpected status of an unknown tracker: %q %+v", status.text, status.menu)
	}
}
