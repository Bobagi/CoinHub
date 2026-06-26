package service

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"coin-hub/internal/domain"

	"github.com/gorilla/websocket"
)

// User-data stream: Binance pushes order updates (executionReport) over a per-user WebSocket keyed by a
// listenKey, so a filled take-profit / external cancellation is reconciled within ~a second instead of
// waiting up to the 30s monitor tick — and without the per-IP weight of polling GetOrderStatus for every
// open order every tick. This is the biggest REST-weight reducer as the user base grows.
//
// CRITICAL: this is an ACCELERATOR, not a replacement. On any executionReport it simply asks the worker
// to run its normal reconcile (monitorUser) for that user NOW; the 30s poller keeps running untouched as
// the correctness backstop. So a dropped socket, an expired listenKey, an env switch, or any bug here can
// only make reconciliation SLOWER (back to 30s), never wrong — no fill is ever lost.

const (
	userStreamKeepAliveEvery  = 30 * time.Minute // refresh the listenKey well before its 60-min expiry
	userStreamReadTimeout     = 6 * time.Minute  // > Binance's ~3-min server ping; a quiet stream is fine
	userStreamReconnectMin    = 3 * time.Second
	userStreamReconcileDebounce = 2 * time.Second // coalesce bursts of executionReports into one reconcile
)

// userOrderReconciler runs the worker's normal per-user reconcile (wired to AutomationWorker.monitorUser).
type userOrderReconciler func(reconcileContext context.Context, userIdentifier int64)

// UserDataStreamManager keeps one user-data stream per active user, started/stopped as the worker's
// active-user set changes. It runs on the leader (driven from the worker's monitor loop), so with worker
// sharding the WS connections are naturally split across instances too.
type UserDataStreamManager struct {
	parentContext     context.Context
	credentialService *UserCredentialService
	reconcile         userOrderReconciler
	mutex             sync.Mutex
	streams           map[int64]*userDataStream
	// unsupported is set once Binance reports the classic listenKey endpoint is gone (HTTP 410/404 —
	// Binance deprecated it in favour of the WebSocket-API session, which needs Ed25519 keys). When set,
	// the manager stops opening streams entirely and the 30s poller remains the sole reconciler.
	unsupported bool
}

func NewUserDataStreamManager(parentContext context.Context, credentialService *UserCredentialService, reconcile userOrderReconciler) *UserDataStreamManager {
	return &UserDataStreamManager{
		parentContext:     parentContext,
		credentialService: credentialService,
		reconcile:         reconcile,
		streams:           make(map[int64]*userDataStream),
	}
}

// EnsureUsers reconciles the set of live streams to exactly the given active users: it starts a stream
// for any new user and stops the stream for any user no longer in the set.
func (manager *UserDataStreamManager) EnsureUsers(activeUserIdentifiers []int64) {
	if manager == nil {
		return
	}
	active := make(map[int64]bool, len(activeUserIdentifiers))
	for _, userIdentifier := range activeUserIdentifiers {
		active[userIdentifier] = true
	}

	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.unsupported {
		return // Binance retired the listenKey endpoint; the poller handles reconciliation.
	}
	for userIdentifier, stream := range manager.streams {
		if !active[userIdentifier] {
			stream.cancel()
			delete(manager.streams, userIdentifier)
		}
	}
	for _, userIdentifier := range activeUserIdentifiers {
		if _, present := manager.streams[userIdentifier]; present {
			continue
		}
		streamContext, cancel := context.WithCancel(manager.parentContext)
		stream := &userDataStream{
			userIdentifier:    userIdentifier,
			credentialService: manager.credentialService,
			reconcile:         manager.reconcile,
			cancel:            cancel,
			manager:           manager,
		}
		manager.streams[userIdentifier] = stream
		go stream.run(streamContext)
	}
}

