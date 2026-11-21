package main

import (
	"github.com/dta1tin22/song-recommendation-server/backend/internal/app"
	"github.com/dta1tin22/song-recommendation-server/backend/internal/platform/config"
)

func main() {
	cfg := config.LoadConfig()
	app.Run(cfg)
}
