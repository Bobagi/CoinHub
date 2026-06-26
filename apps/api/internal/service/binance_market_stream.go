package service

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Market data over WebSocket: instead of REST-polling the ticker for every open position's coin every
// 30s (per-IP weight that grows with users), the worker subscribes to a push stream of mini-tickers and
// feeds the SAME shared price cache the REST path uses. Stop-loss and the SPA then read live prices from
// cache. This is an ACCELERATOR: if the stream drops, cache entries expire (5s TTL) and the code falls
// straight back to REST — correctness never depends on the socket being up.
//
// One stream per environment (keyed by REST base URL, since TESTNET and PRODUCTION quote different
// prices and use different hosts). Symbols are dynamic: the worker "watches" the coins it currently
// holds; a symbol not re-watched for a few ticks ages out and the stream reconnects to the smaller set.

const (
	marketSymbolStaleAfter   = 95 * time.Second // drop a symbol not re-watched within ~3 monitor ticks
	marketStreamCheckEvery   = 10 * time.Second // how often to reconcile the desired symbol set
	marketStreamReadTimeout  = 35 * time.Second // wake ReadMessage periodically to re-check the set
	marketStreamReconnectMin = 2 * time.Second
	marketStreamReconnectMax = 60 * time.Second
)

// MarketStreamManager lazily runs one market stream per environment (REST base URL). The worker calls
// Watch with the coins it cares about; the manager spins up the env's stream on first use.
type MarketStreamManager struct {
	parentContext context.Context
	mutex         sync.Mutex
	streams       map[string]*binanceMarketStream // keyed by REST base URL
}

func NewMarketStreamManager(parentContext context.Context) *MarketStreamManager {
	return &MarketStreamManager{parentContext: parentContext, streams: make(map[string]*binanceMarketStream)}
}

// Watch tells the manager the worker is interested in live prices for these symbols in the given
// environment. It lazily starts that environment's stream and refreshes the symbols' last-seen time.
func (manager *MarketStreamManager) Watch(restBaseURL string, symbols []string) {
	if manager == nil || restBaseURL == "" || len(symbols) == 0 {
		return
	}
	manager.mutex.Lock()
	stream, present := manager.streams[restBaseURL]
	if !present {
		stream = newBinanceMarketStream(restBaseURL)
		manager.streams[restBaseURL] = stream
		go stream.run(manager.parentContext)
	}
	manager.mutex.Unlock()
	stream.watch(symbols)
}

type binanceMarketStream struct {
	restBaseURL string
	wsBaseURL   string
	mutex       sync.Mutex
	lastSeen    map[string]time.Time // symbol (upper) -> last time the worker watched it
}

func newBinanceMarketStream(restBaseURL string) *binanceMarketStream {
	return &binanceMarketStream{
		restBaseURL: restBaseURL,
		wsBaseURL:   binanceStreamBaseURL(restBaseURL),
		lastSeen:    make(map[string]time.Time),
	}
}

func (stream *binanceMarketStream) watch(symbols []string) {
	now := time.Now()
	stream.mutex.Lock()
	for _, symbol := range symbols {
		trimmed := strings.ToUpper(strings.TrimSpace(symbol))
		if trimmed != "" {
			stream.lastSeen[trimmed] = now
		}
	}
	stream.mutex.Unlock()
}

