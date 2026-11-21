package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/auth"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Compile-time proof that the pgx repositories satisfy the contracts the service
// depends on. Breaking either interface is a build failure, not a runtime one.
var (
	_ auth.UserRepository         = (*auth.PostgresUserRepository)(nil)
	_ auth.RefreshTokenRepository = (*auth.PostgresRefreshTokenRepository)(nil)
)

const testSecret = "test-secret"

var testPassword = "correct horse battery"

// --- fakes -----------------------------------------------------------------

// fakeUserRepository is an in-memory UserRepository. Every method that mutates
// state records the call so tests can assert on policy (threshold, duration,
// reset ordering) rather than on database effects.
type fakeUserRepository struct {
	user *auth.User

	getUserByIDErr   error
	recordFailedErr  error
	recordSuccessErr error

	failedLoginCalls int
	lastMaxAttempts  int16
	lastLockDuration time.Duration
	successCalls     int

	// onFailedLogin lets a test simulate the repository's threshold behaviour
	// (i.e. the account actually becoming locked on the Nth attempt).
	onFailedLogin func()
}

func (f *fakeUserRepository) CreateUser(context.Context, *auth.User) error { return nil }

func (f *fakeUserRepository) GetUserByUsername(_ context.Context, username string) (*auth.User, error) {
	if f.user == nil || f.user.Username != username {
		return nil, auth.ErrUserNotFound
	}
	return f.user, nil
}

func (f *fakeUserRepository) GetUserByID(_ context.Context, id int64) (*auth.User, error) {
	if f.getUserByIDErr != nil {
		return nil, f.getUserByIDErr
	}
	if f.user == nil || f.user.ID != id {
		return nil, auth.ErrUserNotFound
	}
	return f.user, nil
}

func (f *fakeUserRepository) RecordFailedLogin(_ context.Context, _ int64, maxAttempts int16, lockDuration time.Duration) error {
	f.failedLoginCalls++
	f.lastMaxAttempts = maxAttempts
	f.lastLockDuration = lockDuration
	if f.onFailedLogin != nil {
		f.onFailedLogin()
	}
	return f.recordFailedErr
}

func (f *fakeUserRepository) RecordSuccessfulLogin(context.Context, int64) error {
	f.successCalls++
	return f.recordSuccessErr
}

// fakeRefreshRepository is an in-memory RefreshTokenRepository keyed by hash,
// mirroring the uniqueness the database enforces on token_hash.
type fakeRefreshRepository struct {
	tokens map[string]*auth.RefreshToken

	createErr error
	deleteErr error
	rotateErr error

	creates     []*auth.RefreshToken
	deletes     []string
	rotations   int
	rotatedFrom []string
}

func newFakeRefreshRepository() *fakeRefreshRepository {
	return &fakeRefreshRepository{tokens: map[string]*auth.RefreshToken{}}
}

func (f *fakeRefreshRepository) Create(_ context.Context, token *auth.RefreshToken) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.creates = append(f.creates, token)
	f.tokens[token.TokenHash] = token
	return nil
}

func (f *fakeRefreshRepository) GetByHash(_ context.Context, hash string) (*auth.RefreshToken, error) {
	token, ok := f.tokens[hash]
	if !ok {
		return nil, auth.ErrRefreshTokenNotFound
	}
	return token, nil
}

func (f *fakeRefreshRepository) DeleteByHash(_ context.Context, hash string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deletes = append(f.deletes, hash)
	delete(f.tokens, hash)
	return nil
}

func (f *fakeRefreshRepository) Rotate(_ context.Context, oldHash string, newToken *auth.RefreshToken) error {
	if f.rotateErr != nil {
		return f.rotateErr
	}
	if _, ok := f.tokens[oldHash]; !ok {
		return auth.ErrRefreshTokenNotFound
	}
	delete(f.tokens, oldHash)
	f.rotations++
	f.rotatedFrom = append(f.rotatedFrom, oldHash)
	f.tokens[newToken.TokenHash] = newToken
	return nil
}

// seed inserts a session directly, bypassing Create, so a test can set up an
// expired or deactivated-user token without going through login.
func (f *fakeRefreshRepository) seed(raw string, userID int64, expiresAt time.Time) *auth.RefreshToken {
	token := &auth.RefreshToken{UserID: userID, TokenHash: sha256Hex(raw), ExpiresAt: expiresAt}
	f.tokens[token.TokenHash] = token
	return token
}