// markUnsupported disables user-data streaming process-wide after Binance reports the listenKey endpoint
// is gone, cancelling every open stream so they stop retrying. Logged once. The poller keeps reconciling.
func (manager *UserDataStreamManager) markUnsupported() {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.unsupported {
		return
	}
	manager.unsupported = true
	log.Printf("user-data stream: Binance reports the listenKey endpoint is gone (deprecated) — disabling real-time order push; the 30s poller remains the reconciler. (Real-time push now needs the WebSocket-API session with Ed25519 keys.)")
	for userIdentifier, stream := range manager.streams {
		stream.cancel()
		delete(manager.streams, userIdentifier)
	}
}

type userDataStream struct {
	userIdentifier    int64
	credentialService *UserCredentialService
	reconcile         userOrderReconciler
	cancel            context.CancelFunc
	manager           *UserDataStreamManager
	debounceMutex     sync.Mutex
	lastReconcileAt   time.Time
}

func (stream *userDataStream) run(streamContext context.Context) {
	reconnectDelay := userStreamReconnectMin
	for streamContext.Err() == nil {
		connected := stream.connectAndServe(streamContext)
		if connected {
			reconnectDelay = userStreamReconnectMin
		}
		if sleepInterruptible(streamContext, reconnectDelay) {
			return
		}
		reconnectDelay = nextBackoff(reconnectDelay)
	}
}

// connectAndServe creates a listenKey, connects the WS, and serves order events until the connection
// drops or the context is cancelled. Returns true if it managed to connect (so backoff resets).
func (stream *userDataStream) connectAndServe(streamContext context.Context) bool {
	environmentConfiguration, configError := stream.credentialService.LoadActiveEnvironmentConfiguration(streamContext, stream.userIdentifier)
	if configError != nil || environmentConfiguration == nil {
		return false // no connected keys for the active env — the poller still covers this user
	}

	listenKey, listenKeyError := createUserDataListenKey(streamContext, environmentConfiguration)
	if listenKeyError != nil {
		// Binance retired the listenKey endpoint (410/404) ⇒ disable user-data streaming everywhere
		// instead of looping per user. The poller stays the reconciler.
		if deprecationError, ok := listenKeyError.(*binanceListenKeyError); ok && deprecationError.isDeprecated() && stream.manager != nil {
			stream.manager.markUnsupported()
			return false
		}
		log.Printf("user-data stream (user %d): could not create listenKey: %v", stream.userIdentifier, listenKeyError)
		return false
	}

	dialContext, dialCancel := context.WithTimeout(streamContext, 15*time.Second)
	connection, _, dialError := binanceWebsocketDialer().DialContext(dialContext, binanceStreamBaseURL(environmentConfiguration.RESTBaseURL)+"/ws/"+listenKey, nil)
	dialCancel()
	if dialError != nil {
		log.Printf("user-data stream (user %d): connect failed: %v", stream.userIdentifier, dialError)
		return false
	}
	defer connection.Close()

	// Scope the helper goroutines to THIS connection (not the whole stream): cancelled when
	// connectAndServe returns, so a reconnect doesn't leak a conn-closer + keepAliveLoop each time.
	connectionContext, cancelConnection := context.WithCancel(streamContext)
	defer cancelConnection()

	// Close the connection when the context ends so the blocking ReadMessage below returns promptly.
	go func() {
		<-connectionContext.Done()
		_ = connection.Close()
	}()

	// Keep the listenKey alive (it expires ~60 min without a keepalive).
	go stream.keepAliveLoop(connectionContext, environmentConfiguration, listenKey)

	// Liveness: Binance pings ~every 3 min; gorilla auto-pongs. Refresh the read deadline on any ping or
	// message, so a genuinely dead connection trips the deadline and we reconnect, but a quiet (no-orders)
	// stream stays up.
	_ = connection.SetReadDeadline(time.Now().Add(userStreamReadTimeout))
	connection.SetPingHandler(func(appData string) error {
		_ = connection.SetReadDeadline(time.Now().Add(userStreamReadTimeout))
		return connection.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
	})

	log.Printf("user-data stream (user %d): connected (%s)", stream.userIdentifier, environmentConfiguration.EnvironmentName)
	for streamContext.Err() == nil {
		_, payload, readError := connection.ReadMessage()
		if readError != nil {
			return true // connected then dropped — reconnect (backoff already reset)
		}
		_ = connection.SetReadDeadline(time.Now().Add(userStreamReadTimeout))
		stream.handleMessage(streamContext, payload)
	}
	return true
}

