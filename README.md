# Price Tracker Telegram Bot

[![CodeQL Advanced](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/codeql.yml/badge.svg)](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/codeql.yml)
[![DevSkim](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/devskim.yml/badge.svg)](https://github.com/georgs-tumans/price-tracker-bot/actions/workflows/devskim.yml)

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
- Docker installed (if you want to run this in a container)
- GO installed (if you want to run it as a regular console app)
- ngrok running for local development (if using the webhooks approach)

## Use

1. Register a bot with Telegram
2. Build and run this app
3. Use commands to interact with your new Telegram bot :)


## Initial setup

### The default approach

The default approach is when the bot determines whether to initialize with webhooks or long polling based on the value of the `ENVIROMENT` environment variable (see the Development section) upon starting.

In case of local development follow these steps:

1. Create an `.env` file; use this [example](/.env.example) to fill out the values.

2. Use [this](/docker_build_and_run.ps1) included powershell script to build (or rebuild) and run the bot as a Docker container.

Afterwards you can use [this other script](/docker_run.ps1) to run the container without rebuilding the image.

Also, you can press `F5` if using VS Code to run via a launch profile or just use the CMD command `go run main.go` in the root of the project.

In other cases, see below.


### Using the webhooks approach (for testing/modifying webhook initialization)

1. Run ngrok locally - you will need it for exposing localhost to the internet so that Telegram can reach the bot when running locally (during development). 

There is a [powershell script](/docker_run_ngrok.ps1) for hassle free setup of ngrok via Docker but in order to use it:

* Create an ngrok configuration file `ngrok.yml` based on this [template](./ngrok.yml.example)
* Edit the [script](/docker_run_ngrok.ps1) and set the location of the newly created `ngrok.yml`
* Run the script

You will need to know the ngrok generated URL that tunnels your locally run app to the internet - open `http://localhost:4040/status` in a browser to view the ngrok panel

2. Create an `.env` file; use this [example](/.env.example) to fill out the values.

3. Use [this](/docker_build_and_run.ps1) included powershell script to build (or rebuild) and run the bot as a Docker container.

Afterwards you can use [this other script](/docker_run.ps1) to run the container without rebuilding the image.


## Development

In order to develop and run the bot locally via you IDE you must set the environmental variable `ENVIROMENT` to `local`. When running the bot, this will result in automatic deletion of any webhooks registered for the given Telegram bot API key and switching to the long polling approach which works much better for local development.

However, if you need to run the bot in a container or host it somewhere, it is recommended to set the `ENVIROMENT` to `cloud`/`docker` which will the register a new webhook upon instantiation and use that for getting updates.

**NB**:
You cannot run the bot in the long polling mode while there are actively registered webhooks for the same bot API key!

### Linting

A golangci-lint configuration file is included, some useful commands to run in git bash:


 - Export lint result to a file: `golangci-lint run --out-format json > lint-results.json`
 - Run lint and fix issues where possible: `golangci-lint run --fix`


## Deployment

~~The bot is currently hosted on Google Apps; pushing to `master` triggers a build.~~