// --- helpers ---------------------------------------------------------------

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func newUser(t *testing.T, password string) *auth.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return &auth.User{
		ID:           42,
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: string(hash),
		Role:         "user",
		IsActive:     true,
	}
}

func newService(users auth.UserRepository, tokens auth.RefreshTokenRepository) *auth.Service {
	return auth.NewService(users, tokens, testSecret, time.Hour, 168*time.Hour, bcrypt.MinCost)
}

func lockUntil(t *testing.T, user *auth.User, when time.Time) {
	t.Helper()
	user.LockedUntil = &when
}

func login(t *testing.T, svc *auth.Service, password string) (*auth.Result, error) {
	t.Helper()
	return svc.Login(context.Background(), auth.LoginInput{Username: "alice", Password: password})
}

func parseRegisteredClaims(t *testing.T, raw string) *jwt.RegisteredClaims {
	t.Helper()
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims,
		func(*jwt.Token) (any, error) { return []byte(testSecret), nil },
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer("music-backend"),
	)
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("access token must verify")
	}
	return claims
}

// --- login: lockout --------------------------------------------------------

func TestLoginWrongPasswordAdvancesThrottle(t *testing.T) {
	users := &fakeUserRepository{user: newUser(t, testPassword)}
	tokens := newFakeRefreshRepository()
	svc := newService(users, tokens)

	_, err := login(t, svc, "wrong-password")

	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	if users.failedLoginCalls != 1 {
		t.Fatalf("want 1 recorded failure, got %d", users.failedLoginCalls)
	}
	if users.lastMaxAttempts != 5 {
		t.Fatalf("want threshold 5, got %d", users.lastMaxAttempts)
	}
	if users.lastLockDuration != 15*time.Minute {
		t.Fatalf("want 15m lockout, got %v", users.lastLockDuration)
	}
	if users.successCalls != 0 {
		t.Fatal("a failed login must not run the success path")
	}
	if len(tokens.creates) != 0 {
		t.Fatal("a failed login must not mint a refresh token")
	}
}

func TestLoginThresholdAttemptReturns401AndLockAppliesNext(t *testing.T) {
	users := &fakeUserRepository{user: newUser(t, testPassword)}
	// Simulate the repository: the account locks once the 5th failure is
	// recorded, exactly as RecordFailedLogin does in SQL.
	users.onFailedLogin = func() {
		if users.failedLoginCalls >= 5 && users.user.LockedUntil == nil {
			until := time.Now().Add(15 * time.Minute)
			users.user.LockedUntil = &until
		}
	}
	svc := newService(users, newFakeRefreshRepository())

	for attempt := 1; attempt <= 5; attempt++ {
		if _, err := login(t, svc, "wrong-password"); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: want ErrInvalidCredentials (threshold must not be confirmed), got %v", attempt, err)
		}
	}

	// The account is now locked: the next attempt reports it.
	if _, err := login(t, svc, "wrong-password"); !errors.Is(err, auth.ErrAccountLocked) {
		t.Fatalf("attempt 6: want ErrAccountLocked, got %v", err)
	}
}

func TestLoginLockedAccountRejectsEvenCorrectPassword(t *testing.T) {
	user := newUser(t, testPassword)
	lockUntil(t, user, time.Now().Add(10*time.Minute))
	users := &fakeUserRepository{user: user}
	tokens := newFakeRefreshRepository()
	svc := newService(users, tokens)

	_, err := login(t, svc, testPassword)

	if !errors.Is(err, auth.ErrAccountLocked) {
		t.Fatalf("want ErrAccountLocked, got %v", err)
	}
	if users.failedLoginCalls != 0 {
		t.Fatalf("a locked attempt must not change the counter, got %d writes", users.failedLoginCalls)
	}
	if users.successCalls != 0 {
		t.Fatal("a locked attempt must not clear the lock")
	}
	if len(tokens.creates) != 0 {
		t.Fatal("a locked attempt must not mint a refresh token")
	}
}

func TestLoginExpiredLockAllowsLogin(t *testing.T) {
	user := newUser(t, testPassword)
	lockUntil(t, user, time.Now().Add(-time.Second))
	users := &fakeUserRepository{user: user}
	svc := newService(users, newFakeRefreshRepository())

	if _, err := login(t, svc, testPassword); err != nil {
		t.Fatalf("an expired lock must allow login, got %v", err)
	}
	if users.successCalls != 1 {
		t.Fatalf("want success path once, got %d", users.successCalls)
	}
}

