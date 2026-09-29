package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	// RequestTimeout is shared by all outgoing tracker requests.
	RequestTimeout  = 20 * time.Second
	maxResponseSize = 10 << 20 // 10 MB

	// DefaultUserAgent is sent with every request. Default client user agents (Go-http-client, colly) are
	// blocked outright by some sites.
	DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"

	// Accept headers for API and web page requests; the latter together with AcceptLanguage match what a browser sends.
	AcceptJSON     = "application/json"
	AcceptHTML     = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	AcceptLanguage = "en-US,en;q=0.9"
)

// HTTPStatusError is returned when a server responds with a non-OK status code.
type HTTPStatusError struct {
	StatusCode int
	// How long the server asked to wait before the next request; 0 when not provided
	RetryAfter time.Duration
}

func NewHTTPStatusError(statusCode int, header http.Header) *HTTPStatusError {
	return &HTTPStatusError{StatusCode: statusCode, RetryAfter: parseRetryAfter(header.Get("Retry-After"), time.Now())}
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("status code is not OK: %d", e.StatusCode)
}

// IsBlocking reports whether the status means the server is blocking or rate limiting requests.
func (e *HTTPStatusError) IsBlocking() bool {
	return e.StatusCode == http.StatusForbidden || e.StatusCode == http.StatusTooManyRequests || e.StatusCode == http.StatusServiceUnavailable
}

// Parses a Retry-After header value, which is either a number of seconds or an HTTP date.
func parseRetryAfter(value string, now time.Time) time.Duration {
	if value == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0
		}

		return time.Duration(seconds) * time.Second
	}

	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}

	return 0
}

// Shared client with a timeout so that a stalled server cannot block a tracker forever.
var httpClient = &http.Client{Timeout: RequestTimeout}

func doRequest(url string, requestMethod string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), requestMethod, url, nil)
	if err != nil {
		log.Println("[DoRequest] Error creating request", err)

		return nil, err
	}

	req.Header.Set("Accept", AcceptJSON)
	req.Header.Set("User-Agent", DefaultUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Println("[DoRequest] Error doing request", err)

		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("[DoRequest] Status code is not OK", resp.StatusCode)

		return nil, NewHTTPStatusError(resp.StatusCode, resp.Header)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		log.Println("[DoRequest] Error reading response body", err)

		return nil, err
	}

	return body, nil
}

func GetRequest(url string) ([]byte, error) {
	return doRequest(url, http.MethodGet)
}
