package botfixer

import (
	"context"
	"log"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"pricetrackerbot/config"
	"pricetrackerbot/handlers"
)

const (
	// How long Telegram holds a getUpdates request open while waiting for new updates
	pollTimeoutSeconds = 60
	// Must be longer than the poll timeout, otherwise every idle poll would time out. Without any timeout,
	// a request on a silently dropped connection would hang forever and the bot would stop receiving updates.
	httpClientTimeout = (pollTimeoutSeconds + 30) * time.Second
)

type BotFixer struct {
	Bot            *tgbotapi.BotAPI
	Config         *config.Configuration
	CommandHandler *handlers.CommandHandler
}

func NewBotFixer() *BotFixer {
	botFixer := &BotFixer{
		Config: config.GetConfig(),
	}

	var err error
	botFixer.Bot, err = tgbotapi.NewBotAPIWithClient(botFixer.Config.BotAPIKey, tgbotapi.APIEndpoint, &http.Client{Timeout: httpClientTimeout})
	if err != nil {
		log.Panic(err)
		return nil
	}

	botFixer.CommandHandler = handlers.NewCommandHandler(botFixer.Bot)

	return botFixer
}

// Run receives updates via long polling until the context is cancelled.
func (b *BotFixer) Run(ctx context.Context) {
	// Set this to true to log all interactions with telegram servers
	b.Bot.Debug = false

	// Telegram rejects getUpdates while a webhook is registered, e.g. one left over from the old webhook mode
	if err := b.deleteWebhook(); err != nil {
		log.Printf("[Bot fixer] Error deleting webhook: %v", err)
	}

	b.CommandHandler.ResumeTrackers()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = pollTimeoutSeconds

	// `updates` is a golang channel which receives telegram updates. If another instance polls with the same
	// bot API key, the library logs "Conflict: terminated by other getUpdates request" and keeps retrying.
	updates := b.Bot.GetUpdatesChan(u)

	log.Println("[Bot fixer] Bot initialized via long polling; listening for updates")

	for {
		select {
		case <-ctx.Done():
			b.Bot.StopReceivingUpdates()
			log.Println("[Bot fixer] Shutting down")

			return
		case update, ok := <-updates:
			if !ok {
				return
			}

			b.handleUpdate(update)
		}
	}
}

func (b *BotFixer) deleteWebhook() error {
	// Done through the library so the bot token never ends up in a logged request URL
	_, err := b.Bot.Request(tgbotapi.DeleteWebhookConfig{})
	if err != nil {
		return err
	}

	log.Println("[Bot fixer] Webhook deleted")

	return nil
}