func TestLoginUnknownUserIsIndistinguishable(t *testing.T) {
	users := &fakeUserRepository{}
	tokens := newFakeRefreshRepository()
	svc := newService(users, tokens)

	_, err := login(t, svc, "whatever")

	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	if users.failedLoginCalls != 0 {
		t.Fatal("an unknown username has no row to throttle")
	}
}

func TestLoginEmptyInput(t *testing.T) {
	svc := newService(&fakeUserRepository{}, newFakeRefreshRepository())

	for _, in := range []auth.LoginInput{
		{Username: "", Password: "x"},
		{Username: "alice", Password: ""},
		{Username: "  ", Password: "x"},
	} {
		if _, err := svc.Login(context.Background(), in); !errors.Is(err, auth.ErrInvalidInput) {
			t.Fatalf("want ErrInvalidInput for %+v, got %v", in, err)
		}
	}
}

func TestLoginInactiveAccountIssuesNothing(t *testing.T) {
	user := newUser(t, testPassword)
	user.IsActive = false
	users := &fakeUserRepository{user: user}
	tokens := newFakeRefreshRepository()
	svc := newService(users, tokens)

	_, err := login(t, svc, testPassword)

	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	if users.successCalls != 0 || len(tokens.creates) != 0 {
		t.Fatal("an inactive account must neither reset state nor receive tokens")
	}
}

// --- login: token issuing --------------------------------------------------

