package auth

import (
	"context"
	"time"
)

// UserRepository is the persistence contract the auth service depends on. It is
// an interface so handlers/services can be tested against fakes without a
// database, and so the pgx implementation stays swappable.
type UserRepository interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByID(ctx context.Context, id int64) (*User, error)

	// RecordFailedLogin applies the brute-force throttle policy supplied by the
	// service. The repository owns atomicity; the service owns the threshold.
	RecordFailedLogin(ctx context.Context, userID int64, maxAttempts int16, lockDuration time.Duration) error

	// RecordSuccessfulLogin clears throttle state and stamps last_login_at.
	RecordSuccessfulLogin(ctx context.Context, userID int64) error
}

// RefreshTokenRepository is the persistence contract for refresh-token sessions.
// Only hashes cross this boundary: the raw token never reaches the database.
type RefreshTokenRepository interface {
	Create(ctx context.Context, token *RefreshToken) error
	GetByHash(ctx context.Context, hash string) (*RefreshToken, error)

	// DeleteByHash is idempotent so logout can be repeated safely.
	DeleteByHash(ctx context.Context, hash string) error

	// Rotate atomically consumes oldHash and stores newToken.
	Rotate(ctx context.Context, oldHash string, newToken *RefreshToken) error
}
