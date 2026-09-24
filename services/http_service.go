package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	requestTimeout  = 20 * time.Second
	maxResponseSize = 10 << 20 // 10 MB
)

// Shared client with a timeout so that a stalled server cannot block a tracker forever.
var httpClient = &http.Client{Timeout: requestTimeout}

func doRequest(url string, requestMethod string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), requestMethod, url, nil)
	if err != nil {
		log.Println("[DoRequest] Error creating request", err)

		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Println("[DoRequest] Error doing request", err)

		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("[DoRequest] Status code is not OK", resp.StatusCode)

		return nil, fmt.Errorf("status code is not OK: %d", resp.StatusCode)
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
