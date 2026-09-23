# Price Tracker Bot - Architecture & Agents Guide

## Overview

A Telegram bot that tracks prices from APIs or scraped websites and notifies users when values meet specified criteria. Built in Go with a modular architecture supporting both API-based and HTML scraping data sources.

---

## Project Structure

```
price-tracker-bot/
├── main.go                    # Entry point, orchestrates bot initialization
├── go.mod / go.sum            # Dependencies (Go 1.23.2)
├── .env                       # Environment variables (must be created for each run)
│
├── config/
│   └── config.go              # Configuration loading, validation, tracker data accessors
│
├── botfixer/                  # Core bot logic and Telegram integration
│   ├── bot_fixer.go           # BotFixer struct, webhook vs long-polling initialization
│   └── udpate.go              # Webhook handler for incoming Telegram updates
│
├── handlers/                  # Command processing and tracker lifecycle management
│   ├── command_handler.go     # All / commands implementation (run, stop, status, interval)
│   ├── tracker.go             # Tracker struct, execution loop, status tracking
│   └── tracker_behavior.go    # Strategy pattern: APITrackerBehavior vs ScraperTrackerBehavior
│
├── clients/                   # Data fetching implementations
│   ├── client.go              # Client interface, ProcessNotificationCriteria
│   ├── api_client.go          # Fetches from JSON APIs using gjson path extraction
│   └── scraper_client.go      # Scrapes HTML using gocolly
│
├── services/                  # Low-level HTTP utilities
│   └── http_service.go        # fasthttp-based GET requests
│
├── helpers/                   # Utility functions
│   ├── messages.go            # Telegram message sending/editing helpers
│   ├── logical_operations.go  # Number comparison logic for criteria evaluation
│   └── string_formatting.go   # String manipulation utilities
│
├── utilities/                 # Duration parsing, string helpers
│   ├── interval_util.go       # Parse "1h", "30m", "2d" duration strings
│   └── string_util.go         # Pointer conversion helpers
│
├── tracker_configs/           # Tracker definitions (JSON files)
│   ├── api_trackers.json      # API-based trackers configuration
│   └── scraper_trackers.json  # Scraper-based trackers configuration
│
├── deployment/                # Docker build/run scripts
└── Dockerfile                 # Multi-stage Docker build
```

---

## Core Architecture Patterns

### 1. Strategy Pattern (TrackerBehavior)
- **Interface**: `Execute(trackerData *config.Tracker, chatID int64) (string, error)`
- **Implementations**: 
  - `APITrackerBehavior` → uses `PublicAPIClient`
  - `ScraperTrackerBehavior` → uses `ScraperClient`
- **Benefit**: Easy to add new data sources without modifying existing code

### 2. Singleton Configuration (`config.Configuration`)
- Loaded once at startup via `GetConfig()`
- Validates all fields using `go-playground/validator/v10`
- Stores both API and scraper tracker definitions separately
- Thread-safe access patterns needed when reading during concurrent operations

### 3. Tracker Lifecycle Management
Each `Tracker` has:
- Context with cancel function for graceful shutdown
- Time.Ticker for periodic execution
- Status tracking (runs, errors, timestamps)
- Error limit counter before user notification

---

## Configuration Structure

### Environment Variables (`.env`)
```bash
PORT=8080                    # Webhook server port (for cloud deployment)
BOT_API_KEY=123456:ABC...    # Telegram Bot API token
WEBHOOK_URL=https://ngrok-url/webhook  # Public URL for webhook mode
ENVIRONMENT=local|cloud      # "local" = long polling, "cloud" = webhooks
TZ=Europe/Riga               # Timezone (default UTC)
ERROR_NOTIFY_LIMIT=3         # Errors before user notification

# Tracker config file paths
API_TRACKERS_FILE=tracker_configs/api_trackers.json
SCRAPER_TRACKERS_FILE=tracker_configs/scraper_trackers.json
```

### Tracker JSON Schema (`api_trackers.json` / `scraper_trackers.json`)
```json
{
  "code": "bonds12m",                    # Unique identifier (no _ or /)
  "dataUrl": "https://...",              # API endpoint or HTML page to scrape
  "viewUrl": "https://...",              # Human-readable link for notifications
  "interval": "6h",                      # Execution frequency ("10m", "1h", "2d")
  "notifyCriteria": [                    # Conditions triggering notifications
    {
      "operator": ">=",                  // <=, <, =, >=, >
      "value": "3.5"                     // Threshold value
    }
  ],
  "dataExtractionPath": "#(period==12).interestRate"  // gjson path for JSON or CSS selector for HTML
}
```

---

## Key Components Explained

### BotFixer (`botfixer/bot_fixer.go`)
- **Dual-mode initialization**: 
  - `InitializeBotLongPolling()` → blocks forever, good for local dev
  - `InitializeBotWebhook()` → HTTP server on PORT, better for production
- **Critical constraint**: Cannot switch between modes with same bot API key (Telegram limitation)
- Uses fasthttp for webhook responses

### CommandHandler (`handlers/command_handler.go`)
- Manages all `/` commands via commandMap
- Maintains navigation stack per chatID for multi-step commands
- Thread-safe tracker registry with mutex protection
- **Commands**:
  - `/run [code]` → start tracker(s)
  - `/stop [code]` → stop tracker(s)
  - `/status [code]` → show status
  - `/interval <code> <duration>` → change frequency
  - `/help` → list all commands

