package handlers

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"web_scraper_bot/config"
	"web_scraper_bot/helpers"
	"web_scraper_bot/utilities"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	generalType = "general"
	trackerType = "tracker"
	bothType    = "both"
)

type Command struct {
	Command            string
	Type               string
	DescriptionGeneral string
	DescriptionTracker string
	Hidden             bool // Whether the command shows up in the help menu
	Params             []string
	Handler            CommandFunc
}

type CommandHandler struct {
	AwaitingUserInput    bool
	CustomKeyboardActive bool
	config               *config.Configuration
	runningTrackers      []*Tracker
	commandMap           map[string]*Command
	bot                  *tgbotapi.BotAPI
	mu                   sync.Mutex
	Navigation           map[int64]*NavigationState
}

type CommandFunc func(code string, chatId int64, commandParam *string) error

func NewCommandHandler(bot *tgbotapi.BotAPI) *CommandHandler {
	ch := &CommandHandler{
		config:               config.GetConfig(),
		bot:                  bot,
		AwaitingUserInput:    false,
		CustomKeyboardActive: false,
		Navigation:           make(map[int64]*NavigationState),
	}

	ch.commandMap = map[string]*Command{
		"start":    {Type: generalType, DescriptionGeneral: "Bot start command", Handler: ch.handleHelp, Hidden: true, Params: []string{"tracker_code"}},
		"run":      {Type: bothType, DescriptionTracker: "Run a tracker", DescriptionGeneral: "Run all available trackers", Handler: ch.handleStart, Hidden: false, Params: []string{"tracker_code"}},
		"stop":     {Type: bothType, DescriptionTracker: "Stop a tracker", DescriptionGeneral: "Stop all running trackers", Handler: ch.handleStop, Hidden: false, Params: []string{"tracker_code"}},
		"interval": {Type: trackerType, DescriptionTracker: "Change the tracker run interval", Handler: ch.handleSetInterval, Hidden: false, Params: []string{"tracker_code", "interval*"}},
		"status":   {Type: bothType, DescriptionTracker: "View a particular tracker status", DescriptionGeneral: "View status of all available trackers", Handler: ch.handleStatus, Hidden: false, Params: []string{"tracker_code"}},
		"help":     {Type: generalType, DescriptionGeneral: "View all available commands", Handler: ch.handleHelp, Hidden: false},
	}

	return ch
}

func (ch *CommandHandler) HandleCommand(chatId int64, commandString string, callbackMessageID *int, isReturn bool) error {
	// Message ID is only available when handling commands as a result of a button callback
	ch.GetUserNavigationState(chatId).CallbackMessageID = callbackMessageID

	// Most commands have one parameter - tracker code - but it is possible that some may have more
	commandParts := strings.Split(commandString, " ")
	command := strings.ReplaceAll(commandParts[0], "/", "")
	var trackerCode, commandParam *string

	if len(commandParts) > 1 {
		trackerCode = &commandParts[1]
	}

	if len(commandParts) > 2 {
		commandParam = &commandParts[2]
	}

	log.Printf("[CommandHandler] Handling command: %s", commandString)

	if c, exists := ch.commandMap[command]; exists {
		if !isReturn {
			ch.GetUserNavigationState(chatId).Push(
				&Command{
					Command: command,
					Params:  []string{utilities.GetStringPointerValue(trackerCode), utilities.GetStringPointerValue(commandParam)},
				},
			)
		}

		if err := c.Handler(utilities.GetStringPointerValue(trackerCode), chatId, commandParam); err != nil {
			return err
		}

	} else {
		log.Printf("[CommandHandler] Unknown command: %s", command)
		helpers.SendMessageHTML(ch.bot, chatId, "Unrecognized command", nil)

		return errors.New("unknown command")
	}

	return nil
}

func (ch *CommandHandler) HandleReturn(chatId int64, callbackMessageID *int) error {
	ch.GetUserNavigationState(chatId).Pop()
	gotoCommand := ch.GetUserNavigationState(chatId).Peek()

	commandString := "/" + gotoCommand.Command
	if len(gotoCommand.Params) > 0 {
		commandString += " " + strings.Join(gotoCommand.Params, " ")
	}

	if err := ch.HandleCommand(chatId, commandString, callbackMessageID, true); err != nil {
		return err
	}

	return nil
}

func (ch *CommandHandler) HandleUserInput(chatId int64, userInput string, callbackMessageID *int) error {
	ch.AwaitingUserInput = false
	// Hide keyboard after user input
	if ch.CustomKeyboardActive {
		helpers.SendMessageRemoveKeyboard(ch.bot, chatId)
		ch.CustomKeyboardActive = false
	}

	gotoCommand := ch.GetUserNavigationState(chatId).Peek()
	commandString := "/" + gotoCommand.Command
	if len(gotoCommand.Params) > 0 {
		commandString += " " + strings.Join(gotoCommand.Params, " ")
	}

	commandString = strings.TrimSpace(commandString) + " " + userInput

	if err := ch.HandleCommand(chatId, commandString, callbackMessageID, true); err != nil {
		return err
	}

	return nil
}

