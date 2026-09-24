package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
)

type NotifyCriteria struct {
	Operator string `json:"operator" validate:"required,oneof='<=' '<' '=' '>=' '>'"`
	Value    string `json:"value" validate:"required,numeric"`
}

type Tracker struct {
	Code               string           `json:"code" validate:"required,excludesall=_/ "`
	DataURL            string           `json:"dataUrl" validate:"required,url"`
	ViewURL            string           `json:"viewUrl" validate:"omitempty,url"`
	Interval           string           `json:"interval" validate:"required"`
	NotifyCriteria     []NotifyCriteria `json:"notifyCriteria" validate:"dive"`
	DataExtractionPath string           `json:"dataExtractionPath" validate:"required"`
}

const defaultStateFile = "data/state.json"

type Configuration struct {
	BotAPIKey        string `validate:"required"`
	ErrorNotifyLimit int    `validate:"min=1"`
	AllowedChatIDs   []int64
	StateFile        string     `validate:"required"`
	APITrackers      []*Tracker `validate:"dive"`
	ScraperTrackers  []*Tracker `validate:"dive"`
}

var config *Configuration

func GetConfig() *Configuration {
	if config == nil {
		log.Println("[Config] Loading configuration")
		err := godotenv.Load()
		if err != nil {
			log.Println("[GetConfig] No .env file loaded, using environment variables only")
		}

		config = &Configuration{
			BotAPIKey: os.Getenv("BOT_API_KEY"),
			StateFile: os.Getenv("STATE_FILE"),
		}

		if config.StateFile == "" {
			config.StateFile = defaultStateFile
		}

		errorLimit := os.Getenv("ERROR_NOTIFY_LIMIT")
		if errorLimit != "" {
			config.ErrorNotifyLimit, err = strconv.Atoi(errorLimit)
			if err != nil {
				log.Fatalf("[GetConfig] Error parsing ERROR_NOTIFY_LIMIT: %v", err)
			}
		} else {
			config.ErrorNotifyLimit = 3
		}

		config.AllowedChatIDs, err = parseChatIDs(os.Getenv("ALLOWED_CHAT_IDS"))
		if err != nil {
			log.Fatalf("[GetConfig] Error parsing ALLOWED_CHAT_IDS: %v", err)
		}

		if len(config.AllowedChatIDs) == 0 {
			log.Println("[GetConfig] WARNING: ALLOWED_CHAT_IDS is not set; the bot will accept commands from any chat")
		}

		config.APITrackers, err = loadTrackers("API_TRACKERS_FILE")
		if err != nil {
			log.Fatalf("[GetConfig] Error loading API trackers: %v", err)
		}

		config.ScraperTrackers, err = loadTrackers("SCRAPER_TRACKERS_FILE")
		if err != nil {
			log.Fatalf("[GetConfig] Error loading scraper trackers: %v", err)
		}

		if len(config.APITrackers) == 0 && len(config.ScraperTrackers) == 0 {
			log.Fatalf("[GetConfig] No trackers defined in the configuration")
		}

		config.ValidateConfig()

		// For debugging purposes
		// configJSON, err := json.MarshalIndent(config, "", "  ")
		// if err != nil {
		// 	log.Fatalf("[GetConfig] Error serializing configuration to JSON: %v", err)
		// }
		// log.Printf("[GetConfig] Loaded configuration: %s\n", configJSON)
	}

	return config
}

func loadTrackers(fileVar string) ([]*Tracker, error) {
	// Check if a file path is provided
	filePath := os.Getenv(fileVar)
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, errors.New("failed to read tracker file")
		}

		var trackers []*Tracker
		if err := json.Unmarshal(data, &trackers); err != nil {
			return nil, errors.New("failed to parse JSON from file")
		}

		return trackers, nil
	}

	return nil, nil
}

// Parses a comma separated list of Telegram chat IDs, e.g. "12345678,-100987654321".
func parseChatIDs(value string) ([]int64, error) {
	var chatIDs []int64

	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		chatID, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid chat ID '%s'", part)
		}

		chatIDs = append(chatIDs, chatID)
	}

	return chatIDs, nil
}

// Whether the bot should respond to the given chat. If no allowed chats are configured, all chats are allowed.
func (c *Configuration) IsChatAllowed(chatID int64) bool {
	if len(c.AllowedChatIDs) == 0 {
		return true
	}

	return slices.Contains(c.AllowedChatIDs, chatID)
}

func (c *Configuration) ValidateConfig() {
	validate := validator.New()
	if err := validate.Struct(c); err != nil {
		log.Fatalf("[GetConfig] Config validation error: %v", err)
	}
}

func (c *Configuration) GetAPITrackerData(code string) *Tracker {
	for _, tracker := range c.APITrackers {
		if tracker.Code == code {
			return tracker
		}
	}

	return nil
}

func (c *Configuration) GetScraperTrackerData(code string) *Tracker {
	for _, tracker := range c.ScraperTrackers {
		if tracker.Code == code {
			return tracker
		}
	}

	return nil
}

func (c *Configuration) GetTrackerData(code string) *Tracker {
	if tracker := c.GetAPITrackerData(code); tracker != nil {
		return tracker
	}

	return c.GetScraperTrackerData(code)
}
