package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/apikey"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/auth"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/config"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/postgres"
)

func Run(cfg *config.Config) {
	ctx := context.Background()
	dbPool := postgres.NewPool(ctx, cfg.DBUrl)
	defer dbPool.Close()

	var userRepo auth.UserRepository = auth.NewUserRepository(dbPool)
	var refreshRepo auth.RefreshTokenRepository = auth.NewRefreshTokenRepository(dbPool)
	var apiKeyRepo apikey.Repository = apikey.NewRepository(dbPool)

	authService := auth.NewService(userRepo, refreshRepo, cfg.JWTSecret, cfg.JWTTTL, cfg.RefreshTokenTTL, cfg.BCryptCost)
	authHandler := auth.NewHandler(authService)
	apiKeyService := apikey.NewService(apiKeyRepo)
	apiKeyHandler := apikey.NewHandler(apiKeyService)

	r := NewRouter(cfg, authHandler, apiKeyHandler)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("Starting server on %s", srv.Addr)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed to start: %v", err)
		}
	case sig := <-sigCh:
		log.Printf("Received signal %s, shutting down", sig)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown failed: %v", err)
			_ = srv.Close()
		}
	}
}
