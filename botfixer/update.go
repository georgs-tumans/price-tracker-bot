package botfixer

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *BotFixer) handleUpdate(update tgbotapi.Update) {
	// A panic while handling one update must not take down the whole bot
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Bot fixer] Recovered from panic while handling update %d: %v", update.UpdateID, r)
		}
	}()

	if chat := updateChat(update); chat == nil || !b.Config.IsChatAllowed(chat.ID) {
		if chat != nil {
			log.Printf("[Bot fixer] Ignoring update from chat %d (not in ALLOWED_CHAT_IDS)", chat.ID)
		}

		return
	}

	switch {
	// Handle messages
	case update.Message != nil:
		b.handleMessage(update.Message)

	// Handle button clicks
	case update.CallbackQuery != nil:
		b.handleButton(update.CallbackQuery)
	}
}

// Returns the chat an update belongs to. Unlike tgbotapi's Update.FromChat() it does not panic
// on callback queries that have no message attached (e.g. from inline mode).
func updateChat(update tgbotapi.Update) *tgbotapi.Chat {
	switch {
	case update.Message != nil:
		return update.Message.Chat
	case update.CallbackQuery != nil && update.CallbackQuery.Message != nil:
		return update.CallbackQuery.Message.Chat
	default:
		return nil
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
	if b.CommandHandler.GetUserNavigationState(message.Chat.ID).AwaitingUserInput {
		b.CommandHandler.GetUserNavigationState(message.Chat.ID).BackButtonEnabled = true
		if err := b.CommandHandler.HandleUserInput(message.Chat.ID, text, nil); err != nil {
			log.Printf("[Bot fixer] An error occurred while handling user input: %s", err.Error())

			return
		}

		return
	}
}

func (b *BotFixer) handleButton(query *tgbotapi.CallbackQuery) {
	// Tells Telegram the click was received, otherwise the button keeps showing a loading spinner
	if _, err := b.Bot.Request(tgbotapi.NewCallback(query.ID, "")); err != nil {
		log.Printf("[Bot fixer] Error answering callback query: %s", err.Error())
	}

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
