package apikey_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/apikey"
)

// Compile-time proof that the pgx repository satisfies the contract the service
// depends on. Breaking the interface is a build failure, not a runtime one.
var _ apikey.Repository = (*apikey.PostgresRepository)(nil)

// --- fake ------------------------------------------------------------------

type fakeAPIKeyRepository struct {
	createCalls int
	createErr   error
	createdKey  *apikey.Key

	listCalls      int
	listErr        error
	listKeys       []apikey.Key
	lastListUserID int64

	revokeCalls      int
	revokeErr        error
	lastRevokeUserID int64
	lastRevokeKeyID  int64

	hashCalls int
	hashErr   error
	hashKey   *apikey.Key
	lastHash  string

	touchCalls   int
	touchErr     error
	lastTouchID  int64
	lastThrottle time.Duration
}

func (f *fakeAPIKeyRepository) Create(_ context.Context, key *apikey.Key) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	f.createdKey = key
	key.ID = 123
	key.CreatedAt = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	return nil
}

func (f *fakeAPIKeyRepository) ListByUser(_ context.Context, userID int64) ([]apikey.Key, error) {
	f.listCalls++
	f.lastListUserID = userID
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listKeys, nil
}

func (f *fakeAPIKeyRepository) Revoke(_ context.Context, userID, keyID int64) error {
	f.revokeCalls++
	f.lastRevokeUserID = userID
	f.lastRevokeKeyID = keyID
	return f.revokeErr
}

func (f *fakeAPIKeyRepository) GetByHash(_ context.Context, hash string) (*apikey.Key, error) {
	f.hashCalls++
	f.lastHash = hash
	if f.hashErr != nil {
		return nil, f.hashErr
	}
	if f.hashKey == nil {
		return nil, apikey.ErrAPIKeyNotFound
	}
	return f.hashKey, nil
}

func (f *fakeAPIKeyRepository) TouchLastUsed(_ context.Context, keyID int64, throttle time.Duration) error {
	f.touchCalls++
	f.lastTouchID = keyID
	f.lastThrottle = throttle
	return f.touchErr
}

// apiKeyTestHash recomputes the digest independently of the service, so the test
// proves the persisted value is SHA-256(raw) rather than merely echoing it.
func apiKeyTestHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// --- create ----------------------------------------------------------------

