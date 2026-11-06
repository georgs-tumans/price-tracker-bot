package handlers

import (
	"fmt"

	"pricetrackerbot/clients"
	"pricetrackerbot/config"
)

/*
TrackerBehavior interface will be implemented by the concrete types of behaviors;
these behaviors represent different ways of fetching data - either through an API or by scraping a website
or whatever other way we might come up with in the future. This abstraction is supposed to make it easier
to add new ways of fetching data without changing the existing code.
It is NOT meant for implementing the data fetching logic itself - that will be done in the clients.
*/
type TrackerBehavior interface {
	// Returns the recorded value and the notification message to send (empty if no criteria are met).
	Execute(trackerData *config.Tracker) (value string, notification string, err error)
}

type APITrackerBehavior struct {
	client *clients.PublicAPIClient
}

func NewAPITrackerBehavior() *APITrackerBehavior {
	return &APITrackerBehavior{
		client: clients.NewPublicAPIClient(),
	}
}

func (tb *APITrackerBehavior) Execute(trackerData *config.Tracker) (string, string, error) {
	result, err := tb.client.FetchAndExtractData(trackerData)
	if err != nil {
		return "", "", err
	}

	return fmt.Sprintf("%.2f", result.CurrentValue), result.NotificationMessage, nil
}

type ScraperTrackerBehavior struct {
	client *clients.ScraperClient
}

func NewScraperTrackerBehavior() *ScraperTrackerBehavior {
	return &ScraperTrackerBehavior{
		client: clients.NewScraperClient(),
	}
}

func (tb *ScraperTrackerBehavior) Execute(trackerData *config.Tracker) (string, string, error) {
	result, err := tb.client.FetchAndExtractData(trackerData)
	if err != nil {
		return "", "", err
	}

	return fmt.Sprintf("%.2f", result.CurrentValue), result.NotificationMessage, nil
}
