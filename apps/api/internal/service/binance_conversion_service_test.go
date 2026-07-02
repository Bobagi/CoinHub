package service

import (
	"context"
	"errors"
	"math"
	"testing"
)

func conversionFixture(prices map[string]float64) (symbolExistsFunc, priceLookupFunc) {
	symbolExists := func(symbol string) bool {
		_, present := prices[symbol]
		return present
	}
	lookupPrice := func(_ context.Context, symbol string) (float64, error) {
		price, present := prices[symbol]
		if !present {
			return 0, errors.New("unexpected price lookup for " + symbol)
		}
		return price, nil
	}
	return symbolExists, lookupPrice
}

func assertRate(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("rate = %v, want %v", got, want)
	}
}

func TestResolveConversionRateSameAsset(t *testing.T) {
	symbolExists, lookupPrice := conversionFixture(nil)
	rate, err := resolveConversionRate(context.Background(), "BRL", "BRL", symbolExists, lookupPrice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRate(t, rate, 1)
}

func TestResolveConversionRateDirectPair(t *testing.T) {
	// USDTBRL is listed: 1 USDT = 5.50 BRL.
	symbolExists, lookupPrice := conversionFixture(map[string]float64{"USDTBRL": 5.5})
	rate, err := resolveConversionRate(context.Background(), "USDT", "BRL", symbolExists, lookupPrice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRate(t, rate, 5.5)
}

func TestResolveConversionRateInversePair(t *testing.T) {
	// Only USDTBRL is listed; BRL→USDT must invert it.
	symbolExists, lookupPrice := conversionFixture(map[string]float64{"USDTBRL": 5.5})
	rate, err := resolveConversionRate(context.Background(), "BRL", "USDT", symbolExists, lookupPrice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRate(t, rate, 1/5.5)
}

func TestResolveConversionRateBridgedThroughUSDT(t *testing.T) {
	// No EUR↔BRL pair; go EUR→USDT→BRL: 1 EUR = 1.08 USDT, 1 USDT = 5.50 BRL ⇒ 5.94 BRL.
	symbolExists, lookupPrice := conversionFixture(map[string]float64{
		"EURUSDT": 1.08,
		"USDTBRL": 5.5,
	})
	rate, err := resolveConversionRate(context.Background(), "EUR", "BRL", symbolExists, lookupPrice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRate(t, rate, 1.08*5.5)
}

func TestResolveConversionRateBridgedWithInverseLeg(t *testing.T) {
	// BRL→ETH with only USDTBRL + ETHUSDT listed: BRL→USDT (inverse) then USDT→ETH (inverse).
	symbolExists, lookupPrice := conversionFixture(map[string]float64{
		"USDTBRL": 5.0,
		"ETHUSDT": 2500.0,
	})
	rate, err := resolveConversionRate(context.Background(), "BRL", "ETH", symbolExists, lookupPrice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRate(t, rate, (1/5.0)*(1/2500.0))
}

func TestResolveConversionRateNoPath(t *testing.T) {
	symbolExists, lookupPrice := conversionFixture(map[string]float64{"BTCUSDT": 100000})
	_, err := resolveConversionRate(context.Background(), "BRL", "EUR", symbolExists, lookupPrice)
	if !errors.Is(err, ErrNoConversionPath) {
		t.Fatalf("expected ErrNoConversionPath, got %v", err)
	}
}

func TestResolveConversionRateZeroInversePriceFailsClosed(t *testing.T) {
	symbolExists, lookupPrice := conversionFixture(map[string]float64{"USDTBRL": 0})
	_, err := resolveConversionRate(context.Background(), "BRL", "USDT", symbolExists, lookupPrice)
	if !errors.Is(err, ErrNoConversionPath) {
		t.Fatalf("expected ErrNoConversionPath for a zero price, got %v", err)
	}
}

func TestResolveConversionRateZeroDirectPriceFailsClosed(t *testing.T) {
	symbolExists, lookupPrice := conversionFixture(map[string]float64{"USDTBRL": 0})
	_, err := resolveConversionRate(context.Background(), "USDT", "BRL", symbolExists, lookupPrice)
	if !errors.Is(err, ErrNoConversionPath) {
		t.Fatalf("expected ErrNoConversionPath for a zero direct price, got %v", err)
	}
}
