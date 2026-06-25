package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"coin-hub/internal/domain"
	"coin-hub/internal/repository"
	"coin-hub/internal/security"
	"coin-hub/internal/service"
)

// postLoginRedirectPath is where the browser lands after a successful Google sign-in.
const postLoginRedirectPath = "/"

// stepUpWindow is how long a step-up ("sudo") re-authentication stays valid before a sensitive
// action requires re-proving identity again.
const stepUpWindow = 10 * time.Minute

var errNotAuthenticated = errors.New("not authenticated")

// AuthHandler exposes the authentication endpoints and session-cookie handling. It is
// self-contained and registers itself onto a mux, so it does not depend on the legacy Server.
type AuthHandler struct {
	AuthService         *service.AuthService
	SessionService      *service.SessionService
	GoogleOAuthService  *service.GoogleOAuthService // nil when Google sign-in is not configured
	AccountEmailService *service.AccountEmailService
	AccessLogService    *service.AccessLogService
	AgreementService    *service.AgreementService
	CookieName          string
	OAuthStateCookie    string
	StepUpStateCookie   string
	SecureCookies       bool
	SecretCipher        *security.SecretCipher // signs the Google step-up state cookie; may be nil
	loginThrottle       *loginThrottle
}

func NewAuthHandler(authService *service.AuthService, sessionService *service.SessionService, googleOAuthService *service.GoogleOAuthService, accountEmailService *service.AccountEmailService, accessLogService *service.AccessLogService, agreementService *service.AgreementService, secretCipher *security.SecretCipher, secureCookies bool) *AuthHandler {
	return &AuthHandler{
		AuthService:         authService,
		SessionService:      sessionService,
		GoogleOAuthService:  googleOAuthService,
		AccountEmailService: accountEmailService,
		AccessLogService:    accessLogService,
		AgreementService:    agreementService,
		CookieName:          "coin_hub_session",
		OAuthStateCookie:    "coin_hub_oauth_state",
		StepUpStateCookie:   "coin_hub_stepup",
		SecureCookies:       secureCookies,
		SecretCipher:        secretCipher,
		loginThrottle:       newLoginThrottle(),
	}
}

// recordAccess logs a successful sign-in to the durable access history (and, for a new device on an
// existing account, triggers the security-alert email). Best-effort and asynchronous.
func (handler *AuthHandler) recordAccess(request *http.Request, user *domain.User, authMethod string) {
	if handler.AccessLogService == nil || user == nil {
		return
	}
	handler.AccessLogService.RecordLoginAsync(
		user.Identifier,
		user.Email,
		clientIPAddress(request),
		request.UserAgent(),
		authMethod,
		resolveRequestLocale(request, ""),
	)
}

func (handler *AuthHandler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("/auth/signup", handler.handleSignup)
	router.HandleFunc("/auth/login", handler.handleLogin)
	router.HandleFunc("/auth/logout", handler.handleLogout)
	router.HandleFunc("/auth/me", handler.handleCurrentUser)
	router.HandleFunc("/auth/providers", handler.handleProviders)
	router.HandleFunc("/auth/google/login", handler.handleGoogleLogin)
	router.HandleFunc("/auth/google/callback", handler.handleGoogleCallback)
	router.HandleFunc("/auth/password/forgot", handler.handleForgotPassword)
	router.HandleFunc("/auth/password/reset", handler.handleResetPassword)
	router.HandleFunc("/auth/email/verify", handler.handleVerifyEmail)
	router.HandleFunc("/auth/email/resend", handler.handleResendVerification)
	router.HandleFunc("/auth/step-up", handler.handleStepUpStatus)
	router.HandleFunc("/auth/step-up/password", handler.handleStepUpPassword)
	router.HandleFunc("/auth/step-up/google/start", handler.handleStepUpGoogleStart)
}

type signupRequestPayload struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Locale      string `json:"locale"`
}

type loginRequestPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponsePayload struct {
	Identifier      int64  `json:"id"`
	Email           string `json:"email"`
	DisplayName     string `json:"display_name"`
	HasPassword     bool   `json:"has_password"`
	GoogleConnected bool   `json:"google_connected"`
	IsAdmin         bool   `json:"is_admin"`
	EmailVerified   bool   `json:"email_verified"`
	CreatedAt       string `json:"created_at"`
	// TermsAccepted reports whether the user has accepted the version of the Terms of Use + Privacy
	// Policy currently in force (domain.CurrentAgreementVersion). When false the SPA shows a blocking
	// acceptance gate and the API refuses money/robot actions (code: terms_not_accepted).
	TermsAccepted bool `json:"terms_accepted"`
	// TermsVersion is the version tag of the documents currently in force, so the UI records consent
	// against the same version the server expects.
	TermsVersion string `json:"terms_version"`
	// AvatarURL is the same-origin proxy path for the user's Google picture, or empty when there is
	// none. The image bytes are streamed by GET /api/v1/account/avatar (kept off googleusercontent so
	// it loads under the strict img-src 'self' CSP).
	AvatarURL string `json:"avatar_url"`
}

func (handler *AuthHandler) handleSignup(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload signupRequestPayload
	if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
		writeJSONError(responseWriter, http.StatusBadRequest, "Invalid request body.")
		return
	}

	registrationContext, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()

	createdUser, registrationError := handler.AuthService.Register(registrationContext, payload.Email, payload.Password, payload.DisplayName)
	if registrationError != nil {
		handler.writeRegistrationError(responseWriter, registrationError)
		return
	}

	// Best-effort: send the email-confirmation link. A failure must not block signup.
	if sendError := handler.AccountEmailService.SendVerificationEmail(registrationContext, createdUser.Identifier, createdUser.Email, resolveRequestLocale(request, payload.Locale)); sendError != nil {
		log.Printf("Could not send verification email for user %d: %v", createdUser.Identifier, sendError)
	}

	handler.recordAccess(request, createdUser, domain.AccessMethodSignup)
	handler.issueSessionAndRespond(responseWriter, request, createdUser)
}

func (handler *AuthHandler) handleLogin(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload loginRequestPayload
	if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
		writeJSONError(responseWriter, http.StatusBadRequest, "Invalid request body.")
		return
	}

	// Per-account lockout: too many recent failures for this email get a 429 regardless of source IP,
	// blunting distributed credential-stuffing that slips past nginx's per-IP limit.
	if handler.loginThrottle.IsLocked(payload.Email) {
		writeJSONError(responseWriter, http.StatusTooManyRequests, "Too many sign-in attempts. Please wait a few minutes and try again.")
		return
	}

	authenticationContext, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()

	authenticatedUser, authenticationError := handler.AuthService.Authenticate(authenticationContext, payload.Email, payload.Password)
	if authenticationError != nil {
		if errors.Is(authenticationError, service.ErrInvalidCredentials) || errors.Is(authenticationError, service.ErrAccountDisabled) {
			handler.loginThrottle.RegisterFailure(payload.Email)
			writeJSONError(responseWriter, http.StatusUnauthorized, authenticationError.Error())
			return
		}
		log.Printf("Login failed unexpectedly: %v", authenticationError)
		writeJSONError(responseWriter, http.StatusInternalServerError, "Could not sign in.")
		return
	}

	handler.loginThrottle.RegisterSuccess(payload.Email)
	handler.recordAccess(request, authenticatedUser, domain.AccessMethodPassword)
	handler.issueSessionAndRespond(responseWriter, request, authenticatedUser)
}

func (handler *AuthHandler) handleLogout(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if sessionCookie, cookieError := request.Cookie(handler.CookieName); cookieError == nil {
		revokeContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
		defer cancel()
		if revokeError := handler.SessionService.RevokeSession(revokeContext, sessionCookie.Value); revokeError != nil {
			log.Printf("Could not revoke session on logout: %v", revokeError)
		}
	}

	handler.clearSessionCookie(responseWriter)
	writeJSON(responseWriter, http.StatusOK, map[string]string{"message": "Signed out."})
}

func (handler *AuthHandler) handleCurrentUser(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	userIdentifier, authenticationError := handler.ResolveAuthenticatedUserIdentifier(request)
	if authenticationError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}

	lookupContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	currentUser, lookupError := handler.AuthService.GetUserByIdentifier(lookupContext, userIdentifier)
	if lookupError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}

	termsAccepted := termsAcceptedFor(lookupContext, handler.AgreementService, currentUser.Identifier)
	writeJSON(responseWriter, http.StatusOK, toUserResponse(currentUser, termsAccepted))
}

