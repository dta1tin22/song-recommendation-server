package apikey

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/httpx"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/identity"
)

const (
	// apiKeyHeader is where external plugins present their credential. It is
	// never `Authorization`: that header belongs to the JWT gate, and keeping the
	// two apart is what makes it unambiguous which credential authorized a
	// request.
	apiKeyHeader = "X-API-Key"

	// maxAPIKeyBytes caps the header before it reaches the service. A real key is
	// 46 characters; the cap keeps a deliberately huge header from costing a hash
	// or a database round trip, mirroring maxAccessTokenBytes on the JWT path.
	maxAPIKeyBytes = 512

	// maxConcurrentLastUsedStamps bounds the background writers. last_used_at is
	// telemetry, so a saturated queue drops the stamp instead of growing an
	// unbounded number of goroutines.
	maxConcurrentLastUsedStamps = 64

	// lastUsedStampTimeout bounds one usage write. It is generous for a single
	// indexed UPDATE but finite, so a stuck database cannot pin a goroutine
	// forever.
	lastUsedStampTimeout = 3 * time.Second
)

type contextKey int

// apiKeyKey stores the authenticating key's non-secret identity. The user ID
// lives in the identity package, whose unexported key type can never collide
// with this one.
const apiKeyKey contextKey = iota

// Authenticator is the slice of the API-key service this middleware needs.
// Declaring it here, rather than depending on *Service, lets the gate be tested
// with a fake and keeps the transport layer from acquiring a second reason to
// import the concrete service.
type Authenticator interface {
	AuthenticateAPIKey(ctx context.Context, raw string) (*Key, error)
	TouchAPIKeyUsage(ctx context.Context, keyID int64) error
}

// KeyIdentity is the non-secret part of the key that authenticated a request.
// The digest stays inside the service, so nothing downstream can accidentally
// serialise or log it.
type KeyIdentity struct {
	ID     int64
	Scopes []string
}

// RequireAPIKey rejects requests without a usable `X-API-Key` and stores the
// authenticated user ID — plus the key's non-secret identity — in the request
// context.
//
// Unlike RequireAuth this is not stateless: every accepted request costs one
// indexed lookup on api_keys.key_hash. That is the price of revocable
// credentials, and an in-memory cache is deliberately avoided because it would
// make revocation and expiry non-immediate.
//
// Scope: it verifies that the key exists, is not revoked and is not expired. It
// does not re-check users.is_active (matching RequireAuth), does not enforce the
// key's scopes yet, and never inspects the Authorization header.
func RequireAPIKey(auth Authenticator) func(http.Handler) http.Handler {
	// One semaphore per middleware instance, shared by every request. It is
	// buffered, so acquiring it never blocks.
	stamps := make(chan struct{}, maxConcurrentLastUsedStamps)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimSpace(r.Header.Get(apiKeyHeader))
			if raw == "" || len(raw) > maxAPIKeyBytes {
				writeUnauthorized(w)
				return
			}

			key, err := auth.AuthenticateAPIKey(r.Context(), raw)
			if err != nil {
				if errors.Is(err, ErrInvalidAPIKey) {
					writeUnauthorized(w)
					return
				}

				// Not a credential problem: logging and answering 500 avoids
				// telling a caller with a valid key that its key is invalid just
				// because the database is unavailable.
				log.Printf("middleware: api key authentication failed: %v", err)
				httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			}

			// Best effort and off the request path: the stamp must never delay or
			// fail an authenticated request.
			stampLastUsed(stamps, auth, key.ID, r.Context())

			ctx := WithKey(r.Context(), KeyIdentity{ID: key.ID, Scopes: key.Scopes})
			next.ServeHTTP(w, r.WithContext(identity.WithUserID(ctx, key.UserID)))
		})
	}
}

// stampLastUsed records a usage stamp in the background. The semaphore is
// acquired non-blockingly: when all slots are busy the stamp is dropped, because
// last_used_at is observational data and never a security control.
func stampLastUsed(sem chan struct{}, auth Authenticator, keyID int64, reqCtx context.Context) {
	select {
	case sem <- struct{}{}:
	default:
		return
	}

	go func() {
		defer func() { <-sem }()

		// WithoutCancel detaches from the request lifetime: a client that hangs up
		// after being served must not cancel the write. The timeout keeps a stuck
		// database from pinning the slot.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(reqCtx), lastUsedStampTimeout)
		defer cancel()

		if err := auth.TouchAPIKeyUsage(ctx, keyID); err != nil {
			log.Printf("middleware: update api key last_used_at: %v", err)
		}
	}()
}

// WithKey returns a context carrying the authenticating key's non-secret
// identity.
func WithKey(ctx context.Context, key KeyIdentity) context.Context {
	return context.WithValue(ctx, apiKeyKey, key)
}

// KeyFromContext reports which API key authenticated the request. Handlers
// that do not care about the key itself keep using UserIDFromContext; this is
// for the scope enforcement that lands in a later branch.
func KeyFromContext(ctx context.Context) (KeyIdentity, bool) {
	key, ok := ctx.Value(apiKeyKey).(KeyIdentity)
	return key, ok
}

func writeUnauthorized(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
}
