package apikey

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/secret"
)

const (
	// apiKeyScheme marks a credential as an API key at a glance and gives every
	// key a recognisable, searchable shape.
	apiKeyScheme = "mk_"

	// apiKeyBytes is the entropy behind the secret part of a key. 256 bits makes
	// the value unguessable, which is why the fast SHA-256 digest below is
	// sufficient (as with refresh tokens, there is no low-entropy secret).
	apiKeyBytes = 32

	// apiKeyPrefixLen is how many leading characters are persisted as the
	// display identifier: the scheme plus 9 random characters (~54 bits). That is
	// enough to tell two keys apart in a list and useless as a credential.
	// It must fit api_keys.key_prefix VARCHAR(12).
	apiKeyPrefixLen = 12

	// apiKeyRawLen is the exact length of a raw key this service mints: the
	// 3-character scheme plus 43 base64url characters for 32 bytes. The request
	// path uses it as a cheap shape pre-filter, so an obviously wrong header is
	// rejected before hashing and before a database round trip.
	apiKeyRawLen = len(apiKeyScheme) + 43

	// apiKeyLastUsedThrottle is the minimum spacing between two last_used_at
	// writes for the same key. Authentication is read-only and must stay fast, so
	// the stamp is best-effort telemetry: writing it once a minute is plenty to
	// tell a user whether a key is in use. The repository receives the interval
	// as a parameter and must not hardcode it.
	apiKeyLastUsedThrottle = time.Minute

	// maxAPIKeyNameLen mirrors api_keys.name VARCHAR(100). PostgreSQL counts
	// characters, so validation counts runes rather than bytes.
	maxAPIKeyNameLen = 100

	apiKeyScopeRead  = "read"
	apiKeyScopeWrite = "write"
)

// allowedAPIKeyScopes is the closed set of scopes a key may carry. Anything else
// is a client error: silently storing an unknown scope would create a key whose
// privileges depend on whatever the eventual enforcement layer decides.
var allowedAPIKeyScopes = map[string]struct{}{
	apiKeyScopeRead:  {},
	apiKeyScopeWrite: {},
}

// generateAPIKey returns a new API key as three representations:
//
//   - raw is handed to the client exactly once and never stored.
//   - prefix is the non-secret display identifier persisted for listings.
//   - hash is the SHA-256 hex digest that is persisted and, in a later branch,
//     looked up when the key is presented as a bearer credential.
//
// raw is the scheme plus base64url of 32 random bytes: 46 characters total, with
// key_prefix CHAR(12) and key_hash CHAR(64) fitting the schema exactly.
func generateAPIKey() (raw, prefix, hash string, err error) {
	buf := make([]byte, apiKeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generate api key: %w", err)
	}

	raw = apiKeyScheme + base64.RawURLEncoding.EncodeToString(buf)
	prefix = raw[:apiKeyPrefixLen]

	return raw, prefix, secret.Hash(raw), nil
}

// normalizeAPIKeyName trims the name and enforces the column width. Blank names
// are rejected rather than defaulted: a key the user cannot recognise in a
// listing is worse than a validation error at creation time.
func normalizeAPIKeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n == 0 || n > maxAPIKeyNameLen {
		return "", fmt.Errorf("%w: name must be between 1 and %d characters", ErrInvalidInput, maxAPIKeyNameLen)
	}

	return name, nil
}

// normalizeAPIKeyScopes validates, normalizes and deduplicates requested scopes.
// Omitted or empty input means the single default scope, 'read'. Scopes are
// case-insensitive and trimmed, matching how usernames are normalized elsewhere.
// Order is preserved so the response mirrors the request.
func normalizeAPIKeyScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return []string{apiKeyScopeRead}, nil
	}

	seen := make(map[string]struct{}, len(scopes))
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope == "" {
			return nil, fmt.Errorf("%w: scope must not be empty", ErrInvalidInput)
		}
		if _, ok := allowedAPIKeyScopes[scope]; !ok {
			return nil, fmt.Errorf("%w: unsupported scope %q", ErrInvalidInput, scope)
		}
		if _, dup := seen[scope]; dup {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}

	return normalized, nil
}

// normalizeAPIKeyExpiry validates the optional expiry and stores it in UTC so
// the database, the JSON response and comparisons all agree. An expiry in the
// past is rejected: creating a key that is already unusable is always a mistake.
func normalizeAPIKeyExpiry(expiresAt *time.Time) (*time.Time, error) {
	if expiresAt == nil {
		return nil, nil
	}

	utc := expiresAt.UTC()
	if !utc.After(time.Now()) {
		return nil, fmt.Errorf("%w: expires_at must be in the future", ErrInvalidInput)
	}

	return &utc, nil
}

// looksLikeAPIKey reports whether raw has the shape of a key this service mints.
// It is a pre-filter, not an authorization check: a value that passes still has
// to match a stored hash. Rejecting obvious garbage here keeps an arbitrary
// header from costing a hash and a database round trip.
func looksLikeAPIKey(raw string) bool {
	return len(raw) == apiKeyRawLen && strings.HasPrefix(raw, apiKeyScheme)
}

// isAPIKeyUsable applies the key's time-based policy. Revocation and expiry are
// the only checks: like the JWT gate, the owning account's is_active flag is
// deliberately not consulted, so a key on a deactivated account keeps working
// until it expires or is revoked.
func isAPIKeyUsable(key *Key, now time.Time) bool {
	if key == nil || key.RevokedAt != nil {
		return false
	}

	return key.ExpiresAt == nil || key.ExpiresAt.After(now)
}
