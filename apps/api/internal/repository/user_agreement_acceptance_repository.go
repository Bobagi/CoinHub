package repository

import (
	"context"
	"database/sql"
	"errors"

	"coin-hub/internal/domain"
)

// UserAgreementAcceptanceRepository persists and reads the append-only consent log (each acceptance of
// the Terms of Use + Privacy Policy). Like every repository here it is user-scoped.
type UserAgreementAcceptanceRepository interface {
	// RecordAcceptance appends one consent row. Re-accepting the same version is allowed (history).
	RecordAcceptance(operationContext context.Context, acceptance domain.UserAgreementAcceptance) error
	// HasAcceptedVersion reports whether the user has at least one acceptance of the given version.
	HasAcceptedVersion(operationContext context.Context, userIdentifier int64, documentVersion string) (bool, error)
	// LatestAcceptance returns the user's most recent acceptance (any version). found=false when none.
	LatestAcceptance(operationContext context.Context, userIdentifier int64) (acceptance domain.UserAgreementAcceptance, found bool, err error)
}

type PostgresUserAgreementAcceptanceRepository struct {
	Database *sql.DB
}

func NewPostgresUserAgreementAcceptanceRepository(database *sql.DB) *PostgresUserAgreementAcceptanceRepository {
	return &PostgresUserAgreementAcceptanceRepository{Database: database}
}

func (repository *PostgresUserAgreementAcceptanceRepository) RecordAcceptance(operationContext context.Context, acceptance domain.UserAgreementAcceptance) error {
	_, executionError := repository.Database.ExecContext(
		operationContext,
		`INSERT INTO user_agreement_acceptances (user_id, document_version, ip_address, user_agent)
		 VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''))`,
		acceptance.UserIdentifier,
		acceptance.DocumentVersion,
		acceptance.IPAddress,
		acceptance.UserAgent,
	)
	return executionError
}

func (repository *PostgresUserAgreementAcceptanceRepository) HasAcceptedVersion(operationContext context.Context, userIdentifier int64, documentVersion string) (bool, error) {
	var exists bool
	scanError := repository.Database.QueryRowContext(
		operationContext,
		`SELECT EXISTS (
		     SELECT 1 FROM user_agreement_acceptances
		     WHERE user_id = $1 AND document_version = $2
		 )`,
		userIdentifier,
		documentVersion,
	).Scan(&exists)
	return exists, scanError
}

func (repository *PostgresUserAgreementAcceptanceRepository) LatestAcceptance(operationContext context.Context, userIdentifier int64) (domain.UserAgreementAcceptance, bool, error) {
	var acceptance domain.UserAgreementAcceptance
	scanError := repository.Database.QueryRowContext(
		operationContext,
		`SELECT id, user_id, document_version, accepted_at
		 FROM user_agreement_acceptances
		 WHERE user_id = $1
		 ORDER BY accepted_at DESC, id DESC
		 LIMIT 1`,
		userIdentifier,
	).Scan(&acceptance.Identifier, &acceptance.UserIdentifier, &acceptance.DocumentVersion, &acceptance.AcceptedAt)
	if errors.Is(scanError, sql.ErrNoRows) {
		return domain.UserAgreementAcceptance{}, false, nil
	}
	if scanError != nil {
		return domain.UserAgreementAcceptance{}, false, scanError
	}
	return acceptance, true, nil
}
