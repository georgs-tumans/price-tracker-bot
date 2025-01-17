package helpers

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func SendMessageHTML(bot *tgbotapi.BotAPI, chatID int64, text string, entities []tgbotapi.MessageEntity) {
	msg := tgbotapi.NewMessage(chatID, text)
	if len(entities) > 0 {
		msg.Entities = entities
	}
	msg.ParseMode = tgbotapi.ModeHTML
	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("[Bot fixer] Error sending a message: %s", err.Error())
	}

	log.Printf("[Bot fixer] Sent message to chat: %d; Message: %s", chatID, text)
}

func SendMessageHTMLWithMenu(bot *tgbotapi.BotAPI, chatID int64, text string, entities []tgbotapi.MessageEntity, menu *tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	if len(entities) > 0 {
		msg.Entities = entities
	}

	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = menu

	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("[Bot fixer] Error sending a message: %s", err.Error())
	}

	log.Printf("[Bot fixer] Sent message to chat: %d; Message: %s", chatID, text)
}

func EditMessageWithMenu(bot *tgbotapi.BotAPI, chatID int64, messageID int, text string, menu *tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, text, *menu)
	msg.ParseMode = tgbotapi.ModeHTML

	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("[Bot fixer] Error editing a message: %s", err.Error())
	}

	log.Printf("[Bot fixer] Edited existing message %d in chat %d; new message: %s", messageID, chatID, text)
}

func SendMessageHTMLWithKeyboard(bot *tgbotapi.BotAPI, chatID int64, text string, entities []tgbotapi.MessageEntity, keyboard *tgbotapi.ReplyKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	if len(entities) > 0 {
		msg.Entities = entities
	}

	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard

	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("[Bot fixer] Error sending a message: %s", err.Error())
	}

	log.Printf("[Bot fixer] Sent message to chat: %d; Message: %s", chatID, text)
}

// The only way to remove a custom keyboard is to send a new text message (text cannot be empty) with a remove keyboard markup.
// Therefore we also attempt to immediately delete the message to avoid cluttering the chat.
func SendMessageRemoveKeyboard(bot *tgbotapi.BotAPI, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "processing input...")
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)

	sentMsg, err := bot.Send(msg)
	if err != nil {
		log.Printf("[Bot fixer] Error sending a remove keyboard message: %s", err.Error())
		return
	}

	log.Printf("[Bot fixer] Sent keyboard remove message to chat: %d", chatID)
	DeleteMessage(bot, chatID, sentMsg.MessageID)
}

func DeleteMessage(bot *tgbotapi.BotAPI, chatID int64, messageID int) {
	msg := tgbotapi.NewDeleteMessage(chatID, messageID)

	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("[Bot fixer] Error deleting a message: %s", err.Error())
	}

	log.Printf("[Bot fixer] Deleted message %d in chat %d", messageID, chatID)
}
