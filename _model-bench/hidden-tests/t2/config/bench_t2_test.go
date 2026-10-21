package config

import (
	"os"
	"path/filepath"
	"testing"
)

func benchT2Files(t *testing.T, apiJSON, scraperJSON string) {
	t.Helper()

	dir := t.TempDir()
	values := map[string]string{"BOT_API_KEY": "key"}

	for envVar, content := range map[string]string{"API_TRACKERS_FILE": apiJSON, "SCRAPER_TRACKERS_FILE": scraperJSON} {
		if content == "" {
			continue
		}

		path := filepath.Join(dir, envVar+".json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		values[envVar] = path
	}

	for _, key := range []string{"BOT_API_KEY", "STATE_FILE", "ERROR_NOTIFY_LIMIT", "MIN_INTERVAL", "ALLOWED_CHAT_IDS", "API_TRACKERS_FILE", "SCRAPER_TRACKERS_FILE"} {
		t.Setenv(key, values[key])
	}
}

func benchT2Tracker(code, extra string) string {
	return `{"code": "` + code + `", "dataUrl": "https://example.com/` + code + `", "interval": "1h", "dataExtractionPath": "price"` + extra + `}`
}

func TestBenchT2LoadSetsTypeFromFileInOrder(t *testing.T) {
	// The "type" key in the API file must be ignored: the type comes from the file, never from JSON.
	benchT2Files(t,
		"["+benchT2Tracker("a1", `, "type": "scraper"`)+", "+benchT2Tracker("a2", "")+"]",
		"["+benchT2Tracker("s1", `, "type": "api"`)+"]",
	)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	want := []struct {
		code string
		typ  TrackerType
	}{{"a1", TrackerTypeAPI}, {"a2", TrackerTypeAPI}, {"s1", TrackerTypeScraper}}

	if len(cfg.Trackers) != len(want) {
		t.Fatalf("loaded %d trackers, want %d", len(cfg.Trackers), len(want))
	}

	for i, w := range want {
		if got := cfg.Trackers[i]; got.Code != w.code || got.Type != w.typ {
			t.Errorf("Trackers[%d] = %s/%s, want %s/%s", i, got.Code, got.Type, w.code, w.typ)
		}
	}

	if got := cfg.GetTrackerData("s1"); got == nil || got.Type != TrackerTypeScraper {
		t.Errorf("GetTrackerData(\"s1\") = %+v, want the scraper tracker", got)
	}

	if got := cfg.GetTrackerData("missing"); got != nil {
		t.Errorf("GetTrackerData(\"missing\") = %+v, want nil", got)
	}
}

func TestBenchT2LoadOnlyScraperFile(t *testing.T) {
	benchT2Files(t, "", "["+benchT2Tracker("s1", "")+"]")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	if len(cfg.Trackers) != 1 || cfg.Trackers[0].Type != TrackerTypeScraper {
		t.Errorf("Trackers = %+v, want one scraper tracker", cfg.Trackers)
	}
}

func TestBenchT2DuplicateCodesAreRejected(t *testing.T) {
	tests := map[string]struct{ api, scraper string }{
		"across files":         {"[" + benchT2Tracker("x", "") + "]", "[" + benchT2Tracker("x", "") + "]"},
		"within the API file":  {"[" + benchT2Tracker("x", "") + ", " + benchT2Tracker("x", "") + "]", ""},
		"within scraper file":  {"[" + benchT2Tracker("a", "") + "]", "[" + benchT2Tracker("x", "") + ", " + benchT2Tracker("x", "") + "]"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			benchT2Files(t, tt.api, tt.scraper)

			if _, err := loadConfig(); err == nil {
				t.Error("loadConfig accepted a duplicate tracker code")
			}
		})
	}

	// Distinct codes in both files still load.
	benchT2Files(t, "["+benchT2Tracker("a", "")+"]", "["+benchT2Tracker("s", "")+"]")

	if _, err := loadConfig(); err != nil {
		t.Errorf("loadConfig rejected distinct codes: %v", err)
	}
}
