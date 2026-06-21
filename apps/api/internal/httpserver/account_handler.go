package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"coin-hub/internal/service"
)

// avatarProxyClient fetches Google profile pictures server-side so they can be served same-origin
// under the strict CSP. Redirects are pinned to googleusercontent.com to avoid SSRF via redirect.
var avatarProxyClient = &http.Client{
	Timeout: 8 * time.Second,
	CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if !isGoogleUserContentHost(request.URL.Hostname()) {
			return errors.New("avatar redirect to a disallowed host")
		}
		return nil
	},
}

// isGoogleUserContentHost reports whether a host belongs to Google's user-content CDN (where profile
// pictures live), used both to validate the stored URL and to pin redirects.
func isGoogleUserContentHost(host string) bool {
	host = strings.ToLower(host)
	return host == "googleusercontent.com" || strings.HasSuffix(host, ".googleusercontent.com")
}

// AccountHandler serves the session-protected account-management endpoints: editing the profile,
// setting/changing the password, permanently deleting the account, and listing the account's
// sign-in (access) history.
type AccountHandler struct {
	authService      *service.AuthService
	sessionService   *service.SessionService
	accessLogService *service.AccessLogService
	agreementService *service.AgreementService
	cookieName       string
	secureCookies    bool
}

func NewAccountHandler(authService *service.AuthService, sessionService *service.SessionService, accessLogService *service.AccessLogService, agreementService *service.AgreementService, cookieName string, secureCookies bool) *AccountHandler {
	return &AccountHandler{
		authService:      authService,
		sessionService:   sessionService,
		accessLogService: accessLogService,
		agreementService: agreementService,
		cookieName:       cookieName,
		secureCookies:    secureCookies,
	}
}

func (handler *AccountHandler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("/api/v1/account/profile", handler.handleProfile)
	router.HandleFunc("/api/v1/account/password", handler.handlePassword)
	router.HandleFunc("/api/v1/account/access", handler.handleAccessHistory)
	router.HandleFunc("/api/v1/account/avatar", handler.handleAvatar)
	router.HandleFunc("/api/v1/account/agreement/accept", handler.handleAcceptAgreement)
	router.HandleFunc("/api/v1/account", handler.handleDeleteAccount)
}

// handleAcceptAgreement records the authenticated user's consent to the current Terms of Use + Privacy
// Policy (version, timestamp, IP, user agent) and returns the refreshed user so the SPA can drop its
// blocking acceptance gate. This is the durable, server-side proof of consent.
func (handler *AccountHandler) handleAcceptAgreement(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userIdentifier, authenticated := handler.requireUser(responseWriter, request)
	if !authenticated {
		return
	}

	operationContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	if recordError := handler.agreementService.Accept(operationContext, userIdentifier, clientIPAddress(request), request.UserAgent()); recordError != nil {
		writeJSONError(responseWriter, http.StatusInternalServerError, "Could not record your acceptance. Please try again.")
		return
	}

	currentUser, lookupError := handler.authService.GetUserByIdentifier(operationContext, userIdentifier)
	if lookupError != nil || currentUser == nil {
		writeJSONError(responseWriter, http.StatusInternalServerError, "Could not load your account.")
		return
	}
	writeJSON(responseWriter, http.StatusOK, toUserResponse(currentUser, true))
}

