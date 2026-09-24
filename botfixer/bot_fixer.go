package botfixer

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"pricetrackerbot/config"
	"pricetrackerbot/handlers"
	"pricetrackerbot/helpers"
)

const (
	// How long a getUpdates request may take; Telegram is asked to hold it open for one second less.
	pollTimeout = 60 * time.Second
	// Extra time on top of the poll timeout before the HTTP client gives up on a request.
	pollTimeoutMargin = 30 * time.Second
	// Must be longer than the poll timeout, otherwise every idle poll would time out. Without any timeout,
	// a request on a silently dropped connection would hang forever and the bot would stop receiving updates.
	httpClientTimeout = pollTimeout + pollTimeoutMargin
)

type BotFixer struct {
	Bot            *bot.Bot
	Config         *config.Configuration
	CommandHandler *handlers.CommandHandler

	// The library runs each update handler in its own goroutine. Updates are handled one at a time,
	// as before, so commands from different chats can't interleave (e.g. two /run all at once).
	updateMu sync.Mutex
}

func NewBotFixer() *BotFixer {
	botFixer, err := newBotFixer(config.GetConfig())
	if err != nil {
		log.Panic(err)
	}

	return botFixer
}

// newBotFixer creates the bot; extra options let tests point it at a fake Telegram API server.
func newBotFixer(cfg *config.Configuration, extraOptions ...bot.Option) (*BotFixer, error) {
	botFixer := &BotFixer{
		Config: cfg,
	}

	options := append([]bot.Option{
		bot.WithDefaultHandler(botFixer.handleUpdate),
		bot.WithHTTPClient(pollTimeout, &http.Client{Timeout: httpClientTimeout}),
		bot.WithErrorsHandler(handleBotError),
	}, extraOptions...)

	var err error
	botFixer.Bot, err = bot.New(cfg.BotAPIKey, options...)
	if err != nil {
		return nil, err
	}

	botFixer.CommandHandler = handlers.NewCommandHandler(helpers.NewTelegramMessenger(botFixer.Bot), cfg)

	return botFixer, nil
}

// Run receives updates via long polling until the context is cancelled.
func (b *BotFixer) Run(ctx context.Context) {
	// Telegram rejects getUpdates while a webhook is registered, e.g. one left over from the old webhook mode
	if _, err := b.Bot.DeleteWebhook(ctx, &bot.DeleteWebhookParams{}); err != nil {
		log.Printf("[Bot fixer] Error deleting webhook: %v", err)
	} else {
		log.Println("[Bot fixer] Webhook deleted")
	}

	// Trackers run until they are stopped explicitly, independent of this context
	b.CommandHandler.ResumeTrackers() //nolint:contextcheck

	log.Println("[Bot fixer] Bot initialized via long polling; listening for updates")

	// Blocks until the context is cancelled; failed polls are retried by the library with a growing delay
	b.Bot.Start(ctx)

	log.Println("[Bot fixer] Shutting down")
}

func handleBotError(err error) {
	if errors.Is(err, bot.ErrorConflict) {
		log.Printf("[Bot fixer] Another instance is polling with this bot API key; use a separate bot for development: %v", err)
		return
	}

	log.Printf("[Bot fixer] Telegram API error: %v", err)
}