// TODO: implement interval setting here
func (ch *CommandHandler) handleStart(code string, chatId int64, commandParam *string) error {
	// Start all trackers
	if code == "" {
		var errors map[string]error = make(map[string]error)

		for _, tracker := range ch.config.APITrackers {
			if tr := ch.GetActiveTracker(tracker.Code); tr == nil {
				if newTracker, err := CreateTracker(ch.bot, tracker.Code, 0, ch.config, chatId); err != nil {
					errors[tracker.Code] = err
				} else {
					ch.AddRunningTracker(newTracker)
					newTracker.Start()
					log.Printf("[CommandHandler] Starting tracker: %s", newTracker.Code)
				}
			}
		}

		for _, tracker := range ch.config.ScraperTrackers {
			if tr := ch.GetActiveTracker(tracker.Code); tr == nil {
				if newTracker, err := CreateTracker(ch.bot, tracker.Code, 0, ch.config, chatId); err != nil {
					errors[tracker.Code] = err
				} else {
					ch.AddRunningTracker(newTracker)
					newTracker.Start()
					log.Printf("[CommandHandler] Starting tracker: %s", newTracker.Code)
				}
			}
		}

		if len(errors) > 0 {
			var builder strings.Builder
			builder.WriteString("Failed to start the following trackers:\n")
			for code, err := range errors {
				builder.WriteString(fmt.Sprintf(" - %s: %s\n", code, err.Error()))
			}

			ch.handleCommandMessage(chatId, builder.String(), nil)
		} else {
			ch.handleCommandMessage(chatId, "All available trackers have been started", nil)
		}

		return nil
	}

	// Start a specific tracker
	if tracker := ch.GetActiveTracker(code); tracker == nil {
		newTracker, err := CreateTracker(ch.bot, code, 0, ch.config, chatId)
		if err != nil {
			log.Printf("[CommandHandler] Error creating a new tracker: %s", code)
			message := "Failed to start the tracker :("
			if err.Error() == "uncregonzied tracker code" {
				message = "Invalid command, tracker with code '" + code + "' not found :("
			}

			ch.handleCommandMessage(chatId, message, nil)

			return err
		}
		ch.AddRunningTracker(newTracker)
		newTracker.Start()

		ch.handleCommandMessage(chatId, "Tracker '"+code+"' has been started", nil)
		log.Printf("[CommandHandler] Starting tracker: %s", code)
	} else {
		log.Printf("[CommandHandler] Tracker '%s' is already running", code)
		ch.handleCommandMessage(chatId, "Tracker '"+code+"' is already running", nil)
	}

	return nil
}

func (ch *CommandHandler) handleStop(code string, chatId int64, commandParam *string) error {
	// Stop all trackers
	if code == "" {
		ch.StopAllTrackers()
		ch.handleCommandMessage(chatId, "All running trackers have been stopped", nil)

		return nil
	}

	// Stop a specific tracker
	if tracker := ch.GetActiveTracker(code); tracker != nil {
		ch.RemoveRunningTracker(code)
		tracker.Stop()
		ch.handleCommandMessage(chatId, "Tracker '"+code+"' has been stopped", nil)
	} else {
		log.Printf("[CommandHandler] Tracker '%s' is not running", code)
		ch.handleCommandMessage(chatId, "Tracker '"+code+"' is not running", nil)
	}

	return nil
}

func (ch *CommandHandler) StopAllTrackers() {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	for _, tracker := range ch.runningTrackers {
		tracker.Stop()
	}
	ch.runningTrackers = nil
}

