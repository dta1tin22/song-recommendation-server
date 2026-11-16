package main

import (
	"log/slog"
	"os"

	"github.com/dta1tin22/song-recommendation-server/backend/internal/app"
)

func main() {
	logger := slog.New(
		slog.NewJSONHandler(os.Stdout, nil),
	)
	slog.SetDefault(logger)

	application := app.New()

	if err := application.Run(); err != nil {
		slog.Error("Application failed", "error", err)
		os.Exit(1)
	}
}
