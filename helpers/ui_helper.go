package helpers

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

func GetReturnButtonMenu(existingMenu *tgbotapi.InlineKeyboardMarkup) *tgbotapi.InlineKeyboardMarkup {
	backButtonRow := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(" << Return", "back"),
	)

	// If the message being sent already has a menu, attach the back button to it otherwise create a new menu with the back button.
	if existingMenu != nil {
		existingMenu.InlineKeyboard = append(existingMenu.InlineKeyboard, backButtonRow)
		return existingMenu
	}

	backButtonMenu := tgbotapi.NewInlineKeyboardMarkup(
		backButtonRow,
	)

	return &backButtonMenu
}

func GetIntervalCustomMenu() *tgbotapi.ReplyKeyboardMarkup {
	customKeyboard := tgbotapi.NewOneTimeReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("10m"),
			tgbotapi.NewKeyboardButton("1h"),
			tgbotapi.NewKeyboardButton("1d"),
		),
	)

	return &customKeyboard
}

func GetStatusInlineKeyboard() *tgbotapi.InlineKeyboardMarkup {
	statusMenu := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Run all trackers", "/run"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Stop all trackers", "/stop"),
		),
	)

	return &statusMenu
}
