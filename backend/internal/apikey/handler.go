package apikey

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/httpx"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/identity"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes registers the api-key endpoints on the supplied (already protected and
// scoped) router.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/", h.Create)
	r.Get("/", h.List)
	r.Delete("/{id}", h.Revoke)
}

type createRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// keyResponse is an explicit view of an API key. Building a dedicated struct
// (rather than serializing Key) guarantees that the raw key, the hash
// and the owner user ID can never leak into a listing, even if the model grows.
type keyResponse struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// createResponse flattens keyResponse and adds the one-and-only copy of
// the raw key.
type createResponse struct {
	keyResponse
	Key string `json:"key"`
}

type listResponse struct {
	APIKeys []keyResponse `json:"api_keys"`
}

func newKeyResponse(key *Key) keyResponse {
	return keyResponse{
		ID:         key.ID,
		Name:       key.Name,
		KeyPrefix:  key.KeyPrefix,
		Scopes:     key.Scopes,
		ExpiresAt:  key.ExpiresAt,
		LastUsedAt: key.LastUsedAt,
		CreatedAt:  key.CreatedAt,
	}
}

// Create handles POST /api/v1/api-keys. The raw key is returned exactly once and
// the response is marked no-store so no cache retains it.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	var req createRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}

	result, err := h.svc.CreateAPIKey(r.Context(), userID, CreateInput{
		Name:      req.Name,
		Scopes:    req.Scopes,
		ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}

	// Set before WriteJSON, which commits the headers with WriteHeader.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", "/api/v1/api-keys/"+strconv.FormatInt(result.Key.ID, 10))

	httpx.WriteJSON(w, http.StatusCreated, createResponse{
		keyResponse: newKeyResponse(result.Key),
		Key:         result.RawKey,
	})
}

// List handles GET /api/v1/api-keys. The response contains prefixes only: the
// raw key is unrecoverable and the hash never leaves the database.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	keys, err := h.svc.ListAPIKeys(r.Context(), userID)
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}

	// Non-nil so an empty account renders {"api_keys":[]} rather than null.
	response := make([]keyResponse, 0, len(keys))
	for i := range keys {
		response = append(response, newKeyResponse(&keys[i]))
	}

	httpx.WriteJSON(w, http.StatusOK, listResponse{APIKeys: response})
}

// Revoke handles DELETE /api/v1/api-keys/{id}. A key that is unknown, owned by
// another user or already revoked is reported as 404 without distinguishing
// between those cases, which keeps key IDs from being enumerable.
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	keyID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || keyID <= 0 {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "api key not found")
		return
	}

	if err := h.svc.RevokeAPIKey(r.Context(), userID, keyID); err != nil {
		writeAPIKeyError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// authenticatedUserID reads the identity injected by the auth middleware.
// A missing value means the route was mounted without the middleware, so it is
// answered as unauthenticated rather than trusted.
func authenticatedUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := identity.UserIDFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return 0, false
	}

	return userID, true
}

// writeAPIKeyError maps API-key domain errors onto HTTP status codes. Only
// unexpected failures are logged; validation and not-found are expected.
func writeAPIKeyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		message := strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": ")
		httpx.WriteError(w, http.StatusBadRequest, "invalid_input", message)

	case errors.Is(err, ErrAPIKeyNotFound):
		// Covers unknown, foreign and already-revoked keys alike.
		httpx.WriteError(w, http.StatusNotFound, "not_found", "api key not found")

	default:
		log.Printf("handler: unexpected api key error: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
