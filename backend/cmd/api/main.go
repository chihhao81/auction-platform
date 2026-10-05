package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"auction-toy/backend/internal/httpapi"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	addr := envOr("HTTP_ADDR", ":"+envOr("PORT", "8080"))
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	config := httpapi.ConfigFromEnv()
	if err := config.Validate(); err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("connect to PostgreSQL: %v", err)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewRouterWithConfig(db, config),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("API listening on %s", addr)
	log.Fatal(server.ListenAndServe())
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
