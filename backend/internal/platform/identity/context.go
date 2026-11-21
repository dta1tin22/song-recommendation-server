// Package identity carries the authenticated caller's identity through a
// request context. It lives in platform so every feature package can share the
// same accessors without importing another feature (or the middleware that
// populates them).
package identity

import "context"

// contextKey is unexported so no other package can collide with this key: a
// context key compares on both type and value, and an unexported type from this
// package is never equal to a key declared elsewhere.
type contextKey int

const userIDKey contextKey = iota

// WithUserID returns a context carrying the authenticated user ID. Exported so
// handler tests can build the same context the middleware produces.
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext reports the authenticated user ID. Handlers call this and
// must treat a missing value as a 401, even though RequireAuth should have
// already guaranteed it: a route mounted without the middleware is a wiring bug,
// not an anonymous request.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userIDKey).(int64)
	return userID, ok
}
