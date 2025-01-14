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
	Type               string
	DescriptionGeneral string
	DescriptionTracker string
	Handler            CommandFunc
}

type CommandHandler struct {
	config          *config.Configuration
	runningTrackers []*Tracker
	commandMap      map[string]*Command
	bot             *tgbotapi.BotAPI
	mu              sync.Mutex
}

type CommandFunc func(code string, chatId int64, commandParam *string) error

func NewCommandHandler(bot *tgbotapi.BotAPI) *CommandHandler {
	ch := &CommandHandler{
		config: config.GetConfig(),
		bot:    bot,
	}

	ch.commandMap = map[string]*Command{
		"start":    {Type: bothType, DescriptionTracker: "Run a tracker", DescriptionGeneral: "Run all available trackers", Handler: ch.handleStart},
		"stop":     {Type: bothType, DescriptionTracker: "Stop a tracker", DescriptionGeneral: "Stop all running trackers", Handler: ch.handleStop},
		"interval": {Type: trackerType, DescriptionTracker: "Change the tracker run interval", Handler: ch.handleSetInterval},
		"status":   {Type: bothType, DescriptionTracker: "View a particular tracker status", DescriptionGeneral: "View status of all available trackers", Handler: ch.handleStatus},
		"help":     {Type: generalType, DescriptionGeneral: "View all available commands", Handler: ch.handleHelp},
	}

	return ch
}

func (ch *CommandHandler) HandleCommand(chatId int64, commandString string) error {
	// Some commands may have parameters that are separated by a space (/set_interval 5m)
	commandParts := strings.Split(commandString, " ")
	command := strings.ReplaceAll(commandParts[0], "/", "")
	var commandParam *string

	if len(commandParts) > 1 {
		commandParam = &commandParts[1]
	}

	/*Commands mostly consist of two words separated by an underscore;
	  the first word is the command code, the second word is the actual command.
	  There can be exceptions for commands that target all of the bot functionality instead of specific parts/clients.
	*/
	commandSplit := strings.Split(command, "_")
	commandFunction := command
	commandCode := ""
	if len(commandSplit) > 1 {
		commandCode = commandSplit[0]
		commandFunction = commandSplit[1]
	}

	log.Printf("[CommandHandler] Handling command: %s", commandString)

	if command, exists := ch.commandMap[commandFunction]; exists {
		if err := command.Handler(commandCode, chatId, commandParam); err != nil {
			return err
		}
	} else {
		log.Printf("[CommandHandler] Unknown command: %s", commandFunction)
		helpers.SendMessageHTML(ch.bot, chatId, "Unrecognized command", nil)

		return errors.New("unknown command")
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

			helpers.SendMessageHTML(ch.bot, chatId, builder.String(), nil)
		} else {
			helpers.SendMessageHTML(ch.bot, chatId, "All available trackers have been started", nil)
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

			helpers.SendMessageHTML(ch.bot, chatId, message, nil)

			return err
		}
		ch.AddRunningTracker(newTracker)
		newTracker.Start()

		helpers.SendMessageHTML(ch.bot, chatId, "Tracker '"+code+"' has been started", nil)
		log.Printf("[CommandHandler] Starting tracker: %s", code)
	} else {
		log.Printf("[CommandHandler] Tracker '%s' is already running", code)
		helpers.SendMessageHTML(ch.bot, chatId, "Tracker '"+code+"' is already running", nil)
	}

	return nil
}

func (ch *CommandHandler) handleStop(code string, chatId int64, commandParam *string) error {
	// Stop all trackers
	if code == "" {
		ch.StopAllTrackers()
		helpers.SendMessageHTML(ch.bot, chatId, "All running trackers have been stopped", nil)

		return nil
	}

	// Stop a specific tracker
	if tracker := ch.GetActiveTracker(code); tracker != nil {
		ch.RemoveRunningTracker(code)
		tracker.Stop()
		helpers.SendMessageHTML(ch.bot, chatId, "Tracker '"+code+"' has been stopped", nil)
	} else {
		log.Printf("[CommandHandler] Tracker '%s' is not running", code)
		helpers.SendMessageHTML(ch.bot, chatId, "Tracker '"+code+"' is not running", nil)
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
	if commandParam == nil {
		log.Printf("[CommandHandler] No interval value provided")
		helpers.SendMessageHTML(ch.bot, chatId, "No interval value provided", nil)

		return errors.New("no interval value provided")
	}

	newInterval, err := utilities.ParseDurationWithDays(*commandParam)
	if err != nil {
		log.Printf("[CommandHandler] Invalid interval value: %s", err.Error())
		helpers.SendMessageHTML(ch.bot, chatId, "Invalid interval value. Available interval types: 'm'(minute), 'h'(hour), 'd'(day)", nil)

		return err
	}

	if tracker := ch.GetActiveTracker(code); tracker != nil {
		tracker.Stop()
		tracker.UpdateInterval(newInterval)
		tracker.Start()
		log.Printf("[CommandHandler] Updated tracker '%s' interval to %s", code, newInterval)
		helpers.SendMessageHTML(ch.bot, chatId, "Tracker '"+code+"' run interval successfully updated to "+utilities.DurationToString(newInterval), nil)

		return nil
	} else {
		log.Printf("[CommandHandler] Tracker '%s' not found for interval update", code)
		helpers.SendMessageHTML(ch.bot, chatId, "Tracker '"+code+"' not found, it's probably not running", nil)

		return errors.New("tracker not found")
	}
}

func (ch *CommandHandler) handleStatus(code string, chatId int64, commandParam *string) error {
	if code == "" {
		// Handle the case when the user wants to see the status of all trackers
		var builder strings.Builder
		builder.WriteString("<b>All available trackers</b>\n\n")
		for _, tracker := range ch.config.APITrackers {
			activeStatus := "inactive"
			if ch.GetActiveTracker(tracker.Code) != nil {
				activeStatus = "active"
			}

			builder.WriteString(fmt.Sprintf("Tracker: %s | Status: %s | Type: API\n", tracker.Code, activeStatus))
		}

		for _, tracker := range ch.config.ScraperTrackers {
			activeStatus := "inactive"
			if ch.GetActiveTracker(tracker.Code) != nil {
				activeStatus = "active"
			}

			builder.WriteString(fmt.Sprintf("Tracker: %s | Status: %s | Type: Scraper\n", tracker.Code, activeStatus))
		}

		helpers.SendMessageHTML(ch.bot, chatId, builder.String(), nil)
		return nil
	}

	tracker := ch.GetActiveTracker(code)
	if tracker == nil {
		log.Printf("[CommandHandler] Tracker '%s' is not active", code)
		helpers.SendMessageHTML(ch.bot, chatId, "Tracker '"+code+"' is not active", nil)

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

	helpers.SendMessageHTML(ch.bot, chatId, builder.String(), nil)

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
	builder.WriteString("<b>Available general commands</b>\n\n")

	for command, cmd := range ch.commandMap {
		if cmd.Type == generalType || cmd.Type == bothType {
			builder.WriteString(fmt.Sprintf(" - /%s - %s\n", command, cmd.DescriptionGeneral))
		}
	}

	builder.WriteString("\n<b>Available tracker specific commands</b>\n\n")
	for command, cmd := range ch.commandMap {
		if cmd.Type == trackerType || cmd.Type == bothType {
			builder.WriteString(fmt.Sprintf(" - /%s - %s\n", command, cmd.DescriptionTracker))
		}
	}

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
