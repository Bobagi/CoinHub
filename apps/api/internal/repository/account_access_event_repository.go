package repository

import (
	"context"
	"database/sql"
	"time"

	"coin-hub/internal/domain"
)

// AccountAccessEventRepository persists and reads the durable account access (sign-in) log. Like every
// repository here it is user-scoped: reads always filter by user_id.
type AccountAccessEventRepository interface {
	RecordEvent(operationContext context.Context, event domain.AccountAccessEvent) error
	// FingerprintSeen reports whether the user has any prior access with this device fingerprint.
	FingerprintSeen(operationContext context.Context, userIdentifier int64, deviceFingerprint string) (bool, error)
	// CountForUser returns how many access events the user already has (0 = this is their first access).
	CountForUser(operationContext context.Context, userIdentifier int64) (int, error)
	// ListForUser returns the user's accesses, newest first, paged.
	ListForUser(operationContext context.Context, userIdentifier int64, limit int, offset int) ([]domain.AccountAccessEvent, error)
	// PurgeOlderThan deletes access events created before cutoff (retention / data-minimization) and
	// returns how many rows were removed. Not user-scoped: it runs across all users from a leader-gated
	// maintenance loop, not a request handler.
	PurgeOlderThan(operationContext context.Context, cutoff time.Time) (int64, error)
}

type PostgresAccountAccessEventRepository struct {
	Database *sql.DB
}

func NewPostgresAccountAccessEventRepository(database *sql.DB) *PostgresAccountAccessEventRepository {
	return &PostgresAccountAccessEventRepository{Database: database}
}

func (repository *PostgresAccountAccessEventRepository) RecordEvent(operationContext context.Context, event domain.AccountAccessEvent) error {
	_, executionError := repository.Database.ExecContext(
		operationContext,
		`INSERT INTO account_access_events
		     (user_id, ip_address, user_agent, auth_method, device_fingerprint, is_new_device, country_code, country_name, region, city)
		 VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''))`,
		event.UserIdentifier,
		event.IPAddress,
		event.UserAgent,
		event.AuthMethod,
		event.DeviceFingerprint,
		event.IsNewDevice,
		event.CountryCode,
		event.CountryName,
		event.Region,
		event.City,
	)
	return executionError
}

func (repository *PostgresAccountAccessEventRepository) FingerprintSeen(operationContext context.Context, userIdentifier int64, deviceFingerprint string) (bool, error) {
	var exists bool
	scanError := repository.Database.QueryRowContext(
		operationContext,
		`SELECT EXISTS (
		     SELECT 1 FROM account_access_events
		     WHERE user_id = $1 AND device_fingerprint = $2
		 )`,
		userIdentifier,
		deviceFingerprint,
	).Scan(&exists)
	return exists, scanError
}

func (repository *PostgresAccountAccessEventRepository) CountForUser(operationContext context.Context, userIdentifier int64) (int, error) {
	var total int
	scanError := repository.Database.QueryRowContext(
		operationContext,
		`SELECT COUNT(*) FROM account_access_events WHERE user_id = $1`,
		userIdentifier,
	).Scan(&total)
	return total, scanError
}

func (repository *PostgresAccountAccessEventRepository) PurgeOlderThan(operationContext context.Context, cutoff time.Time) (int64, error) {
	result, executionError := repository.Database.ExecContext(
		operationContext,
		`DELETE FROM account_access_events WHERE created_at < $1`,
		cutoff,
	)
	if executionError != nil {
		return 0, executionError
	}
	return result.RowsAffected()
}

func (repository *PostgresAccountAccessEventRepository) ListForUser(operationContext context.Context, userIdentifier int64, limit int, offset int) ([]domain.AccountAccessEvent, error) {
	rows, queryError := repository.Database.QueryContext(
		operationContext,
		`SELECT id, user_id, COALESCE(ip_address, ''), COALESCE(user_agent, ''), auth_method, device_fingerprint, is_new_device,
		        COALESCE(country_code, ''), COALESCE(country_name, ''), COALESCE(region, ''), COALESCE(city, ''), created_at
		 FROM account_access_events
		 WHERE user_id = $1
		 ORDER BY created_at DESC, id DESC
		 LIMIT $2 OFFSET $3`,
		userIdentifier,
		limit,
		offset,
	)
	if queryError != nil {
		return nil, queryError
	}
	defer rows.Close()

	events := make([]domain.AccountAccessEvent, 0, limit)
	for rows.Next() {
		var event domain.AccountAccessEvent
		scanError := rows.Scan(
			&event.Identifier,
			&event.UserIdentifier,
			&event.IPAddress,
			&event.UserAgent,
			&event.AuthMethod,
			&event.DeviceFingerprint,
			&event.IsNewDevice,
			&event.CountryCode,
			&event.CountryName,
			&event.Region,
			&event.City,
			&event.CreatedAt,
		)
		if scanError != nil {
			return nil, scanError
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
