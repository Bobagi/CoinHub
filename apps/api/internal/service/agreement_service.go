package service

import (
	"context"

	"coin-hub/internal/domain"
	"coin-hub/internal/repository"
)

// AgreementService records and checks user consent to the Terms of Use + Privacy Policy. The version in
// force is domain.CurrentAgreementVersion; "has the user accepted?" always means the current version,
// so bumping the version automatically requires everyone to re-accept.
type AgreementService struct {
	repository repository.UserAgreementAcceptanceRepository
}

func NewAgreementService(acceptanceRepository repository.UserAgreementAcceptanceRepository) *AgreementService {
	return &AgreementService{repository: acceptanceRepository}
}

// CurrentVersion returns the version tag of the legal documents currently in force.
func (service *AgreementService) CurrentVersion() string {
	return domain.CurrentAgreementVersion
}

// HasAcceptedCurrent reports whether the user has accepted the version currently in force.
func (service *AgreementService) HasAcceptedCurrent(operationContext context.Context, userIdentifier int64) (bool, error) {
	return service.repository.HasAcceptedVersion(operationContext, userIdentifier, domain.CurrentAgreementVersion)
}

// Accept records the user's consent to the current version, capturing the request's IP and user agent
// as evidence. Re-accepting is harmless (the log is append-only history).
func (service *AgreementService) Accept(operationContext context.Context, userIdentifier int64, ipAddress string, userAgent string) error {
	return service.repository.RecordAcceptance(operationContext, domain.UserAgreementAcceptance{
		UserIdentifier:  userIdentifier,
		DocumentVersion: domain.CurrentAgreementVersion,
		IPAddress:       ipAddress,
		UserAgent:       userAgent,
	})
}
