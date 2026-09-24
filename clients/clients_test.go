package clients

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pricetrackerbot/config"
)

const testPage = `<!DOCTYPE html>
<html>
<head><title>Shop</title><meta property="product:price:amount" content="35.29"></head>
<body>
	<div class="product">
		<h1>Laptop</h1>
		<span class="price">1 234,56 €</span>
	</div>
	<p id="old-price">99.90 EUR</p>
	<table><tr><td class="cell">12.5</td></tr></table>
	<ul class="offers">
		<li>10.00</li>
		<li>20.00</li>
	</ul>
	<span data-price="true">€ 7,99</span>
	<div class="empty"></div>
	<span class="not-a-price">sold out</span>
</body>
</html>`

const testJSON = `{
	"offers": [
		{"period": 6, "interestRate": 2.5},
		{"period": 12, "interestRate": 3.75}
	],
	"price": "19.99",
	"nested": {"value": 42},
	"flag": true
}`

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(testPage))
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testJSON))
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestScraperClient(t *testing.T) {
	server := newTestServer(t)
	client := NewScraperClient()

	tests := []struct {
		selector string
		want     float64
	}{
		{selector: ".product .price", want: 1234.56}, // Spaces, comma decimal separator and currency symbol
		{selector: "#old-price", want: 99.90},
		{selector: "td.cell", want: 12.5},
		{selector: "ul.offers li:nth-child(2)", want: 20},
		{selector: "span[data-price]", want: 7.99},
		{selector: `meta[property="product:price:amount"]`, want: 35.29},
	}

	for _, tt := range tests {
		t.Run(tt.selector, func(t *testing.T) {
			result, err := client.FetchAndExtractData(&config.Tracker{
				Code:               "test",
				DataURL:            server.URL + "/page",
				DataExtractionPath: tt.selector,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !almostEqual(result.CurrentValue, tt.want) {
				t.Errorf("CurrentValue = %v, want %v", result.CurrentValue, tt.want)
			}
		})
	}
}

func TestScraperClientErrors(t *testing.T) {
	server := newTestServer(t)
	client := NewScraperClient()

	tests := []struct {
		name     string
		path     string
		selector string
	}{
		{name: "selector matches nothing", path: "/page", selector: ".does-not-exist"},
		{name: "element is empty", path: "/page", selector: ".empty"},
		{name: "element is not a number", path: "/page", selector: ".not-a-price"},
		{name: "page returns 404", path: "/missing", selector: ".price"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.FetchAndExtractData(&config.Tracker{
				Code:               "test",
				DataURL:            server.URL + tt.path,
				DataExtractionPath: tt.selector,
			})
			if err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestScraperClientNotification(t *testing.T) {
	server := newTestServer(t)

	result, err := NewScraperClient().FetchAndExtractData(&config.Tracker{
		Code:               "laptop",
		DataURL:            server.URL + "/page",
		ViewURL:            "https://example.com/laptop",
		DataExtractionPath: ".product .price",
		NotifyCriteria:     []config.NotifyCriteria{{Operator: "<", Value: "1500"}, {Operator: ">", Value: "2000"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.NotificationMessage, "laptop") || !strings.Contains(result.NotificationMessage, "&lt; 1500") {
		t.Errorf("unexpected notification message: %q", result.NotificationMessage)
	}

	if strings.Contains(result.NotificationMessage, "2000") {
		t.Errorf("notification includes a criterion that is not met: %q", result.NotificationMessage)
	}
}

func TestPublicAPIClient(t *testing.T) {
	server := newTestServer(t)
	client := NewPublicAPIClient()

	tests := []struct {
		path string
		want float64
	}{
		{path: "offers.#(period==12).interestRate", want: 3.75},
		{path: "price", want: 19.99}, // Number stored as a string
		{path: "nested.value", want: 42},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result, err := client.FetchAndExtractData(&config.Tracker{
				Code:               "test",
				DataURL:            server.URL + "/api",
				DataExtractionPath: tt.path,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !almostEqual(result.CurrentValue, tt.want) {
				t.Errorf("CurrentValue = %v, want %v", result.CurrentValue, tt.want)
			}
		})
	}
}

func TestPublicAPIClientErrors(t *testing.T) {
	server := newTestServer(t)
	client := NewPublicAPIClient()

	tests := []struct {
		name string
		url  string
		path string
	}{
		{name: "path not found", url: server.URL + "/api", path: "does.not.exist"},
		{name: "unsupported value type", url: server.URL + "/api", path: "flag"},
		{name: "server returns 404", url: server.URL + "/missing", path: "price"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.FetchAndExtractData(&config.Tracker{Code: "test", DataURL: tt.url, DataExtractionPath: tt.path})
			if err == nil {
				t.Error("expected an error")
			}
		})
	}
}
