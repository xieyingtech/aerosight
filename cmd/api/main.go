package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"aerosight/internal/auth"
	apphttp "aerosight/internal/http"
	"aerosight/internal/store"
)

func main() {
	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		sessionSecret = os.Getenv("NUXT_SESSION_PASSWORD")
	}
	if len(sessionSecret) < 32 {
		log.Fatal("SESSION_SECRET must be at least 32 characters")
	}

	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	if err := db.EnsureDefaultAdmin(ctx); err != nil {
		log.Fatalf("ensure default admin: %v", err)
	}

	server := &http.Server{
		Addr:         env("API_ADDR", ":8080"),
		Handler:      apphttp.NewRouter(db, auth.NewManager([]byte(sessionSecret))),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("AeroSight API listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
