package service

// UserFacingError is an error meant to be shown to the end user. Besides the English Message (a safe
// fallback), it carries a stable machine Code and string Params so the SPA can render a fully
// localized message from its i18n dictionaries instead of relying on the backend's English text.
//
// Handlers detect it with errors.As and forward Code/Params in the JSON error body; see
// writeServiceError in the httpserver package.
type UserFacingError struct {
	Code    string
	Params  map[string]string
	Message string
}

func (userError *UserFacingError) Error() string { return userError.Message }

// newUserError builds a *UserFacingError. message is the English fallback; code + params let the
// frontend localize it. params may be nil for messages without interpolation.
func newUserError(code string, message string, params map[string]string) *UserFacingError {
	return &UserFacingError{Code: code, Params: params, Message: message}
}
