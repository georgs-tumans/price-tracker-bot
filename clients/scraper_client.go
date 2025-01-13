package clients

import (
	config "web_scraper_bot/config"

	"github.com/gocolly/colly/v2"
)

// Client for fetching data from website HTML's and extracting the necessary data as defined in the tracker configuration
type ScraperClient struct {
	trackerData *config.Tracker
	collector   *colly.Collector
}

func NewScraperClient() *ScraperClient {
	return &ScraperClient{collector: colly.NewCollector()}
}

func (c *ScraperClient) FetchAndExtractData(trackerData *config.Tracker) (*DataResult, error) {
	c.trackerData = trackerData
	// TODO: rename APIURL to some more generic name to fit both API and Scraper clients
	c.collector.Visit(trackerData.APIURL)

	return nil, nil
}
