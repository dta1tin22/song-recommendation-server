package apikey_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/apikey"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/identity"
)

const (
	testAPIKeyHeader = "X-API-Key"
	testAPIKeyID     = int64(9)
	testAPIKeyUserID = int64(42)

	// testRawAPIKey is shaped like a generated key: mk_ plus 43 base64url
	// characters.
	testRawAPIKey = "mk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
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

// touchReport is what the fake observed inside the background stamp, which is
// the only way to prove the context handling from outside the middleware.
type touchReport struct {
	keyID       int64
	ctxErr      error
	hasDeadline bool
	deadline    time.Time
}

type fakeAPIKeyAuthenticator struct {
	key      *apikey.Key
	authErr  error
	touchErr error

	authCalls int32
	lastRaw   string
	touched   chan touchReport
}

func (f *fakeAPIKeyAuthenticator) AuthenticateAPIKey(_ context.Context, raw string) (*apikey.Key, error) {
	atomic.AddInt32(&f.authCalls, 1)
	f.lastRaw = raw
	if f.authErr != nil {
		return nil, f.authErr
	}
	return f.key, nil
}

func (f *fakeAPIKeyAuthenticator) TouchAPIKeyUsage(ctx context.Context, keyID int64) error {
	deadline, hasDeadline := ctx.Deadline()
	if f.touched != nil {
		// Buffered and non-blocking: the writer must never stall on the channel.
		select {
		case f.touched <- touchReport{keyID: keyID, ctxErr: ctx.Err(), hasDeadline: hasDeadline, deadline: deadline}:
		default:
		}
	}
	return f.touchErr
}

func (f *fakeAPIKeyAuthenticator) calls() int32 { return atomic.LoadInt32(&f.authCalls) }

// waitTouch blocks until the background stamp runs, or fails the test.
func waitTouch(t *testing.T, f *fakeAPIKeyAuthenticator) touchReport {
	t.Helper()

	select {
	case report := <-f.touched:
		return report
	case <-time.After(2 * time.Second):
		t.Fatal("last_used_at stamp never ran")
		return touchReport{}
	}
}

// echoAPIKey reports the non-secret key identity the middleware injected.
func echoAPIKey(w http.ResponseWriter, r *http.Request) {
	key, ok := apikey.KeyFromContext(r.Context())
	if !ok {
		http.Error(w, "no api key in context", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%d:%s", key.ID, strings.Join(key.Scopes, ","))
}

func decodeErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error envelope: %v (body %q)", err, rec.Body.String())
	}
	if body.Error.Message == "" {
		t.Fatalf("error envelope has no message: %q", rec.Body.String())
	}
	return body.Error.Code
}

func validAPIKey() *apikey.Key {
	return &apikey.Key{
		ID:     testAPIKeyID,
		UserID: testAPIKeyUserID,
		Scopes: []string{"read", "write"},
	}
}

func TestRequireAPIKeyAcceptsValidKey(t *testing.T) {
	fake := &fakeAPIKeyAuthenticator{key: validAPIKey(), touched: make(chan touchReport, 1)}
	handler := apikey.RequireAPIKey(fake)(http.HandlerFunc(echoUserID))

	req := httptest.NewRequest(http.MethodGet, "/plugin", nil)
	req.Header.Set(testAPIKeyHeader, testRawAPIKey)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != fmt.Sprintf("%d", testAPIKeyUserID) {
		t.Fatalf("handler saw user id %q, want %d", got, testAPIKeyUserID)
	}
	if fake.calls() != 1 {
		t.Fatalf("authenticator calls = %d, want 1", fake.calls())
	}
	if fake.lastRaw != testRawAPIKey {
		t.Fatalf("authenticator received %q, want the presented key", fake.lastRaw)
	}
}

func TestRequireAPIKeyRejectsBadHeaders(t *testing.T) {
	tests := []struct {
		name  string
		value string
		set   bool
	}{
		{name: "missing header", set: false},
		{name: "empty header", value: "", set: true},
		{name: "whitespace header", value: "   ", set: true},
		{name: "oversized header", value: strings.Repeat("a", 513), set: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeAPIKeyAuthenticator{key: validAPIKey()}
			handler := apikey.RequireAPIKey(fake)(http.HandlerFunc(echoUserID))

			req := httptest.NewRequest(http.MethodGet, "/plugin", nil)
			if tt.set {
				req.Header.Set(testAPIKeyHeader, tt.value)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := decodeErrorCode(t, rec); got != "unauthorized" {
				t.Fatalf("error code = %q, want unauthorized", got)
			}
			// An unusable header must not reach the service or the database.
			if fake.calls() != 0 {
				t.Fatalf("authenticator calls = %d, want 0", fake.calls())
			}
		})
	}
}

func TestRequireAPIKeyRejectsUnusableKey(t *testing.T) {
	// The service collapses unknown, revoked and expired keys into one sentinel;
	// a wrapped sentinel must map identically.
	errs := map[string]error{
		"direct":  apikey.ErrInvalidAPIKey,
		"wrapped": fmt.Errorf("authenticate plugin key: %w", apikey.ErrInvalidAPIKey),
	}

	for name, authErr := range errs {
		t.Run(name, func(t *testing.T) {
			fake := &fakeAPIKeyAuthenticator{authErr: authErr}
			handler := apikey.RequireAPIKey(fake)(http.HandlerFunc(echoUserID))

			req := httptest.NewRequest(http.MethodGet, "/plugin", nil)
			req.Header.Set(testAPIKeyHeader, testRawAPIKey)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := decodeErrorCode(t, rec); got != "unauthorized" {
				t.Fatalf("error code = %q, want unauthorized", got)
			}
			if strings.Contains(rec.Body.String(), testRawAPIKey) {
				t.Fatal("the presented key leaked into the error body")
			}
		})
	}
}

func TestRequireAPIKeyMapsRepositoryFailureToInternalError(t *testing.T) {
	fake := &fakeAPIKeyAuthenticator{authErr: errors.New("database unavailable")}
	handler := apikey.RequireAPIKey(fake)(http.HandlerFunc(echoUserID))

	req := httptest.NewRequest(http.MethodGet, "/plugin", nil)
	req.Header.Set(testAPIKeyHeader, testRawAPIKey)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if got := decodeErrorCode(t, rec); got != "internal_error" {
		t.Fatalf("error code = %q, want internal_error", got)
	}
	// Internal detail is logged, never returned.
	if strings.Contains(rec.Body.String(), "database unavailable") {
		t.Fatal("the repository error leaked into the response")
	}
}

func TestRequireAPIKeyInjectsKeyIdentity(t *testing.T) {
	fake := &fakeAPIKeyAuthenticator{key: validAPIKey()}
	handler := apikey.RequireAPIKey(fake)(http.HandlerFunc(echoAPIKey))

	req := httptest.NewRequest(http.MethodGet, "/plugin", nil)
	req.Header.Set(testAPIKeyHeader, testRawAPIKey)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got, want := rec.Body.String(), fmt.Sprintf("%d:read,write", testAPIKeyID); got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
}

func TestRequireAPIKeyStampsLastUsed(t *testing.T) {
	fake := &fakeAPIKeyAuthenticator{key: validAPIKey(), touched: make(chan touchReport, 1)}
	handler := apikey.RequireAPIKey(fake)(http.HandlerFunc(echoUserID))

	req := httptest.NewRequest(http.MethodGet, "/plugin", nil)
	req.Header.Set(testAPIKeyHeader, testRawAPIKey)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	report := waitTouch(t, fake)
	if report.keyID != testAPIKeyID {
		t.Fatalf("stamped key %d, want %d", report.keyID, testAPIKeyID)
	}
	if report.ctxErr != nil {
		t.Fatalf("stamp context is already %v, want live", report.ctxErr)
	}
	if !report.hasDeadline {
		t.Fatal("stamp context has no deadline; the 3s timeout is missing")
	}
	if remaining := time.Until(report.deadline); remaining <= 0 || remaining > 3*time.Second {
		t.Fatalf("stamp deadline is %s away, want (0, 3s]", remaining)
	}
}

func TestRequireAPIKeyStampSurvivesCancelledRequest(t *testing.T) {
	fake := &fakeAPIKeyAuthenticator{key: validAPIKey(), touched: make(chan touchReport, 1)}
	handler := apikey.RequireAPIKey(fake)(http.HandlerFunc(echoUserID))

	// The client is already gone when the stamp runs. WithoutCancel must keep the
	// write alive, because last_used_at would otherwise be lost on every timeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, "/plugin", nil).WithContext(ctx)
	req.Header.Set(testAPIKeyHeader, testRawAPIKey)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	report := waitTouch(t, fake)
	if report.ctxErr != nil {
		t.Fatalf("stamp context = %v, want a detached, live context", report.ctxErr)
	}
}

func TestRequireAPIKeyStampFailureDoesNotFailRequest(t *testing.T) {
	fake := &fakeAPIKeyAuthenticator{
		key:      validAPIKey(),
		touchErr: errors.New("write failed"),
		touched:  make(chan touchReport, 1),
	}
	handler := apikey.RequireAPIKey(fake)(http.HandlerFunc(echoUserID))

	req := httptest.NewRequest(http.MethodGet, "/plugin", nil)
	req.Header.Set(testAPIKeyHeader, testRawAPIKey)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even when the stamp fails", rec.Code)
	}
	waitTouch(t, fake)
}

func TestRequireAPIKeyIgnoresAuthorizationHeader(t *testing.T) {
	fake := &fakeAPIKeyAuthenticator{key: validAPIKey()}
	handler := apikey.RequireAPIKey(fake)(http.HandlerFunc(echoUserID))

	// A valid bearer token is not an API key: this gate must not fall back to it.
	req := httptest.NewRequest(http.MethodGet, "/plugin", nil)
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if fake.calls() != 0 {
		t.Fatalf("authenticator calls = %d, want 0", fake.calls())
	}
}
