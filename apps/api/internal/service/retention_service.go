package service

import (
	"context"
	"log"
	"time"

	"coin-hub/internal/repository"
)

// RetentionService enforces data-minimization (LGPD art. 15/16): it deletes personal data once we no
// longer need to keep it. Today it prunes the durable account access (sign-in) log — IP address,
// user-agent and approximate location are PII — keeping only a recent window, long enough for
// new-device detection and for the user to review their own recent sign-ins. Account deletion still
// erases everything immediately via cascade; this just bounds how long data lingers for ACTIVE accounts.
type RetentionService struct {
	accessRepository repository.AccountAccessEventRepository
	accessRetention  time.Duration
}

func NewRetentionService(accessRepository repository.AccountAccessEventRepository, accessRetention time.Duration) *RetentionService {
	return &RetentionService{accessRepository: accessRepository, accessRetention: accessRetention}
}

// AccessLogRetention is the configured retention window. <= 0 means automatic purge is disabled (rows
// are kept until the account is deleted).
func (service *RetentionService) AccessLogRetention() time.Duration {
	if service == nil {
		return 0
	}
	return service.accessRetention
}

// PurgeExpiredAccessEvents deletes access-log rows older than the retention window. It is a no-op (and
// never an error) when retention is disabled, and best-effort otherwise: a failure is logged, not fatal.
// The DELETE is date-bounded and idempotent, so it is safe to call frequently / on every leader takeover.
func (service *RetentionService) PurgeExpiredAccessEvents(parentContext context.Context) {
	if service == nil || service.accessRetention <= 0 {
		return
	}
	cutoff := time.Now().UTC().Add(-service.accessRetention)
	deleteContext, cancel := context.WithTimeout(parentContext, 30*time.Second)
	defer cancel()
	deleted, purgeError := service.accessRepository.PurgeOlderThan(deleteContext, cutoff)
	if purgeError != nil {
		log.Printf("retention: could not purge access events older than %s: %v", cutoff.Format(time.RFC3339), purgeError)
		return
	}
	if deleted > 0 {
		log.Printf("retention: purged %d access-log event(s) older than %s (retention %s)", deleted, cutoff.Format("2006-01-02"), service.accessRetention)
	}
}