func TestCreateAPIKey(t *testing.T) {
	repo := &fakeAPIKeyRepository{}
	svc := apikey.NewService(repo)
	expires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)

	result, err := svc.CreateAPIKey(context.Background(), 7, apikey.CreateInput{
		Name:      "  CI pipeline  ",
		Scopes:    []string{"READ", " write ", "read"},
		ExpiresAt: &expires,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if result.Key == nil {
		t.Fatal("CreateAPIKey returned no key")
	}

	raw := result.RawKey
	if !strings.HasPrefix(raw, "mk_") {
		t.Fatalf("raw key %q does not use the mk_ scheme", raw)
	}
	// mk_ + base64url(32 bytes) = 3 + 43 characters.
	if got, want := len(raw), len("mk_")+43; got != want {
		t.Fatalf("raw key length = %d, want %d", got, want)
	}

	if got, want := result.Key.KeyPrefix, raw[:12]; got != want {
		t.Fatalf("key prefix = %q, want %q", got, want)
	}
	if got, want := len(result.Key.KeyPrefix), 12; got != want {
		t.Fatalf("key prefix length = %d, want %d (VARCHAR(12))", got, want)
	}

	if got, want := result.Key.KeyHash, apiKeyTestHash(raw); got != want {
		t.Fatalf("key hash = %q, want SHA-256(raw) %q", got, want)
	}
	if got, want := len(result.Key.KeyHash), 64; got != want {
		t.Fatalf("key hash length = %d, want %d (CHAR(64))", got, want)
	}
	if _, err := hex.DecodeString(result.Key.KeyHash); err != nil {
		t.Fatalf("key hash is not lowercase hex: %v", err)
	}
	if result.Key.KeyHash == raw {
		t.Fatal("the raw key must never be stored as the hash")
	}

	if got, want := result.Key.Name, "CI pipeline"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	if got, want := result.Key.Scopes, []string{"read", "write"}; !slices.Equal(got, want) {
		t.Fatalf("scopes = %v, want %v (normalized and deduplicated)", got, want)
	}
	if got, want := result.Key.UserID, int64(7); got != want {
		t.Fatalf("user id = %d, want %d", got, want)
	}
	if result.Key.ExpiresAt == nil || !result.Key.ExpiresAt.Equal(expires) {
		t.Fatalf("expires_at = %v, want %v", result.Key.ExpiresAt, expires)
	}
	if result.Key.ExpiresAt.Location() != time.UTC {
		t.Fatalf("expires_at location = %v, want UTC", result.Key.ExpiresAt.Location())
	}

	if repo.createCalls != 1 {
		t.Fatalf("repository Create calls = %d, want 1", repo.createCalls)
	}
	if repo.createdKey != result.Key {
		t.Fatal("the service did not persist the returned key")
	}
	if result.Key.ID != 123 {
		t.Fatalf("generated id was not populated back onto the model: %d", result.Key.ID)
	}
}

func TestCreateAPIKeyDefaultsToReadScope(t *testing.T) {
	svc := apikey.NewService(&fakeAPIKeyRepository{})

	for _, scopes := range [][]string{nil, {}} {
		result, err := svc.CreateAPIKey(context.Background(), 1, apikey.CreateInput{
			Name:   "default",
			Scopes: scopes,
		})
		if err != nil {
			t.Fatalf("CreateAPIKey(scopes=%v): %v", scopes, err)
		}
		if got, want := result.Key.Scopes, []string{"read"}; !slices.Equal(got, want) {
			t.Fatalf("scopes = %v, want default %v", got, want)
		}
	}
}

func TestCreateAPIKeyGeneratesUniqueKeys(t *testing.T) {
	svc := apikey.NewService(&fakeAPIKeyRepository{})

	first, err := svc.CreateAPIKey(context.Background(), 1, apikey.CreateInput{Name: "first"})
	if err != nil {
		t.Fatalf("first CreateAPIKey: %v", err)
	}
	second, err := svc.CreateAPIKey(context.Background(), 1, apikey.CreateInput{Name: "second"})
	if err != nil {
		t.Fatalf("second CreateAPIKey: %v", err)
	}

	if first.RawKey == second.RawKey {
		t.Fatal("two generated keys share the same raw value")
	}
	if first.Key.KeyHash == second.Key.KeyHash {
		t.Fatal("two generated keys share the same hash")
	}
}

func TestCreateAPIKeyAcceptsMaxLengthName(t *testing.T) {
	svc := apikey.NewService(&fakeAPIKeyRepository{})
	name := strings.Repeat("a", 100)

	result, err := svc.CreateAPIKey(context.Background(), 1, apikey.CreateInput{Name: name})
	if err != nil {
		t.Fatalf("CreateAPIKey with a 100-character name: %v", err)
	}
	if result.Key.Name != name {
		t.Fatalf("name = %q, want %q", result.Key.Name, name)
	}
}

func TestCreateAPIKeyRejectsInvalidInput(t *testing.T) {
	past := time.Now().Add(-time.Minute)

	tests := []struct {
		name string
		in   apikey.CreateInput
	}{
		{name: "empty name", in: apikey.CreateInput{Name: ""}},
		{name: "whitespace name", in: apikey.CreateInput{Name: "   "}},
		{name: "name too long", in: apikey.CreateInput{Name: strings.Repeat("a", 101)}},
		{name: "unknown scope", in: apikey.CreateInput{Name: "n", Scopes: []string{"admin"}}},
		{name: "empty scope", in: apikey.CreateInput{Name: "n", Scopes: []string{""}}},
		{name: "past expiry", in: apikey.CreateInput{Name: "n", ExpiresAt: &past}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeAPIKeyRepository{}
			svc := apikey.NewService(repo)

			if _, err := svc.CreateAPIKey(context.Background(), 1, tt.in); !errors.Is(err, apikey.ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
			if repo.createCalls != 0 {
				t.Fatalf("repository Create calls = %d, want 0 for invalid input", repo.createCalls)
			}
		})
	}
}

func TestCreateAPIKeyPropagatesRepositoryError(t *testing.T) {
	repoErr := errors.New("database unavailable")
	svc := apikey.NewService(&fakeAPIKeyRepository{createErr: repoErr})

	_, err := svc.CreateAPIKey(context.Background(), 1, apikey.CreateInput{Name: "key"})
	if !errors.Is(err, repoErr) {
		t.Fatalf("error = %v, want %v", err, repoErr)
	}
}

func TestAPIKeyModelNeverSerialisesSecret(t *testing.T) {
	svc := apikey.NewService(&fakeAPIKeyRepository{})

	result, err := svc.CreateAPIKey(context.Background(), 1, apikey.CreateInput{Name: "serialise"})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	encoded, err := json.Marshal(result.Key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	blob := string(encoded)

	if strings.Contains(blob, result.RawKey) {
		t.Fatal("the raw key leaked into the serialized model")
	}
	if strings.Contains(blob, result.Key.KeyHash) {
		t.Fatal("the key hash leaked into the serialized model")
	}
	if strings.Contains(blob, "key_hash") {
		t.Fatal("the key hash field name leaked into the serialized model")
	}
	if !strings.Contains(blob, result.Key.KeyPrefix) {
		t.Fatal("the safe display prefix is missing from the serialized model")
	}
}

// --- list ------------------------------------------------------------------

func TestListAPIKeys(t *testing.T) {
	want := []apikey.Key{{
		ID:        5,
		UserID:    7,
		Name:      "existing",
		KeyPrefix: "mk_abcdefghi",
		Scopes:    []string{"read"},
	}}
	repo := &fakeAPIKeyRepository{listKeys: want}
	svc := apikey.NewService(repo)

	got, err := svc.ListAPIKeys(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %+v, want %+v", got, want)
	}
	if got, want := repo.lastListUserID, int64(7); got != want {
		t.Fatalf("repository queried user %d, want %d", got, want)
	}
}

func TestListAPIKeysReturnsEmptySlice(t *testing.T) {
	svc := apikey.NewService(&fakeAPIKeyRepository{})

	keys, err := svc.ListAPIKeys(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if keys == nil {
		t.Fatal("ListAPIKeys returned nil; callers must be able to render []")
	}
	if len(keys) != 0 {
		t.Fatalf("keys = %v, want empty", keys)
	}
}

// --- revoke ----------------------------------------------------------------

func TestRevokeAPIKey(t *testing.T) {
	repo := &fakeAPIKeyRepository{}
	svc := apikey.NewService(repo)

	if err := svc.RevokeAPIKey(context.Background(), 7, 99); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	if repo.revokeCalls != 1 {
		t.Fatalf("repository Revoke calls = %d, want 1", repo.revokeCalls)
	}
	if got, want := repo.lastRevokeUserID, int64(7); got != want {
		t.Fatalf("repository scoped revoke to user %d, want %d", got, want)
	}
	if got, want := repo.lastRevokeKeyID, int64(99); got != want {
		t.Fatalf("repository revoked key %d, want %d", got, want)
	}
}

func TestRevokeAPIKeyPropagatesNotFound(t *testing.T) {
	repo := &fakeAPIKeyRepository{revokeErr: apikey.ErrAPIKeyNotFound}
	svc := apikey.NewService(repo)

	err := svc.RevokeAPIKey(context.Background(), 7, 99)
	if !errors.Is(err, apikey.ErrAPIKeyNotFound) {
		t.Fatalf("error = %v, want apikey.ErrAPIKeyNotFound", err)
	}
}

// --- authenticate ----------------------------------------------------------

func TestAuthenticateAPIKey(t *testing.T) {
	want := &apikey.Key{ID: 9, UserID: 7, Name: "plugin", Scopes: []string{"read"}}
	repo := &fakeAPIKeyRepository{hashKey: want}
	svc := apikey.NewService(repo)

	// Whitespace around the header value must not change the key that is hashed.
	got, err := svc.AuthenticateAPIKey(context.Background(), "  "+testRawAPIKey+"  ")
	if err != nil {
		t.Fatalf("AuthenticateAPIKey: %v", err)
	}
	if got != want {
		t.Fatalf("key = %+v, want %+v", got, want)
	}

	if repo.hashCalls != 1 {
		t.Fatalf("repository GetByHash calls = %d, want 1", repo.hashCalls)
	}
	if wantHash := apiKeyTestHash(testRawAPIKey); repo.lastHash != wantHash {
		t.Fatalf("repository looked up %q, want SHA-256 of the trimmed raw key %q", repo.lastHash, wantHash)
	}
	if repo.lastHash == testRawAPIKey {
		t.Fatal("the raw key must never cross the repository boundary")
	}
}

func TestAuthenticateAPIKeyRejectsUnusableKeys(t *testing.T) {
	past := time.Now().Add(-time.Minute)

	tests := []struct {
		name string
		key  *apikey.Key
	}{
		{name: "unknown", key: nil},
		{name: "revoked", key: &apikey.Key{ID: 1, RevokedAt: &past}},
		{name: "expired", key: &apikey.Key{ID: 1, ExpiresAt: &past}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeAPIKeyRepository{hashKey: tt.key}
			svc := apikey.NewService(repo)

			_, err := svc.AuthenticateAPIKey(context.Background(), testRawAPIKey)
			if !errors.Is(err, apikey.ErrInvalidAPIKey) {
				t.Fatalf("error = %v, want ErrInvalidAPIKey", err)
			}
			// The message must not echo the credential or hint at the cause.
			if strings.Contains(err.Error(), testRawAPIKey) {
				t.Fatal("the raw key leaked into the error message")
			}
		})
	}
}

func TestAuthenticateAPIKeyAcceptsUnexpiredKey(t *testing.T) {
	future := time.Now().Add(time.Hour)
	repo := &fakeAPIKeyRepository{hashKey: &apikey.Key{ID: 1, ExpiresAt: &future}}
	svc := apikey.NewService(repo)

	if _, err := svc.AuthenticateAPIKey(context.Background(), testRawAPIKey); err != nil {
		t.Fatalf("AuthenticateAPIKey: %v", err)
	}
}

func TestAuthenticateAPIKeyRejectsMalformedKeyWithoutQuery(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "whitespace", raw: "   "},
		{name: "wrong scheme", raw: "pk_" + strings.Repeat("a", 43)},
		{name: "too short", raw: "mk_abc"},
		{name: "too long", raw: testRawAPIKey + "a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeAPIKeyRepository{hashKey: &apikey.Key{ID: 1, UserID: 7}}
			svc := apikey.NewService(repo)

			if _, err := svc.AuthenticateAPIKey(context.Background(), tt.raw); !errors.Is(err, apikey.ErrInvalidAPIKey) {
				t.Fatalf("error = %v, want ErrInvalidAPIKey", err)
			}
			if repo.hashCalls != 0 {
				t.Fatalf("repository GetByHash calls = %d, want 0 for a malformed key", repo.hashCalls)
			}
		})
	}
}

