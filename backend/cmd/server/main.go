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

	"github.com/jackc/pgx/v5/pgxpool"
	"sweetnet/internal/config"
	"sweetnet/internal/httpapi"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "reason", err.Error())
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pc, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	pc.MaxConns = 12
	pc.ConnConfig.ConnectTimeout = 5 * time.Second
	db, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return errors.New("database configuration failed")
	}
	defer db.Close()
	if err = db.Ping(ctx); err != nil {
		return errors.New("database unavailable")
	}
	lock, err := db.Acquire(ctx)
	if err != nil {
		return errors.New("cannot acquire maintenance lock")
	}
	defer lock.Release()
	if _, err = lock.Exec(ctx, "SELECT pg_advisory_lock_shared($1)", httpapi.MaintenanceLock); err != nil {
		return errors.New("maintenance lock failed")
	}
	defer lock.Exec(context.Background(), "SELECT pg_advisory_unlock_shared($1)", httpapi.MaintenanceLock)
	app, err := httpapi.New(db, cfg)
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: cfg.Address, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	done := make(chan error, 1)
	go func() { slog.Info("SweetNet listening", "address", cfg.Address); done <- srv.ListenAndServe() }()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			return err
		}
	}
	return nil
}