// ResolveAuthenticatedUserIdentifier reads and validates the session cookie. It is exported so
// future protected handlers (and middleware) can reuse it.
func (handler *AuthHandler) ResolveAuthenticatedUserIdentifier(request *http.Request) (int64, error) {
	sessionCookie, cookieError := request.Cookie(handler.CookieName)
	if cookieError != nil {
		return 0, errNotAuthenticated
	}

	resolveContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	userIdentifier, resolveError := handler.SessionService.ResolveUserIdentifier(resolveContext, sessionCookie.Value)
	if resolveError != nil {
		return 0, errNotAuthenticated
	}
	return userIdentifier, nil
}

func (handler *AuthHandler) issueSessionAndRespond(responseWriter http.ResponseWriter, request *http.Request, user *domain.User) {
	if issueError := handler.issueSessionCookie(responseWriter, request, user); issueError != nil {
		writeJSONError(responseWriter, http.StatusInternalServerError, "Could not start a session.")
		return
	}
	termsAccepted := termsAcceptedFor(request.Context(), handler.AgreementService, user.Identifier)
	writeJSON(responseWriter, http.StatusOK, toUserResponse(user, termsAccepted))
}

// issueSessionCookie creates a session for the user and writes the session cookie. Callers decide
// how to respond afterwards (JSON for the email flow, a redirect for the OAuth callback).
func (handler *AuthHandler) issueSessionCookie(responseWriter http.ResponseWriter, request *http.Request, user *domain.User) error {
	sessionContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()

	rawToken, expiresAt, issueError := handler.SessionService.IssueSession(sessionContext, user.Identifier, request.UserAgent(), clientIPAddress(request))
	if issueError != nil {
		log.Printf("Could not issue session for user %d: %v", user.Identifier, issueError)
		return issueError
	}
	handler.setSessionCookie(responseWriter, rawToken, expiresAt)
	return nil
}

// handleProviders reports which third-party sign-in methods are available, so the SPA only renders
// buttons it can actually use.
func (handler *AuthHandler) handleProviders(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]bool{
		"google": handler.GoogleOAuthService != nil,
		"email":  handler.AccountEmailService.EmailEnabled(),
	})
}

func (handler *AuthHandler) handleGoogleLogin(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if handler.GoogleOAuthService == nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?login_error=google_unavailable", http.StatusSeeOther)
		return
	}

	state, stateError := generateOAuthState()
	if stateError != nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?login_error=google", http.StatusSeeOther)
		return
	}
	handler.setStateCookie(responseWriter, state)
	http.Redirect(responseWriter, request, handler.GoogleOAuthService.AuthorizationURL(state), http.StatusSeeOther)
}

func (handler *AuthHandler) handleGoogleCallback(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// A step-up re-confirm reuses this same redirect URI; if its state cookie is present, handle it
	// here and skip the normal sign-in path.
	if handled := handler.completeGoogleStepUp(responseWriter, request); handled {
		return
	}

	handler.clearStateCookie(responseWriter)

	if handler.GoogleOAuthService == nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?login_error=google_unavailable", http.StatusSeeOther)
		return
	}

	stateCookie, cookieError := request.Cookie(handler.OAuthStateCookie)
	queryState := request.URL.Query().Get("state")
	if cookieError != nil || queryState == "" || stateCookie.Value != queryState {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?login_error=google", http.StatusSeeOther)
		return
	}

	authorizationCode := request.URL.Query().Get("code")
	if authorizationCode == "" {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?login_error=google", http.StatusSeeOther)
		return
	}

	exchangeContext, cancel := context.WithTimeout(request.Context(), 12*time.Second)
	defer cancel()
	googleProfile, profileError := handler.GoogleOAuthService.ExchangeCodeForUserInfo(exchangeContext, authorizationCode)
	if profileError != nil {
		log.Printf("Google sign-in failed during code exchange: %v", profileError)
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?login_error=google", http.StatusSeeOther)
		return
	}

	authenticatedUser, authenticationError := handler.AuthService.AuthenticateWithGoogle(exchangeContext, googleProfile)
	if authenticationError != nil {
		log.Printf("Google sign-in could not resolve an account: %v", authenticationError)
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?login_error=google", http.StatusSeeOther)
		return
	}

	if issueError := handler.issueSessionCookie(responseWriter, request, authenticatedUser); issueError != nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?login_error=google", http.StatusSeeOther)
		return
	}
	handler.recordAccess(request, authenticatedUser, domain.AccessMethodGoogle)
	http.Redirect(responseWriter, request, postLoginRedirectPath, http.StatusSeeOther)
}