func TestAuthenticateAPIKeyPropagatesRepositoryError(t *testing.T) {
	repoErr := errors.New("database unavailable")
	repo := &fakeAPIKeyRepository{hashErr: repoErr}
	svc := apikey.NewService(repo)

	_, err := svc.AuthenticateAPIKey(context.Background(), testRawAPIKey)
	if !errors.Is(err, repoErr) {
		t.Fatalf("error = %v, want %v", err, repoErr)
	}
	// An outage must not be disguised as a bad credential.
	if errors.Is(err, apikey.ErrInvalidAPIKey) {
		t.Fatal("an infrastructure failure was reported as ErrInvalidAPIKey")
	}
}

// --- last used -------------------------------------------------------------

func TestTouchAPIKeyUsage(t *testing.T) {
	repo := &fakeAPIKeyRepository{}
	svc := apikey.NewService(repo)

	if err := svc.TouchAPIKeyUsage(context.Background(), 9); err != nil {
		t.Fatalf("TouchAPIKeyUsage: %v", err)
	}
	if repo.touchCalls != 1 {
		t.Fatalf("repository TouchLastUsed calls = %d, want 1", repo.touchCalls)
	}
	if repo.lastTouchID != 9 {
		t.Fatalf("repository touched key %d, want 9", repo.lastTouchID)
	}
	// The throttle is policy: it must come from the service, not the repository.
	if repo.lastThrottle != time.Minute {
		t.Fatalf("throttle = %s, want %s", repo.lastThrottle, time.Minute)
	}
}

