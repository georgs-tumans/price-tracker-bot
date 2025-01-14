# Tracker configurations

Put here two seperate `.json` files with tracker configuration for public API and scraping trackers.

File structure for both of these configurations must be as follows:

```
[
   {
     "code": "<string> trackerCode - an arbitrary value to identify each tracking URL; must be unique for each URL; cannot contain the following symbols: '_', '/', ' ' (space)",
     "apiUrl":"<string> the public API URL for accessing data",
     "viewUrl":"<string> the website URL to add to the user notification message",
     "interval":"<string> tracker run interval; format: '1h'; available interval types: "m" - minutes, "h" - hours, "d" - days", 
     "notifyCriteria":"<[{"operator": "", value: 0}]> a list with the criteria for sending notifications; available operators: '<'|'<='|'='|'>='|'>'; notification calculation logic: [extracted value <notifyCriteria> notifyValue]",
     "responsePath":"<[string] the path to the value in the response JSON; format: uses gson query syntax - https://github.com/tidwall/gjson>"
   }
 ]
 ```

 See the example files for quick configuration:

  - [api_trackers](/api_trackers.json.example) 
  - [scraper_trackers](/scraper_trackers.json.example) 