func (ch *CommandHandler) handleSetInterval(code string, chatId int64, commandParam *string) error {
	// Means command was initiated from a menu with a button
	if ch.GetUserNavigationState(chatId).CallbackMessageID != nil {
		ch.CustomKeyboardActive = true

		helpers.SendMessageHTMLWithKeyboard(
			ch.bot,
			chatId, "Send me the new interval value!\n\nThe format: <i>[number][interval type*]</i>\n\nAvailable interval types: \n'm'(minute), 'h'(hour), 'd'(day)",
			nil,
			helpers.GetIntervalCustomMenu(),
		)
		ch.AwaitingUserInput = true

		return nil
	}

	if commandParam == nil {
		log.Printf("[CommandHandler] No interval value provided")
		ch.handleCommandMessage(chatId, "No interval value provided", nil)

		return errors.New("no interval value provided")
	}

	newInterval, err := utilities.ParseDurationWithDays(*commandParam)
	if err != nil {
		log.Printf("[CommandHandler] Invalid interval value: %s", err.Error())
		ch.handleCommandMessage(chatId, "Invalid interval value. Available interval types: 'm'(minute), 'h'(hour), 'd'(day)", nil)

		return err
	}

	if tracker := ch.GetActiveTracker(code); tracker != nil {
		tracker.Stop()
		tracker.UpdateInterval(newInterval)
		tracker.Start()
		log.Printf("[CommandHandler] Updated tracker <b>%s</b> interval to %s", code, newInterval)
		ch.handleCommandMessage(chatId, "Tracker <b>"+code+"</b> run interval successfully updated to "+utilities.DurationToString(newInterval), nil)

		return nil
	} else {
		log.Printf("[CommandHandler] Tracker '%s' not found for interval update", code)
		ch.handleCommandMessage(chatId, "Tracker <b>"+code+"</b> not found, it's probably not running", nil)

		return errors.New("tracker not found")
	}
}

func (ch *CommandHandler) handleStatus(code string, chatId int64, commandParam *string) error {
	// Handle the case when the user wants to see the status of all trackers
	if code == "" {
		var statusMenu = helpers.GetStatusInlineKeyboard()

		var builder strings.Builder
		builder.WriteString("<b>All available trackers</b>\n\n")
		for _, tracker := range ch.config.APITrackers {
			activeStatus := ch.processTrackerStatus(tracker, statusMenu)
			builder.WriteString(fmt.Sprintf(" - %s | %s | api\n", tracker.Code, activeStatus))
		}

		for _, tracker := range ch.config.ScraperTrackers {
			activeStatus := ch.processTrackerStatus(tracker, statusMenu)
			builder.WriteString(fmt.Sprintf(" - %s | %s | scraper\n", tracker.Code, activeStatus))
		}

		// If we are navigating back to the status menu after a back button click, edit the existing message instead of sending a new one.
		// New message is sent if the status menu is invoked by a written command meaning we are not returning from a back button click.
		if ch.GetUserNavigationState(chatId).CallbackMessageID != nil {
			helpers.EditMessageWithMenu(ch.bot, chatId, *ch.GetUserNavigationState(chatId).CallbackMessageID, builder.String(), statusMenu)
		} else {
			helpers.SendMessageHTMLWithMenu(ch.bot, chatId, builder.String(), nil, statusMenu)
		}

		return nil
	}

	var statusMenu = tgbotapi.NewInlineKeyboardMarkup()

	tracker := ch.GetActiveTracker(code)
	if tracker == nil {
		log.Printf("[CommandHandler] Tracker '%s' is not active", code)
		statusMenu.InlineKeyboard = append(statusMenu.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Run tracker", "/run "+code),
		))
		ch.handleCommandMessage(chatId, "Tracker '"+code+"' is not active", &statusMenu)

		return errors.New("tracker not found")
	}

	lastRun := ""
	if tracker.Status.LastRunTimestamp.IsZero() {
		lastRun = "never"
	} else {
		lastRun = tracker.Status.LastRunTimestamp.Format("02.01.2006 15:04")
	}

	lastRecordedValue := tracker.Status.LastRecordedValue
	if lastRecordedValue == "" {
		lastRecordedValue = "none"
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("<b>Status for tracker %s</b>\n\n", code))
	builder.WriteString("Status: active\n")
	builder.WriteString("Tracker started: " + tracker.Status.StartTimestamp.Format("02.01.2006 15:04") + "\n")
	builder.WriteString("Last run: " + lastRun + "\n")
	builder.WriteString("Total runs: " + strconv.Itoa(tracker.Status.TotalRuns) + "\n")
	builder.WriteString("Last recorded value: " + lastRecordedValue + "\n")
	builder.WriteString(helpers.FormatNotificationCriteriaString(tracker.trackerData.NotifyCriteria) + "\n")
	builder.WriteString("Current run interval: " + utilities.DurationToString(tracker.Status.CurrentInterval) + "\n")
	builder.WriteString("Execution errors count: " + strconv.Itoa(len(tracker.Status.ExecutionErrors)) + "\n")

	statusMenu.InlineKeyboard = append(statusMenu.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Stop tracker", "/stop "+code),
	))
	statusMenu.InlineKeyboard = append(statusMenu.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Change run interval", "/interval "+code),
	))

	ch.handleCommandMessage(chatId, builder.String(), &statusMenu)

	return nil
}

