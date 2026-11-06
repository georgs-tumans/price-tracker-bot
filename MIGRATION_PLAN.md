# 2026 Maintenance Migration

**Date:** September 2026
**Branch:** `maintenance-plan`

This document explains what was changed in the 2026 maintenance round, why, and what still has to be done by hand before the new version runs in production.

---

## Contents

1. [Summary](#1-summary)
2. [To-do before and after deploying](#2-to-do-before-and-after-deploying)
3. [Why the bot stopped working after a few days](#3-why-the-bot-stopped-working-after-a-few-days)
4. [What changed](#4-what-changed)
5. [Configuration changes](#5-configuration-changes)
6. [Decisions and things left out](#6-decisions-and-things-left-out)
7. [Open items](#7-open-items)
8. [Updating dependencies in the future](#8-updating-dependencies-in-the-future)
9. [Commit history](#9-commit-history)

---

## 1. Summary

The bot was written a couple of years ago and had three kinds of problems:

- **It stopped working after a couple of days**, with no error anywhere. There wasn't one bug behind this but several, and any of them ended in a silently dead bot (section 3).
- **The build and deploy setup was out of date:** the Docker image and CI used an older Go than `go.mod`, the linter config was for an old golangci-lint version, and a deploy workflow still targeted a Hetzner server that's no longer used.
- **Dependencies were old.** The Telegram library hadn't had a release since December 2021, and the Docker image had 19 known high-severity vulnerabilities in its Go packages.

All of this is fixed; the few things still open are listed in [section 7](#7-open-items). The biggest changes at a glance:

| Area | Before | After |
|---|---|---|
| Receiving updates | Webhooks (needs a public HTTPS address) | Long polling (outgoing connections only) |
| Network timeouts | None: a dropped connection could hang the bot forever | Every request has a timeout |
| Running trackers after a restart | Lost silently | Saved and resumed, with a message to the chat |
| Container restart after a crash or reboot | No | Yes (`restart: unless-stopped`) |
| Who can use the bot | Anyone on Telegram | Only chats in `ALLOWED_CHAT_IDS` |
| Telegram library | `go-telegram-bot-api/v5` (2021) | `github.com/go-telegram/bot` v1.27.0 |
| Docker image | Alpine, running as root, ~30 MB | `scratch`, non-root, read-only, ~16 MB |
| Deployment | Image built on the server with manual `docker run` scripts | Released images pulled from `ghcr.io`; `./deployment/update.sh` / `start.sh` / `stop.sh` (Docker Compose) |
| Releases | None | Publishing a GitHub release builds, scans and publishes the image |
| Known vulnerabilities (Trivy, HIGH+) | 19 | 0 |
| Tests | None | Unit tests plus end-to-end tests against a fake Telegram server |

---

## 2. To-do before and after deploying

These steps can't be done from the code. Do them in order and tick them off.

### Before the first deploy

- [ ] **Create a dev bot.** In Telegram, message [@BotFather](https://t.me/BotFather), send `/newbot`, and pick a name and username (e.g. `Price Tracker (dev)` / `my_price_tracker_dev_bot`). Put its API key in your **local** `.env` as `BOT_API_KEY`. From now on, the production key only lives in the server's `.env`. ([Why](#running-locally-next-to-production))
- [ ] **Test locally with the dev bot** (`go run .` or F5): every command and button, "<< Return" after restarting the bot, the interval change, and a tracker being resumed after a restart (you should get a "the bot was restarted" message).
- [ ] **Run the dev bot while production is running** (once the new version is deployed, or next to the old one) and check that neither log shows `Conflict` errors and each bot only answers its own chat.
- [ ] *(Optional)* **Run a local session with the race detector** for about an hour, with a tracker on a 1-minute interval, clicking through the menus: `go run -race .`. It reports any unsafe concurrent access the automated tests didn't hit. On Windows the race detector needs a C compiler (e.g. via MSYS2), so it's easier on the Ubuntu server or in WSL.
- [ ] **Run each of your real trackers once** with the dev bot (copy the real `tracker_configs/*.json` from the server) and compare the values with what the production bot showed. The scraping library upgrade is covered by tests, but only with sample HTML.
- [ ] **Update your local golangci-lint.** The installed one was built with Go 1.26 and can't lint Go 1.27 code:
  ```bash
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
  ```
- [ ] **Review and merge** the `maintenance-plan` branch into `develop`, and into `main` (or whichever branch you release from and the server pulls).
- [ ] **Publish the first release** (e.g. `v2.0.0`): GitHub → Releases → Draft a new release → new tag → Publish. The server can't pull anything until this exists. Check that the "Release image" workflow succeeds in the Actions tab.
- [ ] **Make the image public:** after the first release, open the package (your GitHub profile → Packages → `price-tracker-bot` → Package settings), set **Change visibility → Public**, and check that it's linked to this repository. Otherwise the server would need to log in to ghcr.io to pull it.
- [ ] **Delete the old package** `price-tracker-bot/price_tracker_bot` from January 2025 on the same Packages page, so there's only one image and nobody pulls the outdated one.
- [ ] *(Optional)* **Look at why the old version died** before replacing it: `docker logs --since 72h price_tracker_bot`, `docker inspect -f '{{.RestartCount}} {{.State.StartedAt}}' price_tracker_bot`, and `curl -s "https://api.telegram.org/bot<TOKEN>/getWebhookInfo"`.

### On the server

- [ ] **Edit the server's `.env`:**
  - Delete `WEBHOOK_URL`, `PORT` and `ENVIRONMENT` (and the misspelled `ENVIROMENT`, if it's there). They're no longer used.
  - Don't set `STATE_FILE`; the Docker image already points it at the `/data` volume.
  - Add `ALLOWED_CHAT_IDS=0` for now (see "Find your chat ID" below).
- [ ] **Make sure Docker starts on boot** and the compose plugin is installed:
  ```bash
  sudo systemctl enable docker
  docker compose version
  ```
- [ ] **Check the tracker configs are in `tracker_configs/`** in the server's checkout. They're now mounted into the container instead of being built into the image, so the same files keep working.
- [ ] **Deploy** (the checkout should be on the branch you release from, e.g. `main`):
  ```bash
  git pull
  ./deployment/update.sh
  docker logs -f price_tracker_bot
  ```
  The first run removes the old container created by the old scripts. The log should show `Starting bot service (version v2.0.0)` (or whichever version you released), `Webhook deleted` and then `Bot initialized via long polling`, with no `Conflict` errors.
- [ ] **Find your chat ID(s).** With `ALLOWED_CHAT_IDS=0`, send the bot any message. The log shows:
  ```
  [Bot fixer] Ignoring update from chat 123456789 (not in ALLOWED_CHAT_IDS)
  ```
  That number is your chat ID. Group chats have negative IDs; message the bot from each chat you want to use. Put the IDs in `.env`, comma separated (e.g. `ALLOWED_CHAT_IDS=123456789,-100987654321`), and run `./deployment/start.sh` again. For a private chat the ID is your Telegram user ID, so the same value works for the dev bot in your local `.env`.
- [ ] **Start your trackers once** with `/run` (or `/run <code>`). The old version never saved which trackers were running, so there's nothing to resume on the first start. From then on they're resumed automatically.
- [ ] **Remove anything that only existed for webhooks:** a port forward to 7080 on your router, an ngrok agent, a Cloudflare tunnel or reverse proxy entry for the bot, a DDNS entry.
- [ ] **Resilience checks:** `docker kill price_tracker_bot` (it should come back by itself and resume trackers), reboot the server, and unplug the network for a few minutes (the bot should answer again afterwards without a restart).
- [ ] **Timeout check** (can be done locally with the dev bot): point a test API tracker at an address that accepts connections but never answers, e.g. run `nc -l 9999` and use `http://localhost:9999` as its `dataUrl`. Within about 20 seconds the log should show a timeout error, and the tracker should try again at its next interval instead of hanging.

### On GitHub

- [ ] **Remove the Hetzner secrets:** repository Settings → Secrets and variables → Actions → delete `SERVER_IP`, `SERVER_USER` and `SSH_PRIVATE_KEY`.
- [ ] **Delete the stale branches** (after checking nothing in them is needed), and close any open dependabot PRs:
  ```bash
  git push origin --delete deploy_v4 deploy_v5 deploy_6 \
    dependabot/go_modules/golang.org/x/crypto-0.45.0 \
    dependabot/go_modules/golang.org/x/net-0.38.0
  ```
  The `copilot/…` and `experiments/…` branches are yours to judge.
- [ ] **Check the Actions tab** after merging: lint, tests and Trivy should all pass.

### Afterwards

- [ ] **Leave the bot running for a week** before treating "stops working after a couple of days" as fixed. If it does go quiet, `docker logs price_tracker_bot` and `docker inspect price_tracker_bot` are the first things to look at (see the README's troubleshooting section).

---

## 3. Why the bot stopped working after a few days

Each of these could leave the bot dead without any error, and each is fixed:

| Cause | What happened | Fix |
|---|---|---|
| **No network timeouts** (most likely) | The Telegram client and the HTTP client used by API trackers had no timeout. A connection that silently died (common on long-running servers, and more so on a home connection) made the request wait forever: the bot stopped receiving messages, or a tracker stopped running, and nothing was logged. | Every request now has a timeout: 90 s for Telegram polling, 30 s for sending messages, 20 s for API trackers, 10 s for scraping. A failed request is retried. |
| **Trackers lived only in memory** | Any restart (crash, deploy, reboot) left all trackers stopped, and nobody was told. | Running trackers are saved to a state file and resumed on startup, with a "the bot was restarted; resumed N tracker(s)" message. |
| **The container didn't restart** | The old `docker run` scripts didn't set a restart policy, so after a crash or reboot the bot stayed down. | Docker Compose with `restart: unless-stopped`. |
| **Crashes** | A few bugs could take down the whole process: pressing "<< Return" on an old message after a restart, a panic inside a tracker run, and unsynchronized shared state. | Nil checks, panic recovery in tracker runs and update handling, and proper locking (verified with Go's race detector). |
| **Webhook fragility** | Webhooks need a public HTTPS address. On a home server that address can change (new IP, ngrok restart, tunnel down), and running the bot locally deleted production's webhook entirely. | Webhooks replaced with long polling, which needs no public address. |
| **Slowly growing scraper** | The scraper added a new set of callbacks on every run, so each scrape got slower over time. | A fresh scraper for every run. |

The earlier fix in commit `6b62f62` (replying to Telegram with `{"ok": true, "result": <update_id>}`) didn't help: Telegram only looks at the HTTP status code of a webhook response. That code was removed along with webhook mode.

---

## 4. What changed

### Long polling instead of webhooks

The bot now asks Telegram for new messages itself: it keeps one request open and gets updates as soon as they arrive. It only makes outgoing connections, so it needs no public URL, HTTPS certificate, port forward, tunnel or dynamic DNS, and it works the same on a home server as on a laptop.

- Removed: the webhook HTTP server, the `WEBHOOK_URL` / `PORT` / `ENVIRONMENT` settings, the port mapping, and the ngrok script and config.
- On startup the bot deletes any webhook still registered for its token, because Telegram refuses polling while one exists. This makes the first deploy switch over by itself.

#### Running locally next to production

Telegram lets only one running copy of a bot receive updates. If a local copy and the server copy use the same token, they fight over it: commands randomly reach one or the other, and the log shows `Conflict: terminated by other getUpdates request`. The fix is a separate dev bot with its own token (see the to-do list and the README). The bot now logs a clear hint when this happens.

### Reliability

- Timeouts on every network request (see section 3).
- Tracker state saved to `STATE_FILE` whenever a tracker starts, stops or changes interval, and resumed on startup. The file is written safely (temporary file, then rename), so a crash can't corrupt it.
- Panics in a tracker run or while handling a message are caught and logged instead of killing the bot.
- Trackers have their own lock, and restarting a tracker with a new interval no longer relies on a one-second sleep.
- The "waiting for your input" state (used by the interval change) is kept per chat instead of being shared by all chats.
- "<< Return" on a message from before a restart falls back to `/status` instead of crashing.
- The "tracker is failing" message is sent once per streak of failures instead of on every failed run, and the stored error history is capped at the last 20.
- Button clicks are acknowledged right away, so the loading spinner on buttons disappears immediately.
- Buttons on messages older than 48 hours still work.
- The bot shuts down cleanly on `docker stop` or Ctrl+C.

### Security

- **`ALLOWED_CHAT_IDS`:** the bot only responds to the listed chats and logs anything else. Without it, anyone who found the bot could start and stop trackers.
- **The container runs as a non-root user** on a read-only filesystem, with all Linux capabilities dropped. The state volume is the only writable place.
- **`.dockerignore`:** the `.env` file with the bot token is no longer copied into the Docker build.
- **The bot token no longer ends up in logs:** webhook deletion used to build a URL containing the token by hand, and a failed request would have logged it.
- **Dependencies updated:** Trivy went from 19 high-severity findings to 0.

### Telegram library

`go-telegram-bot-api/v5` (last release December 2021) was replaced with `github.com/go-telegram/bot` v1.27.0, which is actively maintained and supports the current Bot API.

- Handlers and trackers send messages through a small `Messenger` interface (`helpers/messages.go`) instead of calling the library directly. This kept the migration contained and makes the code testable.
- The library's own polling loop replaces the hand-written one.
- The new library handles updates in parallel by default; the bot deliberately still handles them one at a time, so two commands can't interfere with each other.

### Dependencies

| Module | Before | After |
|---|---|---|
| Telegram library | `go-telegram-bot-api/v5` v5.5.1 | `go-telegram/bot` v1.27.0 |
| `github.com/gocolly/colly/v2` (scraping) | v2.1.0 | v2.3.0 |
| `github.com/go-playground/validator/v10` | v10.23.0 | v10.30.5 |
| `github.com/tidwall/gjson` | v1.18.0 | v1.19.0 |
| `github.com/valyala/fasthttp` | v1.56.0 | removed (standard `net/http` with a timeout) |
| `github.com/joho/godotenv` | v1.5.1 | v1.5.1 (already latest) |
| `golang.org/x/crypto` | v0.31.0 | v0.57.0 |
| `golang.org/x/net` | v0.33.0 | v0.59.0 |

The colly upgrade was checked for behavior changes (robots.txt handling, timeout and size limit are unchanged) and is covered by the new scraper tests.

### Docker image and deployment

- **Image:** the bot is built as a static binary and copied into an empty `scratch` image. It contains only the binary and the root certificates needed for HTTPS; time zone data is embedded in the binary, so `TZ` still works. Image size went from about 30 MB to about 16 MB. Dependencies are downloaded in a separate cached step, so code-only changes rebuild much faster. The release version is built into the binary and shown in the first log line.
- **No shell in the container:** `docker exec … sh` doesn't work. The README's troubleshooting section lists what to use instead.
- **Releases:** publishing a GitHub release (tag like `v1.2.0`) runs the [Release image](.github/workflows/release.yml) workflow. It builds the image for `linux/amd64` and `linux/arm64`, scans it with Trivy (high or critical vulnerabilities stop the release), and pushes it to `ghcr.io/georgs-tumans/price-tracker-bot` tagged `1.2.0`, `1.2` and `latest` (pre-releases don't get `latest`).
- **Public image, private configs:** the image is public so the server can pull it without logging in. The tracker configs are private, so they're no longer built into the image; the compose file mounts the server's `tracker_configs/` directory read-only. The bot token was never in the image (it comes from `.env` at runtime).
- **Deployment:** Docker Compose ([deployment/docker-compose.yml](deployment/docker-compose.yml)) pulls the released image, restarts the container automatically, and keeps tracker state in the `bot-data` volume. `IMAGE_TAG` in `.env` pins a specific version, e.g. for a rollback. Three scripts wrap it:
  - `./deployment/update.sh`: `git pull` for the deployment files, pull the newest image, recreate the container if it changed (a few seconds of downtime).
  - `./deployment/start.sh`: start in the background.
  - `./deployment/stop.sh`: stop and remove the container (the state volume is kept).
- **Local builds:** `deployment/docker-compose.local.yml` builds the image from the local source instead of pulling a release (see the README).
- **Removed:** the old `docker run` scripts (bash and PowerShell), the ngrok script and config, and the Hetzner deploy workflow.

### CI and linting

- The Docker image and CI now use Go 1.27, matching `go.mod`.
- The golangci-lint config was converted to the v2 format, and the findings were fixed.
- The lint workflow uses the official golangci-lint action (pinned to v2.13.2), takes the Go version from `go.mod`, and also runs the tests with the race detector.

### Tests

The project had no tests. Now:

- **End-to-end tests** (`botfixer/bot_fixer_test.go`) run the whole bot against a fake Telegram API server: `/status` and its menu, ignoring chats that aren't allowed, starting a tracker with a button, the tracker's notification, the saved state, changing the interval with the custom keyboard, "<< Return", and resuming trackers after a restart.
- **Scraper and API client tests** (`clients/clients_test.go`) against a local test server with realistic HTML and JSON: different selector types, price formats like `1 234,56 €`, and error cases.
- **Unit tests** for the tracker lifecycle, error handling, the state file and the chat ID parsing.

Run them with `go test ./...`.

### Smaller fixes

- A misspelled error string that was compared by text (`"uncregonzied tracker code"`) is now a proper error value.
- An invalid `ERROR_NOTIFY_LIMIT` now stops startup with an error instead of silently becoming 0.
- The `ENVIROMENT` typo in `.env.example` and the README is gone (the setting itself was removed).
- `botfixer/udpate.go` was renamed to `update.go`.

---

## 5. Configuration changes

| Variable | Status | Notes |
|---|---|---|
| `WEBHOOK_URL` | **Removed** | Webhooks are no longer used. |
| `PORT` | **Removed** | The bot no longer runs an HTTP server. |
| `ENVIRONMENT` | **Removed** | It only switched between webhooks and polling. |
| `ALLOWED_CHAT_IDS` | **New** | Comma-separated chat IDs the bot responds to. If empty, the bot responds to everyone and logs a warning. |
| `STATE_FILE` | **New** (optional) | Where running trackers are saved. Default `data/state.json`; the Docker image uses `/data/state.json`, so leave it unset there. |
| `IMAGE_TAG` | **New** (optional) | Docker image version to run, e.g. `1.2.0`. Default `latest`. Read by the compose file, not the bot. |
| `ERROR_NOTIFY_LIMIT` | Changed | Now must be a valid number of at least 1. |
| `BOT_API_KEY`, `TZ`, `API_TRACKERS_FILE`, `SCRAPER_TRACKERS_FILE` | Unchanged | |

---

## 6. Decisions and things left out

- **Long polling only, no webhook option.** The bot runs as a single instance on a home server without a stable public address; polling is simpler and more robust there. Webhooks could be added back if the bot is ever hosted publicly.
- **Updates are handled one at a time.** Parallel handling (per-chat locks) would only help with many simultaneous users.
- **`scratch` instead of `alpine` or distroless.** The smallest image, and it matches the setup used at work. The trade-off is no shell for debugging.
- **No Docker health check.** It would need a heartbeat that the Telegram library can't provide well, and the timeouts already turn a hung connection into a retry.
- **Releases via GitHub releases and ghcr.io.** The server pulls images instead of building them, so what runs is exactly what CI built and scanned. `latest` is convenient for updates; `IMAGE_TAG` covers rollbacks. The image is built for both amd64 and arm64 so it also runs on e.g. a Raspberry Pi. Automatic updates (e.g. Watchtower) were left out on purpose: updating stays a deliberate `update.sh`.
- **Non-root container, not rootless Docker.** Running the Docker daemon itself rootless is a host-level setup and wasn't needed.
- **Stopping a tracker doesn't wait for a run in progress.** The original plan had `Stop()` wait for the tracker to finish, but that could block a command for up to 20 seconds while a scrape finishes. Since the API and scraper clients no longer keep any state between runs, a run that finishes after `Stop()` can't interfere with a restarted tracker; at worst it sends one last notification.
- **`STATE_FILE` defaults to `data/state.json`** (relative, for local runs) rather than `/data/state.json` as first planned. The Docker image sets `/data/state.json` itself, so both cases work without configuration.
- **Not yet tested against real Telegram.** Everything was tested against a fake Telegram server, and the Docker image was checked against the real API with a fake token. The dev bot test in the to-do list is the final check.

---

## 7. Open items

Planned but not done yet:

- [ ] **Unit tests for the small helpers:** `utilities.ParseDurationWithDays` (including day intervals like `2d` and invalid input), `utilities.DurationToString` and `helpers.CompareNumbers` (every operator, and an invalid one). They're currently only exercised indirectly through the client and end-to-end tests; day intervals aren't covered at all.
- [ ] **Race detector on an interactive session** (`go run -race .`): the automated tests run with `-race`, but a real session wasn't done. Listed in the to-do section as optional.
- [ ] **Verification against the real Telegram API and your real trackers:** see the to-do section. Everything else was verified with automated tests, a fake Telegram server and a Docker smoke test.

---

## 8. Updating dependencies in the future

`go get -u ./...` upgrades every dependency, including indirect ones, beyond the versions their parent libraries were tested with. During this migration it moved `gobwas/glob` to v1.0.0, which broke colly, and it had to be pinned back to v0.2.3. Safer:

```bash
# direct dependencies
go get github.com/go-telegram/bot@latest github.com/gocolly/colly/v2@latest \
       github.com/go-playground/validator/v10@latest github.com/tidwall/gjson@latest \
       github.com/joho/godotenv@latest
# security-relevant indirect modules
go get golang.org/x/net@latest golang.org/x/crypto@latest
go mod tidy && go build ./... && go test ./...
```

Then check for known vulnerabilities with `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`, or let the Trivy workflow do it.

---

## 9. Commit history

| Commit | Content |
|---|---|
| `17eebc8` | Replace fasthttp, add `ALLOWED_CHAT_IDS`, deploy via Docker Compose with start/stop scripts |
| `44d67ca` | Long polling only; timeouts; saved and resumed trackers; crash, concurrency and leak fixes |
| `02957ef` | `scratch` image, non-root, `.dockerignore`, golangci-lint v2, CI cleanup, Hetzner workflow removed |
| `e8c3786` | To-do list and README troubleshooting section |
| `c10e4f1` | Dependency updates, scraper and API client tests |
| `db0945c` | Migration to `github.com/go-telegram/bot`, end-to-end tests |
| `c21bea4`, `55a5ce8` | This document rewritten as a summary; README Migration section |
| `8ec954b` | SonarCloud fixes: workflow actions pinned to commit SHAs, `GetConfig` split up |
| *(not committed yet)* | Release workflow and ghcr.io deployment, `update.sh`, tracker configs mounted instead of baked in |
