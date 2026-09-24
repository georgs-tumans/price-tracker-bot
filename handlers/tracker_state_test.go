package handlers

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestStateFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	saved := []savedTracker{
		{Code: "bonds", ChatID: 123, Interval: "1h0m0s"},
		{Code: "laptop", ChatID: -100456, Interval: "10m0s"},
	}

	if err := writeStateFile(path, saved); err != nil {
		t.Fatalf("writeStateFile: %v", err)
	}

	loaded, err := readStateFile(path)
	if err != nil {
		t.Fatalf("readStateFile: %v", err)
	}

	if !slices.Equal(loaded, saved) {
		t.Errorf("loaded %+v, want %+v", loaded, saved)
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

func TestNavigationPeekOnEmptyStack(t *testing.T) {
	var state NavigationState
	if state.Peek() != nil || state.Pop() != nil {
		t.Error("expected nil from Peek and Pop on an empty navigation stack")
	}
}
