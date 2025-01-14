package clients

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	config "web_scraper_bot/config"
	"web_scraper_bot/helpers"
)

type DataResult struct {
	CurrentValue        float64
	NotificationMessage string
}

// Common client interface that will be implemented by the concrete types of clients
type Client interface {
	FetchAndExtractData(trackerCode string) (*DataResult, error)
}

// Checks if the extracted value meets the notification criteria set for the tracker
// and returns a message to be sent to the user if any criteria are met.
func ProcessNotificationCriteria(trackerData *config.Tracker, extractedValue float64) (string, error) {
	fullfilledCriteria := make([]config.NotifyCriteria, 0)

	for _, criteria := range trackerData.NotifyCriteria {
		notifyValue, err := strconv.ParseFloat(criteria.Value, 64)
		if err != nil {
			log.Println("[Client] Error converting notification criteria value for tracker: "+trackerData.Code, err.Error())
			return "", err
		}

		isFulfilled, err := helpers.CompareNumbers(extractedValue, notifyValue, criteria.Operator)
		if err != nil {
			log.Println("[Client] Error comparing extracted and notification target values for tracker: "+trackerData.Code, err.Error())
			return "", err
		}

		if isFulfilled {
			fullfilledCriteria = append(fullfilledCriteria, criteria)
		}
	}

	if len(fullfilledCriteria) > 0 {
		var builder strings.Builder
		builder.WriteString(fmt.Sprintf("Good news, tracker <b>%s</b> has detected something you might be interested in :)\n\n", trackerData.Code))
		builder.WriteString(fmt.Sprintf("The tracked value is currently at <b>%.2f</b> and thus the following criteria are met:\n", extractedValue))
		for _, criteria := range fullfilledCriteria {
			operatorEscaped := strings.ReplaceAll(strings.ReplaceAll(criteria.Operator, "<", "&lt;"), ">", "&gt;")
			builder.WriteString(fmt.Sprintf(" - value: %.2f %s %s\n", extractedValue, operatorEscaped, criteria.Value))
		}

		builder.WriteString((fmt.Sprintf("\nMore details <a href=\"%s\">here</a>", trackerData.ViewURL)))

		return builder.String(), nil
	}

	return "", nil
}
