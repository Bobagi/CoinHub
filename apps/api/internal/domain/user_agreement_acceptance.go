package domain

import "time"

// CurrentAgreementVersion is the version tag of the Terms of Use + Privacy Policy currently in force.
// Bump this string (keep it date-sortable) whenever the legal text changes materially — doing so makes
// every prior acceptance row stop matching the current version, so users are required to read and
// accept the updated terms before they can use money/robot features again. The same tag is shown to
// the frontend so the consent UI and the recorded version always agree.
const CurrentAgreementVersion = "2026-06-21.2"

// UserAgreementAcceptance is one immutable consent record: a specific user accepted a specific version
// of the legal documents at a specific time, from a specific IP/user-agent. It is the server-side proof
// that consent was given (a front-end checkbox is neither enforceable nor auditable).
type UserAgreementAcceptance struct {
	Identifier      int64
	UserIdentifier  int64
	DocumentVersion string
	IPAddress       string
	UserAgent       string
	AcceptedAt      time.Time
}
