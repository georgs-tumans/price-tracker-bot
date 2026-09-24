package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseChatIDs(t *testing.T) {
	tests := []struct {
		input   string
		want    []int64
		wantErr bool
	}{
		{input: "", want: nil},
		{input: "12345678", want: []int64{12345678}},
		{input: " 123, -100987654321 ,, 456 ", want: []int64{123, -100987654321, 456}},
		{input: "123,abc", wantErr: true},
	}

	for _, tt := range tests {
		got, err := parseChatIDs(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseChatIDs(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}

		if !slices.Equal(got, tt.want) {
			t.Errorf("parseChatIDs(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsChatAllowed(t *testing.T) {
	open := &Configuration{}
	if !open.IsChatAllowed(42) {
		t.Error("expected every chat to be allowed when no allowed chats are configured")
	}

	restricted := &Configuration{AllowedChatIDs: []int64{1, 2}}
	if !restricted.IsChatAllowed(2) {
		t.Error("expected chat 2 to be allowed")
	}

	if restricted.IsChatAllowed(3) {
		t.Error("expected chat 3 to be rejected")
	}
}

func TestLoadConfig(t *testing.T) {
	trackersFile := filepath.Join(t.TempDir(), "trackers.json")
	trackers := `[{"code": "bonds", "dataUrl": "https://example.com/api", "interval": "1h", "dataExtractionPath": "price"}]`
	if err := os.WriteFile(trackersFile, []byte(trackers), 0o600); err != nil {
		t.Fatal(err)
	}

	setEnv := func(t *testing.T, values map[string]string) {
		t.Helper()
		for _, key := range []string{"BOT_API_KEY", "STATE_FILE", "ERROR_NOTIFY_LIMIT", "ALLOWED_CHAT_IDS", "API_TRACKERS_FILE", "SCRAPER_TRACKERS_FILE"} {
			t.Setenv(key, values[key])
		}
	}

	t.Run("defaults", func(t *testing.T) {
		setEnv(t, map[string]string{"BOT_API_KEY": "key", "API_TRACKERS_FILE": trackersFile})

		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.StateFile != defaultStateFile || cfg.ErrorNotifyLimit != defaultErrorNotifyLimit || len(cfg.AllowedChatIDs) != 0 {
			t.Errorf("unexpected defaults: %+v", cfg)
		}

		if len(cfg.APITrackers) != 1 || cfg.APITrackers[0].Code != "bonds" {
			t.Errorf("trackers not loaded: %+v", cfg.APITrackers)
		}
	})

	t.Run("explicit values", func(t *testing.T) {
		setEnv(t, map[string]string{
			"BOT_API_KEY": "key", "API_TRACKERS_FILE": trackersFile,
			"STATE_FILE": "/data/state.json", "ERROR_NOTIFY_LIMIT": "5", "ALLOWED_CHAT_IDS": "1,2",
		})

		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.StateFile != "/data/state.json" || cfg.ErrorNotifyLimit != 5 || !slices.Equal(cfg.AllowedChatIDs, []int64{1, 2}) {
			t.Errorf("unexpected values: %+v", cfg)
		}
	})

	errorCases := map[string]map[string]string{
		"invalid error limit": {"API_TRACKERS_FILE": trackersFile, "ERROR_NOTIFY_LIMIT": "three"},
		"invalid chat ID":     {"API_TRACKERS_FILE": trackersFile, "ALLOWED_CHAT_IDS": "abc"},
		"missing file":        {"API_TRACKERS_FILE": filepath.Join(t.TempDir(), "missing.json")},
		"no trackers":         {},
	}

	for name, values := range errorCases {
		t.Run(name, func(t *testing.T) {
			setEnv(t, values)

			if _, err := loadConfig(); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