// handleForgotPassword emails a password-reset link. It always responds 200 so it cannot be used to
// probe which emails have accounts.
func (handler *AuthHandler) handleForgotPassword(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Email  string `json:"email"`
		Locale string `json:"locale"`
	}
	if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
		writeJSONError(responseWriter, http.StatusBadRequest, "Invalid request body.")
		return
	}
	operationContext, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	if resetError := handler.AccountEmailService.RequestPasswordReset(operationContext, payload.Email, resolveRequestLocale(request, payload.Locale)); resetError != nil {
		log.Printf("Password reset request failed: %v", resetError)
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"message": "If that email has an account, a reset link is on its way."})
}

func (handler *AuthHandler) handleResetPassword(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
		writeJSONError(responseWriter, http.StatusBadRequest, "Invalid request body.")
		return
	}
	if payload.Token == "" {
		writeJSONError(responseWriter, http.StatusBadRequest, "A reset token is required.")
		return
	}
	operationContext, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	resetError := handler.AccountEmailService.ConfirmPasswordReset(operationContext, payload.Token, payload.NewPassword)
	switch {
	case resetError == nil:
		writeJSON(responseWriter, http.StatusOK, map[string]string{"message": "Password updated. You can sign in now."})
	case errors.Is(resetError, repository.ErrAuthTokenInvalid):
		writeJSONError(responseWriter, http.StatusBadRequest, "This reset link is invalid or has expired. Request a new one.")
	case errors.Is(resetError, service.ErrWeakPassword):
		writeJSONError(responseWriter, http.StatusBadRequest, service.ErrWeakPassword.Error())
	default:
		log.Printf("Password reset failed: %v", resetError)
		writeJSONError(responseWriter, http.StatusInternalServerError, "Could not reset your password.")
	}
}

func (handler *AuthHandler) handleVerifyEmail(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Token string `json:"token"`
	}
	if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
		writeJSONError(responseWriter, http.StatusBadRequest, "Invalid request body.")
		return
	}
	if payload.Token == "" {
		writeJSONError(responseWriter, http.StatusBadRequest, "A verification token is required.")
		return
	}
	operationContext, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	verifyError := handler.AccountEmailService.ConfirmEmailVerification(operationContext, payload.Token)
	switch {
	case verifyError == nil:
		writeJSON(responseWriter, http.StatusOK, map[string]string{"message": "Email confirmed."})
	case errors.Is(verifyError, repository.ErrAuthTokenInvalid):
		writeJSONError(responseWriter, http.StatusBadRequest, "This confirmation link is invalid or has expired.")
	default:
		log.Printf("Email verification failed: %v", verifyError)
		writeJSONError(responseWriter, http.StatusInternalServerError, "Could not confirm your email.")
	}
}

func (handler *AuthHandler) handleResendVerification(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userIdentifier, authenticationError := handler.ResolveAuthenticatedUserIdentifier(request)
	if authenticationError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}
	operationContext, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	currentUser, lookupError := handler.AuthService.GetUserByIdentifier(operationContext, userIdentifier)
	if lookupError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}
	if currentUser.IsEmailVerified() {
		writeJSON(responseWriter, http.StatusOK, map[string]string{"message": "Your email is already confirmed."})
		return
	}
	if sendError := handler.AccountEmailService.SendVerificationEmail(operationContext, userIdentifier, currentUser.Email, resolveRequestLocale(request, "")); sendError != nil {
		log.Printf("Could not resend verification email for user %d: %v", userIdentifier, sendError)
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"message": "Verification email sent."})
}

