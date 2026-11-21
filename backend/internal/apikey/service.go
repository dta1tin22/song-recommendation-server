package apikey

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/secret"
)

// CreateInput is the validated-here, client-supplied part of key creation.
// Identity (user ID), the prefix and the hash are never accepted from a client.
type CreateInput struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

// CreateResult carries the one-and-only raw copy of a freshly created key
// alongside the persisted row. RawKey must be serialized to the client and then
// forgotten: it is never logged, stored or recoverable.
type CreateResult struct {
	RawKey string
	Key    *Key
}

// Service owns API-key policy: key format, name/scope/expiry validation
// and the user-scoped calls into persistence. It never reads the request context
// itself; the caller supplies the authenticated user ID as data.
type Service struct {
	keys Repository
}

func NewService(keys Repository) *Service {
	return &Service{keys: keys}
}

// CreateAPIKey validates the input, derives a fresh key and persists only its
// prefix and hash. The raw key is returned once, to the caller, and never leaves
// the process again.
func (s *Service) CreateAPIKey(ctx context.Context, userID int64, in CreateInput) (*CreateResult, error) {
	if userID <= 0 {
		// Not a client error: the authenticated identity is missing, which is a
		// wiring bug. Fail loudly instead of creating an ownerless key.
		return nil, fmt.Errorf("create api key: invalid user id %d", userID)
	}

	name, err := normalizeAPIKeyName(in.Name)
	if err != nil {
		return nil, err
	}

	scopes, err := normalizeAPIKeyScopes(in.Scopes)
	if err != nil {
		return nil, err
	}

	expiresAt, err := normalizeAPIKeyExpiry(in.ExpiresAt)
	if err != nil {
		return nil, err
	}

	raw, prefix, hash, err := generateAPIKey()
	if err != nil {
		return nil, err
	}

	key := &Key{
		UserID:    userID,
		Name:      name,
		KeyPrefix: prefix,
		KeyHash:   hash,
		Scopes:    scopes,
		ExpiresAt: expiresAt,
	}

	if err := s.keys.Create(ctx, key); err != nil {
		return nil, err
	}

	return &CreateResult{RawKey: raw, Key: key}, nil
}

// ListAPIKeys returns the user's active keys. The repository filters by user ID
// and by revoked_at, so the service has no filtering policy of its own.
func (s *Service) ListAPIKeys(ctx context.Context, userID int64) ([]Key, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("list api keys: invalid user id %d", userID)
	}

	keys, err := s.keys.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Keep the JSON contract stable for a caller with no keys: an empty array,
	// never null, regardless of what a repository implementation returns.
	if keys == nil {
		keys = []Key{}
	}

	return keys, nil
}

// RevokeAPIKey soft-deletes one of the user's keys. ErrAPIKeyNotFound
// passes through unchanged so the handler can answer 404 for an unknown, foreign
// or already-revoked key without distinguishing between them.
func (s *Service) RevokeAPIKey(ctx context.Context, userID, keyID int64) error {
	if userID <= 0 {
		return fmt.Errorf("revoke api key: invalid user id %d", userID)
	}

	return s.keys.Revoke(ctx, userID, keyID)
}

// AuthenticateAPIKey resolves a raw key presented on the request path and
// returns the persisted row, which carries the owning user ID and the scopes a
// later authorization layer will enforce.
//
// ErrInvalidAPIKey covers every unusable credential — unknown, revoked, expired
// — so the caller can answer all of them identically. Any other error is an
// infrastructure failure and is returned unchanged rather than disguised as a
// bad credential: a database outage must not look like an invalid key.
//
// The raw value is never persisted or logged; only its digest crosses the
// repository boundary, exactly as with creation.
func (s *Service) AuthenticateAPIKey(ctx context.Context, raw string) (*Key, error) {
	raw = strings.TrimSpace(raw)
	if !looksLikeAPIKey(raw) {
		return nil, ErrInvalidAPIKey
	}

	key, err := s.keys.GetByHash(ctx, secret.Hash(raw))
	if err != nil {
		if errors.Is(err, ErrAPIKeyNotFound) {
			return nil, ErrInvalidAPIKey
		}
		return nil, err
	}

	if !isAPIKeyUsable(key, time.Now()) {
		return nil, ErrInvalidAPIKey
	}

	return key, nil
}

// TouchAPIKeyUsage records that a key authenticated a request. It is throttled
// and best-effort telemetry: callers must never fail a request because the stamp
// did not land.
func (s *Service) TouchAPIKeyUsage(ctx context.Context, keyID int64) error {
	if keyID <= 0 {
		return fmt.Errorf("touch api key usage: invalid key id %d", keyID)
	}

	return s.keys.TouchLastUsed(ctx, keyID, apiKeyLastUsedThrottle)
}
