package handlers

import (
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

	"pricetrackerbot/helpers"
)

const (
	stateDirPermissions  = 0o700
	stateFilePermissions = 0o600
)

// savedTracker is the persisted form of a running tracker, used to resume trackers after a restart.
type savedTracker struct {
	Code     string `json:"code"`
	ChatID   int64  `json:"chatId"`
	Interval string `json:"interval"` // time.Duration string, e.g. "1h0m0s"
}

// saveState writes the currently running trackers to the state file. Errors are logged, not returned:
// failing to persist state should not fail the command that triggered it.
func (ch *CommandHandler) saveState() {
	ch.mu.Lock()
	saved := make([]savedTracker, 0, len(ch.runningTrackers))
	for _, tracker := range ch.runningTrackers {
		saved = append(saved, savedTracker{
			Code:     tracker.Code,
			ChatID:   tracker.ChatID(),
			Interval: tracker.Status().CurrentInterval.String(),
		})
	}
	ch.mu.Unlock()

	if err := writeStateFile(ch.config.StateFile, saved); err != nil {
		log.Printf("[State] Failed to save tracker state to %s: %s", ch.config.StateFile, err)
	}
}

// Writes to a temporary file first and renames it, so a crash mid-write cannot leave a corrupt state file.
func writeStateFile(path string, saved []savedTracker) error {
	data, err := json.MarshalIndent(saved, "", "  ")
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

	var saved []savedTracker
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("invalid state file: %w", err)
	}

	return saved, nil
}

// ResumeTrackers starts the trackers that were running before the bot was restarted
// and lets each affected chat know about the restart.
func (ch *CommandHandler) ResumeTrackers() {
	saved, err := readStateFile(ch.config.StateFile)
	if err != nil {
		log.Printf("[State] Failed to read tracker state from %s: %s", ch.config.StateFile, err)
		return
	}

	resumedPerChat := make(map[int64][]string)

	for _, s := range saved {
		if !ch.config.IsChatAllowed(s.ChatID) {
			log.Printf("[State] Not resuming tracker '%s': chat %d is not allowed", s.Code, s.ChatID)
			continue
		}

		if ch.GetActiveTracker(s.Code) != nil {
			continue
		}

		interval, err := time.ParseDuration(s.Interval)
		if err != nil {
			log.Printf("[State] Not resuming tracker '%s': invalid interval '%s'", s.Code, s.Interval)
			continue
		}

		tracker, err := CreateTracker(ch.bot, s.Code, interval, ch.config, s.ChatID)
		if err != nil {
			log.Printf("[State] Not resuming tracker '%s': %s", s.Code, err)
			continue
		}

		ch.AddRunningTracker(tracker)
		tracker.Start()
		resumedPerChat[s.ChatID] = append(resumedPerChat[s.ChatID], s.Code)
		log.Printf("[State] Resumed tracker '%s' for chat %d with interval %s", s.Code, s.ChatID, interval)
	}

	// Drops trackers that could not be resumed (e.g. removed from the configuration)
	ch.saveState()

	for chatID, codes := range resumedPerChat {
		slices.Sort(codes)
		message := fmt.Sprintf("The bot was restarted; resumed %d tracker(s): <b>%s</b>", len(codes), strings.Join(codes, ", "))
		helpers.SendMessageHTML(ch.bot, chatID, message, nil)
	}
}
