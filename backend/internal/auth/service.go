package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	tokenIssuer = "music-backend"

	minUsernameLen = 3
	maxUsernameLen = 100 // users.username is VARCHAR(100)

	maxEmailLen = 255 // users.email is VARCHAR(255)

	minPasswordLen = 8
	maxPasswordLen = 72 // bcrypt only considers the first 72 bytes; reject, never truncate

	// refresh_tokens.user_agent and .ip_address column widths. Longer values are
	// truncated instead of failing the insert: the audit columns are informative,
	// never authoritative.
	maxUserAgentLen = 255
	maxIPAddressLen = 45
)

type RegisterInput struct {
	Username string
	Email    string
	Password string
}

type LoginInput struct {
	Username string
	Password string

	// UserAgent and IPAddress are recorded on the issued refresh token for audit.
	// Both are optional; empty values are stored as NULL.
	UserAgent string
	IPAddress string
}

type Result struct {
	Token            string
	ExpiresAt        time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	User             *User
}

type RefreshInput struct {
	RefreshToken string
}

// RefreshResult carries the rotated pair. The presented refresh token is spent
// by a successful refresh; clients must replace their stored copy with
// RefreshToken or they will be logged out on the next attempt.
type RefreshResult struct {
	Token            string
	ExpiresAt        time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// Claims is the JWT payload. The auth middleware will parse tokens back
// into this type, so keep the fields stable.
type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type Service struct {
	repo       UserRepository
	refresh    RefreshTokenRepository
	jwtSecret  []byte
	tokenTTL   time.Duration
	refreshTTL time.Duration
	bcryptCost int

	// dummyHash is compared against when a username does not exist, so a failed
	// login costs roughly the same whether or not the account is real.
	dummyHash []byte
}

func NewService(repo UserRepository, refresh RefreshTokenRepository, jwtSecret string, tokenTTL, refreshTTL time.Duration, bcryptCost int) *Service {
	// Cost is validated by config.LoadConfig, so this cannot realistically fail.
	dummyHash, err := bcrypt.GenerateFromPassword([]byte("timing-equalization-dummy"), bcryptCost)
	if err != nil {
		dummyHash = nil
	}

	return &Service{
		repo:       repo,
		refresh:    refresh,
		jwtSecret:  []byte(jwtSecret),
		tokenTTL:   tokenTTL,
		refreshTTL: refreshTTL,
		bcryptCost: bcryptCost,
		dummyHash:  dummyHash,
	}
}

// Register validates the input, hashes the password and persists a new user
// with the 'user' role. Role is never taken from the request.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*User, error) {
	username := normalizeUsername(in.Username)
	email := strings.ToLower(strings.TrimSpace(in.Email))
	password := in.Password // never trimmed: spaces are significant

	if err := validateUsername(username); err != nil {
		return nil, err
	}
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Role:         "user", // hardcoded: never accept a role from the client
		IsActive:     true,
	}

	// Duplicate username/email surface as ErrUsernameTaken/ErrEmailTaken.
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Login verifies credentials, applies the brute-force throttle and issues an
// access token plus a refresh token. Password and existence failures share
// ErrInvalidCredentials; only a locked account is reported distinctly.
func (s *Service) Login(ctx context.Context, in LoginInput) (*Result, error) {
	username := normalizeUsername(in.Username)
	if username == "" || in.Password == "" {
		return nil, fmt.Errorf("%w: username and password are required", ErrInvalidInput)
	}

	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			s.compareDummy(in.Password)
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// The lock is checked before the password is judged, so a locked account
	// cannot be used as an oracle for guessing the password. The bcrypt work still
	// happens, against the *real* hash, so a 423 costs the same as a 401; using
	// dummyHash here would make the locked path measurably cheaper.
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		_ = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password))
		return nil, ErrAccountLocked
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		// The attempt that reaches the threshold is still reported as a plain
		// credential failure: telling the client "you just got locked" would
		// confirm the threshold was hit. The next attempt sees ErrAccountLocked.
		if repoErr := s.repo.RecordFailedLogin(ctx, user.ID, lockoutMaxFailedLogins, lockoutDuration); repoErr != nil {
			return nil, fmt.Errorf("record failed login: %w", repoErr)
		}
		return nil, ErrInvalidCredentials
	}

	// An inactive account must not reach the success path: that would clear the
	// throttle and mint tokens. Its *failed* attempts still count, which is
	// harmless and keeps this branch's timing identical to a live account.
	if !user.IsActive {
		return nil, ErrInvalidCredentials
	}

	accessToken, expiresAt, err := s.generateToken(user)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	// Issue the refresh token before clearing the lockout state. If the insert
	// fails, the login fails and the account stays locked: fail closed rather than
	// report success for a session that does not exist.
	refreshToken, refreshExpiresAt, err := s.issueRefreshToken(ctx, user.ID, in.UserAgent, in.IPAddress)
	if err != nil {
		return nil, err
	}

	if err := s.repo.RecordSuccessfulLogin(ctx, user.ID); err != nil {
		return nil, err
	}

	return &Result{
		Token:            accessToken,
		ExpiresAt:        expiresAt,
		RefreshToken:     refreshToken,
		RefreshExpiresAt: refreshExpiresAt,
		User:             user,
	}, nil
}

