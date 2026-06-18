package service

import (
	"context"
	"errors"

	"coin-hub/internal/domain"
	"coin-hub/internal/repository"
	"coin-hub/internal/security"
)

// ErrCredentialEncryptionUnavailable is returned when the server has no encryption key configured.
var ErrCredentialEncryptionUnavailable = errors.New("credential encryption is not configured on the server")

// CredentialStatus is a non-sensitive summary of a user's stored Binance credentials.
type CredentialStatus struct {
	HasActiveCredential    bool
	ActiveEnvironment      string
	MaskedAPIKey           string
	ConfiguredEnvironments []string
}

// ActiveEnvironmentStore persists each user's chosen active Binance environment, independent of
// whether they have keys for it. Implemented by the user repository.
type ActiveEnvironmentStore interface {
	GetActiveBinanceEnvironment(lookupContext context.Context, userIdentifier int64) (string, error)
	SetActiveBinanceEnvironment(updateContext context.Context, userIdentifier int64, environmentName string) error
}

// UserCredentialService validates, encrypts, stores, and retrieves per-user Binance credentials.
type UserCredentialService struct {
	repository        repository.UserBinanceCredentialRepository
	environmentStore  ActiveEnvironmentStore
	cipher            *security.SecretCipher
	testnetBaseURL    string
	productionBaseURL string
}

func NewUserCredentialService(repositoryInstance repository.UserBinanceCredentialRepository, environmentStore ActiveEnvironmentStore, cipher *security.SecretCipher, testnetBaseURL string, productionBaseURL string) *UserCredentialService {
	return &UserCredentialService{
		repository:        repositoryInstance,
		environmentStore:  environmentStore,
		cipher:            cipher,
		testnetBaseURL:    testnetBaseURL,
		productionBaseURL: productionBaseURL,
	}
}

func (service *UserCredentialService) baseURLForEnvironment(environmentName string) string {
	if domain.NormalizeBinanceEnvironment(environmentName) == domain.BinanceEnvironmentProduction {
		return service.productionBaseURL
	}
	return service.testnetBaseURL
}

// SaveAndValidate validates the keys against Binance, then stores them encrypted as the active credential.
func (service *UserCredentialService) SaveAndValidate(operationContext context.Context, userIdentifier int64, apiKey string, apiSecret string, environmentName string) error {
	if service.cipher == nil {
		return ErrCredentialEncryptionUnavailable
	}

	normalizedEnvironment := domain.NormalizeBinanceEnvironment(environmentName)
	baseURL := service.baseURLForEnvironment(normalizedEnvironment)

	// A fresh validator per call keeps this safe under concurrent requests.
	validator := NewBinanceCredentialValidator(baseURL)
	if validationError := validator.ValidateCredentials(operationContext, apiKey, apiSecret); validationError != nil {
		return validationError
	}

	encryptedAPIKey, keyEncryptionError := service.cipher.EncryptString(apiKey)
	if keyEncryptionError != nil {
		return keyEncryptionError
	}
	encryptedAPISecret, secretEncryptionError := service.cipher.EncryptString(apiSecret)
	if secretEncryptionError != nil {
		return secretEncryptionError
	}

	saveError := service.repository.SaveCredentialForUser(operationContext, userIdentifier, domain.BinanceCredentialRecord{
		APIKey:          encryptedAPIKey,
		APISecret:       encryptedAPISecret,
		EnvironmentName: normalizedEnvironment,
		APIBaseURL:      baseURL,
		IsActive:        true,
	})
	if saveError != nil {
		return saveError
	}
	// Connecting keys also makes that environment the active one.
	return service.environmentStore.SetActiveBinanceEnvironment(operationContext, userIdentifier, normalizedEnvironment)
}