// handleStepUpStatus reports whether the current session has a fresh step-up window, and which
// methods the account can use to re-authenticate (password and/or Google).
func (handler *AuthHandler) handleStepUpStatus(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sessionCookie, cookieError := request.Cookie(handler.CookieName)
	if cookieError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}
	operationContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	expiresAt, fresh, statusError := handler.SessionService.StepUpExpiry(operationContext, sessionCookie.Value, stepUpWindow)
	if statusError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}
	currentUser, lookupError := handler.AuthService.GetUserByIdentifier(operationContext, mustResolveUserID(operationContext, handler, sessionCookie.Value))
	if lookupError != nil || currentUser == nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}
	payload := map[string]interface{}{
		"fresh":           fresh,
		"window_seconds":  int(stepUpWindow.Seconds()),
		"password_method": currentUser.HasPassword(),
		"google_method":   currentUser.HasGoogleLinked() && handler.GoogleOAuthService != nil,
	}
	if fresh {
		payload["expires_at"] = expiresAt.Format(time.RFC3339)
	}
	writeJSON(responseWriter, http.StatusOK, payload)
}

// handleStepUpPassword re-verifies the account password and refreshes the session's step-up window.
func (handler *AuthHandler) handleStepUpPassword(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sessionCookie, cookieError := request.Cookie(handler.CookieName)
	if cookieError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}
	var payload struct {
		Password string `json:"password"`
	}
	if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
		writeJSONError(responseWriter, http.StatusBadRequest, "Invalid request body.")
		return
	}

	operationContext, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	userIdentifier, resolveError := handler.SessionService.ResolveUserIdentifier(operationContext, sessionCookie.Value)
	if resolveError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}
	currentUser, lookupError := handler.AuthService.GetUserByIdentifier(operationContext, userIdentifier)
	if lookupError != nil || currentUser == nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return
	}

	// Reuse the per-account lockout so step-up cannot be used to brute-force the password either.
	if handler.loginThrottle.IsLocked(currentUser.Email) {
		writeJSONError(responseWriter, http.StatusTooManyRequests, "Too many attempts. Please wait a few minutes and try again.")
		return
	}
	verifyError := handler.AuthService.VerifyStepUpPassword(operationContext, userIdentifier, payload.Password)
	if verifyError != nil {
		switch {
		case errors.Is(verifyError, service.ErrPasswordNotSet):
			writeJSONErrorCode(responseWriter, http.StatusBadRequest, "This account has no password. Re-confirm with Google or set a password first.", "password_not_set")
		default:
			handler.loginThrottle.RegisterFailure(currentUser.Email)
			writeJSONError(responseWriter, http.StatusUnauthorized, "Your password is incorrect.")
		}
		return
	}
	handler.loginThrottle.RegisterSuccess(currentUser.Email)
	if markError := handler.SessionService.MarkStepUpByToken(operationContext, sessionCookie.Value); markError != nil {
		writeJSONError(responseWriter, http.StatusInternalServerError, "Could not confirm your identity.")
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{
		"message":    "Identity confirmed.",
		"expires_at": time.Now().Add(stepUpWindow).Format(time.RFC3339),
	})
}

// handleStepUpGoogleStart begins a Google re-confirm (prompt=login). The original session cookie is
// SameSite=Strict and will NOT come back through Google's cross-site redirect, so we stash the
// user id in a signed, Lax state cookie that does survive the round trip; the shared callback reads
// it to complete the step-up.
func (handler *AuthHandler) handleStepUpGoogleStart(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if handler.GoogleOAuthService == nil || handler.SecretCipher == nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=unavailable", http.StatusSeeOther)
		return
	}
	sessionCookie, cookieError := request.Cookie(handler.CookieName)
	if cookieError != nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return
	}
	operationContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	userIdentifier, resolveError := handler.SessionService.ResolveUserIdentifier(operationContext, sessionCookie.Value)
	if resolveError != nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return
	}

	state, stateError := generateOAuthState()
	if stateError != nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return
	}
	// Cookie payload: "state:userID", signed so it cannot be forged. The Google subject match in the
	// callback is the real authorization gate; the signature is defense in depth.
	signedValue := handler.SecretCipher.SignValue(state + ":" + strconv.FormatInt(userIdentifier, 10))
	handler.setStepUpStateCookie(responseWriter, signedValue)
	http.Redirect(responseWriter, request, handler.GoogleOAuthService.ReauthorizationURL(state), http.StatusSeeOther)
}

