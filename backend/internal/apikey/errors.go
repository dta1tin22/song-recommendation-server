package apikey

import "errors"

var (
	// ErrAPIKeyNotFound is returned when a lookup or revoke matches no row that
	// belongs to the requesting user. It intentionally covers "does not exist",
	// "belongs to someone else" and "already revoked", so a handler can answer
	// all three identically without revealing which one happened.
	ErrAPIKeyNotFound = errors.New("api key not found")

	// ErrInvalidInput is returned when a request fails field validation. The
	// message is part of the JSON envelope: the handler trims this prefix to
	// build the "invalid_input" response body, so it must not change.
	ErrInvalidInput = errors.New("invalid input")

	// ErrInvalidAPIKey is returned for an API key that is unknown, revoked or
	// expired. All three collapse into one error so a caller cannot use the
	// response to probe which keys exist or why one stopped working.
	ErrInvalidAPIKey = errors.New("invalid api key")
)