func TestTouchAPIKeyUsageRejectsInvalidID(t *testing.T) {
	repo := &fakeAPIKeyRepository{}
	svc := apikey.NewService(repo)

	if err := svc.TouchAPIKeyUsage(context.Background(), 0); err == nil {
		t.Fatal("TouchAPIKeyUsage accepted a zero key id")
	}
	if repo.touchCalls != 0 {
		t.Fatalf("repository TouchLastUsed calls = %d, want 0", repo.touchCalls)
	}
}

func TestTouchAPIKeyUsagePropagatesRepositoryError(t *testing.T) {
	repoErr := errors.New("database unavailable")
	repo := &fakeAPIKeyRepository{touchErr: repoErr}
	svc := apikey.NewService(repo)

	if err := svc.TouchAPIKeyUsage(context.Background(), 9); !errors.Is(err, repoErr) {
		t.Fatalf("error = %v, want %v", err, repoErr)
	}
}

// --- identity --------------------------------------------------------------

func TestAPIKeyServiceRejectsMissingUserID(t *testing.T) {
	repo := &fakeAPIKeyRepository{}
	svc := apikey.NewService(repo)

	_, createErr := svc.CreateAPIKey(context.Background(), 0, apikey.CreateInput{Name: "key"})
	_, listErr := svc.ListAPIKeys(context.Background(), 0)
	revokeErr := svc.RevokeAPIKey(context.Background(), 0, 1)

	for name, err := range map[string]error{"create": createErr, "list": listErr, "revoke": revokeErr} {
		if err == nil {
			t.Fatalf("%s accepted a zero user id", name)
		}
		// A missing identity is a wiring bug, not client input: it must not be
		// reported as a 400.
		if errors.Is(err, apikey.ErrInvalidInput) {
			t.Fatalf("%s reported a missing user id as ErrInvalidInput", name)
		}
	}
	if repo.createCalls != 0 || repo.listCalls != 0 || repo.revokeCalls != 0 {
		t.Fatal("repository was called without an authenticated user id")
	}
}
