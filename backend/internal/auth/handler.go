package auth

import (
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes registers the auth endpoints on the supplied (already scoped) router.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	r.Post("/refresh", h.Refresh)
	r.Post("/logout", h.Logout)
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// refreshRequest is shared by /refresh and /logout: both authenticate the caller
// solely by possession of a refresh token.
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// userResponse is an explicit view of a user. Building a dedicated struct (rather
// than serializing User) guarantees a new database column can never leak.
type userResponse struct {
	ID            int64     `json:"id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	Role          string    `json:"role"`
	IsActive      bool      `json:"is_active"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
}

type loginResponse struct {
	Token            string       `json:"token"`
	ExpiresAt        time.Time    `json:"expires_at"`
	RefreshToken     string       `json:"refresh_token"`
	RefreshExpiresAt time.Time    `json:"refresh_token_expires_at"`
	User             userResponse `json:"user"`
}

// refreshResponse deliberately omits the user: a refresh only rotates
// credentials, and re-serialising the account on every rotation would be extra
// work for no client benefit.
type refreshResponse struct {
	Token            string    `json:"token"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_token_expires_at"`
}

func newUserResponse(user *User) userResponse {
	return userResponse{
		ID:            user.ID,
		Username:      user.Username,
		Email:         user.Email,
		Role:          user.Role,
		IsActive:      user.IsActive,
		EmailVerified: user.EmailVerified,
		CreatedAt:     user.CreatedAt,
	}
}

// Register handles POST /register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}

	user, err := h.svc.Register(r.Context(), RegisterInput{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		writeAuthError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, newUserResponse(user))
}

// Login handles POST /login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}

	result, err := h.svc.Login(r.Context(), LoginInput{
		Username:  req.Username,
		Password:  req.Password,
		UserAgent: r.UserAgent(),
		IPAddress: clientIP(r),
	})
	if err != nil {
		writeAuthError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, loginResponse{
		Token:            result.Token,
		ExpiresAt:        result.ExpiresAt,
		RefreshToken:     result.RefreshToken,
		RefreshExpiresAt: result.RefreshExpiresAt,
		User:             newUserResponse(result.User),
	})
}

// Refresh handles POST /refresh. The presented refresh token is consumed and
// replaced, so the response's refresh_token must overwrite the client's copy.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}

	result, err := h.svc.Refresh(r.Context(), RefreshInput{RefreshToken: req.RefreshToken})
	if err != nil {
		writeAuthError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, refreshResponse{
		Token:            result.Token,
		ExpiresAt:        result.ExpiresAt,
		RefreshToken:     result.RefreshToken,
		RefreshExpiresAt: result.RefreshExpiresAt,
	})
}

// Logout handles POST /logout. It returns 204 with no body on success, including
// when the token was already gone: logout is idempotent and never reveals
// whether a token existed.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}

	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		writeAuthError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// clientIP extracts the peer address for the refresh-token audit column. No
// ProxyHeaders/RealIP middleware is mounted, so RemoteAddr is the real peer and
// trusting X-Forwarded-For here would let clients forge the recorded address.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// Unparseable address: record nothing rather than a misleading value.
		return ""
	}
	return host
}

// writeAuthError maps domain errors onto HTTP status codes. Only truly
// unexpected failures are logged; validation and auth failures are expected.
func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		// Strip the sentinel prefix so the client sees just the reason.
		message := strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": ")
		httpx.WriteError(w, http.StatusBadRequest, "invalid_input", message)

	case errors.Is(err, ErrInvalidCredentials):
		// Deliberately identical for unknown user, wrong password and inactive account.
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")

	case errors.Is(err, ErrAccountLocked):
		// No attempt count and no unlock time: neither is needed by legitimate
		// clients and both would help an attacker time the window.
		httpx.WriteError(w, http.StatusLocked, "account_locked", "account is temporarily locked, try again later")

	case errors.Is(err, ErrInvalidRefreshToken):
		// One response for unknown, expired, revoked and already-rotated tokens.
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", "invalid or expired refresh token")

	case errors.Is(err, ErrUsernameTaken):
		httpx.WriteError(w, http.StatusConflict, "username_taken", "username is already taken")

	case errors.Is(err, ErrEmailTaken):
		httpx.WriteError(w, http.StatusConflict, "email_taken", "email is already registered")

	default:
		log.Printf("handler: unexpected auth error: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
