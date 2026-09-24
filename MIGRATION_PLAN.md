# Maintenance & Dependency Migration Plan

**Date**: 2026-09-24
**Project**: Price Tracker Bot (`pricetrackerbot`)
**Replaces**: the 2026-09-23 plan, which had wrong version data, an invented `go-telegram/bot` API, and missed the reliability and toolchain problems below.

---

## Summary

The bot does its job but has three kinds of problems. They're listed in the order to fix them:

1. **Reliability.** The bot "stops working after a couple of days." There isn't one bug behind this; there are several failure modes, and any of them ends in silent death (section 2). None of them is fixed by the response body added in `6b62f62`.
2. **Build and deploy.** `go.mod` says Go 1.27, but the Docker image and CI still use Go 1.23. The lint job targets a golangci-lint major version the config wasn't written for. The Hetzner deploy workflow is obsolete (section 3).
3. **Dependencies.** Most direct and indirect dependencies are several minor versions behind, and `golang.org/x/*` is far behind with security fixes pending. The Telegram library hasn't had a release since **December 2021** (sections 4–5).

**Decision: the bot switches to long polling only.** Webhook mode is removed (section 1). The bot runs on a home server, and long polling needs no public URL, TLS certificate, port forward, tunnel or dynamic DNS. That removes a whole category of "stops after a few days" causes and a good amount of code and config.

### Progress

**Batch 1 (commit `17eebc8`):**
- [x] `fasthttp` replaced with `net/http` with a 20s timeout (6.1; also covers the tracker half of 2.1).
- [x] `ALLOWED_CHAT_IDS` (2.5), with tests.
- [x] Docker Compose as the only deploy path, with `deployment/start.sh` / `stop.sh`; old `docker run` scripts removed (3.3).
- [x] Dockerfile on `golang:1.27-alpine` (3.1).
- [x] README: dev bot setup (2.4), chat restriction, compose deployment. `ENVIROMENT` typo fixed.

