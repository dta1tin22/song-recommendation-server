package apikey

import "time"

// Key is one persisted API credential. The raw key is returned to the client
// exactly once, at creation, and is never stored: KeyHash is the SHA-256 hex
// digest of the full key, so a leaked database (or a logged model) cannot be
// replayed against the API.
//
// KeyPrefix is safe to expose and is what listings show: it identifies a key for
// the user without carrying enough entropy to be used as a credential.
type Key struct {
	ID     int64  `json:"-"`
	UserID int64  `json:"-"`
	Name   string `json:"name"`

	KeyPrefix string `json:"key_prefix"`
	// KeyHash follows the same "never serialise" rule as User.PasswordHash.
	KeyHash string   `json:"-"`
	Scopes  []string `json:"scopes"`

	// ExpiresAt is optional; nil means the key never expires.
	ExpiresAt *time.Time `json:"expires_at"`
	// RevokedAt is set by a soft revoke. Revoked rows are kept for audit and are
	// excluded from listings.
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}
