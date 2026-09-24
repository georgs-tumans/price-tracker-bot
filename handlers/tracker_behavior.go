package handlers

import (
	"fmt"

	"pricetrackerbot/clients"
	"pricetrackerbot/config"
	"pricetrackerbot/helpers"
)

/*
TrackerBehavior interface will be implemented by the concrete types of behaviors;
these behaviors represent different ways of fetching data - either through an API or by scraping a website
or whatever other way we might come up with in the future. This abstraction is supposed to make it easier
to add new ways of fetching data without changing the existing code.
It is NOT meant for implementing the data fetching logic itself - that will be done in the clients.
*/
type TrackerBehavior interface {
	Execute(trackerData *config.Tracker, chatID int64) (string, error)
}

type APITrackerBehavior struct {
	messenger helpers.Messenger
	client    *clients.PublicAPIClient
}

func NewAPITrackerBehavior(messenger helpers.Messenger) *APITrackerBehavior {
	return &APITrackerBehavior{
		messenger: messenger,
		client:    clients.NewPublicAPIClient(),
	}
}

func (tb *APITrackerBehavior) Execute(trackerData *config.Tracker, chatID int64) (string, error) {
	result, err := tb.client.FetchAndExtractData(trackerData)
	if err != nil {
		// Notify the user? Add to some failure statistics?
		return "", err
	}

	if result.NotificationMessage != "" {
		tb.messenger.SendHTML(chatID, result.NotificationMessage)
	}

	return fmt.Sprintf("%.2f", result.CurrentValue), nil
}

type ScraperTrackerBehavior struct {
	messenger helpers.Messenger
	client    *clients.ScraperClient
}

func NewScraperTrackerBehavior(messenger helpers.Messenger) *ScraperTrackerBehavior {
	return &ScraperTrackerBehavior{
		messenger: messenger,
		client:    clients.NewScraperClient(),
	}
}

func (tb *ScraperTrackerBehavior) Execute(trackerData *config.Tracker, chatID int64) (string, error) {
	result, err := tb.client.FetchAndExtractData(trackerData)
	if err != nil {
		// Notify the user? Add to some failure statistics?
		return "", err
	}

	if result.NotificationMessage != "" {
		tb.messenger.SendHTML(chatID, result.NotificationMessage)
	}

	return fmt.Sprintf("%.2f", result.CurrentValue), nil
	// return "", nil
}