func TestLoginIssuesHashedRefreshToken(t *testing.T) {
	users := &fakeUserRepository{user: newUser(t, testPassword)}
	tokens := newFakeRefreshRepository()
	svc := newService(users, tokens)

	res, err := svc.Login(context.Background(), auth.LoginInput{
		Username:  "ALICE ",
		Password:  testPassword,
		UserAgent: "test-agent/1.0",
		IPAddress: "203.0.113.7",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if res.Token == "" || res.RefreshToken == "" {
		t.Fatal("both tokens must be returned")
	}
	if len(tokens.creates) != 1 {
		t.Fatalf("want exactly one persisted session, got %d", len(tokens.creates))
	}

	stored := tokens.creates[0]
	if stored.TokenHash == res.RefreshToken {
		t.Fatal("the raw refresh token must never be persisted")
	}
	if want := sha256Hex(res.RefreshToken); stored.TokenHash != want {
		t.Fatalf("stored value must be sha256(raw):\n want %s\n  got %s", want, stored.TokenHash)
	}
	if len(stored.TokenHash) != 64 || strings.ToLower(stored.TokenHash) != stored.TokenHash {
		t.Fatalf("token_hash must be 64 lowercase hex chars for CHAR(64), got %q", stored.TokenHash)
	}
	// 32 random bytes -> 43 unpadded URL-safe base64 characters.
	if len(res.RefreshToken) != 43 {
		t.Fatalf("want 43-char base64 of 32 bytes, got %d (%q)", len(res.RefreshToken), res.RefreshToken)
	}
	if strings.ContainsAny(res.RefreshToken, "+/=") {
		t.Fatalf("refresh token must be URL-safe and unpadded, got %q", res.RefreshToken)
	}
	if stored.UserID != users.user.ID {
		t.Fatalf("session must belong to the user: want %d, got %d", users.user.ID, stored.UserID)
	}
	if time.Until(stored.ExpiresAt) < 167*time.Hour {
		t.Fatalf("refresh TTL should be ~168h from issue, got %v", time.Until(stored.ExpiresAt))
	}
	if stored.UserAgent == nil || *stored.UserAgent != "test-agent/1.0" {
		t.Fatalf("user agent must be recorded, got %v", stored.UserAgent)
	}
	if stored.IPAddress == nil || *stored.IPAddress != "203.0.113.7" {
		t.Fatalf("ip address must be recorded, got %v", stored.IPAddress)
	}
	if users.successCalls != 1 {
		t.Fatalf("want last_login_at reset once, got %d", users.successCalls)
	}
}

func TestLoginOmitsBlankAuditColumns(t *testing.T) {
	users := &fakeUserRepository{user: newUser(t, testPassword)}
	tokens := newFakeRefreshRepository()
	svc := newService(users, tokens)

	if _, err := svc.Login(context.Background(), auth.LoginInput{Username: "alice", Password: testPassword}); err != nil {
		t.Fatalf("login: %v", err)
	}

	stored := tokens.creates[0]
	if stored.UserAgent != nil || stored.IPAddress != nil {
		t.Fatalf("blank audit values must stay NULL, got ua=%v ip=%v", stored.UserAgent, stored.IPAddress)
	}
}

func TestLoginAccessTokensHaveUniqueJTI(t *testing.T) {
	users := &fakeUserRepository{user: newUser(t, testPassword)}
	tokens := newFakeRefreshRepository()
	svc := newService(users, tokens)

	ctx := context.Background()
	first, err := svc.Login(ctx, auth.LoginInput{Username: "alice", Password: testPassword})
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	second, err := svc.Login(ctx, auth.LoginInput{Username: "alice", Password: testPassword})
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	firstClaims := parseRegisteredClaims(t, first.Token)
	secondClaims := parseRegisteredClaims(t, second.Token)

	if firstClaims.ID == "" || secondClaims.ID == "" {
		t.Fatal("every access token must carry a jti")
	}
	if firstClaims.ID == secondClaims.ID {
		t.Fatalf("jti must be unique per token, both were %q", firstClaims.ID)
	}
	// Two tokens for the same user in the same second differ only by jti.
	if first.Token == second.Token {
		t.Fatal("tokens issued in the same second must not be byte-identical")
	}
	if firstClaims.Subject != "42" || firstClaims.Issuer != "music-backend" {
		t.Fatalf("unexpected claims: sub=%q iss=%q", firstClaims.Subject, firstClaims.Issuer)
	}
}

// --- login: fail-closed ordering ------------------------------------------

func TestLoginRefreshTokenFailureKeepsAccountLocked(t *testing.T) {
	users := &fakeUserRepository{user: newUser(t, testPassword)}
	tokens := newFakeRefreshRepository()
	tokens.createErr = errors.New("insert failed")
	svc := newService(users, tokens)

	if _, err := login(t, svc, testPassword); err == nil {
		t.Fatal("login must fail when the session cannot be persisted")
	}
	if users.successCalls != 0 {
		t.Fatal("throttle state must not be cleared when the session was never created")
	}
}

func TestLoginSuccessResetFailureIsReported(t *testing.T) {
	users := &fakeUserRepository{user: newUser(t, testPassword)}
	users.recordSuccessErr = errors.New("update failed")
	svc := newService(users, newFakeRefreshRepository())

	res, err := login(t, svc, testPassword)

	if err == nil {
		t.Fatal("a failed reset must be reported, not swallowed")
	}
	if res != nil {
		t.Fatal("no result may be returned alongside an error")
	}
}

func TestLoginFailedThrottleWriteIsReported(t *testing.T) {
	users := &fakeUserRepository{user: newUser(t, testPassword)}
	users.recordFailedErr = errors.New("update failed")
	svc := newService(users, newFakeRefreshRepository())

	_, err := login(t, svc, "wrong-password")

	if err == nil || errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("a failed throttle write must surface as a server error, got %v", err)
	}
}

// --- refresh ---------------------------------------------------------------

func TestRefreshRotatesToken(t *testing.T) {
	users := &fakeUserRepository{user: newUser(t, testPassword)}
	tokens := newFakeRefreshRepository()
	svc := newService(users, tokens)
	ctx := context.Background()

	first, err := svc.Login(ctx, auth.LoginInput{Username: "alice", Password: testPassword, UserAgent: "agent", IPAddress: "203.0.113.7"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	oldHash := sha256Hex(first.RefreshToken)

	refreshed, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: first.RefreshToken})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if refreshed.Token == "" || refreshed.RefreshToken == "" {
		t.Fatal("refresh must return both tokens")
	}
	if refreshed.RefreshToken == first.RefreshToken {
		t.Fatal("the refresh token must rotate")
	}
	if refreshed.Token == first.Token {
		t.Fatal("the access token must be reissued with a fresh jti")
	}
	if tokens.rotations != 1 {
		t.Fatalf("want exactly one rotation, got %d", tokens.rotations)
	}
	if _, stillPresent := tokens.tokens[oldHash]; stillPresent {
		t.Fatal("the presented token must be consumed")
	}
	if time.Until(refreshed.ExpiresAt) > time.Hour+time.Minute {
		t.Fatalf("access token TTL must be the configured hour, got %v", time.Until(refreshed.ExpiresAt))
	}

	newRow := tokens.tokens[sha256Hex(refreshed.RefreshToken)]
	if newRow == nil {
		t.Fatal("the rotated token must be persisted")
	}
	// Audit columns describe the session, so they carry over.
	if newRow.UserAgent == nil || *newRow.UserAgent != "agent" {
		t.Fatalf("user agent must carry over, got %v", newRow.UserAgent)
	}
	if newRow.IPAddress == nil || *newRow.IPAddress != "203.0.113.7" {
		t.Fatalf("ip address must carry over, got %v", newRow.IPAddress)
	}

	// The rotated token is the one that works now.
	if _, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: refreshed.RefreshToken}); err != nil {
		t.Fatalf("rotated token must be usable: %v", err)
	}
}