// Refresh exchanges a valid refresh token for a new access token and a rotated
// refresh token. The presented token is consumed as part of the rotation, so a
// replay of it fails.
func (s *Service) Refresh(ctx context.Context, in RefreshInput) (*RefreshResult, error) {
	raw := strings.TrimSpace(in.RefreshToken)
	if raw == "" {
		return nil, fmt.Errorf("%w: refresh token is required", ErrInvalidInput)
	}

	hash := hashRefreshToken(raw)

	stored, err := s.refresh.GetByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, err
	}

	if !stored.ExpiresAt.After(time.Now()) {
		// An expired row can never be used again; drop it rather than leave dead
		// sessions behind. A failure here is not worth failing the request over.
		_ = s.refresh.DeleteByHash(ctx, hash)
		return nil, ErrInvalidRefreshToken
	}

	// Re-read the user: the account may have been deactivated (or deleted) since
	// the refresh token was issued, and claims must reflect current state.
	user, err := s.repo.GetUserByID(ctx, stored.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, err
	}
	if !user.IsActive {
		return nil, ErrInvalidRefreshToken
	}

	accessToken, expiresAt, err := s.generateToken(user)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	newRaw, newHash, err := generateRefreshToken()
	if err != nil {
		return nil, err
	}
	refreshExpiresAt := time.Now().Add(s.refreshTTL)

	// The audit columns describe the session, not the individual token, so they
	// carry over from the token being rotated.
	newToken := &RefreshToken{
		UserID:    user.ID,
		TokenHash: newHash,
		ExpiresAt: refreshExpiresAt,
		UserAgent: stored.UserAgent,
		IPAddress: stored.IPAddress,
	}

	if err := s.refresh.Rotate(ctx, hash, newToken); err != nil {
		// ErrRefreshTokenNotFound here means another request consumed this token
		// between our read and the rotation, i.e. a replay or a lost race.
		if errors.Is(err, ErrRefreshTokenNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, err
	}

	return &RefreshResult{
		Token:            accessToken,
		ExpiresAt:        expiresAt,
		RefreshToken:     newRaw,
		RefreshExpiresAt: refreshExpiresAt,
	}, nil
}

// Logout deletes the session identified by the supplied refresh token. It is
// intentionally idempotent and silent about whether the token existed: logging
// out twice, or logging out with a forged token, is not an error worth
// distinguishing to the client.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	raw := strings.TrimSpace(refreshToken)
	if raw == "" {
		return fmt.Errorf("%w: refresh token is required", ErrInvalidInput)
	}

	return s.refresh.DeleteByHash(ctx, hashRefreshToken(raw))
}

// issueRefreshToken creates a session row and returns the raw token for the
// client. Only the hash is persisted.
func (s *Service) issueRefreshToken(ctx context.Context, userID int64, userAgent, ipAddress string) (string, time.Time, error) {
	raw, hash, err := generateRefreshToken()
	if err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().Add(s.refreshTTL)
	token := &RefreshToken{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
		UserAgent: nullableString(userAgent, maxUserAgentLen),
		IPAddress: nullableString(ipAddress, maxIPAddressLen),
	}

	if err := s.refresh.Create(ctx, token); err != nil {
		return "", time.Time{}, err
	}

	return raw, expiresAt, nil
}

// generateToken issues an HS256 access token with the user ID as the subject and
// a random JWT ID. The jti makes every issued token unique: without it, two
// tokens minted for the same user within the same second share every claim
// (jwt/v5 timestamps have second precision) and therefore sign to identical
// bytes, which would make per-token revocation and audit correlation impossible.
func (s *Service) generateToken(user *User) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(s.tokenTTL)

	jti, err := newJWTID()
	if err != nil {
		return "", time.Time{}, err
	}

	claims := Claims{
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(user.ID, 10),
			Issuer:    tokenIssuer,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", time.Time{}, err
	}

	return signed, expiresAt, nil
}

// newJWTID returns a random RFC 4122 version 4 UUID. crypto/rand gives 122 bits
// of entropy, so collision is not a practical concern for a token identifier.
func newJWTID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate jwt id: %w", err)
	}

	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// compareDummy burns the same bcrypt work as a real comparison without
// revealing whether the username exists.
func (s *Service) compareDummy(password string) {
	if s.dummyHash == nil {
		return
	}
	_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// nullableString converts an optional audit value into the pointer form the model
// uses for nullable columns. Blank becomes NULL, and overlong values are cut to
// the column width without leaving an invalid UTF-8 tail.
func nullableString(value string, maxLen int) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > maxLen {
		value = strings.ToValidUTF8(value[:maxLen], "")
	}
	return &value
}

func validateUsername(username string) error {
	if n := utf8.RuneCountInString(username); n < minUsernameLen || n > maxUsernameLen {
		return fmt.Errorf("%w: username must be between %d and %d characters", ErrInvalidInput, minUsernameLen, maxUsernameLen)
	}
	for _, r := range username {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' {
			return fmt.Errorf("%w: username may only contain letters, digits, '_', '-' and '.'", ErrInvalidInput)
		}
	}
	return nil
}

func validateEmail(email string) error {
	if email == "" || len(email) > maxEmailLen {
		return fmt.Errorf("%w: email is not a valid address", ErrInvalidInput)
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return fmt.Errorf("%w: email is not a valid address", ErrInvalidInput)
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < minPasswordLen {
		return fmt.Errorf("%w: password must be at least %d characters", ErrInvalidInput, minPasswordLen)
	}
	if len(password) > maxPasswordLen {
		return fmt.Errorf("%w: password must be at most %d bytes", ErrInvalidInput, maxPasswordLen)
	}
	return nil
}
