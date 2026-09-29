# Tracker configurations

Put two separate `.json` files here: one for API trackers and one for scraper trackers. Point the bot to them with `API_TRACKERS_FILE` and `SCRAPER_TRACKERS_FILE` in `.env`.

Both files have the same structure:

```
[
   {
     "code": "<string> an arbitrary value that identifies the tracker; must be unique; cannot contain '_', '/' or ' ' (space)",
     "dataUrl": "<string> the URL the value is read from: an API endpoint for API trackers, a web page for scraper trackers",
     "viewUrl": "<string, optional> the website URL linked in the notification message",
     "interval": "<string> default run interval, e.g. '1h'; available interval types: 'm' - minutes, 'h' - hours, 'd' - days; values below MIN_INTERVAL are raised to it",
     "notifyCriteria": [
       {
         "operator": "<string> one of '<', '<=', '=', '>=', '>'",
         "value": "<string> a number, e.g. \"100\"; the user is notified when [extracted value] <operator> [value]"
       }
     ],
     "dataExtractionPath": "<string> where the value is in the response; see below"
   }
]
```

`dataExtractionPath` depends on the tracker type:

- **API trackers:** a [gjson](https://github.com/tidwall/gjson) path into the response JSON, e.g. `price` or `offers.#(period==12).interestRate`. The value must be a number or a numeric string.
- **Scraper trackers:** a CSS selector ([goquery](https://pkg.go.dev/github.com/PuerkitoBio/goquery) syntax), e.g. `.product .price` or `meta[property="product:price:amount"]`. The element's text is used (if several elements match, the last one wins, so make the selector specific); for elements without text, like `<meta>`, the `content` attribute is used. Currency symbols and spaces are removed and a decimal comma is accepted, so `1 234,56 €` is read as `1234.56`.

See the example files for a quick start:

- [api_trackers](api_trackers.json.example)
- [scraper_trackers](scraper_trackers.json.example)