func (ch *CommandHandler) handleHelp(code string, chatId int64, commandParam *string) error {
	// Command only available generally for all trackers
	if code != "" {
		log.Printf("[CommandHandler] code passed to the general-only /help command")
		helpers.SendMessageHTML(ch.bot, chatId, "/help is a general command not specific to any trackers", nil)

		return nil
	}

	var builder strings.Builder
	builder.WriteString("<b>Welcome to the bot help section!</b>\n")
	builder.WriteString("You can use the bot to manage the available trackers. There are two types of commands:\n\n")
	builder.WriteString("<b>Available general commands</b>\n")
	builder.WriteString("These are also available from the menu button and they do not accept parameters\n\n")

	for command, cmd := range ch.commandMap {
		if !cmd.Hidden && (cmd.Type == generalType || cmd.Type == bothType) {
			builder.WriteString(fmt.Sprintf(" - /%s - %s\n", command, cmd.DescriptionGeneral))
		}
	}

	builder.WriteString("\n<b>Available tracker specific commands</b>\n")
	builder.WriteString("These require at minimum one parameter - tracker code\n\n")
	for command, cmd := range ch.commandMap {
		if !cmd.Hidden && (cmd.Type == trackerType || cmd.Type == bothType) {
			builder.WriteString(formatCommandWithParams(command, cmd.Params, cmd.DescriptionTracker) + "\n")
		}
	}

	builder.WriteString("\n<b>*</b>Interval parameter format: \n<i>[number][interval type]</i> (e.g. 5m, 1h, 2d)\n")
	builder.WriteString("\nAvailable interval types: \n'm'(minute), 'h'(hour), 'd'(day)\n")

	helpers.SendMessageHTML(ch.bot, chatId, builder.String(), nil)

	return nil
}

/******************Utility******************/

func (ch *CommandHandler) GetActiveTracker(trackerCode string) *Tracker {
	// So that only one goroutine can access the trackers at a time
	ch.mu.Lock()
	defer ch.mu.Unlock()

	for _, tracker := range ch.runningTrackers {
		if tracker.Code == trackerCode {
			return tracker
		}
	}

	return nil
}

func (ch *CommandHandler) AddRunningTracker(tracker *Tracker) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.runningTrackers = append(ch.runningTrackers, tracker)
}

func (ch *CommandHandler) RemoveRunningTracker(trackerCode string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	for i, tracker := range ch.runningTrackers {
		if tracker.Code == trackerCode {
			ch.runningTrackers = append(ch.runningTrackers[:i], ch.runningTrackers[i+1:]...)
			return
		}
	}
}

func formatCommandWithParams(command string, params []string, description string) string {
	var builder strings.Builder
	builder.WriteString(" - /" + command)

	for _, param := range params {
		builder.WriteString(" &lt;" + param + "&gt;")
	}

	builder.WriteString("\n   " + description)

	return builder.String()
}

func (ch *CommandHandler) processTrackerStatus(tracker *config.Tracker, menu *tgbotapi.InlineKeyboardMarkup) string {
	menuRow := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Status ["+tracker.Code+"]", "/status "+tracker.Code),
	)

	var activeStatus string
	if ch.GetActiveTracker(tracker.Code) != nil {
		activeStatus = "active"
		menuRow = append(menuRow, tgbotapi.NewInlineKeyboardButtonData("Stop ["+tracker.Code+"]", "/stop "+tracker.Code))
	} else {
		activeStatus = "inactive"
		menuRow = append(menuRow, tgbotapi.NewInlineKeyboardButtonData("Start ["+tracker.Code+"]", "/run "+tracker.Code))
	}

	menu.InlineKeyboard = append(menu.InlineKeyboard, menuRow)

	return activeStatus
}

func (ch *CommandHandler) handleCommandMessage(chatId int64, message string, menu *tgbotapi.InlineKeyboardMarkup) {
	if ch.GetUserNavigationState(chatId).BackButtonEnabled {
		menu = helpers.GetReturnButtonMenu(menu)

		// If the message was sent as a result of a button click, edit the existing message instead of sending a new one.
		callbackMessageID := ch.GetUserNavigationState(chatId).CallbackMessageID
		if callbackMessageID == nil {
			helpers.SendMessageHTMLWithMenu(ch.bot, chatId, message, nil, menu)
			return
		}

		helpers.EditMessageWithMenu(ch.bot, chatId, *callbackMessageID, message, menu)

		return
	}

	if menu != nil {
		helpers.SendMessageHTMLWithMenu(ch.bot, chatId, message, nil, menu)
		return
	}

	helpers.SendMessageHTML(ch.bot, chatId, message, nil)
}

func (ch *CommandHandler) GetUserNavigationState(chatID int64) *NavigationState {
	if _, exists := ch.Navigation[chatID]; !exists {
		ch.Navigation[chatID] = &NavigationState{}
	}

	return ch.Navigation[chatID]
}
