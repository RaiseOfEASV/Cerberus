// Command vuln-api serves the VulnMan API. `vuln-api migrate` applies the
// database migrations (run as an initContainer with the owner role).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/config"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/db"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log, os.Args[1:]); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	if len(args) > 0 && args[0] == "migrate" {
		url, err := config.LoadMigrate(os.Getenv)
		if err != nil {
			return err
		}
		pool, err := db.Open(ctx, url)
		if err != nil {
			return err
		}
		defer pool.Close()
		applied, err := db.Migrate(ctx, pool)
		log.Info("migrations applied", "applied", applied)
		return err
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Info("vuln-api listening", "port", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
