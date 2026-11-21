package auth

import "errors"

var (
	// ErrUserNotFound is returned by the repository when a lookup matches no row.
	ErrUserNotFound = errors.New("user not found")

	// ErrUsernameTaken is returned when the users.username unique constraint is violated.
	ErrUsernameTaken = errors.New("username already taken")

	// ErrEmailTaken is returned when the users.email unique constraint is violated.
	ErrEmailTaken = errors.New("email already registered")

	// ErrRefreshTokenNotFound is returned by the repository when no refresh
	// token row matches the supplied hash. The service collapses this (together
	// with expiry and revoked users) into a single opaque error, so this
	// sentinel must never reach the handler.
	ErrRefreshTokenNotFound = errors.New("refresh token not found")

	// ErrInvalidCredentials is returned for a wrong password, an unknown user and
	// an inactive account alike, so the API never reveals which one happened.
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrInvalidInput is returned when a request fails field validation.
	ErrInvalidInput = errors.New("invalid input")

	// ErrAccountLocked is returned while an account is inside its lockout window.
	// This is the one deliberate exception to the "all auth failures look alike"
	// rule: surfacing the lock is worth revealing that the username exists.
	ErrAccountLocked = errors.New("account locked")

	// ErrInvalidRefreshToken is returned for an unknown, expired, already-rotated
	// or otherwise unusable refresh token. All of those causes collapse into one
	// error so the endpoint cannot be used to probe token validity.
	ErrInvalidRefreshToken = errors.New("invalid refresh token")

	// ErrInvalidAccessToken is returned for every reason an access token cannot be
	// trusted: malformed, wrong algorithm, bad signature, wrong issuer, expired,
	// or a subject that is not a positive user ID. Collapsing those causes into one
	// error keeps the middleware from leaking which check failed.
	ErrInvalidAccessToken = errors.New("invalid access token")
)
