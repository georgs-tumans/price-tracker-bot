package config

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"

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

type Configuration struct {
	BotAPIKey        string     `validate:"required"`
	WebhookURL       string     `validate:"required,url"`
	Port             string     `validate:"omitempty,numeric"`
	Environment      string     `validate:"required"`
	ErrorNotifyLimit int        `validate:"omitempty,numeric"`
	APITrackers      []*Tracker `validate:"dive"`
	ScraperTrackers  []*Tracker `validate:"dive"`
}

var config *Configuration

func GetConfig() *Configuration {
	if config == nil {
		log.Println("[Config] Loading configuration")
		err := godotenv.Load()
		if err != nil {
			log.Println("[GetConfig] Error loading .env file")
		}

		config = &Configuration{
			BotAPIKey:   os.Getenv("BOT_API_KEY"),
			WebhookURL:  os.Getenv("WEBHOOK_URL"),
			Port:        os.Getenv("PORT"),
			Environment: os.Getenv("ENVIRONMENT"),
		}

		errorLimit := os.Getenv("ERROR_NOTIFY_LIMIT")
		if errorLimit != "" {
			converted, _ := strconv.Atoi(errorLimit)
			config.ErrorNotifyLimit = converted
		} else {
			config.ErrorNotifyLimit = 3
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