// completeGoogleStepUp finishes the Google re-confirm flow when the callback finds a step-up state
// cookie. It returns true if it handled the request (so the normal login path is skipped).
func (handler *AuthHandler) completeGoogleStepUp(responseWriter http.ResponseWriter, request *http.Request) bool {
	stepUpCookie, cookieError := request.Cookie(handler.StepUpStateCookie)
	if cookieError != nil {
		return false
	}
	handler.clearStepUpStateCookie(responseWriter)

	if handler.GoogleOAuthService == nil || handler.SecretCipher == nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return true
	}
	signedPayload, signatureValid := handler.SecretCipher.VerifyValue(stepUpCookie.Value)
	if !signatureValid {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return true
	}
	separatorIndex := strings.LastIndexByte(signedPayload, ':')
	if separatorIndex <= 0 {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return true
	}
	expectedState := signedPayload[:separatorIndex]
	userIdentifier, parseError := strconv.ParseInt(signedPayload[separatorIndex+1:], 10, 64)
	if parseError != nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return true
	}

	queryState := request.URL.Query().Get("state")
	authorizationCode := request.URL.Query().Get("code")
	if queryState == "" || queryState != expectedState || authorizationCode == "" {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return true
	}

	operationContext, cancel := context.WithTimeout(request.Context(), 12*time.Second)
	defer cancel()
	googleProfile, profileError := handler.GoogleOAuthService.ExchangeCodeForUserInfo(operationContext, authorizationCode)
	if profileError != nil {
		log.Printf("Google step-up failed during code exchange: %v", profileError)
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return true
	}

	currentUser, lookupError := handler.AuthService.GetUserByIdentifier(operationContext, userIdentifier)
	if lookupError != nil || currentUser == nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return true
	}
	// Authorization gate: the freshly re-authenticated Google identity must be the SAME account.
	if !googleProfile.EmailVerified || currentUser.GoogleSubject == "" || googleProfile.Subject != currentUser.GoogleSubject {
		log.Printf("Google step-up rejected: subject mismatch for user %d", userIdentifier)
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return true
	}

	if markError := handler.SessionService.MarkStepUpForUser(operationContext, userIdentifier); markError != nil {
		http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=error", http.StatusSeeOther)
		return true
	}
	http.Redirect(responseWriter, request, postLoginRedirectPath+"?step_up=ok", http.StatusSeeOther)
	return true
}

// mustResolveUserID resolves the user id for a raw token, returning 0 on failure (the caller then
// fails its own lookup). Kept tiny to avoid duplicating the resolve+timeout dance.
func mustResolveUserID(operationContext context.Context, handler *AuthHandler, rawToken string) int64 {
	userIdentifier, resolveError := handler.SessionService.ResolveUserIdentifier(operationContext, rawToken)
	if resolveError != nil {
		return 0
	}
	return userIdentifier
}

// resolveRequestLocale picks the email language: the payload's locale if supported, otherwise the
// browser's Accept-Language, otherwise pt-BR.
func resolveRequestLocale(request *http.Request, payloadLocale string) string {
	if isSupportedLocale(payloadLocale) {
		return payloadLocale
	}
	acceptLanguage := strings.ToLower(request.Header.Get("Accept-Language"))
	for _, candidate := range []string{"pt", "es", "en"} {
		if strings.HasPrefix(acceptLanguage, candidate) {
			return candidate
		}
	}
	return "pt"
}

func isSupportedLocale(locale string) bool {
	return locale == "pt" || locale == "en" || locale == "es"
}