### Tracker (`handlers/tracker.go`)
- Runs in its own goroutine with ticker-driven execution
- Tracks: start time, last run, total runs, current value, errors
- **Gotcha**: `Ticker` must be stopped via `Cancel()` on shutdown to prevent resource leaks

---

## Data Flow

```
User Command → CommandHandler → CreateTracker/StartTracker
                                    ↓
                            Tracker.Ticker (periodic)
                                    ↓
                    TrackerBehavior.Execute()
                    ├─ APITrackerBehavior → PublicAPIClient.FetchAndExtractData()
                    │   └─ services.GetRequest() + gjson extraction
                    └─ ScraperTrackerBehavior → ScraperClient.FetchAndExtractData()
                        └─ gocolly scraping + regex cleaning

Result → ProcessNotificationCriteria() → SendMessageHTML() if criteria met
```

---

## Potential Gotchas & Issues

### 1. **Webhook vs Long Polling Conflict** ⚠️
- Once webhooks are registered for a bot API key, you cannot use long polling
- Solution: Always delete webhooks when switching to local development (`ENVIRONMENT=local`)
- **Never run both modes simultaneously with same bot**

### 2. **JSON Path Extraction Errors**
- `dataExtractionPath` must be valid gjson path (e.g., `"#(period==12).interestRate"`)
- If path doesn't exist → tracker fails silently, logs error
- Test paths against actual API responses before deployment

### 3. **Scraper CSS Selector Issues**
- `dataExtractionPath` is treated as CSS selector for HTML scraping
- Text extraction may include non-numeric characters (requires regex cleaning)
- Website structure changes → scraper breaks silently

### 4. **Error Handling Limitations**

---

## Deployment Checklist

### Local Development (`ENVIRONMENT=local`)
1. Set up `.env` with BOT_API_KEY and ENVIRONMENT=local
2. Run: `go run main.go` or F5 in VS Code
3. Use ngrok if webhook mode is needed for testing
4. Bot uses long polling (no webhooks)

### Production/Cloud (`ENVIRONMENT=cloud`)
1. Set up `.env` with all variables including WEBHOOK_URL and PORT
2. Build Docker image: `docker build -t price_tracker_bot .`
3. Run container: `docker run --env-file .env -p 7080:8080 price_tracker_bot`
4. Ensure webhook URL is publicly accessible (ngrok, cloud provider LB, etc.)

### Docker Multi-stage Build
- **Stage 1**: golang:1.23.2-alpine → builds binary
- **Stage 2**: alpine:latest → minimal runtime with tzdata
- Benefits: Small final image (~15MB), no Go dependencies in production

---

## Testing Recommendations

### Unit Tests to Add
```go
// clients/api_client.go
func TestExtractDataFromPublicAPIResponse(t *testing.T) {
    // Test various gjson paths and data types (string, float64)
}

// handlers/tracker_behavior.go
func TestAPITrackerBehavior_Execute(t *testing.T) {
    // Mock HTTP responses for different scenarios
}

// helpers/messages.go
func TestSendMessageHTML(t *testing.T) {
    // Verify message formatting and entity handling
}
```

### Integration Tests
- Test tracker start/stop lifecycle
- Verify notification messages match criteria evaluation
- Test error accumulation up to ERROR_NOTIFY_LIMIT

---

## Future Enhancements (From todo.txt)

1. **Rate limiting** on tracking frequency to avoid API abuse
2. **Currency symbols** in notifications for better UX
3. **Custom keyboard buttons** for tracker control instead of text commands
4. **Condition UI**: Allow users to set >, <, = conditions via bot interface
5. **Per-tracker command lists** printed on demand
6. **Hot-reload configuration** without restart

---

## Quick Reference: Common Commands

```bash
# Local development
ENVIRONMENT=local go run main.go

# Production Docker build and run
cd deployment && ./build-and-run-docker.sh

# Run specific tracker
/interval bonds 1h    # Set interval to 1 hour
/run bonds            # Start bonds tracker
/status bonds         # Check status
/stop bonds           # Stop bonds tracker
```

---

## Dependencies Summary

| Package | Purpose |
|---------|---------|
| `go-telegram-bot-api/v5` | Telegram Bot API client |
| `gocolly/colly/v2` | HTML scraping with CSS selectors |
| `tidwall/gjson` | JSON path extraction for APIs |
| `valyala/fasthttp` | High-performance HTTP server (webhooks) |
| `go-playground/validator/v10` | Configuration validation |
| `joho/godotenv` | Environment variable loading |

---

## License & Attribution

- Original project: georgs-tumans/price-tracker-bot
- Language: Go 1.23.2
- Primary use case: Latvian bond and commodity price tracking via Telegram
- Errors are logged but not always surfaced to users except after hitting ERROR_NOTIFY_LIMIT
- Failed trackers continue running in background until stopped manually
- No retry logic or exponential backoff implemented

### 5. **Context Cancellation Race Conditions**
- Tracker's `Cancel()` function stops the ticker, but execution may be mid-flight
- Consider adding a "grace period" before fully stopping after cancel is called

### 6. **Configuration Validation Timing**
- Config validation happens only at startup (`GetConfig()`)
- Runtime changes to `.env` or tracker files require restart
- No hot-reload mechanism implemented

### 7. **Memory Leaks with fasthttp**
- `AcquireRequest()`/`ReleaseRequest()` pattern used correctly in services/http_service.go
- Ensure all deferred releases are called, especially on error paths

### 8. **Timezone Handling**
- TZ is set via environment variable but may not apply to all Go runtime functions
- Consider using `time.LoadLocation("TZ")` explicitly for consistency