package helpers

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func SendMessageHTML(bot *tgbotapi.BotAPI, chatId int64, text string, entities []tgbotapi.MessageEntity) {
	msg := tgbotapi.NewMessage(chatId, text)
	if len(entities) > 0 {
		msg.Entities = entities
	}
	msg.ParseMode = tgbotapi.ModeHTML
	_, err := bot.Send(msg)

	if err != nil {
		log.Printf("[Bot fixer] Error sending a message: %s", err.Error())
	}

	log.Printf("[Bot fixer] Sent message to chat: %d; Message: %s", chatId, text)
}

func SendMessageHTMLWithMenu(bot *tgbotapi.BotAPI, chatId int64, text string, entities []tgbotapi.MessageEntity, menu tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatId, text)
	if len(entities) > 0 {
		msg.Entities = entities
	}

	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = menu

	_, err := bot.Send(msg)

	if err != nil {
		log.Printf("[Bot fixer] Error sending a message: %s", err.Error())
	}

	log.Printf("[Bot fixer] Sent message to chat: %d; Message: %s", chatId, text)
}

func EditMessageWithMenu(bot *tgbotapi.BotAPI, chatID int64, messageID int, text string, menu tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, text, menu)
	msg.ParseMode = tgbotapi.ModeHTML

	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("[Bot fixer] Error editing a message: %s", err.Error())
	}

	log.Printf("[Bot fixer] Edited existing message %d in chat %d; new message: %s", messageID, chatID, text)
}
