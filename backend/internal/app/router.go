package app

import (
	"net/http"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/apikey"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/auth"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/config"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(cfg *config.Config, authHandler *auth.Handler, apiKeyHandler *apikey.Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "resource not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			authHandler.Routes(r)
		})

		r.Route("/api-keys", func(r chi.Router) {
			r.Use(auth.RequireAuth([]byte(cfg.JWTSecret)))
			apiKeyHandler.Routes(r)
		})
	})

	return r
}
