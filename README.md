# Price Tracker Telegram Bot

[![CodeQL Advanced](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/codeql.yml/badge.svg)](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/codeql.yml)
[![DevSkim](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/devskim.yml/badge.svg)](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/devskim.yml)
[![trivy](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/trivy.yml/badge.svg)](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/trivy.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=georgs-tumans_price-tracker-bot&metric=alert_status)](https://sonarcloud.io/dashboard?id=georgs-tumans_price-tracker-bot)


## About

A Telegram bot that can track prices of things and notify users upon these prices reaching certain criteria.

Tracking can be done using publicly available API for Single Page Applications or by scraping website HTML.

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

The bot runs as a Docker Compose service defined in [deployment/docker-compose.yml](/deployment/docker-compose.yml). The container restarts automatically after crashes and server reboots (`restart: unless-stopped`), as long as the Docker service itself starts on boot (`sudo systemctl enable docker`).

Deploying a new version:

```bash
git pull
./deployment/start.sh
```

[start.sh](/deployment/start.sh) rebuilds the image from the current code and (re)starts the container in the background. [stop.sh](/deployment/stop.sh) stops and removes the container:

```bash
./deployment/stop.sh
```

Follow the logs with `docker logs -f price_tracker_bot`.

The tracker state lives in the `bot-data` Docker volume, so it survives rebuilds. `stop.sh` keeps it; `docker compose down -v` would delete it.

The image is built `FROM scratch`: it contains only the bot binary, the trusted root certificates for HTTPS and the tracker configs. The bot runs as an unprivileged user (UID 65532) with a read-only filesystem; the `/data` volume is the only writable place.

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

The scripts can be run from any directory. The first time `start.sh` runs, it removes a leftover `price_tracker_bot` container created by the old `docker run` scripts, if there is one.


## Development

### Running locally while the server copy is running

Telegram only lets **one running copy of a bot receive updates**. If you start the bot locally with the same bot API key the server uses, the two copies fight over it: each one's requests get rejected with `Conflict: terminated by other getUpdates request` in the logs, commands randomly reach one copy or the other, and each copy's trackers send their own notifications.

The fix is to use a separate bot for development:

1. Message [@BotFather](https://t.me/BotFather) on Telegram, send `/newbot` and pick a name and username, e.g. `My Price Tracker (dev)` / `my_price_tracker_dev_bot`.
2. Put the new bot's API key in your **local** `.env` as `BOT_API_KEY`. Keep the production key only in the server's `.env`.
3. Chat with the dev bot while developing. Both copies can then run at the same time without interfering with each other.

### Running locally

Press `F5` in VS Code (launch profile included) or run `go run .` in the project root. Tracker state is saved to `data/state.json` (ignored by git).

To run it in a container locally instead: `docker compose -f deployment/docker-compose.yml up --build`.

### Linting and tests

The project uses golangci-lint v2 (config in [.golangci.yml](/.golangci.yml)); CI runs it together with `go test -race ./...` on pushes and pull requests to `develop`.

 - Run lint: `golangci-lint run`
 - Run lint and fix issues where possible: `golangci-lint run --fix`
 - Run tests: `go test ./...`

golangci-lint must be built with a Go version at least as new as the one in `go.mod`. If your local binary is older, run it through Docker instead:

```bash
docker run --rm -v "$(pwd):/src" -w /src golangci/golangci-lint:v2.13.2 golangci-lint run
```