**Phase 1 (done):**
- [x] Long polling only (section 1): webhook handler, HTTP server, `WEBHOOK_URL` / `PORT` / `ENVIRONMENT`, the ports mapping, and the ngrok script and config removed. Any leftover webhook is deleted at startup.
- [x] Telegram HTTP client timeout of 90s (2.1).
- [x] Tracker state saved to `STATE_FILE` on every change, resumed on startup, with a "the bot was restarted" message per chat. Compose mounts a `bot-data` volume at `/data` (2.2).
- [x] Concurrency and crash fixes (2.3): per-tracker mutex with a `Status()` snapshot, no sleep-based restart, `recover` in tracker runs and update handling, nil-safe "Return" / user input / message edit, per-chat `AwaitingUserInput` / `CustomKeyboardActive`, locked navigation map.
- [x] Leaks and spam (2.6): new colly collector per run (this also fixes an error from `OnError` being overwritten by `Visit`'s return value), stateless API client, precompiled regex, error notification sent once per failure streak, error history capped at 20, callback queries answered.
- [x] Graceful shutdown on SIGTERM / Ctrl+C.
- [x] Smaller items from 6.3/6.4: sentinel error instead of comparing the misspelled string, redundant `Stop()` / `Start()` around `UpdateInterval` removed, `ERROR_NOTIFY_LIMIT` parse errors now fail startup, `udpate.go` renamed to `update.go`.
- [x] Tests for the tracker lifecycle, error history, panic recovery and state file; `go test -race` passes (run in a `golang:1.27` container).
- 2.4 (409 logging): no code change needed. The current library already logs Telegram's own message ("Conflict: terminated by other getUpdates request; make sure that only one bot instance is running"). Phase 4 routes it through `WithErrorsHandler`.
- 2.7 heartbeat `HEALTHCHECK`: **dropped.** With the scratch image it would need a `-healthcheck` flag in the binary, and the current library doesn't report successful polls, so the heartbeat could only track incoming updates, which are rare for this bot. The client timeouts already turn a hung poll into a retry. Revisit after Phase 4 if needed.

**Phase 2 (done):**
- [x] Scratch Dockerfile (3.4): static binary (`CGO_ENABLED=0`, `-trimpath`, `-s -w`), embedded time zone data, CA certificates copied from the build stage, UID 65532, `/data` owned by that user, module download cached separately from the source with BuildKit cache mounts, `TZ`/`STATE_FILE` defaults. Checked with a fake token: TLS to Telegram works ("Unauthorized", not a certificate error), `TZ=Europe/Riga` is applied, and a new volume is owned by 65532. Image content went from ~30 MB to ~14 MB.
- [x] `.dockerignore`: `.env`, `.git` etc. no longer go into the build context.
- [x] Compose hardening: `read_only`, `cap_drop: ALL`, `no-new-privileges`.
- [x] golangci-lint config migrated to v2 (`gomodguard` → `gomodguard_v2`); the 11 real findings fixed (the other 3 were a Windows line-ending artifact). Lint is clean with v2.13.2.
- [x] Lint workflow: `golangci/golangci-lint-action@v9` pinned to v2.13.2, `go-version-file: go.mod`, and it runs `go test -race` too.
- [x] `.github/workflows/deploy.yml` deleted.
- [x] README: scratch image notes, lint/test instructions (including running golangci-lint through Docker when the local binary is too old).

**Left for you:** see [Manual to-do](#manual-to-do-for-you) below.

**Phase 3 (done):**
- [x] All dependencies updated: validator 10.30.5, colly 2.3.0 (with goquery 1.13, cascadia 1.3.5, antchfx/*), gjson 1.19.0, `golang.org/x/crypto` 0.57.0, `x/net` 0.59.0, `x/sys` 0.48.0, `x/text` 0.42.0.
- [x] **Upgrade trap found:** `go get -u ./...` also moved `gobwas/glob` (a colly dependency) to v1.0.0, whose API changed, so colly stopped compiling. Pinned back to v0.2.3, the version colly requires. See the recipe for future updates in section 4.
- [x] Correction to the original plan: colly 2.3 does **not** drop the old `appengine` / `golang/protobuf` dependencies; they're still there, only newer.
- [x] colly 2.1 → 2.3 behavior checked in source: same defaults for robots.txt (ignored), timeout (10s) and body limit (10 MB). Only the User-Agent string changed slightly.
- [x] New tests in `clients/clients_test.go` run the scraper and API client against a local test server (class/id/attribute/`nth-child` selectors, prices like `1 234,56 €` and `€ 7,99`, gjson queries, and error cases). They passed on the old versions first (baseline) and pass unchanged on the new ones. Tests, `-race` and lint are all clean.
- [x] Security: Trivy (HIGH/CRITICAL, as in CI) found **19 HIGH** in the image from the previous commit and **0** now. `govulncheck`: nothing reachable. The one module-level note is `x/crypto/openpgp`, a deprecated package the bot never imports, with no fix available.
- Your real tracker configs aren't in the repo, so the test server covers typical cases only. **Check your actual scraper trackers once with the dev bot** (added to the manual to-do list).

**Not yet verified against real Telegram.** Needs a run with the dev bot (see the checklist in section 7).

**Phase 4 (done):**
- [x] `go-telegram-bot-api/v5` replaced with `github.com/go-telegram/bot` v1.27.0; the old library is gone from `go.mod`.
- [x] New `helpers.Messenger` interface (`SendHTML`, `SendHTMLWithMenu`, `SendHTMLWithKeyboard`, `EditHTMLWithMenu`, `RemoveKeyboard`), implemented by `TelegramMessenger`. Handlers and trackers only depend on the interface. Each call has a 30s timeout. A nil menu is never put into `ReplyMarkup`, because the library would send it as `null`.
- [x] `botfixer` rewritten around the library's polling loop (`bot.Start`): 60s poll timeout, 90s HTTP client timeout, graceful stop through the context. The hand-written update loop is gone.
- [x] Errors go through `WithErrorsHandler`. A 409 Conflict is logged as "Another instance is polling with this bot API key; use a separate bot for development" (2.4). The library already removes the token from network errors.
- [x] Buttons on messages older than 48 hours (which Telegram sends as "inaccessible") still work; they carry the chat and message ID that's needed.
- [x] Concurrency: the library runs each handler in its own goroutine. Updates are now explicitly handled one at a time (a mutex in `handleUpdate`), the same as before. This is simpler and safer than per-chat locks for a bot with a handful of users. Tracker goroutines are unaffected.
- [x] `NewCommandHandler` takes the config as a parameter instead of reading the global one, which makes it testable.
- [x] **End-to-end tests** (`botfixer/bot_fixer_test.go`) run the whole bot against a fake Telegram API server. They cover: startup webhook deletion; `/status` with its inline menu; ignoring a chat that isn't allowed; a button click that starts a tracker (callback answered, message edited, Return button added); the tracker's notification (with no `reply_markup` sent); the saved state file; the interval change with a reply keyboard, typed input, keyboard removal and the saved new interval; Return, including on an empty history; and resuming trackers after a restart, including dropping a tracker that's no longer in the config. A deliberately broken message text makes the test fail, so it does catch regressions.
- [x] Tests pass with `-race` (3 runs), lint is clean, the image builds and reaches Telegram, and Trivy is clean. The binary grew from 14.1 to 15.9 MB, because the new library has types for the whole current Bot API.

**Not yet verified against real Telegram.** Everything above ran against a fake API server. Test with the dev bot before deploying (manual to-do list).

All planned phases are done. What's left is the manual to-do list and the optional items in section 6.

| Phase | What | Effort | Can ship on its own |
|---|---|---|---|
| 0 | Diagnose production (when possible) | 15–30 min | n/a |
| 1 | Switch to polling only + reliability fixes (current Telegram library) | 0.5–1 day | yes |
| 2 | Toolchain, CI, deploy | 1–2 h | yes |
| 3 | In-place dependency updates | 1 h + smoke test | yes |
| 4 | Migrate to `github.com/go-telegram/bot` | ~1 day | yes |
| 5 | Optional cleanup | as desired | yes |

Phase 1 comes first because it's the problem users actually notice, and it doesn't depend on the library migration. Phases 2 and 3 can be done in parallel with it.

---

## Manual to-do (for you)

Things that can't be done from the code, in the order to do them. Tick them off as you go.

### Before the first deploy

- [ ] **Create a dev bot.** In Telegram, message [@BotFather](https://t.me/BotFather), send `/newbot`, and pick a name and username (e.g. `Price Tracker (dev)` / `my_price_tracker_dev_bot`). Put its API key in your **local** `.env` as `BOT_API_KEY`. From now on, the production key only lives in the server's `.env`.
- [ ] **Test locally with the dev bot** (`go run .` or F5). Go through the local items of the checklist in section 7: every command and button, "<< Return" after restarting the bot, the interval change, and a tracker being resumed after a restart with the "the bot was restarted" message.
- [ ] **Run each of your real trackers once** with the dev bot (copy the real `tracker_configs/*.json` from the server) and compare the values with what the production bot showed. The colly/goquery upgrade is covered by tests, but only with sample HTML.
- [ ] **Update your local golangci-lint** so it can lint Go 1.27 code (the installed one was built with Go 1.26):
  ```bash
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
  ```
- [ ] **Review and merge** the `maintenance-plan` branch into `develop` (and into `main`, or whichever branch the server pulls).
- [ ] *(Optional, not blocking)* **Run the Phase 0 checks** on the server before replacing the old container (section 0). This tells you which failure was actually killing the bot.

### On the server

- [ ] **Edit the server's `.env`:**
  - Delete `WEBHOOK_URL`, `PORT` and `ENVIRONMENT` (also the misspelled `ENVIROMENT`, if it's there). They're no longer used.
  - Don't set `STATE_FILE`; the image already points it at the `/data` volume.
  - Add `ALLOWED_CHAT_IDS`. Until you know your chat ID, set it to `0` (see the next step).
- [ ] **Make sure Docker starts on boot** and the compose plugin is installed:
  ```bash
  sudo systemctl enable docker
  docker compose version
  ```
- [ ] **Deploy:**
  ```bash
  git pull
  ./deployment/start.sh
  docker logs -f price_tracker_bot
  ```
  The first run removes the old container created by the old scripts. The log should show `Webhook deleted` and then `Bot initialized via long polling`, with no `Conflict` errors.
- [ ] **Find your chat ID(s).** With `ALLOWED_CHAT_IDS=0`, send the production bot any message. The log shows:
  ```
  [Bot fixer] Ignoring update from chat 123456789 (not in ALLOWED_CHAT_IDS)
  ```
  That number is your chat ID. Group chats have negative IDs; message the bot from each chat you want to use. Put the IDs in `.env` (comma separated, e.g. `ALLOWED_CHAT_IDS=123456789,-100987654321`) and run `./deployment/start.sh` again. Do the same with the dev bot for your local `.env` (for a private chat the ID is your Telegram user ID, so it's the same for both bots; group IDs are also the same if both bots are in the group).
- [ ] **Start the trackers again once** with `/run` (or `/run <code>`). The old version never saved which trackers were running, so there's nothing to resume on the first start. From then on they're resumed automatically.
- [ ] **Clean up anything that only existed for webhooks:** a port forward to 7080 on your router, an ngrok agent, a Cloudflare tunnel or reverse proxy entry for the bot, a DDNS entry. Polling needs none of them.
- [ ] **Resilience checks** (section 7): `docker kill price_tracker_bot` (it should come back and resume trackers), reboot the server, unplug the network for a few minutes.

### On GitHub

- [ ] **Remove the Hetzner secrets:** repository Settings → Secrets and variables → Actions → delete `SERVER_IP`, `SERVER_USER`, `SSH_PRIVATE_KEY`.
- [ ] **Delete the stale branches** (after checking nothing in them is needed), and close any open dependabot PRs:
  ```bash
  git push origin --delete deploy_v4 deploy_v5 deploy_6     dependabot/go_modules/golang.org/x/crypto-0.45.0     dependabot/go_modules/golang.org/x/net-0.38.0
  ```
  The `copilot/…` and `experiments/…` branches are yours to judge.
- [ ] **Check the Actions tab** after merging: lint + tests should pass. Trivy should pass once Phase 3 is merged.

### Afterwards

- [ ] **Leave the bot running for a week** before treating "stops working after a couple of days" as fixed. If it does go quiet, `docker logs price_tracker_bot` and `docker inspect price_tracker_bot` are the first things to look at.

---

## 0. Diagnose production (when possible; not blocking)

These checks can't be done right now, and nothing below depends on them. Every fix in Phase 1 is worth doing regardless. If you get the chance **before deploying the new version**, a few minutes here tells you which failure mode was actually hitting production:

```bash
# On the home server
docker ps -a --filter name=price_tracker_bot    # is it running? when did it start?
docker inspect -f '{{.RestartCount}} {{.State.StartedAt}} {{.HostConfig.RestartPolicy.Name}}' price_tracker_bot
docker logs --since 72h price_tracker_bot | tail -200

# What Telegram thinks about the (old) webhook
curl -s "https://api.telegram.org/bot<TOKEN>/getWebhookInfo"
```
- The container has exited → a crash with no restart policy (2.2, 2.3).
- The container is running but `/status` shows a tracker whose "Last run" stopped advancing → a hung tracker goroutine (2.1).
- The container restarted recently and `/status` shows every tracker inactive → trackers lost on restart (2.2).
- `getWebhookInfo` has an empty `url`, or shows `last_error_message` → webhook delivery was failing (a local run deleted it, the public address changed, or the tunnel was down). Moving to polling makes this category go away.

---

## 1. Switch to long polling only

### Why polling fits here

- The bot runs as **one instance** on a home server, where there's no stable public HTTPS address. Webhooks need one; polling doesn't, because the bot makes outgoing requests to Telegram.
- Polling adds at most a second or so of latency, and Telegram's long-poll call returns as soon as an update arrives. Irrelevant for this bot.
- The one real requirement is an **HTTP client timeout** (2.1). Without it, a long-poll request on a dead connection can hang forever, and the bot silently stops receiving updates. With it, the request fails, the library retries, and the bot recovers by itself after network outages and IP changes.

### Why the `6b62f62` fix doesn't fix anything

The commit makes the webhook handler reply with `{"ok": true, "result": <update_id>}` and adds a comment saying Telegram requires this. It doesn't. Telegram only looks at the **HTTP status code**; any 2xx counts as delivered. A response body is only read as an optional *method call* (for example `{"method":"sendMessage",...}`). The file it touched gets deleted in this phase, so no separate revert is needed.

### What gets removed

| Item | Where |
|---|---|
| `InitializeBotWebhook`, `webhookHandler`, the `net/http` server, the `/webhook` endpoint | `botfixer/bot_fixer.go`, `botfixer/udpate.go` |
| The `local` vs. cloud branch | `main.go` |
| `WEBHOOK_URL`, `PORT`, `ENVIRONMENT` settings (and their validation) | `config/config.go`, `.env.example`, README |
| `ports:` / `-p 7080:8080` | `deployment/docker-compose.yml`, run scripts |
| ngrok script and config | `deployment/docker_run_ngrok.ps1`, `ngrok.yml.example`, README's webhook section |
| The hand-built `deleteWebhook` URL (the only non-tracker use of `services.GetRequest`) | `botfixer/bot_fixer.go` |

### What stays

- **One `deleteWebhook` call at startup.** While a webhook is registered, Telegram rejects `getUpdates` with 409 Conflict. The first deploy after this change has to clear the old webhook, and calling it every startup costs nothing. Use the library's own method (`bot.Request(tgbotapi.DeleteWebhookConfig{})` today, `b.DeleteWebhook` after Phase 4) instead of the hand-built URL.

---

## 2. Phase 1 — Reliability fixes

Ordered by how likely each one is to cause "dead after a few days" in production. Each item names the files it touches.

### 2.1 No network timeouts anywhere (most likely cause)

| Where | What happens today |
|---|---|
| `tgbotapi.NewBotAPI` creates `&http.Client{}` with **no timeout** | Every `bot.Send` (from command handlers *and* tracker goroutines) can block forever on a half-open TCP connection. `getUpdates` can too, and then the bot never receives another update. |
| `services/http_service.go` uses `fasthttp.Do`, whose default client has **no read timeout** | An API tracker whose server stalls blocks its goroutine forever. The `time.Ticker` silently drops ticks, `/status` shows "Last run" frozen, and no error is ever logged. |
| colly | Already has a 10s default timeout. Fine. |

Half-open connections are exactly the kind of thing that shows up "after a few days" on a long-running server, and a home connection makes them more likely: router/NAT table expiry, ISP IP changes, brief outages.

**Fix:**
- `botfixer/bot_fixer.go`: create the bot with `tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, &http.Client{Timeout: 90 * time.Second})`. It has to be longer than the 60s long-poll timeout (`u.Timeout = 60`).
- `services/http_service.go`: use `fasthttp.DoTimeout(req, resp, 20*time.Second)`, or switch to `net/http` with a timeout (see 6.1).

### 2.2 Tracker state lives only in memory, and the container doesn't restart

- Trackers only exist after someone sends `/run`. Any process restart (crash, deploy, host reboot, Docker daemon restart) leaves **every tracker stopped, and nobody is told**. From the user's point of view, the bot "stopped working."
- Deployment is manual on a home server: `git pull`, then start the container by hand. None of the repo's scripts (`build-and-run-docker.sh`, `docker_run.ps1`) pass `--restart` to `docker run`, and a container keeps whatever restart policy it was **created** with. Unless you added `--restart` yourself, the bot stays down after a crash, a server reboot or a power cut. Home servers get rebooted and lose power far more often than cloud VMs. Check with `docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' price_tracker_bot`.
- `docker start` on an existing container runs the **image it was created from**. If your manual step after `git pull` is `docker start` rather than a rebuild, the pulled code never actually runs. Worth confirming.

**Fix:**
- Replace the manual steps with one command that rebuilds and applies the restart policy every time: `docker compose up -d --build` (see 3.3).
- Save running trackers (code, chat ID, current interval) to a small JSON file, for example `/data/state.json` on a mounted volume, whenever a tracker starts, stops or changes interval. On startup, load it and resume those trackers.
- On startup, send one message to each chat that had trackers: "Bot restarted, resumed N trackers." Restarts then become visible instead of silent.

### 2.3 Crashes and hangs from concurrency and nil pointers

| Issue | Location | Effect |
|---|---|---|
| `HandleReturn` / `HandleUserInput` dereference `Peek()` without a nil check. The navigation stack is empty after a restart, so pressing "<< Return" on an old message, or typing text while `AwaitingUserInput` is stale, panics. | `handlers/command_handler.go` | In polling mode the update is handled on a plain goroutine, so the panic **kills the process**. |
| Tracker goroutines have no `recover()`. The `recover` in `main.go` only protects the main goroutine. | `handlers/tracker.go` (`Start`) | Any panic during a scrape or notification kills the whole process. |
| `EditMessageWithMenu` dereferences `*menu` unconditionally. | `helpers/messages.go` | Panics if it's ever called with a nil menu. |
| `Tracker.running` and `Tracker.Status` are written by the tracker goroutine and read by `/status` and `Stop()` with no lock. `UpdateInterval` relies on `time.Sleep(1s)` to avoid racing the old goroutine. | `handlers/tracker.go` | Data races. Can end up with two goroutines for one tracker, or a tracker reported as stopped while it's still running. |
| `CommandHandler.Navigation` map has no lock. `AwaitingUserInput` / `CustomKeyboardActive` are global booleans shared by every chat. | `handlers/command_handler.go` | With today's polling loop, updates are handled one at a time, so the map is safe *for now*. After Phase 4 handlers run concurrently, and a concurrent map write is a `fatal error` that **can't be recovered**. The shared booleans are wrong with more than one chat either way. |

**Fix:**
- Nil checks for `Peek()`: fall back to `/status` or `/help`.
- `defer func(){ if r := recover(); r != nil { log... } }()` inside each tracker goroutine and at the top of `handleUpdate`.
- Give `Tracker` its own mutex. Replace the `running` flag and the sleep with a `done` channel that `Stop()` waits on.
- One `sync.Mutex` around `Navigation`, and move `AwaitingUserInput` / `CustomKeyboardActive` into `NavigationState` so they're per chat. Do it now so Phase 4 doesn't introduce a crash.
- Run the bot locally for a while with `go run -race .` and click through the menus to catch the rest.

### 2.4 Local development vs. the production instance

Telegram allows **only one `getUpdates` caller per bot token**. If you run the bot locally with the same token while the server copy is running, one of them gets `409 Conflict: terminated by other getUpdates request` on each poll, and both keep retrying. The result is that commands randomly go to one instance or the other, and each instance's trackers send notifications independently. Switching to polling doesn't fix this; it just changes how it breaks (today a local run *deletes production's webhook* instead).

**Fix:**
- **Use a separate dev bot.** Create a second bot with @BotFather (free, about two minutes) and put its token in your local `.env`. The production token then only exists on the server. Both instances run side by side with no interference, and dev trackers can't spam the real chat. This is the only real fix; nothing in code can make two pollers share one token.
- Log 409 Conflict clearly ("another instance is polling with this token") so a mistake is obvious in the logs instead of looking like random flakiness.

### 2.5 Anyone can control the bot

Anyone on Telegram who finds the bot's username can send `/run`, `/stop`, `/interval`. Webhook forgery goes away with webhooks, but this doesn't.

**Fix:** an `ALLOWED_CHAT_IDS` env var (comma-separated), and ignore updates from any other chat. Log rejected chat IDs so you can find your own ID the first time.

### 2.6 Resource leaks and notification spam

| Issue | Location | Fix |
|---|---|---|
| `ScraperClient.FetchAndExtractData` calls `collector.OnHTML` / `OnError` **on every run**. colly keeps adding callbacks, so after N runs each scrape runs N callbacks (memory grows without limit, CPU cost per run grows linearly, and errors get logged N times). | `clients/scraper_client.go` | Create a new collector for each run (they're cheap), or register the callbacks once in the constructor. |
| `regexp.MustCompile` runs on every scrape. | `clients/scraper_client.go` | Package-level `var`. |
| Once `ExecutionErrors` reaches `errorLimit`, **every** later failed run sends a Telegram message, and the slice grows forever. | `handlers/tracker.go` | Notify once when the limit is crossed, reset the count on the next success, and cap the stored history (for example the last 20). |
| Callback queries are never answered, so button spinners hang until the client gives up. | `botfixer/udpate.go` | Call `answerCallbackQuery` in `handleButton`. |

### 2.7 Observability

There's no HTTP server any more, so no `/healthz`. Instead:
- Log startups, resumed trackers and polling errors (including 409s) clearly, so `docker logs` answers "what happened" next time.
- The startup message from 2.2 already makes restarts visible in Telegram.
- Optional: have the bot update a heartbeat file (for example `/data/heartbeat`) after each successful poll, and add a Docker `HEALTHCHECK` that fails if it's more than a few minutes old. Then `docker ps` shows `unhealthy` if polling ever hangs.

---

## 3. Phase 2 — Toolchain, CI and deploy

### 3.1 Go version is inconsistent across the repo

| File | Now | Should be |
|---|---|---|
| `go.mod` | `go 1.27` | keep |
| `Dockerfile` | `golang:1.23.2-alpine` | `golang:1.27-alpine` |
| `.github/workflows/golangci-lint.yml` | `go-version: 1.23.4` | `go-version-file: go.mod` |

The official `golang` images set `GOTOOLCHAIN=local`, so a Go 1.23 image **should refuse to build** a module that declares `go 1.27`. The next deploy is expected to fail at `docker build`. Also pin the runtime stage to `alpine:3.x` instead of `alpine:latest`.

### 3.2 golangci-lint

- The workflow installs golangci-lint from `master`, which is now **v2**. `.golangci.yml` is in v1 format (`linters.presets`, `issues.exclude-files`, `linters-settings`), and `--skip-dirs` no longer exists in v2.
- **Fix:** run `golangci-lint migrate` to convert the config. Replace the curl install with `golangci/golangci-lint-action` pinned to a v2 release. Drop `--skip-dirs` (there's no `vendor/` or `third_party/` anyway).

### 3.3 Deployment (home server, manual)

Deployment is now done by hand on a home server: `git pull`, then start the container. Hetzner is no longer used.

- **Delete `.github/workflows/deploy.yml`.** It deploys to Hetzner over SSH on every push to `main`, so it fails or does nothing today. Also remove the `SERVER_IP` / `SERVER_USER` / `SSH_PRIVATE_KEY` repository secrets on GitHub.
- **Make `docker compose` the single deploy command.** `deployment/docker-compose.yml` currently references `ghcr.io/.../price_tracker_bot:latest`, which nothing publishes any more. Change it to build from the repo. No ports are needed with polling:
  ```yaml
  services:
    price_tracker_bot:
      build: ..
      container_name: price_tracker_bot
      env_file: ../.env
      restart: unless-stopped
      volumes:
        - bot-data:/data          # tracker state from 2.2
  volumes:
    bot-data:
  ```
  Then a deploy is always:
  ```bash
  git pull && docker compose -f deployment/docker-compose.yml up -d --build
  ```
  That one command rebuilds from the pulled code, replaces the old container and applies the restart policy every time, so there's no way to forget `--restart` or to `docker start` a stale image.
- **Make sure Docker itself starts on boot** on the home server (`systemctl enable docker` on Linux, or "Start Docker Desktop when you sign in" plus auto-login on Windows). `restart: unless-stopped` only helps if the Docker daemon is running.
- **Clean up the old scripts.** Delete `build-and-run-docker.sh`, `docker_build_and_run.ps1`, `docker_run.ps1`, `docker_run_ngrok.ps1` and `ngrok.yml.example`, or reduce the run scripts to the compose command above. Rewrite the README's setup and deployment sections: no ngrok, no webhook mode, and a note that local development uses a separate dev bot token (2.4).
- Close the stale remote branches (`deploy_v4`, `deploy_v5`, `deploy_6`, the two dependabot branches — Phase 3 supersedes them).

### 3.4 Dockerfile: scratch image, non-root, better caching

**Today:** build in `golang:1.27-alpine`, run in `alpine:latest` as **root**. `COPY . .` comes before `go mod download`, so every code change downloads all modules again. There's no `.dockerignore`, so `.env` (with the bot token) and `.git` get sent into the build. `ENV TZ=${TZ:-UTC}` refers to a variable that doesn't exist at build time (Docker warns about it).

**Target:** the same pattern as Woodpecker at work: build a static binary, then copy it into an empty `scratch` image. It works for this bot because everything is pure Go (no cgo), but scratch has nothing in it, so three things have to be provided explicitly:

| Needed | Why | How |
|---|---|---|
| CA certificates | HTTPS to Telegram and every tracker URL | `COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/` |
| Time zone data | `TZ` controls the times shown in `/status` | Build with `-tags timetzdata`, which embeds Go's zone database in the binary (~450 KB) |
| A non-root user | Security | `USER 65532:65532`. Scratch has no `/etc/passwd`, but a numeric UID works. |

Other changes:
- Copy `go.mod`/`go.sum` and download modules **before** copying the source, and use BuildKit cache mounts for the module and build caches. Code-only changes then rebuild in seconds.
- `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"` gives a smaller, reproducible static binary.
- Add a `.dockerignore` (`.env`, `.git`, `.github`, `.vscode`, `*.md`, `deployment/`), but **not** `tracker_configs/*.json`, which get baked into the image.
- Create `/data` in the build stage and `COPY --chown=65532:65532` it into the image. Docker initializes a new named volume from the image's directory, ownership included, so the non-root user can write the state file (2.2).
- `ENV TZ=UTC` as the default; the `.env` value overrides it at runtime.
- Compose hardening that comes almost free with this image: `read_only: true`, `cap_drop: [ALL]`, `security_opt: [no-new-privileges:true]`. The only writable place is the `/data` volume.

**Result:** image size drops from ~20 MB (alpine + binary) to roughly the binary itself (~10–12 MB). There's no shell, package manager or OS packages for Trivy to flag, and the process can't write anywhere except `/data`.

**Trade-offs:**
- There's no shell in the container, so no `docker exec -it … sh` for debugging. Logs still work, and `docker cp` works for files.
- A Docker `HEALTHCHECK` (2.7) can't run shell commands, so the binary itself would need a `-healthcheck` flag to check the heartbeat file.
- `gcr.io/distroless/static-debian12:nonroot` is the alternative: scratch plus CA certificates, time zone data and a non-root user, maintained by Google. Equally valid. Scratch is chosen because it's what you use at work and adds no extra registry dependency.

**On "rootless":** this is about the container's process not running as root, which covers the realistic risk. "Rootless Docker" (running the Docker daemon itself as a normal user) is a separate host-level setup and not needed here.

### 3.5 Config

- Remove `WEBHOOK_URL`, `PORT` and `ENVIRONMENT` (section 1). This also gets rid of the `ENVIROMENT` typo in `.env.example` and the README, which currently makes startup fail for anyone copying the example.
- Add `STATE_FILE` (default `/data/state.json`) and `ALLOWED_CHAT_IDS` to `config.go`, `.env.example` and the README.
- `ERROR_NOTIFY_LIMIT` parse errors are silently ignored, giving a limit of 0. Fail validation instead.

---

## 4. Phase 3 — Update dependencies in place

Versions from `go list -m -u all` and proxy.golang.org on 2026-09-24:

| Module | Current | Latest | Notes |
|---|---|---|---|
| `github.com/go-playground/validator/v10` | v10.23.0 | v10.30.5 | Minor updates, no API change for our use. |
| `github.com/valyala/fasthttp` | v1.56.0 | v1.74.0 | Or drop it (6.1). |
| `github.com/gocolly/colly/v2` | v2.1.0 (2020) | v2.3.0 | Pulls in much newer goquery/antchfx and drops the old `appengine` + deprecated `golang/protobuf` chain. **Most likely to change scraping behavior; smoke-test every scraper tracker.** |
| `github.com/tidwall/gjson` | v1.18.0 | v1.19.0 | Used directly in `clients/api_client.go`; **keep it**. |
| `github.com/joho/godotenv` | v1.5.1 | v1.5.1 | Already latest; keep. |
| `golang.org/x/net` (indirect) | v0.33.0 | v0.59.0 | Security fixes. |
| `golang.org/x/crypto` (indirect) | v0.31.0 | v0.57.0 | Security fixes. |
| `golang.org/x/sys`, `x/text` (indirect) | v0.28 / v0.21 | v0.48 / v0.42 | |

**Steps (as done):**
```bash
go get -u ./...
go get github.com/gobwas/glob@v0.2.3   # undo the incompatible bump, see below
go mod tidy
go build ./... && go vet ./... && go test ./...
```

**Recipe for future updates.** `go get -u ./...` upgrades *every* dependency, including indirect ones, past the versions their parents were tested with. That's what broke colly via `gobwas/glob`. Safer:
```bash
# direct dependencies, one at a time or together
go get github.com/gocolly/colly/v2@latest github.com/go-playground/validator/v10@latest        github.com/tidwall/gjson@latest github.com/joho/godotenv@latest
# security-relevant indirect modules
go get golang.org/x/net@latest golang.org/x/crypto@latest
go mod tidy && go build ./... && go test ./...
```
Then check with `govulncheck` (`go run golang.org/x/vuln/cmd/govulncheck@latest ./...`) or let the Trivy workflow do it.

---

## 5. Phase 4 — Migrate `go-telegram-bot-api/v5` → `github.com/go-telegram/bot`

### 5.1 Why

- `go-telegram-bot-api/v5` v5.5.1 (2021-12-13) is the last release. It's missing every Bot API feature added since then, and bug fixes aren't coming.
- `github.com/go-telegram/bot` v1.27.0 (2026-09-11) is actively maintained and has no dependencies. Its built-in polling loop (`b.Start(ctx)`) with `WithHTTPClient(pollTimeout, client)` replaces the hand-written update loop and makes timeouts explicit (2.1). `DeleteWebhook` is a normal method.

### 5.2 Scope (real numbers)

About 63 `tgbotapi.` references across 7 files today; fewer once Phase 1 has deleted the webhook code:

| File | Refs | What changes |
|---|---|---|
| `helpers/messages.go` | 17 | All send/edit/delete helpers → `SendMessage` / `EditMessageText` / `DeleteMessage` with a `ctx`. |
| `helpers/ui_helper.go` | 16 | Keyboard builders → `models.InlineKeyboardMarkup` / `models.ReplyKeyboardMarkup` literals. |
| `handlers/command_handler.go` | 15 | `*tgbotapi.BotAPI` field, inline keyboards in `handleStatus` / `processTrackerStatus`. |
| `botfixer/udpate.go` | 5 | Mostly deleted: the library's dispatcher replaces the manual polling loop. |
| `botfixer/bot_fixer.go` | 4 | Rewritten around `bot.New` + `Start`. |
| `handlers/tracker_behavior.go` | 4 | Bot type in behaviors. |
| `handlers/tracker.go` | 2 | Bot type in `Tracker`. |

**Recommended first step:** add a small interface in `helpers`, for example:
```go
type Messenger interface {
    SendHTML(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) (int, error)
    EditHTML(ctx context.Context, chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error
    Delete(ctx context.Context, chatID int64, messageID int) error
}
```
Handlers and trackers then depend on `Messenger` instead of the library type. That keeps the migration contained and makes the handlers unit-testable with a fake.

### 5.3 Real API mapping (checked against v1.27.0)

| Old (`tgbotapi`) | New (`bot` / `models`) |
|---|---|
| `tgbotapi.NewBotAPIWithClient(token, endpoint, client)` | `bot.New(token, bot.WithHTTPClient(60*time.Second, client), ...)` → `(*bot.Bot, error)` |
| `GetUpdatesChan(u)` + manual loop | `bot.WithDefaultHandler(h)` then `b.Start(ctx)` |
| `bot.Request(tgbotapi.DeleteWebhookConfig{})` | `b.DeleteWebhook(ctx, &bot.DeleteWebhookParams{})` |
| `tgbotapi.Update` | `*models.Update` (handler signature `func(ctx, *bot.Bot, *models.Update)`) |
| `message.IsCommand()` | check `update.Message.Entities` for `models.MessageEntityTypeBotCommand`, or `strings.HasPrefix(text, "/")` |
| `tgbotapi.NewMessage` + `ParseMode = ModeHTML` + `bot.Send` | `b.SendMessage(ctx, &bot.SendMessageParams{ChatID: id, Text: t, ParseMode: models.ParseModeHTML, ReplyMarkup: m})` |
| `NewEditMessageTextAndMarkup` | `b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID, MessageID, Text, ParseMode, ReplyMarkup})` |
| `NewDeleteMessage` | `b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID, MessageID})` |
| `NewInlineKeyboardMarkup(NewInlineKeyboardRow(NewInlineKeyboardButtonData(t, d)))` | `&models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: t, CallbackData: d}}}}` |
| `NewOneTimeReplyKeyboard(...)` | `&models.ReplyKeyboardMarkup{Keyboard: [][]models.KeyboardButton{...}, OneTimeKeyboard: true, ResizeKeyboard: true}` |
| `NewRemoveKeyboard(true)` | `&models.ReplyKeyboardRemove{RemoveKeyboard: true}` |
| `AnswerCallbackQuery` (added in 2.6) | `b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID})` |

**Watch out:** handlers run **asynchronously** by default (one goroutine per update). The locking from 2.3 isn't optional after this change. *(Resolved in Phase 4: updates are handled one at a time through a mutex in `handleUpdate`, so a chat's `NavigationState` and multi-step commands like "/run all" can't interleave. Per-chat locks would allow more parallelism, but a bot with a handful of users doesn't need it.)*

### 5.4 Target `main` / bot setup (sketch)

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

b, err := bot.New(cfg.BotAPIKey,
    bot.WithDefaultHandler(dispatcher.Handle),
    bot.WithHTTPClient(60*time.Second, &http.Client{Timeout: 90 * time.Second}),
    bot.WithErrorsHandler(func(err error) { log.Printf("[bot] %v", err) }),
)
if err != nil { log.Fatal(err) }

if _, err := b.DeleteWebhook(ctx, &bot.DeleteWebhookParams{}); err != nil {
    log.Printf("[bot] deleteWebhook: %v", err)
}
trackers.Resume(ctx)      // from state file (2.2)
b.Start(ctx)              // blocks until SIGTERM
trackers.SaveState()
```
This also adds graceful shutdown on SIGTERM (`docker stop`), which today just kills the process. It's the right place to save tracker state (2.2).

---

## 6. Phase 5 — Optional

1. **Replace `fasthttp` with `net/http`.** After section 1 it's only used by the API trackers, in one 37-line file, for a handful of requests per interval. The speed advantage doesn't matter here, and `net/http` makes timeouts and context cancellation natural. Removes a dependency tree (`fasthttp`, `brotli`, `klauspost/compress`, `bytebufferpool`).
2. **Tests.** The repo has none. Worth adding: `utilities.ParseDurationWithDays` / `DurationToString`, `clients.ProcessNotificationCriteria`, `helpers.CompareNumbers`, `NavigationState`, and command handling against a fake `Messenger`. Run them with `-race` in CI.
3. **Small bugs spotted along the way:** the typo `"uncregonzied tracker code"` is compared by string in `handleStart` (use a sentinel `errors.New` var). `handleSetInterval` calls `Stop()`, then `UpdateInterval` (which stops and starts), then `Start()` again, which is redundant.
4. **Rename** `botfixer/udpate.go` → `update.go`.

### Keeping as-is

- `github.com/joho/godotenv`: already latest, does one job.
- `github.com/go-playground/validator/v10`: after the update, fine.
- `github.com/tidwall/gjson`: core to API trackers.
- `github.com/gocolly/colly/v2`: after the update, fine.

---

## 7. Verification checklist

- [ ] `go build ./... && go vet ./...` and `golangci-lint run` pass locally and in CI.
- [ ] `docker build .` succeeds with the Go 1.27 image.
- [ ] Local run with the **dev** bot token while production keeps running: no 409 errors in either log, and each bot answers only its own chat.
- [ ] Every command and button works, including "<< Return" after a restart (no panic) and interval change via the custom keyboard.
- [ ] `go run -race .` session with trackers on a 1m interval for ~1 hour: no race reports.
- [ ] First production deploy: the log shows the old webhook being deleted and polling starting without 409s.
- [ ] Kill the container (`docker kill`): it comes back by itself, trackers resume, and a "restarted" message arrives.
- [ ] Reboot the home server: Docker and the bot come back without anyone logging in, and trackers resume.
- [ ] Unplug the server's network for a few minutes, plug it back in: the bot answers commands again without a restart (proves the client timeout works).
- [ ] Point a tracker at an endpoint that never responds (for example `nc -l`): it logs a timeout error within ~20s and keeps running on the next tick.
- [ ] A message from a chat not in `ALLOWED_CHAT_IDS` is ignored and logged.
- [ ] Leave production running a full week before treating the "stops after a few days" issue as closed.
