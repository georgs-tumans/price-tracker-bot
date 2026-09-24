package helpers

import (
	"context"
	"log"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Upper bound for a single send/edit/delete call, so a slow Telegram API cannot block a tracker or command for long.
const requestTimeout = 30 * time.Second

// Messenger sends and edits Telegram messages. Handlers and trackers depend on this interface
// instead of the Telegram library, which also lets tests replace it with a fake.
type Messenger interface {
	SendHTML(chatID int64, text string)
	SendHTMLWithMenu(chatID int64, text string, menu *models.InlineKeyboardMarkup)
	SendHTMLWithKeyboard(chatID int64, text string, keyboard *models.ReplyKeyboardMarkup)
	EditHTMLWithMenu(chatID int64, messageID int, text string, menu *models.InlineKeyboardMarkup)
	RemoveKeyboard(chatID int64)
}

// TelegramMessenger implements Messenger with the Telegram Bot API. Errors are logged, not returned:
// a failed message should not fail the command or tracker run that sent it.
type TelegramMessenger struct {
	bot *bot.Bot
}

func NewTelegramMessenger(b *bot.Bot) *TelegramMessenger {
	return &TelegramMessenger{bot: b}
}

func (m *TelegramMessenger) SendHTML(chatID int64, text string) {
	m.send(chatID, text, nil)
}

func (m *TelegramMessenger) SendHTMLWithMenu(chatID int64, text string, menu *models.InlineKeyboardMarkup) {
	// A nil pointer inside the ReplyMarkup interface would be sent as "null", so only set it when there is a menu
	if menu == nil {
		m.send(chatID, text, nil)
		return
	}

	m.send(chatID, text, menu)
}

func (m *TelegramMessenger) SendHTMLWithKeyboard(chatID int64, text string, keyboard *models.ReplyKeyboardMarkup) {
	if keyboard == nil {
		m.send(chatID, text, nil)
		return
	}

	m.send(chatID, text, keyboard)
}

func (m *TelegramMessenger) send(chatID int64, text string, markup models.ReplyMarkup) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	_, err := m.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: markup,
	})
	if err != nil {
		log.Printf("[Bot fixer] Error sending a message: %s", err.Error())
		return
	}

	log.Printf("[Bot fixer] Sent message to chat: %d; Message: %s", chatID, text)
}

func (m *TelegramMessenger) EditHTMLWithMenu(chatID int64, messageID int, text string, menu *models.InlineKeyboardMarkup) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	params := &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}
	if menu != nil {
		params.ReplyMarkup = menu
	}

	if _, err := m.bot.EditMessageText(ctx, params); err != nil {
		log.Printf("[Bot fixer] Error editing a message: %s", err.Error())
		return
	}

	log.Printf("[Bot fixer] Edited existing message %d in chat %d; new message: %s", messageID, chatID, text)
}

// RemoveKeyboard hides a custom reply keyboard. The only way to do that is to send a new text message
// (text cannot be empty) with a remove keyboard markup, so the message is deleted again right away to avoid
// cluttering the chat.
func (m *TelegramMessenger) RemoveKeyboard(chatID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	sentMsg, err := m.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "processing input...",
		ReplyMarkup: &models.ReplyKeyboardRemove{RemoveKeyboard: true},
	})
	if err != nil {
		log.Printf("[Bot fixer] Error sending a remove keyboard message: %s", err.Error())
		return
	}

	log.Printf("[Bot fixer] Sent keyboard remove message to chat: %d", chatID)

	if _, err := m.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: sentMsg.ID}); err != nil {
		log.Printf("[Bot fixer] Error deleting a message: %s", err.Error())
		return
	}

	log.Printf("[Bot fixer] Deleted message %d in chat %d", sentMsg.ID, chatID)
}
