package clients

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	config "web_scraper_bot/config"

	"github.com/gocolly/colly/v2"
)

// Client for fetching data from website HTML's and extracting the necessary data as defined in the tracker configuration
type ScraperClient struct {
	trackerData *config.Tracker
	collector   *colly.Collector
}

func NewScraperClient() *ScraperClient {
	return &ScraperClient{collector: colly.NewCollector(colly.AllowURLRevisit())}
}

func (c *ScraperClient) FetchAndExtractData(trackerData *config.Tracker) (*DataResult, error) {
	var price string
	var executionError error
	c.trackerData = trackerData

	// On every a element which has href attribute call callback
	c.collector.OnHTML(c.trackerData.DataExtractionPath, func(e *colly.HTMLElement) {
		price = e.Text
	})

	c.collector.OnError(func(r *colly.Response, err error) {
		log.Printf("[Scraper Client] Error while making scraping request for tracker %s: %s", c.trackerData.Code, err.Error())
		executionError = err
	})

	// Before making a request
	c.collector.OnRequest(func(r *colly.Request) {
		log.Println("[Scraper Client] Making a scraping request for tracker: " + c.trackerData.Code)
	})

	c.collector.Visit(trackerData.DataURL)

	if executionError != nil {
		return nil, executionError
	}

	if price == "" {
		log.Println("[Scraper Client] Price value not found in the scraped HTML element for tracker: " + c.trackerData.Code)
		return nil, errors.New("price not found")
	}

	reg := regexp.MustCompile(`[^0-9.,]`)
	cleanPrice := reg.ReplaceAllString(strings.TrimSpace(price), "")
	if strings.Contains(cleanPrice, ",") {
		cleanPrice = strings.Replace(cleanPrice, ",", ".", -1)
	}

	priceFloat, err := strconv.ParseFloat(cleanPrice, 64)
	if err != nil {
		log.Println(fmt.Sprintf("[Scraper Client] Failed to parse scraped value for tracker %s: %s", trackerData.Code, err.Error()))
		return nil, fmt.Errorf("failed to parse price: %v", err)
	}

	notification, err := ProcessNotificationCriteria(c.trackerData, priceFloat)
	if err != nil {
		return nil, err
	}

	c.trackerData = nil

	return &DataResult{
		CurrentValue:        priceFloat,
		NotificationMessage: notification,
	}, nil
}