// handleAvatar proxies the authenticated user's Google profile picture same-origin, so the header
// avatar loads under the vhost CSP (img-src 'self'), which would block googleusercontent.com directly.
// Any failure (no avatar, expired URL, non-image) returns 404 so the SPA falls back to the initial.
func (handler *AccountHandler) handleAvatar(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userIdentifier, authenticated := handler.requireUser(responseWriter, request)
	if !authenticated {
		return
	}

	lookupContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	account, lookupError := handler.authService.GetUserByIdentifier(lookupContext, userIdentifier)
	if lookupError != nil || strings.TrimSpace(account.AvatarURL) == "" {
		http.NotFound(responseWriter, request)
		return
	}

	parsedURL, parseError := url.Parse(strings.TrimSpace(account.AvatarURL))
	if parseError != nil || parsedURL.Scheme != "https" || !isGoogleUserContentHost(parsedURL.Hostname()) {
		http.NotFound(responseWriter, request)
		return
	}

	fetchContext, fetchCancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer fetchCancel()
	upstreamRequest, requestError := http.NewRequestWithContext(fetchContext, http.MethodGet, parsedURL.String(), nil)
	if requestError != nil {
		http.NotFound(responseWriter, request)
		return
	}
	upstreamResponse, responseError := avatarProxyClient.Do(upstreamRequest)
	if responseError != nil {
		http.NotFound(responseWriter, request)
		return
	}
	defer upstreamResponse.Body.Close()

	contentType := upstreamResponse.Header.Get("Content-Type")
	if upstreamResponse.StatusCode != http.StatusOK || !strings.HasPrefix(contentType, "image/") {
		http.NotFound(responseWriter, request)
		return
	}

	responseWriter.Header().Set("Content-Type", contentType)
	responseWriter.Header().Set("Cache-Control", "private, max-age=3600")
	responseWriter.WriteHeader(http.StatusOK)
	_, _ = io.Copy(responseWriter, io.LimitReader(upstreamResponse.Body, 5<<20))
}

func (handler *AccountHandler) requireUser(responseWriter http.ResponseWriter, request *http.Request) (int64, bool) {
	sessionCookie, cookieError := request.Cookie(handler.cookieName)
	if cookieError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return 0, false
	}
	resolveContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	userIdentifier, resolveError := handler.sessionService.ResolveUserIdentifier(resolveContext, sessionCookie.Value)
	if resolveError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return 0, false
	}
	return userIdentifier, true
}

func (handler *AccountHandler) handleProfile(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPut {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userIdentifier, authenticated := handler.requireUser(responseWriter, request)
	if !authenticated {
		return
	}

	var payload struct {
		DisplayName string `json:"display_name"`
	}
	if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
		writeJSONError(responseWriter, http.StatusBadRequest, "Invalid request body.")
		return
	}
	if len(strings.TrimSpace(payload.DisplayName)) > 120 {
		writeJSONError(responseWriter, http.StatusBadRequest, "Name is too long (max 120 characters).")
		return
	}

	operationContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	updatedUser, updateError := handler.authService.UpdateDisplayName(operationContext, userIdentifier, payload.DisplayName)
	if updateError != nil {
		writeJSONError(responseWriter, http.StatusInternalServerError, "Could not update your profile.")
		return
	}
	termsAccepted := termsAcceptedFor(operationContext, handler.agreementService, updatedUser.Identifier)
	writeJSON(responseWriter, http.StatusOK, toUserResponse(updatedUser, termsAccepted))
}

func (handler *AccountHandler) handlePassword(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userIdentifier, authenticated := handler.requireUser(responseWriter, request)
	if !authenticated {
		return
	}

	var payload struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
		writeJSONError(responseWriter, http.StatusBadRequest, "Invalid request body.")
		return
	}

	operationContext, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	changeError := handler.authService.SetOrChangePassword(operationContext, userIdentifier, payload.CurrentPassword, payload.NewPassword)
	if changeError != nil {
		switch {
		case errors.Is(changeError, service.ErrIncorrectPassword):
			writeJSONError(responseWriter, http.StatusBadRequest, "Your current password is incorrect.")
		case errors.Is(changeError, service.ErrWeakPassword):
			writeJSONError(responseWriter, http.StatusBadRequest, service.ErrWeakPassword.Error())
		default:
			writeJSONError(responseWriter, http.StatusInternalServerError, "Could not update your password.")
		}
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"message": "Password updated."})
}

