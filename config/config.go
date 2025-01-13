package config

import (
	"encoding/json"
	"log"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
)

type NotifyCriteria struct {
	Operator string `json:"operator" validate:"required,oneof==< <= = >= >"`
	Value    string `json:"value" validate:"required,numeric"`
}

type Tracker struct {
	Code           string           `json:"code" validate:"required,excludesall=_/ "`
	APIURL         string           `json:"apiUrl" validate:"required,url"`
	ViewURL        string           `json:"viewUrl" validate:"omitempty,url"`
	Interval       string           `json:"interval" validate:"required"`
	NotifyCriteria []NotifyCriteria `json:"notifyCriteria" validate:"dive"`
	ResponsePath   string           `json:"responsePath" validate:"required"`
}

type Configuration struct {
	BotAPIKey       string     `validate:"required"`
	WebhookURL      string     `validate:"required,url"`
	Port            string     `validate:"omitempty,numeric"`
	Environment     string     `validate:"required"`
	APITrackers     []*Tracker `validate:"dive"`
	ScraperTrackers []*Tracker `validate:"dive"`
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

		apiTrackersString := os.Getenv("API_TRACKERS")
		if apiTrackersString != "" {
			var apiTrackers []*Tracker

			if err := json.Unmarshal([]byte(apiTrackersString), &apiTrackers); err != nil {
				log.Fatalf("[GetConfig] Error reading and processing environmental variable 'API_TRACKERS': %v", err)
			}

			config.APITrackers = apiTrackers
		}

		scraperTrackersString := os.Getenv("SCRAPER_TRACKERS")
		if scraperTrackersString != "" {
			var scraperTrackers []*Tracker

			if err := json.Unmarshal([]byte(scraperTrackersString), &scraperTrackers); err != nil {
				log.Fatalf("[GetConfig] Error reading and processing environmental variable 'SCRAPER_TRACKERS': %v", err)
			}

			config.ScraperTrackers = scraperTrackers
		}

		if len(config.APITrackers) == 0 && len(config.ScraperTrackers) == 0 {
			log.Fatalf("[GetConfig] No trackers defined in the configuration")
		}

		config.ValidateConfig()

		//For debugging purposes
		// configJSON, err := json.MarshalIndent(config, "", "  ")
		// if err != nil {
		// 	log.Fatalf("[GetConfig] Error serializing configuration to JSON: %v", err)
		// }
		// log.Printf("[GetConfig] Loaded configuration: %s\n", configJSON)
	}

	return config
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
