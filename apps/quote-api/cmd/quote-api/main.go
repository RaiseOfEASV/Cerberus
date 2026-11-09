// Command quote-api serves a random quote over HTTP.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/quote-api/internal/server"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	port := getenv("PORT", "8080")
	allowedOrigin := getenv("ALLOWED_ORIGIN", "*")

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           server.New(allowedOrigin),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("quote-api listening on :%s (allowed origin: %s)", port, allowedOrigin)
	if err := srv.ListenAndServe(); err != nil {
		log.Printf("server error: %v", err)
		os.Exit(1)
	}
}