func (stream *userDataStream) keepAliveLoop(streamContext context.Context, environmentConfiguration *domain.BinanceEnvironmentConfiguration, listenKey string) {
	ticker := time.NewTicker(userStreamKeepAliveEvery)
	defer ticker.Stop()
	for {
		select {
		case <-streamContext.Done():
			return
		case <-ticker.C:
			if keepAliveError := keepAliveUserDataListenKey(streamContext, environmentConfiguration, listenKey); keepAliveError != nil {
				log.Printf("user-data stream (user %d): keepalive failed: %v", stream.userIdentifier, keepAliveError)
			}
		}
	}
}

// handleMessage triggers a reconcile when an order event arrives. It does NOT itself mutate orders — it
// asks the worker to run its normal reconcile, so all the existing fill/cancel logic (and its safety
// checks) is reused. Debounced so a burst of events causes one reconcile.
func (stream *userDataStream) handleMessage(streamContext context.Context, payload []byte) {
	var event struct {
		EventType string `json:"e"`
	}
	if json.Unmarshal(payload, &event) != nil || event.EventType != "executionReport" {
		return
	}

	stream.debounceMutex.Lock()
	if time.Since(stream.lastReconcileAt) < userStreamReconcileDebounce {
		stream.debounceMutex.Unlock()
		return
	}
	stream.lastReconcileAt = time.Now()
	stream.debounceMutex.Unlock()

	go func() {
		reconcileContext, cancel := context.WithTimeout(streamContext, 30*time.Second)
		defer cancel()
		runUserStepSafely(stream.userIdentifier, "ws-reconcile", func() {
			stream.reconcile(reconcileContext, stream.userIdentifier)
		})
	}()
}

func createUserDataListenKey(requestContext context.Context, environmentConfiguration *domain.BinanceEnvironmentConfiguration) (string, error) {
	request, requestError := http.NewRequestWithContext(requestContext, http.MethodPost, environmentConfiguration.RESTBaseURL+"/api/v3/userDataStream", nil)
	if requestError != nil {
		return "", requestError
	}
	request.Header.Set("X-MBX-APIKEY", environmentConfiguration.APIKey)

	response, responseError := newBinanceHTTPClient(10 * time.Second).Do(request)
	if responseError != nil {
		return "", responseError
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", &binanceListenKeyError{statusCode: response.StatusCode}
	}
	var decoded struct {
		ListenKey string `json:"listenKey"`
	}
	if decodeError := json.NewDecoder(response.Body).Decode(&decoded); decodeError != nil {
		return "", decodeError
	}
	if decoded.ListenKey == "" {
		return "", &binanceListenKeyError{statusCode: response.StatusCode}
	}
	return decoded.ListenKey, nil
}

func keepAliveUserDataListenKey(requestContext context.Context, environmentConfiguration *domain.BinanceEnvironmentConfiguration, listenKey string) error {
	endpoint := environmentConfiguration.RESTBaseURL + "/api/v3/userDataStream?listenKey=" + url.QueryEscape(listenKey)
	request, requestError := http.NewRequestWithContext(requestContext, http.MethodPut, endpoint, nil)
	if requestError != nil {
		return requestError
	}
	request.Header.Set("X-MBX-APIKEY", environmentConfiguration.APIKey)
	response, responseError := newBinanceHTTPClient(10 * time.Second).Do(request)
	if responseError != nil {
		return responseError
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &binanceListenKeyError{statusCode: response.StatusCode}
	}
	return nil
}

type binanceListenKeyError struct{ statusCode int }

func (listenKeyError *binanceListenKeyError) Error() string {
	return "binance userDataStream request returned status " + strconv.Itoa(listenKeyError.statusCode)
}

// isDeprecated reports whether the status means the endpoint itself is gone (Binance deprecated the
// classic listenKey), as opposed to a transient/auth error worth retrying.
func (listenKeyError *binanceListenKeyError) isDeprecated() bool {
	return listenKeyError.statusCode == http.StatusGone || listenKeyError.statusCode == http.StatusNotFound
}