// desiredSymbols returns the currently-active symbol set (sorted), aging out anything not watched
// recently so the stream stops paying for coins the user no longer holds.
func (stream *binanceMarketStream) desiredSymbols() []string {
	cutoff := time.Now().Add(-marketSymbolStaleAfter)
	stream.mutex.Lock()
	defer stream.mutex.Unlock()
	symbols := make([]string, 0, len(stream.lastSeen))
	for symbol, seenAt := range stream.lastSeen {
		if seenAt.Before(cutoff) {
			delete(stream.lastSeen, symbol)
			continue
		}
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	return symbols
}

// run owns the connection lifecycle: it (re)connects whenever the desired symbol set changes and reads
// mini-ticker messages into the shared price cache. It exits only when the context is cancelled.
func (stream *binanceMarketStream) run(runContext context.Context) {
	reconnectDelay := marketStreamReconnectMin
	for runContext.Err() == nil {
		symbols := stream.desiredSymbols()
		if len(symbols) == 0 {
			if sleepInterruptible(runContext, marketStreamCheckEvery) {
				return
			}
			continue
		}

		connection, dialError := stream.dial(runContext, symbols)
		if dialError != nil {
			log.Printf("market stream (%s): connect failed: %v", stream.wsBaseURL, dialError)
			if sleepInterruptible(runContext, reconnectDelay) {
				return
			}
			reconnectDelay = nextBackoff(reconnectDelay)
			continue
		}
		reconnectDelay = marketStreamReconnectMin
		stream.readUntilSetChanges(runContext, connection, symbols)
		_ = connection.Close()
	}
}

func (stream *binanceMarketStream) dial(runContext context.Context, symbols []string) (*websocket.Conn, error) {
	streamNames := make([]string, len(symbols))
	for index, symbol := range symbols {
		streamNames[index] = strings.ToLower(symbol) + "@miniTicker"
	}
	endpoint := stream.wsBaseURL + "/stream?streams=" + strings.Join(streamNames, "/")
	dialContext, cancel := context.WithTimeout(runContext, 15*time.Second)
	defer cancel()
	connection, _, dialError := binanceWebsocketDialer().DialContext(dialContext, endpoint, nil)
	return connection, dialError
}

// readUntilSetChanges reads mini-ticker frames into the cache, returning when the desired symbol set
// changes (so run reconnects) or the connection errors.
func (stream *binanceMarketStream) readUntilSetChanges(runContext context.Context, connection *websocket.Conn, connectedSymbols []string) {
	connectedKey := strings.Join(connectedSymbols, ",")
	for runContext.Err() == nil {
		if strings.Join(stream.desiredSymbols(), ",") != connectedKey {
			return // the set changed — reconnect to the new set
		}
		_ = connection.SetReadDeadline(time.Now().Add(marketStreamReadTimeout))
		_, payload, readError := connection.ReadMessage()
		if readError != nil {
			if netError, ok := readError.(net.Error); ok && netError.Timeout() {
				continue // periodic wake to re-check the set; not a real error
			}
			return // genuine disconnect — reconnect
		}
		stream.handleMessage(payload)
	}
}

type combinedStreamFrame struct {
	Data struct {
		Symbol string `json:"s"`
		Close  string `json:"c"`
	} `json:"data"`
}

func (stream *binanceMarketStream) handleMessage(payload []byte) {
	var frame combinedStreamFrame
	if json.Unmarshal(payload, &frame) != nil || frame.Data.Symbol == "" || frame.Data.Close == "" {
		return
	}
	price, parseError := strconv.ParseFloat(frame.Data.Close, 64)
	if parseError != nil || price <= 0 {
		return
	}
	// Same cache key shape as the REST price path, so reads transparently hit these pushed prices.
	storeCachedPrice(stream.restBaseURL+"|"+strings.ToUpper(frame.Data.Symbol), price)
}

// binanceStreamBaseURL maps a REST base URL to the matching market-data WebSocket base.
func binanceStreamBaseURL(restBaseURL string) string {
	if strings.Contains(strings.ToLower(restBaseURL), "testnet") {
		return "wss://stream.testnet.binance.vision"
	}
	return "wss://stream.binance.com:9443"
}

// binanceWebsocketDialer builds a dialer that honours BINANCE_HTTP_PROXY (so a sharded worker can reach
// Binance from its own egress IP, matching the REST client).
func binanceWebsocketDialer() *websocket.Dialer {
	dialer := &websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	if proxyValue := strings.TrimSpace(os.Getenv("BINANCE_HTTP_PROXY")); proxyValue != "" {
		if parsedProxy, parseError := url.Parse(proxyValue); parseError == nil && parsedProxy.Host != "" {
			dialer.Proxy = func(*http.Request) (*url.URL, error) { return parsedProxy, nil }
		}
	}
	return dialer
}

func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > marketStreamReconnectMax {
		return marketStreamReconnectMax
	}
	return next
}

// sleepInterruptible waits for d or until the context is cancelled; returns true if cancelled.
func sleepInterruptible(waitContext context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-waitContext.Done():
		return true
	case <-timer.C:
		return false
	}
}