func TestRefreshRejectsReplayedToken(t *testing.T) {
	users := &fakeUserRepository{user: newUser(t, testPassword)}
	svc := newService(users, newFakeRefreshRepository())
	ctx := context.Background()

	first, err := svc.Login(ctx, auth.LoginInput{Username: "alice", Password: testPassword})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: first.RefreshToken}); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	_, err = svc.Refresh(ctx, auth.RefreshInput{RefreshToken: first.RefreshToken})
	if !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("replay must be ErrInvalidRefreshToken, got %v", err)
	}
}

func TestRefreshRejectionsAreOpaque(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown token", func(t *testing.T) {
		svc := newService(&fakeUserRepository{user: newUser(t, testPassword)}, newFakeRefreshRepository())
		if _, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: "never-issued"}); !errors.Is(err, auth.ErrInvalidRefreshToken) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("blank token", func(t *testing.T) {
		svc := newService(&fakeUserRepository{}, newFakeRefreshRepository())
		if _, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: "   "}); !errors.Is(err, auth.ErrInvalidInput) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("expired token is cleaned up", func(t *testing.T) {
		users := &fakeUserRepository{user: newUser(t, testPassword)}
		tokens := newFakeRefreshRepository()
		tokens.seed("expired-token", users.user.ID, time.Now().Add(-time.Second))
		svc := newService(users, tokens)

		if _, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: "expired-token"}); !errors.Is(err, auth.ErrInvalidRefreshToken) {
			t.Fatalf("got %v", err)
		}
		if len(tokens.deletes) != 1 {
			t.Fatalf("an expired row should be deleted, deletes=%d", len(tokens.deletes))
		}
		if tokens.rotations != 0 {
			t.Fatal("an expired token must never rotate")
		}
	})

	t.Run("deactivated user", func(t *testing.T) {
		users := &fakeUserRepository{user: newUser(t, testPassword)}
		users.user.IsActive = false
		tokens := newFakeRefreshRepository()
		tokens.seed("inactive-user-token", users.user.ID, time.Now().Add(time.Hour))
		svc := newService(users, tokens)

		if _, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: "inactive-user-token"}); !errors.Is(err, auth.ErrInvalidRefreshToken) {
			t.Fatalf("got %v", err)
		}
		if tokens.rotations != 0 {
			t.Fatal("a deactivated account must not receive a new token pair")
		}
	})

	t.Run("deleted user", func(t *testing.T) {
		tokens := newFakeRefreshRepository()
		tokens.seed("orphan-token", 999, time.Now().Add(time.Hour))
		svc := newService(&fakeUserRepository{}, tokens)

		if _, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: "orphan-token"}); !errors.Is(err, auth.ErrInvalidRefreshToken) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("lost rotation race", func(t *testing.T) {
		users := &fakeUserRepository{user: newUser(t, testPassword)}
		tokens := newFakeRefreshRepository()
		tokens.seed("racing-token", users.user.ID, time.Now().Add(time.Hour))
		tokens.rotateErr = auth.ErrRefreshTokenNotFound
		svc := newService(users, tokens)

		if _, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: "racing-token"}); !errors.Is(err, auth.ErrInvalidRefreshToken) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("unexpected repository error propagates", func(t *testing.T) {
		users := &fakeUserRepository{user: newUser(t, testPassword)}
		tokens := newFakeRefreshRepository()
		tokens.seed("token", users.user.ID, time.Now().Add(time.Hour))
		boom := errors.New("connection reset")
		tokens.rotateErr = boom
		svc := newService(users, tokens)

		_, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: "token"})
		if !errors.Is(err, boom) {
			t.Fatalf("an infrastructure error must not be masked as an invalid token, got %v", err)
		}
	})
}

