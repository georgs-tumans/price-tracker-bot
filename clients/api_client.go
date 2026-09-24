package clients

import (
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/tidwall/gjson"
	config "pricetrackerbot/config"
	"pricetrackerbot/services"
)

// Client for fetching data from public APIs and extracting the necessary data as defined in the tracker configuration.
type PublicAPIClient struct{}

func NewPublicAPIClient() *PublicAPIClient {
	return &PublicAPIClient{}
}

func (c *PublicAPIClient) FetchAndExtractData(trackerData *config.Tracker) (*DataResult, error) {
	dataJSON, err := c.getDataFromPublicAPI(trackerData)
	if err != nil {
		log.Println("[Public API Client] Error getting data from public API for tracker: "+trackerData.Code, err.Error())
		return nil, err
	}

	extractedValue, err := c.extractDataFromPublicAPIResponse(trackerData, dataJSON)
	if err != nil {
		log.Println("[Public API Client] Error extracting data from public API response for tracker: "+trackerData.Code, err.Error())
		return nil, err
	}

	extractedValueFloat, extractedErr := strconv.ParseFloat(extractedValue, 64)
	if extractedErr != nil {
		log.Println("[Public API Client] Error converting values for tracker: "+trackerData.Code, extractedErr.Error())
		return nil, extractedErr
	}

	notification, err := ProcessNotificationCriteria(trackerData, extractedValueFloat)
	if err != nil {
		return nil, err
	}

	return &DataResult{
		CurrentValue:        extractedValueFloat,
		NotificationMessage: notification,
	}, nil
}

func (c *PublicAPIClient) getDataFromPublicAPI(trackerData *config.Tracker) ([]byte, error) {
	response, err := services.GetRequest(trackerData.DataURL)
	if err != nil {
		log.Println("[Public API Client] Error getting data from public API for tracker: "+trackerData.Code, err.Error())

		return nil, err
	}

	return response, nil
}

func (c *PublicAPIClient) extractDataFromPublicAPIResponse(trackerData *config.Tracker, responseJSON []byte) (string, error) {
	if responseJSON == nil {
		return "", errors.New("nil response data")
	}

	result := gjson.GetBytes(responseJSON, trackerData.DataExtractionPath)
	if !result.Exists() {
		log.Println("[Public API Client] Error extracting data from public API response via the provided JSON path for tracker: "+trackerData.Code, "JSON path not found")

		return "", errors.New("json path not found")
	}

	switch result.Value().(type) {
	case string:
		return result.String(), nil
	case float64:
		return fmt.Sprintf("%f", result.Float()), nil
	default:
		log.Println("[Public API Client] Unrecognized extracted response data type for tracker: "+trackerData.Code, "Unsupported data type")
		return "", errors.New("unsupported extracted response data type")
	}
}
