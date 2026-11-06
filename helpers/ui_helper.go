package helpers

import "github.com/go-telegram/bot/models"

// InlineButton creates an inline keyboard button that sends the given callback data when clicked.
func InlineButton(text, callbackData string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: text, CallbackData: callbackData}
}

func GetReturnButtonMenu(existingMenu *models.InlineKeyboardMarkup) *models.InlineKeyboardMarkup {
	backButtonRow := []models.InlineKeyboardButton{InlineButton(" << Return", "back")}

	// If the message being sent already has a menu, attach the back button to it otherwise create a new menu with the back button.
	if existingMenu != nil {
		existingMenu.InlineKeyboard = append(existingMenu.InlineKeyboard, backButtonRow)
		return existingMenu
	}

	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{backButtonRow}}
}

func GetIntervalCustomMenu() *models.ReplyKeyboardMarkup {
	return &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{{Text: "10m"}, {Text: "1h"}, {Text: "1d"}},
		},
		OneTimeKeyboard: true,
		ResizeKeyboard:  true,
	}
}

// NotificationSourceParam marks a /mute or /unmute button that sits on a notification message.
const NotificationSourceParam = "n"

// GetNotificationMenu returns the single pause/resume button shown on tracker notification messages.
func GetNotificationMenu(code string, paused bool) *models.InlineKeyboardMarkup {
	button := InlineButton("Pause notifications", "/mute "+code+" "+NotificationSourceParam)
	if paused {
		button = InlineButton("Resume notifications", "/unmute "+code+" "+NotificationSourceParam)
	}

	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{button}}}
}

func GetStatusInlineKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{InlineButton("Run all trackers", "/run")},
			{InlineButton("Stop all trackers", "/stop")},
		},
	}
}
