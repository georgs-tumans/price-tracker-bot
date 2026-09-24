package botfixer

import (
	"context"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (b *BotFixer) handleUpdate(ctx context.Context, _ *bot.Bot, update *models.Update) {
	// A panic while handling one update must not take down the whole bot
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Bot fixer] Recovered from panic while handling update %d: %v", update.ID, r)
		}
	}()

	chatID, ok := updateChatID(update)
	if !ok {
		return
	}

	if !b.Config.IsChatAllowed(chatID) {
		log.Printf("[Bot fixer] Ignoring update from chat %d (not in ALLOWED_CHAT_IDS)", chatID)
		return
	}

	b.updateMu.Lock()
	defer b.updateMu.Unlock()

	switch {
	// Handle messages
	case update.Message != nil:
		b.handleMessage(update.Message)

	// Handle button clicks
	case update.CallbackQuery != nil:
		b.handleButton(ctx, update.CallbackQuery)
	}
}

// Returns the chat an update belongs to, if it's a kind of update the bot handles.
func updateChatID(update *models.Update) (int64, bool) {
	switch {
	case update.Message != nil:
		return update.Message.Chat.ID, true
	case update.CallbackQuery != nil:
		return callbackMessageRef(update.CallbackQuery)
	default:
		return 0, false
	}
}

// Returns the chat of the message a button belongs to. Telegram sends messages older than 48 hours as
// "inaccessible", with only the chat and message ID; those are enough here.
func callbackMessageRef(query *models.CallbackQuery) (int64, bool) {
	switch {
	case query.Message.Message != nil:
		return query.Message.Message.Chat.ID, true
	case query.Message.InaccessibleMessage != nil:
		return query.Message.InaccessibleMessage.Chat.ID, true
	default:
		return 0, false // e.g. buttons on inline mode messages, which this bot doesn't use
	}
}

func callbackMessageID(query *models.CallbackQuery) int {
	if query.Message.Message != nil {
		return query.Message.Message.ID
	}

	return query.Message.InaccessibleMessage.MessageID
}

// Whether the message starts with a bot command such as "/status".
func isCommand(message *models.Message) bool {
	return len(message.Entities) > 0 &&
		message.Entities[0].Type == models.MessageEntityTypeBotCommand &&
		message.Entities[0].Offset == 0
}

func (b *BotFixer) handleMessage(message *models.Message) {
	user := message.From
	text := message.Text
	chatID := message.Chat.ID

	if user == nil {
		return
	}

	log.Printf("[Bot fixer] %s wrote %s", user.FirstName, text)

	if isCommand(message) {
		b.CommandHandler.GetUserNavigationState(chatID).BackButtonEnabled = false
		if err := b.CommandHandler.HandleCommand(chatID, text, nil, false); err != nil {
			log.Printf("[Bot fixer] An error occurred while handling command: %s", err.Error())
		}

		return
	}

	// Handle user input after a certain command/action has requested it
	if b.CommandHandler.GetUserNavigationState(chatID).AwaitingUserInput {
		b.CommandHandler.GetUserNavigationState(chatID).BackButtonEnabled = true
		if err := b.CommandHandler.HandleUserInput(chatID, text, nil); err != nil {
			log.Printf("[Bot fixer] An error occurred while handling user input: %s", err.Error())
		}
	}
}

func (b *BotFixer) handleButton(ctx context.Context, query *models.CallbackQuery) {
	// Tells Telegram the click was received, otherwise the button keeps showing a loading spinner
	if _, err := b.Bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
		log.Printf("[Bot fixer] Error answering callback query: %s", err.Error())
	}

	chatID, _ := callbackMessageRef(query)
	messageID := callbackMessageID(query)
	command := query.Data
	b.CommandHandler.GetUserNavigationState(chatID).BackButtonEnabled = true

	if command == "back" {
		if err := b.CommandHandler.HandleReturn(chatID, &messageID); err != nil {
			log.Printf("[Bot fixer] An error occurred while handling button: %s", err.Error())
		}

		return
	}

	if err := b.CommandHandler.HandleCommand(chatID, command, &messageID, false); err != nil {
		log.Printf("[Bot fixer] An error occurred while handling button: %s", err.Error())
	}
}