// LoadActiveEnvironmentConfiguration returns the decrypted credential for the user's active
// environment, ready for the Binance clients, or (nil, nil) when the active environment has no keys
// (i.e. the user switched to it but has not connected yet).
func (service *UserCredentialService) LoadActiveEnvironmentConfiguration(operationContext context.Context, userIdentifier int64) (*domain.BinanceEnvironmentConfiguration, error) {
	activeEnvironment := service.ActiveEnvironmentName(operationContext, userIdentifier)
	record, loadError := service.repository.LoadLatestCredentialForUserByEnvironment(operationContext, userIdentifier, activeEnvironment)
	if loadError != nil {
		return nil, loadError
	}
	if record == nil {
		return nil, nil
	}
	if service.cipher == nil {
		return nil, ErrCredentialEncryptionUnavailable
	}

	apiKey, keyDecryptionError := service.cipher.DecryptString(record.APIKey)
	if keyDecryptionError != nil {
		return nil, keyDecryptionError
	}
	apiSecret, secretDecryptionError := service.cipher.DecryptString(record.APISecret)
	if secretDecryptionError != nil {
		return nil, secretDecryptionError
	}

	return &domain.BinanceEnvironmentConfiguration{
		EnvironmentName: record.EnvironmentName,
		RESTBaseURL:     record.APIBaseURL,
		APIKey:          apiKey,
		APISecret:       apiSecret,
	}, nil
}

// ActivateEnvironment switches the user's active environment. Unlike before, this no longer requires
// stored keys: a user can switch to PRODUCTION (and see it flagged "not connected") before saving
// credentials. If the environment does have a stored credential, its is_active flag is synced too.
func (service *UserCredentialService) ActivateEnvironment(operationContext context.Context, userIdentifier int64, environmentName string) error {
	normalizedEnvironment := domain.NormalizeBinanceEnvironment(environmentName)
	if setError := service.environmentStore.SetActiveBinanceEnvironment(operationContext, userIdentifier, normalizedEnvironment); setError != nil {
		return setError
	}
	// Best-effort: keep the credential is_active flag aligned when keys exist for this environment.
	if existing, _ := service.repository.LoadLatestCredentialForUserByEnvironment(operationContext, userIdentifier, normalizedEnvironment); existing != nil {
		_ = service.repository.ActivateEnvironmentForUser(operationContext, userIdentifier, normalizedEnvironment)
	}
	return nil
}

// ActiveEnvironmentName returns the user's active Binance environment preference, defaulting to
// TESTNET. Used to scope settings/operations to one environment.
func (service *UserCredentialService) ActiveEnvironmentName(operationContext context.Context, userIdentifier int64) string {
	environmentName, loadError := service.environmentStore.GetActiveBinanceEnvironment(operationContext, userIdentifier)
	if loadError != nil || environmentName == "" {
		return domain.BinanceEnvironmentTestnet
	}
	return environmentName
}

// DeleteCredentials removes the user's stored Binance keys for an environment from the database and
// returns how many credential rows were deleted (0 = nothing was stored for that environment).
func (service *UserCredentialService) DeleteCredentials(operationContext context.Context, userIdentifier int64, environmentName string) (int64, error) {
	normalizedEnvironment := domain.NormalizeBinanceEnvironment(environmentName)
	return service.repository.DeleteCredentialsForUserByEnvironment(operationContext, userIdentifier, normalizedEnvironment)
}

// GetStatus returns a non-sensitive summary suitable for the dashboard.
func (service *UserCredentialService) GetStatus(operationContext context.Context, userIdentifier int64) (CredentialStatus, error) {
	configuredEnvironments, listError := service.repository.ListConfiguredEnvironmentsForUser(operationContext, userIdentifier)
	if listError != nil {
		return CredentialStatus{}, listError
	}

	activeEnvironment := service.ActiveEnvironmentName(operationContext, userIdentifier)
	status := CredentialStatus{
		ConfiguredEnvironments: configuredEnvironments,
		ActiveEnvironment:      activeEnvironment,
	}

	// HasActiveCredential means the ACTIVE environment has keys. When the user has switched to an
	// environment they have not connected yet, this is false (the UI then shows it as not connected).
	record, loadError := service.repository.LoadLatestCredentialForUserByEnvironment(operationContext, userIdentifier, activeEnvironment)
	if loadError != nil {
		return CredentialStatus{}, loadError
	}
	if record == nil {
		return status, nil
	}

	status.HasActiveCredential = true
	status.MaskedAPIKey = service.maskAPIKey(record.APIKey)
	return status, nil
}

func (service *UserCredentialService) maskAPIKey(encryptedAPIKey string) string {
	if service.cipher == nil {
		return "****"
	}
	apiKey, decryptionError := service.cipher.DecryptString(encryptedAPIKey)
	if decryptionError != nil || len(apiKey) < 4 {
		return "****"
	}
	return "****" + apiKey[len(apiKey)-4:]
}
