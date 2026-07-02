package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"coin-hub/internal/domain"
)

// SpotBalance is one asset's spot-wallet balance: Free is what's available to spend on new orders,
// Locked is reserved by open orders (e.g. a resting take-profit sell).
type SpotBalance struct {
	Asset  string  `json:"asset"`
	Free   float64 `json:"free"`
	Locked float64 `json:"locked"`
}

// Balances only change when the user trades/deposits, and /api/v3/account is a weight-20 call, so a
// short per-user+environment cache keeps dashboard polling cheap. Trade paths invalidate it (see
// UserTradingService.logExecution) so a just-executed order shows immediately.
const spotBalanceCacheTTL = 15 * time.Second

type cachedBalancesEntry struct {
	balances  []SpotBalance
	expiresAt time.Time
}

var (
	spotBalanceCacheMutex sync.Mutex
	spotBalanceCache      = make(map[string]cachedBalancesEntry)
)

func spotBalanceCacheKey(userIdentifier int64, environmentName string) string {
	return fmt.Sprintf("%d|%s", userIdentifier, environmentName)
}

// InvalidateCachedSpotBalances drops the cached balances for a user+environment so the next read
// reflects an order that just executed.
func InvalidateCachedSpotBalances(userIdentifier int64, environmentName string) {
	spotBalanceCacheMutex.Lock()
	defer spotBalanceCacheMutex.Unlock()
	delete(spotBalanceCache, spotBalanceCacheKey(userIdentifier, environmentName))
}

// BinanceAccountService reads the user's spot-wallet balances with their signed credentials.
type BinanceAccountService struct {
	HTTPClient *http.Client
}

func NewBinanceAccountService() *BinanceAccountService {
	return &BinanceAccountService{HTTPClient: newBinanceHTTPClient(8 * time.Second)}
}

type binanceAccountBalancesResponse struct {
	Balances []struct {
		Asset  string `json:"asset"`
		Free   string `json:"free"`
		Locked string `json:"locked"`
	} `json:"balances"`
}

// FetchSpotBalances returns the non-zero spot balances for the credential in the configuration
// (omitZeroBalances keeps the payload to assets the user actually holds).
func (service *BinanceAccountService) FetchSpotBalances(requestContext context.Context, userIdentifier int64, configuration domain.BinanceEnvironmentConfiguration) ([]SpotBalance, error) {
	cacheKey := spotBalanceCacheKey(userIdentifier, configuration.EnvironmentName)
	spotBalanceCacheMutex.Lock()
	if entry, present := spotBalanceCache[cacheKey]; present && time.Now().Before(entry.expiresAt) {
		spotBalanceCacheMutex.Unlock()
		return entry.balances, nil
	}
	spotBalanceCacheMutex.Unlock()

	accountEndpoint, parseError := url.Parse(configuration.RESTBaseURL)
	if parseError != nil {
		return nil, parseError
	}
	accountEndpoint.Path = "/api/v3/account"

	parameters := accountEndpoint.Query()
	parameters.Set("omitZeroBalances", "true")
	parameters.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	signature := computeHMACSignature(parameters.Encode(), configuration.APISecret)
	parameters.Set("signature", signature)
	accountEndpoint.RawQuery = parameters.Encode()

	accountRequest, buildError := http.NewRequestWithContext(requestContext, http.MethodGet, accountEndpoint.String(), nil)
	if buildError != nil {
		return nil, buildError
	}
	accountRequest.Header.Set("X-MBX-APIKEY", configuration.APIKey)

	accountResponse, responseError := service.HTTPClient.Do(accountRequest)
	if responseError != nil {
		return nil, responseError
	}
	defer accountResponse.Body.Close()

	if accountResponse.StatusCode != http.StatusOK {
		// Read (bounded) so the error is diagnosable, but never echo credentials.
		responseBody, _ := io.ReadAll(io.LimitReader(accountResponse.Body, 512))
		return nil, fmt.Errorf("Binance account endpoint returned status %d: %s", accountResponse.StatusCode, string(responseBody))
	}

	var parsedResponse binanceAccountBalancesResponse
	if decodeError := json.NewDecoder(accountResponse.Body).Decode(&parsedResponse); decodeError != nil {
		return nil, decodeError
	}

	balances := make([]SpotBalance, 0, len(parsedResponse.Balances))
	for _, rawBalance := range parsedResponse.Balances {
		free, freeError := strconv.ParseFloat(rawBalance.Free, 64)
		locked, lockedError := strconv.ParseFloat(rawBalance.Locked, 64)
		if freeError != nil || lockedError != nil {
			continue
		}
		if free == 0 && locked == 0 {
			continue
		}
		balances = append(balances, SpotBalance{Asset: rawBalance.Asset, Free: free, Locked: locked})
	}

	spotBalanceCacheMutex.Lock()
	spotBalanceCache[cacheKey] = cachedBalancesEntry{balances: balances, expiresAt: time.Now().Add(spotBalanceCacheTTL)}
	spotBalanceCacheMutex.Unlock()

	return balances, nil
}
