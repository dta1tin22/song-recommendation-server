package auth

import "time"

type User struct {
	ID            int64  `json:"id"`
	Username      string `json:"username"`
	Email         string `json:"email"`
	PasswordHash  string `json:"-"`
	Role          string `json:"role"`
	IsActive      bool   `json:"is_active"`
	EmailVerified bool   `json:"email_verified"`
	// FailedLogins and LockedUntil are internal throttling state. Never expose them:
	// they would tell an attacker how close an account is to being locked.
	FailedLogins int16      `json:"-"`
	LockedUntil  *time.Time `json:"-"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// RefreshToken is one persisted refresh-token session. The raw token is never
// stored or modelled: TokenHash is the SHA-256 hex digest of the value handed to
// the client, so a leaked database cannot be replayed against the API.
type RefreshToken struct {
	ID     int64 `json:"-"`
	UserID int64 `json:"-"`
	// TokenHash has the same "never serialise" rule as User.PasswordHash.
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"-"`
	// UserAgent and IPAddress are nullable audit columns.
	UserAgent *string   `json:"-"`
	IPAddress *string   `json:"-"`
	CreatedAt time.Time `json:"-"`
}
