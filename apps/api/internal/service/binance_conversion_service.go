package service

import (
	"context"
	"errors"

	"coin-hub/internal/domain"
)

// BinanceConversionService resolves a current exchange rate between two assets (e.g. USDT→BRL) from
// live pair prices, so the SPA can convert mixed-quote-currency totals into one display currency.
// It rides the shared 5s price cache and the shared exchangeInfo cache, so a conversion costs at
// most a couple of cached ticker lookups — it adds no per-user API weight at scale.
type BinanceConversionService struct {
	priceService  *BinancePriceService
	symbolService *BinanceSymbolService
}

// ErrNoConversionPath means no direct, inverse or bridged pair connects the two assets on this
// environment (common on testnet, whose symbol list is tiny).
var ErrNoConversionPath = errors.New("no conversion path between these assets")

// Bridges tried (in order) when no direct/inverse pair exists: FROM→bridge→TO.
var conversionBridgeAssets = []string{"USDT", "BTC", "BNB"}

func NewBinanceConversionService(environmentConfiguration domain.BinanceEnvironmentConfiguration) *BinanceConversionService {
	return &BinanceConversionService{
		priceService:  NewBinancePriceService(environmentConfiguration),
		symbolService: NewBinanceSymbolService(environmentConfiguration),
	}
}

// Rate returns how many units of toAsset one unit of fromAsset is currently worth.
func (service *BinanceConversionService) Rate(requestContext context.Context, fromAsset string, toAsset string) (float64, error) {
	infos, symbolsError := service.symbolService.FetchSymbolDetails(requestContext)
	if symbolsError != nil {
		return 0, symbolsError
	}
	tradableSymbols := make(map[string]bool, len(infos))
	for _, info := range infos {
		tradableSymbols[info.Symbol] = true
	}
	symbolExists := func(symbol string) bool { return tradableSymbols[symbol] }
	lookupPrice := func(lookupContext context.Context, symbol string) (float64, error) {
		return service.priceService.GetCurrentPrice(lookupContext, symbol)
	}
	return resolveConversionRate(requestContext, fromAsset, toAsset, symbolExists, lookupPrice)
}

type symbolExistsFunc func(symbol string) bool
type priceLookupFunc func(lookupContext context.Context, symbol string) (float64, error)

// resolveConversionRate is the pure conversion logic (unit-tested): direct pair, then inverse pair,
// then a two-leg bridge (FROM→USDT→TO, FROM→BTC→TO, …).
func resolveConversionRate(requestContext context.Context, fromAsset string, toAsset string, symbolExists symbolExistsFunc, lookupPrice priceLookupFunc) (float64, error) {
	if fromAsset == toAsset {
		return 1, nil
	}

	if rate, rateError := directPairRate(requestContext, fromAsset, toAsset, symbolExists, lookupPrice); rateError == nil {
		return rate, nil
	} else if !errors.Is(rateError, ErrNoConversionPath) {
		return 0, rateError
	}

	for _, bridgeAsset := range conversionBridgeAssets {
		if bridgeAsset == fromAsset || bridgeAsset == toAsset {
			continue
		}
		fromToBridge, firstLegError := directPairRate(requestContext, fromAsset, bridgeAsset, symbolExists, lookupPrice)
		if firstLegError != nil {
			if errors.Is(firstLegError, ErrNoConversionPath) {
				continue
			}
			return 0, firstLegError
		}
		bridgeToTarget, secondLegError := directPairRate(requestContext, bridgeAsset, toAsset, symbolExists, lookupPrice)
		if secondLegError != nil {
			if errors.Is(secondLegError, ErrNoConversionPath) {
				continue
			}
			return 0, secondLegError
		}
		return fromToBridge * bridgeToTarget, nil
	}

	return 0, ErrNoConversionPath
}

// directPairRate converts via a single listed pair: symbol FROM+TO quotes "TO per FROM" (its price IS
// the rate); TO+FROM quotes the opposite, so the rate is its inverse.
func directPairRate(requestContext context.Context, fromAsset string, toAsset string, symbolExists symbolExistsFunc, lookupPrice priceLookupFunc) (float64, error) {
	if symbolExists(fromAsset + toAsset) {
		price, priceError := lookupPrice(requestContext, fromAsset+toAsset)
		if priceError != nil {
			return 0, priceError
		}
		if price == 0 {
			// A listed-but-untraded pair can quote 0 — that's "no usable rate", not a rate of 0.
			return 0, ErrNoConversionPath
		}
		return price, nil
	}
	if symbolExists(toAsset + fromAsset) {
		price, priceError := lookupPrice(requestContext, toAsset+fromAsset)
		if priceError != nil {
			return 0, priceError
		}
		if price == 0 {
			return 0, ErrNoConversionPath
		}
		return 1 / price, nil
	}
	return 0, ErrNoConversionPath
}
