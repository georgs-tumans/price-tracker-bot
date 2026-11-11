package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"pricetrackerbot/config"
)

const (
	stateDirPermissions  = 0o700
	stateFilePermissions = 0o600

	// Version 1 was a plain list of running trackers; version 2 also keeps the settings of stopped trackers.
	stateFileVersion = 2
)

// stateFile is the persisted form of the trackers, used to resume them after a restart. The version lets an
// older bot fail to read a newer file (and resume nothing) instead of misreading it.
type stateFile struct {
	Version  int            `json:"version"`
	Trackers []savedTracker `json:"trackers"`
}

// savedTracker is a running tracker, or a stopped one with settings changed by the user.
type savedTracker struct {
	Code    string `json:"code"`
	Running bool   `json:"running"`
	ChatID  int64  `json:"chatId,omitempty"`

	// The changed settings; empty when they are as in the tracker config
	Interval       string                   `json:"interval,omitempty"` // time.Duration string, e.g. "1h0m0s"
	NotifyCriteria *[]config.NotifyCriteria `json:"notifyCriteria,omitempty"`

	NotificationsPaused bool `json:"notificationsPaused,omitempty"`
}

func newSavedTracker(code string, settings TrackerSettings) savedTracker {
	saved := savedTracker{Code: code, NotifyCriteria: settings.clone().NotifyCriteria}
	if settings.Interval != 0 {
		saved.Interval = settings.Interval.String()
	}

	return saved
}

// Returns the changed settings, checked like the tracker config is.
func (s savedTracker) settings() (TrackerSettings, error) {
	var settings TrackerSettings

	if s.Interval != "" {
		interval, err := time.ParseDuration(s.Interval)
		if err != nil {
			return settings, fmt.Errorf("invalid interval '%s'", s.Interval)
		}

		settings.Interval = interval
	}

	if s.NotifyCriteria != nil {
		if err := config.ValidateNotifyCriteria(*s.NotifyCriteria); err != nil {
			return settings, fmt.Errorf("invalid notify criteria: %w", err)
		}

		settings.NotifyCriteria = s.NotifyCriteria
	}

	return settings.clone(), nil
}

// saveState writes the running trackers and the changed settings to the state file. Errors are logged,
// not returned: failing to persist state should not fail the command that triggered it.
func (ch *CommandHandler) saveState() {
	ch.mu.Lock()
	saved := make([]savedTracker, 0, len(ch.runningTrackers)+len(ch.settings))
	running := make(map[string]bool, len(ch.runningTrackers))

	for _, tracker := range ch.runningTrackers {
		entry := newSavedTracker(tracker.Code, ch.settings[tracker.Code])
		entry.Running = true
		entry.ChatID = tracker.ChatID()
		entry.NotificationsPaused = tracker.Status().NotificationsPaused
		saved = append(saved, entry)
		running[tracker.Code] = true
	}

	for code, settings := range ch.settings {
		if !running[code] {
			saved = append(saved, newSavedTracker(code, settings))
		}
	}
	ch.mu.Unlock()

	// A stable order keeps the file readable and diffable
	slices.SortFunc(saved, func(a, b savedTracker) int { return strings.Compare(a.Code, b.Code) })

	if err := writeStateFile(ch.config.StateFile, saved); err != nil {
		log.Printf("[State] Failed to save tracker state to %s: %s", ch.config.StateFile, err)
	}
}

// Writes to a temporary file first and renames it, so a crash mid-write cannot leave a corrupt state file.
func writeStateFile(path string, saved []savedTracker) error {
	data, err := json.MarshalIndent(stateFile{Version: stateFileVersion, Trackers: saved}, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), stateDirPermissions); err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, stateFilePermissions); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func readStateFile(path string) ([]savedTracker, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	// Version 1: a list of running trackers, each with its current interval
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")) {
		var saved []savedTracker
		if err := json.Unmarshal(data, &saved); err != nil {
			return nil, fmt.Errorf("invalid state file: %w", err)
		}

		for i := range saved {
			saved[i].Running = true
		}

		return saved, nil
	}

	var state stateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("invalid state file: %w", err)
	}

	if state.Version != stateFileVersion {
		return nil, fmt.Errorf("unsupported state file version %d", state.Version)
	}

	return state.Trackers, nil
}

// ResumeTrackers restores the changed settings, starts the trackers that were running before the bot was
// restarted and lets each affected chat know about the restart.
func (ch *CommandHandler) ResumeTrackers() {
	saved, err := readStateFile(ch.config.StateFile)
	if err != nil {
		log.Printf("[State] Failed to read tracker state from %s: %s", ch.config.StateFile, err)
		return
	}

	resumedPerChat := make(map[int64][]string)

	for _, s := range saved {
		if ch.config.GetTrackerData(s.Code) == nil {
			log.Printf("[State] Dropping tracker '%s': it is no longer in the configuration", s.Code)
			continue
		}

		settings, err := s.settings()
		if err != nil {
			log.Printf("[State] Dropping tracker '%s': %s", s.Code, err)
			continue
		}

		// A version 1 interval is the one the tracker ran with; setSettings drops it if it's the configured one
		ch.setSettings(s.Code, settings)

		if !s.Running {
			continue
		}

		if !ch.config.IsChatAllowed(s.ChatID) {
			log.Printf("[State] Not resuming tracker '%s': chat %d is not allowed", s.Code, s.ChatID)
			continue
		}

		if ch.GetActiveTracker(s.Code) != nil {
			continue
		}

		tracker, err := CreateTracker(ch.messenger, s.Code, ch.trackerSettings(s.Code), ch.config, s.ChatID)
		if err != nil {
			log.Printf("[State] Not resuming tracker '%s': %s", s.Code, err)
			continue
		}

		tracker.SetNotificationsPaused(s.NotificationsPaused)
		ch.AddRunningTracker(tracker)
		tracker.Start()
		resumedPerChat[s.ChatID] = append(resumedPerChat[s.ChatID], s.Code)
		log.Printf("[State] Resumed tracker '%s' for chat %d with interval %s", s.Code, s.ChatID, tracker.Status().CurrentInterval)
	}

	// Drops trackers that could not be restored (e.g. removed from the configuration)
	ch.saveState()

	for chatID, codes := range resumedPerChat {
		slices.Sort(codes)
		message := fmt.Sprintf("The bot was restarted; resumed %d tracker(s): <b>%s</b>", len(codes), strings.Join(codes, ", "))
		ch.messenger.SendHTML(chatID, message)
	}
}