// --- logout ----------------------------------------------------------------

func TestLogoutDeletesSessionAndIsIdempotent(t *testing.T) {
	users := &fakeUserRepository{user: newUser(t, testPassword)}
	tokens := newFakeRefreshRepository()
	svc := newService(users, tokens)
	ctx := context.Background()

	res, err := svc.Login(ctx, auth.LoginInput{Username: "alice", Password: testPassword})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	hash := sha256Hex(res.RefreshToken)

	if err := svc.Logout(ctx, res.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, ok := tokens.tokens[hash]; ok {
		t.Fatal("logout must delete the session")
	}
	if _, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: res.RefreshToken}); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("a used refresh token must not work after logout, got %v", err)
	}

	if err := svc.Logout(ctx, res.RefreshToken); err != nil {
		t.Fatalf("repeat logout must succeed, got %v", err)
	}
	if err := svc.Logout(ctx, "forged-token"); err != nil {
		t.Fatalf("forged logout must not be distinguishable, got %v", err)
	}
	if err := svc.Logout(ctx, "  "); !errors.Is(err, auth.ErrInvalidInput) {
		t.Fatalf("blank token must be invalid input, got %v", err)
	}
}

func TestLogoutReportsRepositoryFailure(t *testing.T) {
	tokens := newFakeRefreshRepository()
	tokens.deleteErr = errors.New("delete failed")
	svc := newService(&fakeUserRepository{}, tokens)

	if err := svc.Logout(context.Background(), "some-token"); err == nil {
		t.Fatal("an infrastructure failure must be reported")
	}
}

// --- flat-timing defense ---------------------------------------------------

// TestLoginLockedTimingMatchesWrongPassword guards the one security property
// that unit tests cannot observe directly: that a locked account does not answer
// measurably faster than a wrong password. If the bcrypt comparison were skipped
// on the locked path, the locked response would drop from roughly the cost of one
// bcrypt evaluation to microseconds.
//
// This is the one test that deliberately does not use bcrypt.MinCost: at cost 4 a
// comparison is smaller than the platform's clock granularity (it measures as 0s
// on Windows), which would make the assertion meaningless. Cost 10 keeps the
// whole test under a second.
//
// Timing tests are inherently noisy, so this compares best-of-N runs and only
// fails on a gross difference (the regression it guards against is ~1000x).
func TestLoginLockedTimingMatchesWrongPassword(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive; skipped under -short")
	}

	const (
		runs = 7
		cost = 10
	)

	newMeasuredUser := func() *auth.User {
		hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), cost)
		if err != nil {
			t.Fatalf("hash password: %v", err)
		}
		user := newUser(t, testPassword)
		user.PasswordHash = string(hash)
		return user
	}

	measure := func(name string, user *auth.User) time.Duration {
		users := &fakeUserRepository{user: user}
		svc := auth.NewService(users, newFakeRefreshRepository(), testSecret, time.Hour, 168*time.Hour, cost)

		best := time.Duration(1<<62 - 1)
		for i := 0; i < runs; i++ {
			start := time.Now()
			if _, err := login(t, svc, "not-the-password"); err == nil {
				t.Fatalf("%s: expected a failure", name)
			}
			if elapsed := time.Since(start); elapsed < best {
				best = elapsed
			}
		}
		t.Logf("%s: best of %d = %v", name, runs, best)
		return best
	}

	wrongPasswordBest := measure("wrong password", newMeasuredUser())

	lockedUser := newMeasuredUser()
	lockUntil(t, lockedUser, time.Now().Add(10*time.Minute))
	lockedBest := measure("locked account", lockedUser)

	// Sanity check: the baseline must be far above clock granularity, otherwise
	// this test would pass no matter what the locked path does.
	if wrongPasswordBest < 5*time.Millisecond {
		t.Fatalf("baseline is too fast (%v) to be meaningful: is bcrypt actually running?", wrongPasswordBest)
	}
	if lockedBest*4 < wrongPasswordBest {
		t.Fatalf("locked path is suspiciously faster than a wrong password (locked %v vs wrong %v): "+
			"the bcrypt comparison is probably being skipped, which leaks lock state by timing",
			lockedBest, wrongPasswordBest)
	}
}
