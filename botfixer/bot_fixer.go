package botfixer

import (
	"context"
	"log"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"pricetrackerbot/config"
	"pricetrackerbot/handlers"
	"pricetrackerbot/services"
)

const webhookEndpoint = "/webhook"

type BotFixer struct {
	Bot               *tgbotapi.BotAPI
	Config            *config.Configuration
	BondsClientActive bool
	CommandHandler    *handlers.CommandHandler
	TelegramBotAPI    string
}

func NewBotFixer() *BotFixer {
	botFixer := &BotFixer{
		Config:         config.GetConfig(),
		TelegramBotAPI: "https://api.telegram.org/bot",
	}

	var err error
	botFixer.Bot, err = tgbotapi.NewBotAPI(botFixer.Config.BotAPIKey)
	if err != nil {
		log.Panic(err)
		return nil
	}

	botFixer.CommandHandler = handlers.NewCommandHandler(botFixer.Bot)

	return botFixer
}

func (b *BotFixer) InitializeBotLongPolling() {
	// Set this to true to log all interactions with telegram servers
	b.Bot.Debug = false

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Create a new cancellable background context. Calling `cancel()` leads to the cancellation of the context
	ctx := context.Background()

	// `updates` is a golang channel which receives telegram updates
	updates := b.Bot.GetUpdatesChan(u)

	// Pass cancellable context to goroutine
	go b.longPollingHandler(ctx, updates)

	// Tell the user the bot is online
	log.Println("[Bot fixer] Bot initialized via the long polling approach; listening for updates")

	select {}
}

func (b *BotFixer) InitializeBotWebhook() {
	// Set this to true to log all interactions with telegram servers
	b.Bot.Debug = false

	wh, err := tgbotapi.NewWebhook(b.Config.WebhookURL + webhookEndpoint)
	if err != nil {
		log.Fatalf("[Bot fixer] Error creating webhook: %v", err)
	}

	_, err = b.Bot.Request(wh)
	if err != nil {
		log.Fatalf("[Bot fixer] Error setting webhook: %v", err)
	}

	log.Printf("[Bot fixer] Webhook set: %s", b.Config.WebhookURL+webhookEndpoint)

	http.HandleFunc(webhookEndpoint, b.webhookHandler)

	log.Println("[Bot fixer] Starting server on port " + b.Config.Port)
	log.Println("[Bot fixer] Bot initialized via the webhook approach; listening for updates")

	//nolint:mnd
	srv := &http.Server{
		Addr:              ":" + b.Config.Port,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           nil,
	}
	log.Fatal(srv.ListenAndServe())
}

func (b *BotFixer) DeleteWebhook() error {
	url := b.TelegramBotAPI + b.Config.BotAPIKey + "/deleteWebhook"
	_, err := services.GetRequest(url)
	if err != nil {
		log.Fatalf("[Bot fixer] Error deleting webhook: %v", err)
		return err
	}

	log.Println("[Bot fixer] Webhook deleted")

	return nil
}
