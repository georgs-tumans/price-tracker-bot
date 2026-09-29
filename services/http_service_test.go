package services

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "missing", value: "", want: 0},
		{name: "seconds", value: "120", want: 2 * time.Minute},
		{name: "negative seconds", value: "-5", want: 0},
		{name: "HTTP date", value: now.Add(90 * time.Second).Format(http.TimeFormat), want: 90 * time.Second},
		{name: "HTTP date in the past", value: now.Add(-time.Hour).Format(http.TimeFormat), want: 0},
		{name: "garbage", value: "soon", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRetryAfter(tt.value, now); got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %s, want %s", tt.value, got, tt.want)
			}
		})
	}
}

func TestHTTPStatusErrorIsBlocking(t *testing.T) {
	for code, want := range map[int]bool{403: true, 429: true, 503: true, 404: false, 500: false} {
		if got := (&HTTPStatusError{StatusCode: code}).IsBlocking(); got != want {
			t.Errorf("IsBlocking() for %d = %v, want %v", code, got, want)
		}
	}
}
