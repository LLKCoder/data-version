package main

import (
	"log"
	"net/http"
	"time"

	"data-vision/backend/internal/config"
	"data-vision/backend/internal/database"
	api "data-vision/backend/internal/http"
)

func main() {
	cfg := config.Load()

	db, err := database.Open(cfg.DSN)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	if err := database.MigrateAndSeed(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewRouter(db, cfg),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("data vision api listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}
