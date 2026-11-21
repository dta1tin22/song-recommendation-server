package apikey

import (
	"context"
	"time"
)

// Repository is the persistence contract for user API keys. Every method
// takes the owning user ID and applies it in SQL: there is deliberately no
// lookup-or-mutation that can reach a key without knowing its owner, so a
// guessed key ID cannot escape the caller's account. GetByHash is the one
// exception, and it is keyed on the secret digest rather than an ID.
type Repository interface {
	Create(ctx context.Context, key *Key) error

	// ListByUser returns only active keys (revoked_at IS NULL), newest first.
	ListByUser(ctx context.Context, userID int64) ([]Key, error)

	// Revoke soft-deletes a key owned by userID, returning
	// ErrAPIKeyNotFound for an unknown, foreign or already-revoked key.
	Revoke(ctx context.Context, userID, keyID int64) error

	// GetByHash returns the key whose SHA-256 digest matches hash, including
	// revoked and expired rows: usability is service policy, not a query
	// predicate. ErrAPIKeyNotFound means no row matched.
	GetByHash(ctx context.Context, hash string) (*Key, error)

	// TouchLastUsed stamps last_used_at unless it was already stamped inside
	// throttle. The interval is policy supplied by the service; the repository
	// must not hardcode it. Skipping the write is success, not an error.
	TouchLastUsed(ctx context.Context, keyID int64, throttle time.Duration) error
}
