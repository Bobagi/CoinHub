package domain

import "time"

// Authentication methods recorded on an access event.
const (
	AccessMethodPassword = "PASSWORD"
	AccessMethodGoogle   = "GOOGLE"
	AccessMethodSignup   = "SIGNUP"
)

// AccountAccessEvent is one durable record of a successful sign-in: which device/IP, how the user
// authenticated, and whether the device/network was new at the time. It outlives the session it came
// from, forming the account's access history.
type AccountAccessEvent struct {
	Identifier        int64
	UserIdentifier    int64
	IPAddress         string
	UserAgent         string
	AuthMethod        string
	DeviceFingerprint string
	IsNewDevice       bool
	// Coarse geolocation resolved from IPAddress at record time (may be empty when unknown).
	CountryCode string
	CountryName string
	Region      string
	City        string
	CreatedAt   time.Time
}
