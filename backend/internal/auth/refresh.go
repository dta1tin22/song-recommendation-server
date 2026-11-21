package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/secret"
)

const (
	// refreshTokenBytes is the entropy of a refresh token. 256 bits makes the
	// value unguessable, which is why a fast hash (SHA-256) is sufficient below:
	// unlike a password there is no low-entropy secret to brute-force.
	refreshTokenBytes = 32
)

// generateRefreshToken returns a new refresh token as two representations:
//
//   - raw is handed to the client and never stored.
//   - hash is the SHA-256 hex digest that is persisted and looked up.
//
// Hashing a 256-bit random value needs no stretching, and using hex keeps the
// digest at exactly 64 characters, matching refresh_tokens.token_hash CHAR(64).
func generateRefreshToken() (raw string, hash string, err error) {
	buf := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}

	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashRefreshToken(raw), nil
}

// hashRefreshToken derives the stored form of a client-supplied refresh token.
// It is deliberately the same derivation used at issue time, so a token can only
// ever match the row it was created from.
func hashRefreshToken(raw string) string {
	return secret.Hash(raw)
}
