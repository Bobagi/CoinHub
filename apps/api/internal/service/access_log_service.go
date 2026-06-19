package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"
	"time"

	"coin-hub/internal/domain"
	"coin-hub/internal/geoip"
	"coin-hub/internal/repository"
)

// AccessLogService records successful sign-ins to the durable account access log and, when a device or
// network is new for an existing account, emails a security alert. Recording is best-effort and runs
// off the request path, so it can never slow down or fail a login.
type AccessLogService struct {
	accessRepository repository.AccountAccessEventRepository
	emailService     *AccountEmailService
	geoLocator       *geoip.Locator
}

func NewAccessLogService(accessRepository repository.AccountAccessEventRepository, emailService *AccountEmailService, geoLocator *geoip.Locator) *AccessLogService {
	return &AccessLogService{accessRepository: accessRepository, emailService: emailService, geoLocator: geoLocator}
}

// RecordLoginAsync records a successful sign-in in the background. It returns immediately; the actual
// work (fingerprinting, new-device detection, optional alert email) happens in a goroutine on its own
// context so the login response is never delayed by it.
func (service *AccessLogService) RecordLoginAsync(userIdentifier int64, recipientEmail string, ipAddress string, userAgent string, authMethod string, locale string) {
	if service == nil {
		return
	}
	go service.recordLogin(userIdentifier, recipientEmail, ipAddress, userAgent, authMethod, locale)
}

func (service *AccessLogService) recordLogin(userIdentifier int64, recipientEmail string, ipAddress string, userAgent string, authMethod string, locale string) {
	operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fingerprint := deviceFingerprint(userAgent, ipAddress)

	seen, seenError := service.accessRepository.FingerprintSeen(operationContext, userIdentifier, fingerprint)
	if seenError != nil {
		// Fail safe: on a read error, treat the device as known so we never spam a false "new access".
		log.Printf("access log: could not check device fingerprint for user %d: %v", userIdentifier, seenError)
		seen = true
	}
	priorCount, countError := service.accessRepository.CountForUser(operationContext, userIdentifier)
	if countError != nil {
		log.Printf("access log: could not count prior accesses for user %d: %v", userIdentifier, countError)
	}

	location := service.geoLocator.Lookup(ipAddress, geoip.LanguageKey(locale))

	isNewDevice := !seen
	event := domain.AccountAccessEvent{
		UserIdentifier:    userIdentifier,
		IPAddress:         ipAddress,
		UserAgent:         userAgent,
		AuthMethod:        authMethod,
		DeviceFingerprint: fingerprint,
		IsNewDevice:       isNewDevice,
		CountryCode:       location.CountryCode,
		CountryName:       location.CountryName,
		Region:            location.Region,
		City:              location.City,
	}
	if recordError := service.accessRepository.RecordEvent(operationContext, event); recordError != nil {
		log.Printf("access log: could not record access for user %d: %v", userIdentifier, recordError)
		return
	}

	// Alert only on a genuinely new device for an account that already had at least one access — so the
	// very first sign-in (and the sign-up) never alerts, but any later unrecognized device does.
	if isNewDevice && priorCount > 0 && service.emailService != nil {
		whenText := time.Now().UTC().Format("2006-01-02 15:04 UTC")
		service.emailService.SendNewAccessAlert(recipientEmail, locale, userAgent, ipAddress, formatLocation(location), whenText)
	}
}

// formatLocation renders a location as "City, Region, Country" (dropping empty parts), for the alert
// email. Returns an empty string when nothing is known.
func formatLocation(location geoip.Location) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{location.City, location.Region, location.CountryName} {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ", ")
}

// ListAccess returns a page of the user's access history (newest first) plus the total count.
func (service *AccessLogService) ListAccess(operationContext context.Context, userIdentifier int64, limit int, offset int) ([]domain.AccountAccessEvent, int, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	total, countError := service.accessRepository.CountForUser(operationContext, userIdentifier)
	if countError != nil {
		return nil, 0, countError
	}
	events, listError := service.accessRepository.ListForUser(operationContext, userIdentifier, limit, offset)
	if listError != nil {
		return nil, 0, listError
	}
	return events, total, nil
}

// deviceFingerprint is a stable id for a device/network combination: SHA-256(user_agent + '|' + ip).
func deviceFingerprint(userAgent string, ipAddress string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(userAgent) + "|" + strings.TrimSpace(ipAddress)))
	return hex.EncodeToString(digest[:])
}
