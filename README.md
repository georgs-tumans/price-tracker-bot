# Price Tracker Telegram Bot

[![CodeQL Advanced](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/codeql.yml/badge.svg)](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/codeql.yml)
[![DevSkim](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/devskim.yml/badge.svg)](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/devskim.yml)
[![trivy](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/trivy.yml/badge.svg)](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/trivy.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=georgs-tumans_price-tracker-bot&metric=alert_status)](https://sonarcloud.io/dashboard?id=georgs-tumans_price-tracker-bot)


## About

A Telegram bot that can track prices of things and notify users upon these prices reaching certain criteria.

Tracking can be done using publicly available API for Single Page Applications or by scraping website HTML.

## Migration

In September 2026 the bot went through a maintenance round: it now uses long polling instead of webhooks, survives restarts and network drops, runs in a minimal non-root Docker image, and uses up-to-date dependencies, including a new Telegram library. The full description of what changed and why is in **[MIGRATION_PLAN.md](/MIGRATION_PLAN.md)**.

If you're updating an installation from before that, the changes that need your attention are:

- **Configuration:** `WEBHOOK_URL`, `PORT` and `ENVIRONMENT` were removed; delete them from `.env`. `ALLOWED_CHAT_IDS` is new (see [Restricting who can use the bot](#restricting-who-can-use-the-bot)).
- **Deployment:** the server now pulls released images from `ghcr.io` instead of building locally. Use `./deployment/update.sh`, `start.sh` and `stop.sh` instead of the old `docker run` scripts (see [Deployment](#deployment-linux-server-docker-compose)). The first run replaces the old container.
- **Tracker configs** are mounted into the container from `tracker_configs/` on the server instead of being built into the image.
- **Local development:** use a separate dev bot so a local copy doesn't take updates away from the server copy (see [Running locally while the server copy is running](#running-locally-while-the-server-copy-is-running)).
- **Trackers:** start them once with `/run` after the first deploy; from then on they're resumed automatically after every restart.
- **Webhook infrastructure** (port forward, ngrok, tunnel) is no longer needed.

The step-by-step checklist is in the [to-do section](/MIGRATION_PLAN.md#2-to-do-before-and-after-deploying) of the migration document.

## Available tools/functionality

### Available bot commands:

General commands:
 - `/status` - prints status of the configured trackers
 - `/help` - prints all available commands
 - `/run` - runs all available trackers
 - `/stop` - stops all running trackers

 Tracker specific commands:
 - `/run <tracker_code>` - starts a tracker
 - `/stop <tracker_code>` - stops a tracker
 - `/status <tracker_code>` - prints tracker status
 - `/interval <tracker_code> <interval_value>` - sets tracker run interval. Example command: `/interval bonds 1h`. Available interval types: 'm'(minute), 'h'(hour), 'd'(day)

## Preconditions

- A Telegram bot API key which means you must register a bot. Learn how to do it [here](https://core.telegram.org/bots#how-do-i-create-a-bot).
- **A second bot for local development** (see [Running locally while the server copy is running](#running-locally-while-the-server-copy-is-running)).
- Docker with the compose plugin (for running on a server)
- Go installed (for running it locally as a regular console app)

## Use

1. Register a bot with Telegram
2. Build and run this app
3. Use commands to interact with your new Telegram bot :)


## How it works

The bot receives updates from Telegram via **long polling**: it keeps a request open to Telegram's servers and gets new messages as soon as they arrive. It only makes outgoing connections, so it needs no public URL, HTTPS certificate, port forwarding or tunnel, and it works the same on a home server as on a laptop.

Running trackers are saved to a state file (`STATE_FILE`). After a restart (crash, deploy, server reboot) the bot resumes them and sends a "the bot was restarted" message to the chats they belong to.

## Configuration

Create an `.env` file in the project root; use this [example](/.env.example) to fill out the values. Tracker configuration files are described [here](/tracker_configs/README.md).

### Restricting who can use the bot

By default the bot responds to anyone on Telegram who finds it. To limit it to your own chats, set `ALLOWED_CHAT_IDS` to a comma separated list of chat IDs:

```
ALLOWED_CHAT_IDS=12345678,87654321
```

Updates from any other chat are ignored and logged as `Ignoring update from chat <id> (not in ALLOWED_CHAT_IDS)`. The bot logs a warning on startup when the variable is empty.

To find your chat ID, set `ALLOWED_CHAT_IDS` to any placeholder value (e.g. `0`), send the bot a message and read the ID from that log line. For group chats the ID is negative.


## Deployment (Linux server, Docker Compose)

The bot is published as a Docker image on the GitHub Container Registry, `ghcr.io/georgs-tumans/price-tracker-bot`, and runs as a Docker Compose service defined in [deployment/docker-compose.yml](/deployment/docker-compose.yml). The server doesn't build anything: it pulls the released image.

The container restarts automatically after crashes and server reboots (`restart: unless-stopped`), as long as the Docker service itself starts on boot (`sudo systemctl enable docker`).

### Setting up the server

The server needs a checkout of this repository (for the compose file and scripts), an `.env` file in the project root, and the tracker configuration files in `tracker_configs/`:

```bash
git clone https://github.com/georgs-tumans/price-tracker-bot.git
cd price-tracker-bot
cp .env.example .env            # then fill in the values
# put api_trackers.json / scraper_trackers.json into tracker_configs/
./deployment/start.sh
```

### Scripts

| Script | What it does |
|---|---|
| [`./deployment/update.sh`](/deployment/update.sh) | Updates to the latest release: pulls the latest deployment files with `git pull`, pulls the image and recreates the container if the image changed. The bot is only down for a few seconds, and nothing happens if it's already up to date. |
| [`./deployment/start.sh`](/deployment/start.sh) | Starts the bot in the background (pulls the image first if it isn't on the server yet). |
| [`./deployment/stop.sh`](/deployment/stop.sh) | Stops and removes the container. The tracker state and the image are kept. |

The scripts can be run from any directory. They print the container status at the end; follow the logs with `docker logs -f price_tracker_bot`. The first line of the log shows the running version, e.g. `Starting bot service (version v1.2.0)`. On their first run, `start.sh` and `update.sh` remove a leftover `price_tracker_bot` container created by the old `docker run` scripts, if there is one.

### Versions and rollback

By default the server runs the `latest` image, which is the newest stable release. To run a specific version, set `IMAGE_TAG` in `.env` (without the `v`) and run `update.sh`:

```bash
IMAGE_TAG=1.2.0
```

Remove the line again to go back to `latest`. Every release is available as `1.2.3` and `1.2` (the newest patch of 1.2).

### Tracker configuration

The tracker configuration files are **not** part of the image: they're private, and the image is public. The compose file mounts the server's `tracker_configs/` directory into the container (read-only). After changing a tracker file, restart the bot with `docker restart price_tracker_bot`.

### Data

The tracker state lives in the `bot-data` Docker volume, so it survives updates. `stop.sh` keeps it; `docker compose down -v` would delete it.

The image is built `FROM scratch`: it contains only the bot binary and the trusted root certificates for HTTPS. The bot runs as an unprivileged user (UID 65532) with a read-only filesystem; the `/data` volume is the only writable place.

### Creating a release

1. Make sure the code you want to release is on `main` (or whichever branch you release from) and CI is green.
2. On GitHub, go to **Releases → Draft a new release**, create a new tag following [semantic versioning](https://semver.org/) (e.g. `v1.2.0`), add release notes and click **Publish release**.
3. The [Release image](/.github/workflows/release.yml) workflow builds the image for `linux/amd64` and `linux/arm64`, scans it with Trivy (a release with high or critical vulnerabilities is not published), and pushes it tagged `1.2.0`, `1.2` and `latest`. Pre-releases like `v1.3.0-rc1` don't get the `latest` tag.
4. On the server, run `./deployment/update.sh`.

### Troubleshooting (there is no shell in the container)

Because the image is empty apart from the bot, **`docker exec -it price_tracker_bot sh` does not work**: there is no shell, `ls`, `cat` or any other tool inside. Use these instead:

| To... | Run |
|---|---|
| See what the bot is doing | `docker logs -f price_tracker_bot` (add `--since 24h` to limit it) |
| Check whether it's running, restarts, uptime | `docker ps -a --filter name=price_tracker_bot` and `docker inspect -f '{{.RestartCount}} {{.State.StartedAt}}' price_tracker_bot` |
| Look at the saved tracker state | `docker cp price_tracker_bot:/data/state.json -` (prints it) or `docker cp price_tracker_bot:/data/state.json .` (copies it out) |
| Look around the state volume with real tools | `docker run --rm -it -v price-tracker-bot_bot-data:/data alpine sh` (a throwaway Alpine container with the same volume) |
| Check the environment the bot got | `docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' price_tracker_bot` (prints secrets too, so mind where you paste it) |
| Reset the saved trackers | `./deployment/stop.sh`, then `docker volume rm price-tracker-bot_bot-data`, then `./deployment/start.sh` |

If you really need a shell next to the bot, for example to test network access from its point of view, attach a throwaway container to its network namespace: `docker run --rm -it --network container:price_tracker_bot alpine sh`.


## Development

### Running locally while the server copy is running

Telegram only lets **one running copy of a bot receive updates**. If you start the bot locally with the same bot API key the server uses, the two copies fight over it: each one's requests get rejected with `Conflict: terminated by other getUpdates request` in the logs, commands randomly reach one copy or the other, and each copy's trackers send their own notifications.

The fix is to use a separate bot for development:

1. Message [@BotFather](https://t.me/BotFather) on Telegram, send `/newbot` and pick a name and username, e.g. `My Price Tracker (dev)` / `my_price_tracker_dev_bot`.
2. Put the new bot's API key in your **local** `.env` as `BOT_API_KEY`. Keep the production key only in the server's `.env`.
3. Chat with the dev bot while developing. Both copies can then run at the same time without interfering with each other.

### Running locally

Press `F5` in VS Code (launch profile included) or run `go run .` in the project root. Tracker state is saved to `data/state.json` (ignored by git).

To run it in a container built from your local source instead of a released image, use the local override file (from the `deployment` directory):

```bash
docker compose --env-file ../.env -f docker-compose.yml -f docker-compose.local.yml up -d --build
```

Stop it with `./stop.sh`.

### Linting and tests

The project uses golangci-lint v2 (config in [.golangci.yml](/.golangci.yml)); CI runs it together with `go test -race ./...` on pushes and pull requests to `develop`.

 - Run lint: `golangci-lint run`
 - Run lint and fix issues where possible: `golangci-lint run --fix`
 - Run tests: `go test ./...`

golangci-lint must be built with a Go version at least as new as the one in `go.mod`. If your local binary is older, run it through Docker instead:

```bash
docker run --rm -v "$(pwd):/src" -w /src golangci/golangci-lint:v2.13.2 golangci-lint run
```
