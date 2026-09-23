# Architecture Overview

The project is a Telegram bot designed to track prices from various sources (APIs and web scrapers). It follows a modular architecture that separates configuration, bot initialization, tracking logic, and data fetching.

## Key Components

- **Bot Initialization (`botfixer`)**: Handles the setup of the Telegram Bot API client. Supports both **Long Polling** (for local development) and **Webhooks** (for production).
- **Configuration (`config`)**: Centralized configuration management using `.env` files. Trackers are defined in JSON files (`api_trackers.json`, `scraper_trackers.json`), allowing for easy extension without code changes.
- **Tracker Management (`handlers/tracker.go`)**: Each tracker runs in its own goroutine with a dedicated ticker. It manages the lifecycle (Start, Stop, UpdateInterval) and maintains state like execution history and error counts.
- **Behavioral Abstraction (`handlers/tracker_behavior.go`)**: Uses an interface to decouple *what* a tracker does from *how* it fetches data. This allows for easy addition of new fetching methods (e.g., adding a "Database" or "GraphQL" behavior).
- **Clients (`clients/`)**: Low-level implementations for interacting with external APIs and performing web scraping.

## Architecture Facts

1.  **Concurrency Model**: Independent goroutines per tracker ensure that a slow scraper doesn't block other trackers from running on time.
2.  **Extensibility**: New tracking types can be added by implementing the `TrackerBehavior` interface and updating the configuration files.
3.  **Environment Agnostic**: The dual-mode initialization (Long Polling vs Webhook) makes it easy to move from a local machine to a production server.
4.  **Config-Driven**: All tracker definitions live in JSON files, enabling hot-swappable configurations without recompilation.
5.  **Graceful Shutdown**: Each tracker maintains its own `context.Context` which can be cancelled when stopping or updating intervals.
6.  **Message Helpers**: Centralized Telegram operations (send/edit/delete) in `helpers/messages.go` keep business logic clean.

## File Structure

```
pricetrackerbot/
├── main.go                          # Entry point with panic recovery
├── agents.md                        # This documentation
├── .env.example                     # Environment variable template
├── .github/workflows/*.yml          # CI/CD: CodeQL, deploy, devskim, golangci-lint, trivy
├── deployment/build-and-run-docker.sh
├── botfixer/
│   └── bot_fixer.go                 # Bot initialization (long polling / webhooks)
├── config/
│   └── config.go                    # Configuration singleton with validation
├── handlers/
│   ├── command_handler.go           # /start, /stop, /status, /help, /interval, /run commands
│   ├── tracker_behavior.go          # TrackerBehavior interface + implementations
│   └── tracker.go                   # Tracker lifecycle (Start/Stop/UpdateInterval)
├── clients/
│   ├── api_client.go                # PublicAPIClient using gjson for JSON extraction
│   ├── scraper_client.go            # ScraperClient with multiple source support
│   └── client.go                    # DataResult, Client interface, ProcessNotificationCriteria
├── helpers/
│   └── messages.go                  # Telegram message operations
├── utilities/
│   └── interval_util.go             # Duration parsing (m/h/d units)
├── tracker_configs/
│   ├── api_trackers.json            # 5 API trackers: bonds12m, bonds6m, bitcoin, goldSpot, silverSpot
│   └── scraper_trackers.json        # 6 scraper trackers: BENU medicines + pharmacy sites
```

## CI/CD Workflows

- **CodeQL Advanced**: Runs on `develop` pushes/PRs and scheduled Thu 00:35 UTC. Analyzes Go code for security vulnerabilities.
- **Deploy to Hetzner**: Deploys via SSH to production server.
- **DevSecOps Scanning**: GitHub DevSecOps Security (devskim) action.
- **golangci-lint**: Static code analysis.
- **Trivy**: Container vulnerability scanning for Docker images.

## Security & Operations

- **Containerization**: Docker script (`deployment/build-and-run-docker.sh`) enables reproducible deployments.
- **Vulnerability Scanning**: Trivy integrated in CI; also run via `trivy image <image>` post-build.
- **Environment Variables**: All secrets/config via `.env` (BOT_API_KEY, WEBHOOK_URL, ENVIRONMENT, TZ).
- **Test Coverage**: Currently 0% - tests would be added to individual packages (`*_test.go`).

## Gotchas & Notes

- **Error Notification Threshold**: Notifications for tracker errors are only sent once the `errorLimit` is reached. This prevents spam but might delay awareness of persistent issues (e.g., if limit=10 and first 9 errors occur silently).
- **Context Lifecycle**: When updating a tracker's interval, the context and ticker are recreated. Ensure any long-running operations within behaviors respect the context cancellation.
- **Hardcoded Webhook Path**: The `/webhook` endpoint is currently hardcoded in `botfixer/bot_fixer.go`.
- **JSON Configuration**: Trackers must have unique codes across both API and Scraper configurations to avoid collisions during lookup.
- **Interval Format**: Duration strings use `m`, `h`, `d` suffixes (e.g., `"15m"`, `"1h"`, `"3d"`). Days parsed with "d" suffix get a 24-hour fallback if malformed.

## Gotchas & Notes

- **Error Notification Threshold**: Notifications for tracker errors are only sent once the `errorLimit` is reached. This is intended to prevent spam but might delay awareness of persistent issues.
- **Context Lifecycle**: When updating a tracker's interval, the context and ticker are recreated. Ensure any long-running operations within behaviors respect the context cancellation.
- **Hardcoded Webhook Path**: The `/webhook` endpoint is currently hardcoded in `botfixer/bot_fixer.go`.
- **JSON Configuration**: Trackers must have unique codes across both API and Scraper configurations to avoid collisions during lookup.
