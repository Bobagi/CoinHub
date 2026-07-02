package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"coin-hub/internal/domain"
)

// BinanceSymbolInfo is one tradable pair with its two sides split out (BTCBRL → base BTC, quote BRL),
// so callers never have to guess the quote asset from the symbol string.
type BinanceSymbolInfo struct {
	Symbol     string `json:"symbol"`
	BaseAsset  string `json:"base"`
	QuoteAsset string `json:"quote"`
}

// The exchange's symbol list is public, identical for every user and changes rarely, so it is cached
// process-wide per environment (base URL). Handlers build a fresh BinanceSymbolService per request;
// a per-instance cache would never be hit, and exchangeInfo is a weight-20 call.
const sharedSymbolCacheTTL = 10 * time.Minute

type cachedSymbolsEntry struct {
	infos     []BinanceSymbolInfo
	fetchedAt time.Time
}

var (
	sharedSymbolCacheMutex sync.Mutex
	sharedSymbolCache      = make(map[string]cachedSymbolsEntry)
)

type BinanceSymbolService struct {
	EnvironmentConfiguration domain.BinanceEnvironmentConfiguration
	HTTPClient               *http.Client
}

type binanceExchangeInfoResponse struct {
	Symbols []struct {
		Symbol               string `json:"symbol"`
		Status               string `json:"status"`
		IsSpotTradingAllowed bool   `json:"isSpotTradingAllowed"`
		QuoteAsset           string `json:"quoteAsset"`
		BaseAsset            string `json:"baseAsset"`
	} `json:"symbols"`
}

func NewBinanceSymbolService(environmentConfiguration domain.BinanceEnvironmentConfiguration) *BinanceSymbolService {
	return &BinanceSymbolService{
		EnvironmentConfiguration: environmentConfiguration,
		HTTPClient:               newBinanceHTTPClient(8 * time.Second),
	}
}

func (service *BinanceSymbolService) UpdateEnvironmentConfiguration(newConfiguration domain.BinanceEnvironmentConfiguration) {
	service.EnvironmentConfiguration = newConfiguration
}

// FetchSymbolDetails returns every TRADING spot pair with base/quote assets, from the shared cache
// when fresh.
func (service *BinanceSymbolService) FetchSymbolDetails(fetchContext context.Context) ([]BinanceSymbolInfo, error) {
	cacheKey := service.EnvironmentConfiguration.RESTBaseURL

	sharedSymbolCacheMutex.Lock()
	entry, present := sharedSymbolCache[cacheKey]
	sharedSymbolCacheMutex.Unlock()
	if present && time.Since(entry.fetchedAt) < sharedSymbolCacheTTL {
		return entry.infos, nil
	}

	exchangeInfoEndpoint := service.EnvironmentConfiguration.RESTBaseURL + "/api/v3/exchangeInfo"
	exchangeInfoRequest, requestError := http.NewRequestWithContext(fetchContext, http.MethodGet, exchangeInfoEndpoint, nil)
	if requestError != nil {
		return nil, requestError
	}

	response, responseError := service.HTTPClient.Do(exchangeInfoRequest)
	if responseError != nil {
		return nil, responseError
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, errors.New("Binance symbols endpoint responded with a non-OK status")
	}

	var exchangeInformation binanceExchangeInfoResponse
	if decodeError := json.NewDecoder(response.Body).Decode(&exchangeInformation); decodeError != nil {
		return nil, decodeError
	}

	infos := extractTradableSymbolInfos(exchangeInformation)

	sharedSymbolCacheMutex.Lock()
	sharedSymbolCache[cacheKey] = cachedSymbolsEntry{infos: infos, fetchedAt: time.Now()}
	sharedSymbolCacheMutex.Unlock()

	return infos, nil
}

func extractTradableSymbolInfos(exchangeInformation binanceExchangeInfoResponse) []BinanceSymbolInfo {
	var infos []BinanceSymbolInfo
	for _, symbol := range exchangeInformation.Symbols {
		if strings.EqualFold(symbol.Status, "TRADING") && symbol.IsSpotTradingAllowed {
			infos = append(infos, BinanceSymbolInfo{Symbol: symbol.Symbol, BaseAsset: symbol.BaseAsset, QuoteAsset: symbol.QuoteAsset})
		}
	}
	return infos
}
