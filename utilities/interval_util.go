package utilities

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Parses durations from a string including support for "d" as days.
//
// Available time units are "m", "h", "d". Anything other than these is not allowed.
//
// Will default to 24 hours when failing to parse a duration ending with "d".
func ParseDurationWithDays(s string) (time.Duration, error) {
	if s == "" {
		return 0, errors.New("empty_interval_value")
	}

	intervalType := extractLetter(s)
	if intervalType != "m" && intervalType != "h" && intervalType != "d" {
		return 0, errors.New("invalid_interval_value")
	}

	if len(s) > 0 && s[len(s)-1] == 'd' {
		// If the duration ends with 'd', parse it as days
		days, err := time.ParseDuration(s[:len(s)-1] + "h")
		if err != nil {
			return 0, err
		}

		return days * 24, nil
	}

	return time.ParseDuration(s)
}

func DurationToString(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}

	return fmt.Sprintf("%ds", seconds)
}

func extractLetter(s string) string {
	// Matches one or more letters
	re := regexp.MustCompile("[a-zA-Z]+")

	return re.FindString(s)
}
