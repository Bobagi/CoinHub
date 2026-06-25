package httpserver

import (
	"context"
	"net/http"
	"time"

	"coin-hub/internal/service"
)

// SystemHandler exposes operational health: a session-protected status the SPA polls to drive the
// header indicator/banner, and a public liveness probe for an external uptime monitor.
type SystemHandler struct {
	sessionService *service.SessionService
	statusService  *service.OperationalStatusService
	cookieName     string
}

func NewSystemHandler(sessionService *service.SessionService, statusService *service.OperationalStatusService, cookieName string) *SystemHandler {
	return &SystemHandler{sessionService: sessionService, statusService: statusService, cookieName: cookieName}
}

func (handler *SystemHandler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("/api/v1/system/status", handler.handleStatus)
	// Public liveness probe for an external uptime monitor: 200 while automation is alive, 503 when the
	// worker's heartbeat is stale (so a monitor can page even if the whole API process is wedged/dead).
	router.HandleFunc("/health/worker", handler.handleWorkerHealth)
}

// handleStatus returns the aggregate operational status (worker liveness + Binance rate-limit gate). It
// requires a session so internal pressure signals aren't exposed to the public.
func (handler *SystemHandler) handleStatus(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sessionCookie, cookieError := request.Cookie(handler.cookieName)
	if cookieError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}
	resolveContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	if _, resolveError := handler.sessionService.ResolveUserIdentifier(resolveContext, sessionCookie.Value); resolveError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}

	writeJSON(responseWriter, http.StatusOK, handler.statusService.Current(resolveContext))
}

// handleWorkerHealth is an unauthenticated liveness probe: 200 when the worker heartbeat is fresh, 503
// when it is stale. Designed for an external uptime monitor (UptimeRobot, etc.).
func (handler *SystemHandler) handleWorkerHealth(responseWriter http.ResponseWriter, request *http.Request) {
	checkContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	if handler.statusService.WorkerStalled(checkContext) {
		responseWriter.WriteHeader(http.StatusServiceUnavailable)
		_, _ = responseWriter.Write([]byte("worker stalled"))
		return
	}
	responseWriter.WriteHeader(http.StatusOK)
	_, _ = responseWriter.Write([]byte("ok"))
}
