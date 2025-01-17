package botfixer

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *BotFixer) webhookHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[Bot fixer] Error reading request body: %v", err)
		http.Error(w, "Could not read request body", http.StatusBadRequest)

		return
	}

	// Parse the body as a Telegram update
	var update tgbotapi.Update
	if err := json.Unmarshal(body, &update); err != nil {
		log.Printf("[Bot fixer] Error parsing update: %v", err)
		http.Error(w, "Could not parse update", http.StatusBadRequest)

		return
	}

	// Handle the update
	b.handleUpdate(update)

	// Respond with a 200 OK status to Telegram
	w.WriteHeader(http.StatusOK)
}

func (b *BotFixer) longPollingHandler(ctx context.Context, updates tgbotapi.UpdatesChannel) {
	// `for {` means the loop is infinite until we manually stop it
	for {
		select {
		// stop looping if ctx is cancelled
		case <-ctx.Done():
			return
		// receive update from channel and then handle it
		case update := <-updates:
			b.handleUpdate(update)
		}
	}
}

func (b *BotFixer) handleUpdate(update tgbotapi.Update) {
	switch {
	// Handle messages
	case update.Message != nil:
		b.handleMessage(update.Message)

	// Handle button clicks
	case update.CallbackQuery != nil:
		b.handleButton(update.CallbackQuery)
	}
}

func (b *BotFixer) handleMessage(message *tgbotapi.Message) {
	user := message.From
	text := message.Text

	if user == nil {
		return
	}

	log.Printf("[Bot fixer] %s wrote %s", user.FirstName, text)

	// TODO switch to the tgbotapi methods for working with messages/commands - message.IsCommand(), message.CommandArguments(), etc.
	if message.IsCommand() {
		b.CommandHandler.GetUserNavigationState(message.Chat.ID).BackButtonEnabled = false
		if err := b.CommandHandler.HandleCommand(message.Chat.ID, text, nil, false); err != nil {
			log.Printf("[Bot fixer] An error occurred while handling command: %s", err.Error())

			return
		}

		return
	}

	// Handle user input after a certain command/action has requested it
	if b.CommandHandler.AwaitingUserInput {
		b.CommandHandler.GetUserNavigationState(message.Chat.ID).BackButtonEnabled = true
		if err := b.CommandHandler.HandleUserInput(message.Chat.ID, text, nil); err != nil {
			log.Printf("[Bot fixer] An error occurred while handling user input: %s", err.Error())

			return
		}

		return
	}
}

func (b *BotFixer) handleButton(query *tgbotapi.CallbackQuery) {
	command := query.Data
	b.CommandHandler.GetUserNavigationState(query.Message.Chat.ID).BackButtonEnabled = true

	if command == "back" {
		if err := b.CommandHandler.HandleReturn(query.Message.Chat.ID, &query.Message.MessageID); err != nil {
			log.Printf("[Bot fixer] An error occurred while handling button: %s", err.Error())

			return
		}

		return
	}

	if err := b.CommandHandler.HandleCommand(query.Message.Chat.ID, command, &query.Message.MessageID, false); err != nil {
		log.Printf("[Bot fixer] An error occurred while handling button: %s", err.Error())

		return
	}
}
