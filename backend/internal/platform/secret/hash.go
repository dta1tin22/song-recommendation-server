package secret

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hash returns the lowercase hex SHA-256 digest of value.
//
// This is the one derivation used for every high-entropy bearer credential the
// service persists (refresh tokens and API keys). A fast hash is correct here
// because the input already carries 256 bits of entropy: unlike a password there
// is no low-entropy secret to brute-force, and verification must stay cheap on
// the request path. Hex keeps the digest at exactly 64 characters, matching the
// CHAR(64) hash columns.
func Hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
