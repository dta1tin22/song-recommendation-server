package auth

import (
	"strconv"

	"github.com/golang-jwt/jwt/v5"
)

// ParseAccessToken verifies a bearer token and returns its claims. It lives next
// to generateToken, so token policy lives in exactly one place: the middleware
// only decides how to answer a rejection.
//
// The checks are deliberately strict:
//   - only HS256 is accepted, so a token can never switch the verifier onto an
//     asymmetric or "none" algorithm;
//   - the issuer must match the one generateToken stamps;
//   - an expiry claim is mandatory, so a token without exp cannot live forever;
//   - the subject must parse as a positive user ID, because every protected
//     handler keys its queries off it.
func ParseAccessToken(raw string, secret []byte) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(
		raw,
		claims,
		func(*jwt.Token) (any, error) {
			return secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(tokenIssuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid {
		return nil, ErrInvalidAccessToken
	}

	if _, err := claims.UserID(); err != nil {
		return nil, ErrInvalidAccessToken
	}

	return claims, nil
}

// UserID returns the user ID carried in the token's subject claim.
func (c *Claims) UserID() (int64, error) {
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrInvalidAccessToken
	}

	return id, nil
}