type accessEventPayload struct {
	Identifier  int64  `json:"id"`
	IPAddress   string `json:"ip_address"`
	Device      string `json:"user_agent"`
	AuthMethod  string `json:"auth_method"`
	IsNewDevice bool   `json:"is_new_device"`
	CountryCode string `json:"country_code"`
	CountryName string `json:"country_name"`
	Region      string `json:"region"`
	City        string `json:"city"`
	CreatedAt   string `json:"created_at"`
}

// handleAccessHistory returns a page of the account's durable sign-in history (newest first), so the
// user can review when and from where their account was accessed.
func (handler *AccountHandler) handleAccessHistory(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userIdentifier, authenticated := handler.requireUser(responseWriter, request)
	if !authenticated {
		return
	}

	page := parsePositiveQuery(request, "page", 1)
	pageSize := parsePositiveQuery(request, "page_size", 10)
	if pageSize > 50 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	operationContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	events, total, listError := handler.accessLogService.ListAccess(operationContext, userIdentifier, pageSize, offset)
	if listError != nil {
		writeJSONError(responseWriter, http.StatusInternalServerError, "Could not load your access history.")
		return
	}

	payload := make([]accessEventPayload, 0, len(events))
	for _, event := range events {
		payload = append(payload, accessEventPayload{
			Identifier:  event.Identifier,
			IPAddress:   event.IPAddress,
			Device:      event.UserAgent,
			AuthMethod:  event.AuthMethod,
			IsNewDevice: event.IsNewDevice,
			CountryCode: event.CountryCode,
			CountryName: event.CountryName,
			Region:      event.Region,
			City:        event.City,
			CreatedAt:   event.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(responseWriter, http.StatusOK, map[string]interface{}{"events": payload, "total": total})
}

// parsePositiveQuery reads a positive integer query parameter, returning a fallback for missing or
// invalid values.
func parsePositiveQuery(request *http.Request, name string, fallback int) int {
	raw := request.URL.Query().Get(name)
	if raw == "" {
		return fallback
	}
	value, parseError := strconv.Atoi(raw)
	if parseError != nil || value < 1 {
		return fallback
	}
	return value
}

func (handler *AccountHandler) handleDeleteAccount(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodDelete {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userIdentifier, authenticated := handler.requireUser(responseWriter, request)
	if !authenticated {
		return
	}

	var payload struct {
		Password string `json:"password"`
		Confirm  bool   `json:"confirm"`
	}
	if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
		writeJSONError(responseWriter, http.StatusBadRequest, "Invalid request body.")
		return
	}
	if !payload.Confirm {
		writeJSONError(responseWriter, http.StatusBadRequest, "Account deletion must be explicitly confirmed.")
		return
	}

	operationContext, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	deletionError := handler.authService.DeleteAccount(operationContext, userIdentifier, payload.Password)
	if deletionError != nil {
		if errors.Is(deletionError, service.ErrIncorrectPassword) {
			writeJSONError(responseWriter, http.StatusBadRequest, "Your password is incorrect.")
			return
		}
		writeJSONError(responseWriter, http.StatusInternalServerError, "Could not delete your account.")
		return
	}

	// Best-effort revoke of the current session, then clear the cookie so the browser is signed out.
	if sessionCookie, cookieError := request.Cookie(handler.cookieName); cookieError == nil {
		revokeContext, revokeCancel := context.WithTimeout(request.Context(), 5*time.Second)
		defer revokeCancel()
		_ = handler.sessionService.RevokeSession(revokeContext, sessionCookie.Value)
	}
	handler.clearSessionCookie(responseWriter)
	writeJSON(responseWriter, http.StatusOK, map[string]string{"message": "Account deleted."})
}

func (handler *AccountHandler) clearSessionCookie(responseWriter http.ResponseWriter) {
	http.SetCookie(responseWriter, &http.Cookie{
		Name:     handler.cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   handler.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}
