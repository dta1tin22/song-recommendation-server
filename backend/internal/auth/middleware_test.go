package auth_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/auth"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/identity"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwtTestSecret = "test-secret-value-long-enough-for-hs256"
	testIssuer    = "music-backend"
	testUserID    = int64(42)
	oversizeBody  = 5000 // > middleware's 4096-byte cap
)

// echoUserID is the protected handler under test: it proves the middleware put
// the right user ID in the context before calling through.
func echoUserID(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "no user id in context", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%d", userID)
}

func validClaims() auth.Claims {
	now := time.Now()
	return auth.Claims{
		Username: "alice",
		Role:     "user",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", testUserID),
			Issuer:    testIssuer,
			ID:        "test-jti",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
}

func sign(t *testing.T, method jwt.SigningMethod, key any, claims jwt.Claims) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestRequireAuth(t *testing.T) {
	secret := []byte(jwtTestSecret)

	// Builders keep each case's token setup next to the case itself.
	hs256 := func(claims jwt.Claims) string { return sign(t, jwt.SigningMethodHS256, secret, claims) }
	hs384 := func(claims jwt.Claims) string { return sign(t, jwt.SigningMethodHS384, secret, claims) }
	unsigned := func(claims jwt.Claims) string {
		return sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, claims)
	}

	claimsWith := func(mutate func(*auth.Claims)) jwt.Claims {
		claims := validClaims()
		mutate(&claims)
		return claims
	}

	tests := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{
			name:       "valid token",
			header:     "Bearer " + hs256(validClaims()),
			wantStatus: http.StatusOK,
		},
		{
			name:       "scheme is case insensitive",
			header:     "bearer " + hs256(validClaims()),
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing header",
			header:     "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong scheme",
			header:     "Basic " + hs256(validClaims()),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "scheme without token",
			header:     "Bearer",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty token",
			header:     "Bearer ",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "malformed token",
			header:     "Bearer not-a-jwt",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong secret",
			header:     "Bearer " + sign(t, jwt.SigningMethodHS256, []byte("another-secret"), validClaims()),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong algorithm hmac",
			header:     "Bearer " + hs384(validClaims()),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "unsigned token",
			header:     "Bearer " + unsigned(validClaims()),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "expired token",
			header: "Bearer " + hs256(claimsWith(func(c *auth.Claims) {
				c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
			})),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "missing expiry",
			header: "Bearer " + hs256(claimsWith(func(c *auth.Claims) {
				c.ExpiresAt = nil
			})),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "wrong issuer",
			header: "Bearer " + hs256(claimsWith(func(c *auth.Claims) {
				c.Issuer = "another-service"
			})),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "missing issuer",
			header: "Bearer " + hs256(claimsWith(func(c *auth.Claims) {
				c.Issuer = ""
			})),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "non numeric subject",
			header: "Bearer " + hs256(claimsWith(func(c *auth.Claims) {
				c.Subject = "alice"
			})),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "zero subject",
			header: "Bearer " + hs256(claimsWith(func(c *auth.Claims) {
				c.Subject = "0"
			})),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "missing subject",
			header: "Bearer " + hs256(claimsWith(func(c *auth.Claims) {
				c.Subject = ""
			})),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "oversized token",
			header:     "Bearer " + strings.Repeat("a", oversizeBody),
			wantStatus: http.StatusUnauthorized,
		},
	}

	handler := auth.RequireAuth(secret)(http.HandlerFunc(echoUserID))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusOK {
				if got := rec.Body.String(); got != fmt.Sprintf("%d", testUserID) {
					t.Fatalf("user id = %q, want %d", got, testUserID)
				}
				return
			}

			var body struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error envelope: %v (body %q)", err, rec.Body.String())
			}
			if body.Error.Code != "unauthorized" || body.Error.Message == "" {
				t.Fatalf("error envelope = %+v, want code unauthorized with a message", body.Error)
			}
		})
	}
}
