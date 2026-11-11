package handlers

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"pricetrackerbot/config"
)

func TestStateFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	criteria := []config.NotifyCriteria{{Operator: "<=", Value: "45.5"}}
	noCriteria := []config.NotifyCriteria{}
	saved := []savedTracker{
		{Code: "bonds", Running: true, ChatID: 123, Interval: "1h0m0s"},
		{Code: "laptop", Running: true, ChatID: -100456, NotifyCriteria: &criteria, NotificationsPaused: true},
		{Code: "phone", NotifyCriteria: &noCriteria}, // Stopped, with every criterion removed
	}

	if err := writeStateFile(path, saved); err != nil {
		t.Fatalf("writeStateFile: %v", err)
	}

	loaded, err := readStateFile(path)
	if err != nil {
		t.Fatalf("readStateFile: %v", err)
	}

	if !reflect.DeepEqual(loaded, saved) {
		t.Errorf("loaded %+v, want %+v", loaded, saved)
	}
}

// State files written before settings were saved are a plain list of running trackers.
func TestReadStateFileVersion1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data := `[{"code": "bonds", "chatId": 123, "interval": "1h0m0s", "notificationsPaused": true}]`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := readStateFile(path)
	if err != nil {
		t.Fatalf("readStateFile: %v", err)
	}

	want := []savedTracker{{Code: "bonds", Running: true, ChatID: 123, Interval: "1h0m0s", NotificationsPaused: true}}
	if !reflect.DeepEqual(loaded, want) {
		t.Errorf("loaded %+v, want %+v", loaded, want)
	}
}

func TestReadStateFileUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"version": 99, "trackers": []}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := readStateFile(path); err == nil {
		t.Error("expected an error for a state file from a newer version")
	}
}

func TestReadStateFileMissing(t *testing.T) {
	loaded, err := readStateFile(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || loaded != nil {
		t.Errorf("got %+v, %v; want nil, nil for a missing file", loaded, err)
	}
}

func TestReadStateFileCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := readStateFile(path); err == nil {
		t.Error("expected an error for a corrupt state file")
	}
}

func TestResumeTrackersKeepsSettingsOfStoppedTrackers(t *testing.T) {
	ch, _ := newTestCommandHandler(t)
	state := `{"version": 2, "trackers": [
		{"code": "bonds", "running": false, "interval": "2h0m0s", "notifyCriteria": [{"operator": ">", "value": "7"}]},
		{"code": "removed-from-config", "running": false, "interval": "2h0m0s"},
		{"code": "laptop", "running": true, "chatId": 1, "notifyCriteria": [{"operator": "!=", "value": "7"}]}
	]}`
	if err := os.WriteFile(ch.config.StateFile, []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}

	ch.ResumeTrackers()

	if ch.GetActiveTracker("bonds") != nil {
		t.Error("a stopped tracker was started on resume")
	}

	if ch.GetActiveTracker("laptop") != nil {
		t.Error("a tracker with invalid saved criteria was started on resume")
	}

	settings := ch.trackerSettings("bonds")
	if settings.Interval != 2*time.Hour || settings.NotifyCriteria == nil || (*settings.NotifyCriteria)[0].Value != "7" {
		t.Errorf("settings of the stopped tracker were not restored: %+v", settings)
	}

	data, _ := os.ReadFile(ch.config.StateFile)
	if !strings.Contains(string(data), `"interval": "2h0m0s"`) {
		t.Errorf("state file lost the settings of the stopped tracker: %s", data)
	}

	if strings.Contains(string(data), "removed-from-config") || strings.Contains(string(data), "laptop") {
		t.Errorf("state file still contains trackers that could not be restored: %s", data)
	}
}

func TestNavigationPeekOnEmptyStack(t *testing.T) {
	var state NavigationState
	if state.Peek() != nil || state.Pop() != nil {
		t.Error("expected nil from Peek and Pop on an empty navigation stack")
	}
}
