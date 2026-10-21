package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"pricetrackerbot/config"
)

const benchT3Timeout = 5 * time.Second

type benchT3Messenger struct {
	mu     sync.Mutex
	texts  []string
	onSend func() // Called on every send, outside the messenger's own lock
}

func (m *benchT3Messenger) add(text string) {
	if m.onSend != nil {
		m.onSend()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.texts = append(m.texts, text)
}

func (m *benchT3Messenger) all() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return slices.Clone(m.texts)
}

func (m *benchT3Messenger) last() string {
	texts := m.all()
	if len(texts) == 0 {
		return ""
	}

	return texts[len(texts)-1]
}

func (m *benchT3Messenger) SendHTML(_ int64, text string) { m.add(text) }
func (m *benchT3Messenger) SendHTMLWithMenu(_ int64, text string, _ *models.InlineKeyboardMarkup) {
	m.add(text)
}
func (m *benchT3Messenger) SendHTMLWithKeyboard(_ int64, text string, _ *models.ReplyKeyboardMarkup) {
	m.add(text)
}
func (m *benchT3Messenger) EditHTMLWithMenu(_ int64, _ int, text string, _ *models.InlineKeyboardMarkup) {
	m.add(text)
}
func (m *benchT3Messenger) EditMenu(_ int64, _ int, _ *models.InlineKeyboardMarkup) {}
func (m *benchT3Messenger) RemoveKeyboard(_ int64)                                  {}