func (handler *AuthHandler) setStateCookie(responseWriter http.ResponseWriter, state string) {
	http.SetCookie(responseWriter, &http.Cookie{
		Name:     handler.OAuthStateCookie,
		Value:    state,
		Path:     "/auth",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   handler.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (handler *AuthHandler) clearStateCookie(responseWriter http.ResponseWriter) {
	http.SetCookie(responseWriter, &http.Cookie{
		Name:     handler.OAuthStateCookie,
		Value:    "",
		Path:     "/auth",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   handler.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (handler *AuthHandler) setStepUpStateCookie(responseWriter http.ResponseWriter, value string) {
	http.SetCookie(responseWriter, &http.Cookie{
		Name:     handler.StepUpStateCookie,
		Value:    value,
		Path:     "/auth",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   handler.SecureCookies,
		// Lax (not Strict): this cookie must be sent when Google redirects the browser back to our
		// callback, which is a cross-site top-level navigation. It carries no session authority on its
		// own — it is signed and only names which account to elevate, gated by the Google subject match.
		SameSite: http.SameSiteLaxMode,
	})
}

func (handler *AuthHandler) clearStepUpStateCookie(responseWriter http.ResponseWriter) {
	http.SetCookie(responseWriter, &http.Cookie{
		Name:     handler.StepUpStateCookie,
		Value:    "",
		Path:     "/auth",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   handler.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func generateOAuthState() (string, error) {
	randomBytes := make([]byte, 32)
	if _, randomError := rand.Read(randomBytes); randomError != nil {
		return "", randomError
	}
	return base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func (handler *AuthHandler) setSessionCookie(responseWriter http.ResponseWriter, rawToken string, expiresAt time.Time) {
	http.SetCookie(responseWriter, &http.Cookie{
		Name:     handler.CookieName,
		Value:    rawToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   handler.SecureCookies,
		// Strict: the session cookie is never attached to cross-site requests, so it cannot be
		// ridden by CSRF. All API calls are same-origin XHR from the SPA, so this does not affect
		// normal use; the OAuth state cookie stays Lax because it must survive Google's redirect.
		SameSite: http.SameSiteStrictMode,
	})
}

func (handler *AuthHandler) clearSessionCookie(responseWriter http.ResponseWriter) {
	http.SetCookie(responseWriter, &http.Cookie{
		Name:     handler.CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   handler.SecureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

func (handler *AuthHandler) writeRegistrationError(responseWriter http.ResponseWriter, registrationError error) {
	switch {
	case errors.Is(registrationError, service.ErrInvalidEmail), errors.Is(registrationError, service.ErrWeakPassword):
		writeJSONError(responseWriter, http.StatusBadRequest, registrationError.Error())
	case errors.Is(registrationError, repository.ErrEmailAlreadyRegistered):
		writeJSONError(responseWriter, http.StatusConflict, "That email is already registered.")
	default:
		log.Printf("Registration failed unexpectedly: %v", registrationError)
		writeJSONError(responseWriter, http.StatusInternalServerError, "Could not create the account.")
	}
}

func toUserResponse(user *domain.User, termsAccepted bool) userResponsePayload {
	avatarURL := ""
	if trimmedAvatar := strings.TrimSpace(user.AvatarURL); trimmedAvatar != "" {
		// The proxy path is identical for every user, and the response is cacheable
		// (Cache-Control: private, max-age). Without a per-picture cache key the browser would keep
		// serving a previously cached avatar after switching accounts (sign out + sign in as someone
		// else) — even on reload, since reloads don't refetch cached images. Append a stable hash of
		// the upstream Google picture URL so each distinct picture is a distinct cacheable resource
		// (and a rotated Google URL naturally busts the cache for the same user).
		digest := sha256.Sum256([]byte(trimmedAvatar))
		avatarURL = "/api/v1/account/avatar?v=" + hex.EncodeToString(digest[:])[:12]
	}
	return userResponsePayload{
		Identifier:      user.Identifier,
		Email:           user.Email,
		DisplayName:     user.DisplayName,
		HasPassword:     user.HasPassword(),
		GoogleConnected: user.HasGoogleLinked(),
		IsAdmin:         user.IsAdmin,
		EmailVerified:   user.IsEmailVerified(),
		CreatedAt:       user.CreatedAt.Format(time.RFC3339),
		TermsAccepted:   termsAccepted,
		TermsVersion:    domain.CurrentAgreementVersion,
		AvatarURL:       avatarURL,
	}
}

// termsAcceptedFor returns whether the user has accepted the current legal documents. It fails CLOSED
// (returns false on a missing service or a read error) so a consent gap can never be silently skipped:
// the worst case is the user is asked to accept again, never that an un-consented user slips through.
func termsAcceptedFor(operationContext context.Context, agreementService *service.AgreementService, userIdentifier int64) bool {
	if agreementService == nil {
		return false
	}
	accepted, lookupError := agreementService.HasAcceptedCurrent(operationContext, userIdentifier)
	if lookupError != nil {
		log.Printf("terms: could not check acceptance for user %d: %v", userIdentifier, lookupError)
		return false
	}
	return accepted
}

// enforceAgreementAccepted writes 403 (code terms_not_accepted) unless the user has accepted the
// current Terms of Use + Privacy Policy. Mirrors enforceEmailVerified; used to block money/robot
// actions until consent is on record (defense in depth behind the SPA's blocking gate).
func enforceAgreementAccepted(operationContext context.Context, responseWriter http.ResponseWriter, agreementService *service.AgreementService, userIdentifier int64) bool {
	if termsAcceptedFor(operationContext, agreementService, userIdentifier) {
		return true
	}
	writeJSONErrorCode(responseWriter, http.StatusForbidden, "Please accept the Terms of Use and Privacy Policy to continue.", "terms_not_accepted")
	return false
}

// enforceVerifiedAndAgreed gates a money/robot action behind BOTH a confirmed email and acceptance of
// the current Terms of Use + Privacy Policy. It writes the matching 403 (email_unverified or
// terms_not_accepted) and returns false on the first failed check.
func enforceVerifiedAndAgreed(operationContext context.Context, responseWriter http.ResponseWriter, authService *service.AuthService, agreementService *service.AgreementService, userIdentifier int64) bool {
	if !enforceEmailVerified(operationContext, responseWriter, authService, userIdentifier) {
		return false
	}
	return enforceAgreementAccepted(operationContext, responseWriter, agreementService, userIdentifier)
}

func writeJSON(responseWriter http.ResponseWriter, statusCode int, payload interface{}) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(statusCode)
	if encodeError := json.NewEncoder(responseWriter).Encode(payload); encodeError != nil {
		log.Printf("Could not encode JSON response: %v", encodeError)
	}
}

func writeJSONError(responseWriter http.ResponseWriter, statusCode int, message string) {
	writeJSON(responseWriter, statusCode, map[string]string{"error": message})
}

// writeJSONErrorCode is writeJSONError plus a machine-readable code the SPA can branch on (e.g. to
// show a specific dialog) without parsing the human message.
func writeJSONErrorCode(responseWriter http.ResponseWriter, statusCode int, message string, code string) {
	writeJSON(responseWriter, statusCode, map[string]string{"error": message, "code": code})
}

// enforceEmailVerified loads the user and writes 403 if the email is not confirmed yet. Used to block
// sensitive actions (connecting Binance, trading, robots) until the account confirms its email.
func enforceEmailVerified(operationContext context.Context, responseWriter http.ResponseWriter, authService *service.AuthService, userIdentifier int64) bool {
	currentUser, lookupError := authService.GetUserByIdentifier(operationContext, userIdentifier)
	if lookupError != nil || currentUser == nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return false
	}
	if !currentUser.IsEmailVerified() {
		writeJSONErrorCode(responseWriter, http.StatusForbidden, "Confirm your email before using this feature.", "email_unverified")
		return false
	}
	return true
}

// enforceStepUp writes 403 (code step_up_required) unless the request's session has a fresh step-up
// window. Used to gate the highest-risk actions (connecting Binance keys, enabling live trading).
func enforceStepUp(operationContext context.Context, responseWriter http.ResponseWriter, sessionService *service.SessionService, request *http.Request, cookieName string) bool {
	sessionCookie, cookieError := request.Cookie(cookieName)
	if cookieError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return false
	}
	fresh, statusError := sessionService.IsStepUpFresh(operationContext, sessionCookie.Value, stepUpWindow)
	if statusError != nil {
		writeJSONError(responseWriter, http.StatusUnauthorized, "Not authenticated.")
		return false
	}
	if !fresh {
		writeJSONErrorCode(responseWriter, http.StatusForbidden, "Please re-confirm your identity to continue.", "step_up_required")
		return false
	}
	return true
}

// clientIPAddress resolves the real public IP of the caller for the durable sign-in history,
// geolocation and new-device detection. It must NOT trust the leftmost X-Forwarded-For entry: nginx
// fronts this API with `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for`, which APPENDS the
// true peer to whatever the client sent — so XFF[0] is attacker-controlled. Trusting it would let a
// caller forge the IP/location written to the access log and quietly dodge the new-device alert
// (device fingerprint = SHA-256(user_agent + '|' + ip)). We instead prefer nginx's X-Real-IP, set from
// $remote_addr (the actual TCP peer, unforgeable), falling back to the RIGHTMOST XFF hop (the one our
// own proxy appended) and finally the raw RemoteAddr.
func clientIPAddress(request *http.Request) string {
	if realIP := strings.TrimSpace(request.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	if forwardedFor := request.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		hops := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(hops[len(hops)-1])
	}
	host, _, splitError := net.SplitHostPort(request.RemoteAddr)
	if splitError != nil {
		return request.RemoteAddr
	}
	return host
}
