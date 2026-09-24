package clients

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/gocolly/colly/v2"
	config "pricetrackerbot/config"
)

var nonPriceCharacters = regexp.MustCompile(`[^0-9.,]`)

// Client for fetching data from website HTML's and extracting the necessary data as defined in the tracker configuration.
type ScraperClient struct{}

func NewScraperClient() *ScraperClient {
	return &ScraperClient{}
}

func (c *ScraperClient) FetchAndExtractData(trackerData *config.Tracker) (*DataResult, error) {
	var price string
	var executionError error

	// A new collector per run: colly callbacks accumulate on a collector, so reusing one would register
	// another set of callbacks on every run
	collector := colly.NewCollector(colly.AllowURLRevisit())

	collector.OnHTML(trackerData.DataExtractionPath, func(e *colly.HTMLElement) {
		price = e.Text
		// Elements without text such as <meta> tags carry the value in the content attribute
		if strings.TrimSpace(price) == "" {
			price = e.Attr("content")
		}
	})

	collector.OnError(func(_ *colly.Response, err error) {
		log.Printf("[Scraper Client] Error while making scraping request for tracker %s: %s", trackerData.Code, err.Error())
		executionError = err
	})

	log.Println("[Scraper Client] Making a scraping request for tracker: " + trackerData.Code)
	if err := collector.Visit(trackerData.DataURL); err != nil {
		return nil, err
	}

	if executionError != nil {
		return nil, executionError
	}

	if price == "" {
		log.Println("[Scraper Client] Price value not found in the scraped HTML element for tracker: " + trackerData.Code)
		return nil, errors.New("price not found")
	}

	cleanPrice := nonPriceCharacters.ReplaceAllString(strings.TrimSpace(price), "")
	cleanPrice = strings.ReplaceAll(cleanPrice, ",", ".")

	priceFloat, err := strconv.ParseFloat(cleanPrice, 64)
	if err != nil {
		log.Printf("[Scraper Client] Failed to parse scraped value for tracker %s: %s", trackerData.Code, err.Error())
		return nil, fmt.Errorf("failed to parse price: %w", err)
	}

	notification, err := ProcessNotificationCriteria(trackerData, priceFloat)
	if err != nil {
		return nil, err
	}

	return &DataResult{
		CurrentValue:        priceFloat,
		NotificationMessage: notification,
	}, nil
}