// A config whose API trackers read a local server; the high error limit keeps trackers from sending alerts.
func benchT3Config(t *testing.T, codes ...string) *config.Configuration {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"price": 5}`))
	}))
	t.Cleanup(server.Close)

	cfg := &config.Configuration{
		ErrorNotifyLimit: 1000,
		MinInterval:      10 * time.Minute,
		StateFile:        filepath.Join(t.TempDir(), "state.json"),
	}

	for _, code := range codes {
		cfg.APITrackers = append(cfg.APITrackers, &config.Tracker{Code: code, DataURL: server.URL, Interval: "1h", DataExtractionPath: "price"})
	}

	return cfg
}

func benchT3Manager(t *testing.T, cfg *config.Configuration, messenger *benchT3Messenger) *TrackerManager {
	t.Helper()

	m := NewTrackerManager(messenger, cfg)
	t.Cleanup(m.StopAll)

	return m
}

func benchT3State(t *testing.T, cfg *config.Configuration) map[string]savedTracker {
	t.Helper()

	saved, err := readStateFile(cfg.StateFile)
	if err != nil {
		t.Fatalf("reading the state file: %v", err)
	}

	byCode := make(map[string]savedTracker, len(saved))
	for _, s := range saved {
		byCode[s.Code] = s
	}

	return byCode
}

func benchT3StateCodes(t *testing.T, cfg *config.Configuration) []string {
	t.Helper()

	var codes []string
	for code := range benchT3State(t, cfg) {
		codes = append(codes, code)
	}

	slices.Sort(codes)

	return codes
}

func TestBenchT3StartStopAndSettingsSaveState(t *testing.T) {
	cfg := benchT3Config(t, "a", "b")
	m := benchT3Manager(t, cfg, &benchT3Messenger{})

	if err := m.Start("a", 42); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if tr := m.Get("a"); tr == nil || tr.ChatID() != 42 {
		t.Fatalf("Get(\"a\") = %v, want a tracker for chat 42", tr)
	}

	if s, ok := benchT3State(t, cfg)["a"]; !ok || s.ChatID != 42 || s.Interval != "1h0m0s" {
		t.Errorf("state after Start = %+v, want a for chat 42 with 1h0m0s", benchT3State(t, cfg))
	}

	if err := m.Start("a", 42); !errors.Is(err, ErrTrackerAlreadyRunning) {
		t.Errorf("second Start = %v, want ErrTrackerAlreadyRunning", err)
	}

	if err := m.Start("unknown", 42); !errors.Is(err, errUnrecognizedTracker) {
		t.Errorf("Start of an unknown code = %v, want errUnrecognizedTracker", err)
	}

	if err := m.SetNotificationsPaused("a", true); err != nil {
		t.Fatalf("SetNotificationsPaused: %v", err)
	}

	if !m.Get("a").Status().NotificationsPaused || !benchT3State(t, cfg)["a"].NotificationsPaused {
		t.Error("pausing notifications was not applied or not saved")
	}

	if err := m.SetInterval("a", 2*time.Hour); err != nil {
		t.Fatalf("SetInterval: %v", err)
	}

	if m.Get("a").Status().CurrentInterval != 2*time.Hour || benchT3State(t, cfg)["a"].Interval != "2h0m0s" {
		t.Error("the new interval was not applied or not saved")
	}

	if err := m.SetInterval("a", time.Minute); !errors.Is(err, ErrIntervalBelowMinimum) {
		t.Errorf("SetInterval below the minimum = %v, want ErrIntervalBelowMinimum", err)
	}

	if got := m.Get("a").Status().CurrentInterval; got != 2*time.Hour {
		t.Errorf("a rejected interval changed the tracker to %s", got)
	}

	// "b" is configured but not running; the minimum is checked first
	if err := m.SetInterval("b", 2*time.Hour); !errors.Is(err, ErrTrackerNotRunning) {
		t.Errorf("SetInterval of a stopped tracker = %v, want ErrTrackerNotRunning", err)
	}

	if err := m.SetInterval("b", time.Minute); !errors.Is(err, ErrIntervalBelowMinimum) {
		t.Errorf("SetInterval below the minimum of a stopped tracker = %v, want ErrIntervalBelowMinimum", err)
	}

	if err := m.SetNotificationsPaused("b", true); !errors.Is(err, ErrTrackerNotRunning) {
		t.Errorf("SetNotificationsPaused of a stopped tracker = %v, want ErrTrackerNotRunning", err)
	}

	if !m.Stop("a") {
		t.Error("Stop of a running tracker returned false")
	}

	if m.Get("a") != nil || len(benchT3State(t, cfg)) != 0 {
		t.Errorf("after Stop: Get = %v, state = %+v; want nil and empty", m.Get("a"), benchT3State(t, cfg))
	}

	if m.Stop("a") {
		t.Error("Stop of a stopped tracker returned true")
	}
}

func TestBenchT3StartAllAndStopAll(t *testing.T) {
	cfg := benchT3Config(t, "a", "b")
	cfg.APITrackers = append(cfg.APITrackers, &config.Tracker{Code: "bad", DataURL: "https://example.com", Interval: "never", DataExtractionPath: "price"})
	m := benchT3Manager(t, cfg, &benchT3Messenger{})

	if err := m.Start("a", 7); err != nil {
		t.Fatalf("Start: %v", err)
	}

	errs := m.StartAll(7)
	if len(errs) != 1 || errs["bad"] == nil {
		t.Errorf("StartAll errors = %v, want exactly one, for \"bad\"", errs)
	}

	if m.Get("a") == nil || m.Get("b") == nil || m.Get("bad") != nil {
		t.Error("StartAll did not start exactly the valid trackers")
	}

	if got := benchT3StateCodes(t, cfg); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("state after StartAll = %v, want [a b]", got)
	}

	m.StopAll()

	if m.Get("a") != nil || m.Get("b") != nil || len(benchT3State(t, cfg)) != 0 {
		t.Error("StopAll left trackers running or in the state file")
	}
}

func TestBenchT3ConcurrentStartOfOneCodeStartsItOnce(t *testing.T) {
	cfg := benchT3Config(t, "a")
	m := benchT3Manager(t, cfg, &benchT3Messenger{})

	const callers = 32

	results := make(chan error, callers)

	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() { results <- m.Start("a", 1) })
	}

	wg.Wait()
	close(results)

	started := 0
	for err := range results {
		switch {
		case err == nil:
			started++
		case !errors.Is(err, ErrTrackerAlreadyRunning):
			t.Errorf("unexpected Start error: %v", err)
		}
	}

	if started != 1 {
		t.Errorf("%d concurrent Start calls succeeded, want exactly 1", started)
	}
}

func TestBenchT3ConcurrentChangesKeepStateInSync(t *testing.T) {
	const count = 16

	codes := make([]string, count)
	for i := range codes {
		codes[i] = fmt.Sprintf("c%02d", i)
	}

	cfg := benchT3Config(t, codes...)
	m := benchT3Manager(t, cfg, &benchT3Messenger{})

	var wg sync.WaitGroup
	for _, code := range codes {
		wg.Go(func() {
			if err := m.Start(code, 1); err != nil {
				t.Errorf("Start(%q): %v", code, err)
			}
		})
		wg.Go(func() { _ = m.Get(code) })
	}

	wg.Wait()

	if got := benchT3StateCodes(t, cfg); !slices.Equal(got, codes) {
		t.Fatalf("state after concurrent starts = %v, want all %d codes", got, count)
	}

	for _, code := range codes[:count/2] {
		wg.Go(func() { m.Stop(code) })
		wg.Go(func() { _ = m.SetNotificationsPaused(code, true) })
	}

	for _, code := range codes[count/2:] {
		wg.Go(func() {
			if err := m.SetNotificationsPaused(code, true); err != nil {
				t.Errorf("SetNotificationsPaused(%q): %v", code, err)
			}
		})
	}

	wg.Wait()

	state := benchT3State(t, cfg)
	if got := benchT3StateCodes(t, cfg); !slices.Equal(got, codes[count/2:]) {
		t.Fatalf("state after concurrent stops = %v, want %v", got, codes[count/2:])
	}

	for code, s := range state {
		if !s.NotificationsPaused {
			t.Errorf("state lost the paused flag of %q", code)
		}
	}
}

func TestBenchT3ResumeMessagesWithoutHoldingTheLock(t *testing.T) {
	cfg := benchT3Config(t, "a")

	state := `[{"code": "a", "chatId": 5, "interval": "1h0m0s"}, {"code": "gone", "chatId": 5, "interval": "1h0m0s"}]`
	if err := os.WriteFile(cfg.StateFile, []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}

	messenger := &benchT3Messenger{}
	m := NewTrackerManager(messenger, cfg)

	// Deadlocks if the manager sends a message while holding its lock
	messenger.onSend = func() { _ = m.Get("a") }

	done := make(chan struct{})

	go func() {
		defer close(done)
		m.Resume()
	}()

	select {
	case <-done:
	case <-time.After(benchT3Timeout):
		t.Fatal("Resume deadlocked: it called the messenger while holding the manager lock")
	}

	t.Cleanup(m.StopAll)

	if tr := m.Get("a"); tr == nil || tr.ChatID() != 5 {
		t.Errorf("Get(\"a\") after Resume = %v, want a tracker for chat 5", tr)
	}

	if got := benchT3StateCodes(t, cfg); !slices.Equal(got, []string{"a"}) {
		t.Errorf("state after Resume = %v, want [a] (the unknown tracker dropped)", got)
	}

	if texts := messenger.all(); len(texts) != 1 || !strings.Contains(texts[0], "resumed 1 tracker(s): <b>a</b>") {
		t.Errorf("restart messages = %q, want one listing tracker a", texts)
	}
}

func TestBenchT3CommandHandlerUsesTheManager(t *testing.T) {
	cfg := benchT3Config(t, "a")
	messenger := &benchT3Messenger{}
	ch := NewCommandHandler(messenger, cfg)
	t.Cleanup(ch.Trackers().StopAll)

	if err := ch.HandleCommand(1, "/run a", nil, false); err != nil {
		t.Fatalf("/run a: %v", err)
	}

	if ch.Trackers().Get("a") == nil || messenger.last() != "Tracker 'a' has been started" {
		t.Errorf("/run a: running = %v, message = %q", ch.Trackers().Get("a") != nil, messenger.last())
	}

	if err := ch.HandleCommand(1, "/stop a", nil, false); err != nil {
		t.Fatalf("/stop a: %v", err)
	}

	if ch.Trackers().Get("a") != nil || messenger.last() != "Tracker 'a' has been stopped" || len(benchT3State(t, cfg)) != 0 {
		t.Errorf("/stop a: running = %v, message = %q", ch.Trackers().Get("a") != nil, messenger.last())
	}

	if err := ch.HandleCommand(1, "/run unknown", nil, false); err == nil {
		t.Error("/run of an unknown tracker returned no error")
	}

	if want := "Invalid command, tracker with code 'unknown' not found :("; messenger.last() != want {
		t.Errorf("message = %q, want %q", messenger.last(), want)
	}
}

// Guards the hidden tests themselves: the state file must stay the JSON array of savedTracker.
func TestBenchT3StateFileFormat(t *testing.T) {
	cfg := benchT3Config(t, "a")
	m := benchT3Manager(t, cfg, &benchT3Messenger{})

	if err := m.Start("a", 3); err != nil {
		t.Fatalf("Start: %v", err)
	}

	data, err := os.ReadFile(cfg.StateFile)
	if err != nil {
		t.Fatal(err)
	}

	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil || len(raw) != 1 || raw[0]["code"] != "a" || raw[0]["chatId"] != float64(3) {
		t.Errorf("state file = %s, want [{\"code\": \"a\", \"chatId\": 3, ...}]", data)
	}
}
