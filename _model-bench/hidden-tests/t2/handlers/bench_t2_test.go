package handlers

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"pricetrackerbot/config"
)

const benchT2Type config.TrackerType = "bench-fake"

type benchT2Behavior struct{}

func (benchT2Behavior) Execute(_ *config.Tracker) (string, string, error) { return "1.00", "", nil }

type benchT2Messenger struct {
	mu    sync.Mutex
	texts []string
}

func (m *benchT2Messenger) add(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.texts = append(m.texts, text)
}

func (m *benchT2Messenger) last() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.texts) == 0 {
		return ""
	}

	return m.texts[len(m.texts)-1]
}

func (m *benchT2Messenger) SendHTML(_ int64, text string) { m.add(text) }
func (m *benchT2Messenger) SendHTMLWithMenu(_ int64, text string, _ *models.InlineKeyboardMarkup) {
	m.add(text)
}
func (m *benchT2Messenger) SendHTMLWithKeyboard(_ int64, text string, _ *models.ReplyKeyboardMarkup) {
	m.add(text)
}
func (m *benchT2Messenger) EditHTMLWithMenu(_ int64, _ int, text string, _ *models.InlineKeyboardMarkup) {
	m.add(text)
}
func (m *benchT2Messenger) EditMenu(_ int64, _ int, _ *models.InlineKeyboardMarkup) {}
func (m *benchT2Messenger) RemoveKeyboard(_ int64)                                  {}

func benchT2Config(t *testing.T, trackers ...*config.Tracker) *config.Configuration {
	t.Helper()

	return &config.Configuration{
		MinInterval:      time.Minute,
		ErrorNotifyLimit: 1000,
		StateFile:        filepath.Join(t.TempDir(), "state.json"),
		Trackers:         trackers,
	}
}

func benchT2Def(code string, typ config.TrackerType) *config.Tracker {
	return &config.Tracker{Code: code, Type: typ, DataURL: "https://example.com/" + code, Interval: "1h", DataExtractionPath: "price"}
}

func TestBenchT2BuiltInTypesAreRegistered(t *testing.T) {
	for _, typ := range []config.TrackerType{config.TrackerTypeAPI, config.TrackerTypeScraper} {
		factory, ok := trackerBehaviors[typ]
		if !ok || factory == nil || factory() == nil {
			t.Errorf("no behavior registered for tracker type %q", typ)
		}
	}
}

func TestBenchT2NewTypeNeedsOnlyARegistration(t *testing.T) {
	trackerBehaviors[benchT2Type] = func() TrackerBehavior { return benchT2Behavior{} }
	t.Cleanup(func() { delete(trackerBehaviors, benchT2Type) })

	cfg := benchT2Config(t, benchT2Def("f1", benchT2Type), benchT2Def("f2", benchT2Type))

	tracker, err := CreateTracker(nil, "f1", 0, cfg, 1)
	if err != nil {
		t.Fatalf("CreateTracker for a registered type: %v", err)
	}

	if _, ok := tracker.Behavior.(benchT2Behavior); !ok {
		t.Errorf("behavior = %T, want the registered one", tracker.Behavior)
	}

	messenger := &benchT2Messenger{}
	ch := NewCommandHandler(messenger, cfg)
	t.Cleanup(ch.StopAllTrackers)

	if err := ch.HandleCommand(1, "/run", nil, false); err != nil {
		t.Fatalf("/run: %v", err)
	}

	for _, code := range []string{"f1", "f2"} {
		if ch.GetActiveTracker(code) == nil {
			t.Errorf("/run did not start tracker %q of the new type", code)
		}
	}

	if err := ch.HandleCommand(1, "/status", nil, false); err != nil {
		t.Fatalf("/status: %v", err)
	}

	if status := messenger.last(); !strings.Contains(status, " - f1 | active | bench-fake\n") {
		t.Errorf("status overview doesn't show the new type:\n%s", status)
	}
}

func TestBenchT2UnregisteredTypeFails(t *testing.T) {
	cfg := benchT2Config(t, benchT2Def("u", "no-such-type"))

	if _, err := CreateTracker(nil, "u", 0, cfg, 1); err == nil {
		t.Error("CreateTracker accepted a tracker type without a registered behavior")
	}
}

func TestBenchT2StatusOverviewIsUnchanged(t *testing.T) {
	cfg := benchT2Config(t,
		benchT2Def("a1", config.TrackerTypeAPI),
		benchT2Def("a2", config.TrackerTypeAPI),
		benchT2Def("s1", config.TrackerTypeScraper),
	)

	messenger := &benchT2Messenger{}
	ch := NewCommandHandler(messenger, cfg)

	if err := ch.HandleCommand(1, "/status", nil, false); err != nil {
		t.Fatalf("/status: %v", err)
	}

	want := "<b>All available trackers</b>\n\n - a1 | inactive | api\n - a2 | inactive | api\n - s1 | inactive | scraper\n"
	if got := messenger.last(); got != want {
		t.Errorf("status overview =\n%q\nwant\n%q", got, want)
	}
}
