package bot_fixer

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
		if err := b.CommandHandler.HandleCommand(message.Chat.ID, text); err != nil {
			log.Printf("[Bot fixer] An error occured while handling command: %s", err.Error())

			return
		}
	}
}

func (b *BotFixer) handleButton(query *tgbotapi.CallbackQuery) {
	command := query.Data
	if err := b.CommandHandler.HandleCommand(query.Message.Chat.ID, command); err != nil {
		log.Printf("[Bot fixer] An error occured while handling button: %s", err.Error())

		return
	}
	// var text string
	// markup := tgbotapi.NewInlineKeyboardMarkup()
	// message := query.Message

	// if query.Data == nextButton {
	// 	text = secondMenu
	// 	markup = secondMenuMarkup
	// } else if query.Data == backButton {
	// 	text = firstMenu
	// 	markup = firstMenuMarkup
	// }

	// callbackCfg := tgbotapi.NewCallback(query.ID, "")
	// b.Bot.Send(callbackCfg)

	// // Replace menu text and keyboard
	// msg := tgbotapi.NewEditMessageTextAndMarkup(message.Chat.ID, message.MessageID, text, markup)
	// msg.ParseMode = tgbotapi.ModeHTML
	// b.Bot.Send(msg)
}
