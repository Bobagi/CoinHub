package service

import (
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "net/http"
        "net/url"
        "strconv"
        "sync"
        "time"

        "coin-hub/internal/domain"
)

type BinancePriceService struct {
        EnvironmentConfiguration domain.BinanceEnvironmentConfiguration
        HTTPClient               *http.Client
}

// Spot prices are public market data and identical for every user, so a process-wide cache lets all
// users (and the SPA) share a single fetch per symbol within a short window instead of each re-fetching
// the same price. This is the biggest lever against the per-IP Binance weight limit: e.g. 50 users
// holding BTC = one ticker call per window, not fifty. The TTL is well under the 30s monitor tick, so
// each tick still sees a fresh price; it only collapses the duplicates within a tick. The cache key
// includes the environment's base URL because TESTNET and PRODUCTION quote different prices.
const sharedPriceCacheTTL = 5 * time.Second

type cachedPriceEntry struct {
        price     float64
        expiresAt time.Time
}

var (
        sharedPriceCacheMutex sync.Mutex
        sharedPriceCache      = make(map[string]cachedPriceEntry)
)

func lookupCachedPrice(cacheKey string) (float64, bool) {
        sharedPriceCacheMutex.Lock()
        defer sharedPriceCacheMutex.Unlock()
        entry, present := sharedPriceCache[cacheKey]
        if !present || time.Now().After(entry.expiresAt) {
                return 0, false
        }
        return entry.price, true
}

func storeCachedPrice(cacheKey string, price float64) {
        sharedPriceCacheMutex.Lock()
        defer sharedPriceCacheMutex.Unlock()
        sharedPriceCache[cacheKey] = cachedPriceEntry{price: price, expiresAt: time.Now().Add(sharedPriceCacheTTL)}
}

type binanceTickerPriceResponse struct {
        Symbol string `json:"symbol"`
        Price  string `json:"price"`
}

func NewBinancePriceService(environmentConfiguration domain.BinanceEnvironmentConfiguration) *BinancePriceService {
        return &BinancePriceService{
                EnvironmentConfiguration: environmentConfiguration,
                HTTPClient:               newBinanceHTTPClient(8 * time.Second),
        }
}

func (service *BinancePriceService) UpdateEnvironmentConfiguration(newConfiguration domain.BinanceEnvironmentConfiguration) {
        service.EnvironmentConfiguration = newConfiguration
}

func (service *BinancePriceService) GetCurrentPrice(requestContext context.Context, tradingPairSymbol string) (float64, error) {
        cacheKey := service.EnvironmentConfiguration.RESTBaseURL + "|" + tradingPairSymbol
        if cachedPrice, present := lookupCachedPrice(cacheKey); present {
                return cachedPrice, nil
        }

        tickerEndpoint, urlBuildError := url.Parse(service.EnvironmentConfiguration.RESTBaseURL)
        if urlBuildError != nil {
                return 0, urlBuildError
        }
        tickerEndpoint.Path = "/api/v3/ticker/price"

        queryParameters := tickerEndpoint.Query()
        queryParameters.Set("symbol", tradingPairSymbol)
        tickerEndpoint.RawQuery = queryParameters.Encode()

        tickerRequest, requestBuildError := http.NewRequestWithContext(requestContext, http.MethodGet, tickerEndpoint.String(), nil)
        if requestBuildError != nil {
                return 0, requestBuildError
        }

        tickerResponse, responseError := service.HTTPClient.Do(tickerRequest)
        if responseError != nil {
                return 0, responseError
        }
        defer tickerResponse.Body.Close()

        if tickerResponse.StatusCode != http.StatusOK {
                return 0, fmt.Errorf("Binance price endpoint returned status %d", tickerResponse.StatusCode)
        }

        var parsedResponse binanceTickerPriceResponse
        decodeError := json.NewDecoder(tickerResponse.Body).Decode(&parsedResponse)
        if decodeError != nil {
                return 0, decodeError
        }

        if parsedResponse.Price == "" {
                return 0, errors.New("Binance price response did not include a price")
        }

        parsedPrice, priceParseError := parseDecimalStringToFloat(parsedResponse.Price)
        if priceParseError != nil {
                return 0, priceParseError
        }

        storeCachedPrice(cacheKey, parsedPrice)
        return parsedPrice, nil
}

// PricePoint is one close price at a point in time, used to draw the allocation history chart.
type PricePoint struct {
	Time  int64   `json:"t"`     // candle close time, milliseconds since epoch
	Close float64 `json:"close"` // close price in the pair's quote currency
}

// FetchCloseSeries returns the close-price series for a symbol over a Binance kline interval. It uses
// the public /api/v3/klines endpoint, so it works even without credentials.
func (service *BinancePriceService) FetchCloseSeries(requestContext context.Context, tradingPairSymbol string, interval string, limit int) ([]PricePoint, error) {
	klinesEndpoint, urlBuildError := url.Parse(service.EnvironmentConfiguration.RESTBaseURL)
	if urlBuildError != nil {
		return nil, urlBuildError
	}
	klinesEndpoint.Path = "/api/v3/klines"

	queryParameters := klinesEndpoint.Query()
	queryParameters.Set("symbol", tradingPairSymbol)
	queryParameters.Set("interval", interval)
	queryParameters.Set("limit", strconv.Itoa(limit))
	klinesEndpoint.RawQuery = queryParameters.Encode()

	klinesRequest, requestBuildError := http.NewRequestWithContext(requestContext, http.MethodGet, klinesEndpoint.String(), nil)
	if requestBuildError != nil {
		return nil, requestBuildError
	}

	klinesResponse, responseError := service.HTTPClient.Do(klinesRequest)
	if responseError != nil {
		return nil, responseError
	}
	defer klinesResponse.Body.Close()

	if klinesResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Binance klines endpoint returned status %d", klinesResponse.StatusCode)
	}

	// Each kline is an array: [openTime, open, high, low, close, volume, closeTime, ...].
	var rawKlines [][]json.RawMessage
	if decodeError := json.NewDecoder(klinesResponse.Body).Decode(&rawKlines); decodeError != nil {
		return nil, decodeError
	}

	points := make([]PricePoint, 0, len(rawKlines))
	for _, kline := range rawKlines {
		if len(kline) < 7 {
			continue
		}
		var closeTime int64
		if unmarshalError := json.Unmarshal(kline[6], &closeTime); unmarshalError != nil {
			continue
		}
		var closeText string
		if unmarshalError := json.Unmarshal(kline[4], &closeText); unmarshalError != nil {
			continue
		}
		closePrice, parseError := strconv.ParseFloat(closeText, 64)
		if parseError != nil {
			continue
		}
		points = append(points, PricePoint{Time: closeTime, Close: closePrice})
	}
	return points, nil
}

func parseDecimalStringToFloat(decimalString string) (float64, error) {
        parsedValue, parseError := strconv.ParseFloat(decimalString, 64)
        if parseError != nil {
                return 0, fmt.Errorf("could not parse decimal value %s", decimalString)
        }
        return parsedValue, nil
}
