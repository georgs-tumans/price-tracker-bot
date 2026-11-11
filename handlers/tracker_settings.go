package handlers

import (
	"slices"
	"strconv"
	"time"

	"pricetrackerbot/config"
	"pricetrackerbot/utilities"
)

// TrackerSettings are the values a user changed through the bot. They outlive the tracker run (stop, run,
// restart) until the user resets them. Zero values mean "as in the tracker config".
type TrackerSettings struct {
	Interval time.Duration
	// A pointer, because an empty list (every criterion removed) is a change too
	NotifyCriteria *[]config.NotifyCriteria
}

func (s TrackerSettings) isEmpty() bool {
	return s.Interval == 0 && s.NotifyCriteria == nil
}

func (s TrackerSettings) clone() TrackerSettings {
	if s.NotifyCriteria != nil {
		criteria := slices.Clone(*s.NotifyCriteria)
		if criteria == nil {
			criteria = []config.NotifyCriteria{}
		}

		s.NotifyCriteria = &criteria
	}

	return s
}

func (ch *CommandHandler) trackerSettings(code string) TrackerSettings {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	return ch.settings[code].clone()
}

// setSettings stores the changed settings of a configured tracker. Values equal to the config are dropped,
// so that later edits of the tracker config still apply to them.
func (ch *CommandHandler) setSettings(code string, settings TrackerSettings) {
	trackerData := ch.config.GetTrackerData(code)
	if trackerData == nil {
		return
	}

	settings = settings.clone()

	if settings.Interval != 0 {
		// CreateTracker raises it the same way
		settings.Interval = max(settings.Interval, ch.config.MinInterval)
		if configured, err := configuredInterval(trackerData, ch.config.MinInterval); err == nil && configured == settings.Interval {
			settings.Interval = 0
		}
	}

	if settings.NotifyCriteria != nil && criteriaEqual(*settings.NotifyCriteria, trackerData.NotifyCriteria) {
		settings.NotifyCriteria = nil
	}

	ch.mu.Lock()
	defer ch.mu.Unlock()

	if settings.isEmpty() {
		delete(ch.settings, code)
		return
	}

	ch.settings[code] = settings
}

// Returns the interval from the tracker config, raised to the minimum like CreateTracker does.
func configuredInterval(trackerData *config.Tracker, minInterval time.Duration) (time.Duration, error) {
	interval, err := utilities.ParseDurationWithDays(trackerData.Interval)
	if err != nil {
		return 0, err
	}

	return max(interval, minInterval), nil
}

// Whether both lists have the same criteria in the same order; "100" and "100.0" are the same value.
func criteriaEqual(a, b []config.NotifyCriteria) bool {
	return slices.EqualFunc(a, b, func(x, y config.NotifyCriteria) bool {
		if x.Operator != y.Operator {
			return false
		}

		xValue, xErr := strconv.ParseFloat(x.Value, 64)
		yValue, yErr := strconv.ParseFloat(y.Value, 64)
		if xErr != nil || yErr != nil {
			return x.Value == y.Value
		}

		return xValue == yValue
	})
}
