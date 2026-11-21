package auth

import (
	"net/http"
	"strings"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/httpx"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/identity"
)

// maxAccessTokenBytes caps the Authorization header value before it reaches the
// JWT parser. A valid token is well under 1 KiB; the cap turns a deliberately
// huge header into an immediate 401 instead of parser allocations.
const maxAccessTokenBytes = 4096

// RequireAuth rejects requests without a valid HS256 access token and stores the
// authenticated user ID in the request context.
//
// Scope: this middleware verifies the token's signature, algorithm, issuer and
// expiry only. It deliberately does not re-read the user from the database (a
// deactivated account stays usable until its access token expires), does not
// enforce roles, and has no notion of revoked tokens or API keys. Those belong
// to the API-key authentication path.
func RequireAuth(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok || len(raw) > maxAccessTokenBytes {
				writeUnauthorized(w)
				return
			}

			claims, err := ParseAccessToken(raw, secret)
			if err != nil {
				writeUnauthorized(w)
				return
			}

			userID, err := claims.UserID()
			if err != nil {
				writeUnauthorized(w)
				return
			}

			next.ServeHTTP(w, r.WithContext(identity.WithUserID(r.Context(), userID)))
		})
	}
}

// bearerToken extracts the credentials from an Authorization header value. The
// scheme comparison is case-insensitive, as RFC 7235 requires.
func bearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}

	return token, true
}

func writeUnauthorized(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
}
