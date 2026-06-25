package service

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Binance enforces request limits PER IP, not per API key. Every user's traffic leaves this VPS from
// the same IP, so the IP weight ceiling is the first wall we hit as the user base grows. This file
// gives every Binance HTTP client a shared, process-wide gate that:
//   - reads the X-MBX-USED-WEIGHT-1M header Binance returns on each response and logs when we approach
//     the ceiling, so the pressure is observable in the logs; and
//   - on a 429 (rate limit exceeded) or 418 (IP auto-banned) response, parks ALL Binance requests until
//     the Retry-After window passes — backing off as one, since the limit is shared across the IP.
// Market-data volume is reduced separately by the shared price cache (see binance_price_service.go).

const (
	// Soft ceiling at which we start logging pressure. The spot IP weight limit is ~6000/min; warning at
	// 80% gives early visibility well before Binance starts rejecting.
	binanceWeightWarnThreshold = 4800
	// Fallback cooldowns when Binance omits Retry-After.
	binanceDefaultRateLimitCooldown = 30 * time.Second
	binanceDefaultBanCooldown       = 2 * time.Minute
)

type binanceRateGate struct {
	mutex             sync.Mutex
	cooldownUntil     time.Time
	lastUsedWeight    int
	lastWarnLoggedFor time.Time
	lastWasBan        bool // the active cooldown came from a 418 IP ban (vs a 429 rate-limit)
}

var sharedBinanceRateGate = &binanceRateGate{}

// cooldownRemaining reports how long callers must wait before hitting Binance again (0 if clear).
func (gate *binanceRateGate) cooldownRemaining() time.Duration {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	remaining := time.Until(gate.cooldownUntil)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// BinanceRateGateSnapshot is a read-only view of the shared IP rate-limit gate, for the operational
// status endpoint and the UI ("bots paused — Binance is rate-limiting").
type BinanceRateGateSnapshot struct {
	InCooldown       bool
	SecondsRemaining int
	Banned           bool // true when the cooldown is a 418 IP auto-ban (more severe than a 429)
	LastUsedWeight   int
}

// BinanceRateGateStatus returns the current state of the process-wide Binance rate-limit gate.
func BinanceRateGateStatus() BinanceRateGateSnapshot {
	sharedBinanceRateGate.mutex.Lock()
	defer sharedBinanceRateGate.mutex.Unlock()
	remaining := time.Until(sharedBinanceRateGate.cooldownUntil)
	if remaining < 0 {
		remaining = 0
	}
	inCooldown := remaining > 0
	return BinanceRateGateSnapshot{
		InCooldown:       inCooldown,
		SecondsRemaining: int(remaining.Seconds() + 0.999), // round up so "0s" never shows while still cooling
		Banned:           inCooldown && sharedBinanceRateGate.lastWasBan,
		LastUsedWeight:   sharedBinanceRateGate.lastUsedWeight,
	}
}

// observe inspects a Binance response: it records the used weight and, on 429/418, arms the shared
// cooldown so every other Binance call backs off too.
func (gate *binanceRateGate) observe(response *http.Response) {
	if response == nil {
		return
	}

	usedWeight := 0
	if headerValue := response.Header.Get("X-MBX-USED-WEIGHT-1M"); headerValue != "" {
		if parsed, parseError := strconv.Atoi(headerValue); parseError == nil {
			usedWeight = parsed
		}
	}

	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusTeapot {
		cooldown := binanceDefaultRateLimitCooldown
		if response.StatusCode == http.StatusTeapot {
			cooldown = binanceDefaultBanCooldown
		}
		if retryAfter := response.Header.Get("Retry-After"); retryAfter != "" {
			if seconds, parseError := strconv.Atoi(retryAfter); parseError == nil && seconds > 0 {
				cooldown = time.Duration(seconds) * time.Second
			}
		}
		gate.mutex.Lock()
		until := time.Now().Add(cooldown)
		if until.After(gate.cooldownUntil) {
			gate.cooldownUntil = until
		}
		gate.lastWasBan = response.StatusCode == http.StatusTeapot
		gate.lastUsedWeight = usedWeight
		gate.mutex.Unlock()
		log.Printf("binance: rate limit hit (status %d, used weight %d) — backing off all requests for %s", response.StatusCode, usedWeight, cooldown)
		return
	}

	if usedWeight == 0 {
		return
	}
	gate.mutex.Lock()
	gate.lastUsedWeight = usedWeight
	shouldWarn := usedWeight >= binanceWeightWarnThreshold && time.Since(gate.lastWarnLoggedFor) > 30*time.Second
	if shouldWarn {
		gate.lastWarnLoggedFor = time.Now()
	}
	gate.mutex.Unlock()
	if shouldWarn {
		log.Printf("binance: high IP request weight in the last minute (%d) — approaching the limit", usedWeight)
	}
}

// rateLimitedBinanceTransport wraps the default transport so every Binance request waits out an active
// cooldown first and feeds its response back to the shared gate.
type rateLimitedBinanceTransport struct {
	base http.RoundTripper
	gate *binanceRateGate
}

func (transport *rateLimitedBinanceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if waitDuration := transport.gate.cooldownRemaining(); waitDuration > 0 {
		timer := time.NewTimer(waitDuration)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	}
	response, requestError := transport.base.RoundTrip(request)
	if requestError != nil {
		return response, requestError
	}
	transport.gate.observe(response)
	return response, nil
}

// newBinanceHTTPClient builds an HTTP client whose requests pass through the shared IP rate-limit gate.
// Use it for every Binance REST client so they share one view of the IP's weight budget and back off
// together when Binance pushes back.
func newBinanceHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &rateLimitedBinanceTransport{base: http.DefaultTransport, gate: sharedBinanceRateGate},
	}
}
