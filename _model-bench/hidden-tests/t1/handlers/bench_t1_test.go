package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pricetrackerbot/clients"
	"pricetrackerbot/config"
)

// Fails to compile unless both clients implement the fixed interface (TASKS.md T1).
var (
	_ clients.Client  = clients.NewPublicAPIClient()
	_ clients.Client  = clients.NewScraperClient()
	_ TrackerBehavior = (*ClientTrackerBehavior)(nil)
)

type benchT1Client struct {
	result *clients.DataResult
	err    error
	got    *config.Tracker
}

func (c *benchT1Client) FetchAndExtractData(trackerData *config.Tracker) (*clients.DataResult, error) {
	c.got = trackerData

	return c.result, c.err
}

func TestBenchT1BehaviorWrapsClient(t *testing.T) {
	def := &config.Tracker{Code: "x"}
	client := &benchT1Client{result: &clients.DataResult{CurrentValue: 3.14159, NotificationMessage: "Good news"}}

	value, notification, err := NewClientTrackerBehavior(client).Execute(def)
	if err != nil {
		t.Fatalf("Execute: unexpected error %v", err)
	}

	if value != "3.14" || notification != "Good news" {
		t.Errorf("Execute = (%q, %q), want (\"3.14\", \"Good news\")", value, notification)
	}

	if client.got != def {
		t.Error("the tracker definition was not passed to the client")
	}
}

func TestBenchT1BehaviorReturnsClientError(t *testing.T) {
	want := errors.New("fetch failed")

	value, notification, err := NewClientTrackerBehavior(&benchT1Client{err: want}).Execute(&config.Tracker{Code: "x"})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want the client's error", err)
	}

	if value != "" || notification != "" {
		t.Errorf("on error Execute = (%q, %q), want empty strings", value, notification)
	}
}

func TestBenchT1CreateTrackerUsesClientForType(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"price": 5}`))
	})
	mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><span class="price">7,99 €</span></body></html>`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	cfg := &config.Configuration{
		MinInterval:      time.Minute,
		ErrorNotifyLimit: 3,
		APITrackers:      []*config.Tracker{{Code: "a", DataURL: server.URL + "/api", Interval: "1h", DataExtractionPath: "price"}},
		ScraperTrackers:  []*config.Tracker{{Code: "s", DataURL: server.URL + "/page", Interval: "1h", DataExtractionPath: ".price"}},
	}

	// The API client can't parse the HTML page and the scraper finds no ".price" in JSON, so a mixed-up
	// client fails here.
	for code, want := range map[string]string{"a": "5.00", "s": "7.99"} {
		tracker, err := CreateTracker(nil, code, 0, cfg, 1)
		if err != nil {
			t.Fatalf("CreateTracker(%q): %v", code, err)
		}

		if _, ok := tracker.Behavior.(*ClientTrackerBehavior); !ok {
			t.Errorf("tracker %q has behavior %T, want *ClientTrackerBehavior", code, tracker.Behavior)
		}

		value, _, err := tracker.Behavior.Execute(cfg.GetTrackerData(code))
		if err != nil || value != want {
			t.Errorf("tracker %q Execute = (%q, %v), want (%q, nil)", code, value, err, want)
		}
	}
}
