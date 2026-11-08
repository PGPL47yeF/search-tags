// tagsvc отдаёт поисковые теги из ClickHouse: JSON API и страницу поиска.
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

	"github.com/PGPL47yeF/search-tags/internal/config"
	"github.com/PGPL47yeF/search-tags/internal/httpapi"
	"github.com/PGPL47yeF/search-tags/internal/tags"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("service stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.FromEnv(os.LookupEnv)
	if err != nil {
		return err
	}

	db := tags.Connect(tags.ConnConfig{
		Addr:     cfg.ClickHouseAddr,
		Database: cfg.ClickHouseDB,
		User:     cfg.ClickHouseUser,
		Password: cfg.ClickHousePassword,
		Timeout:  cfg.RequestTimeout,
	})
	defer db.Close()
	store := tags.NewStore(db)

	// Не падаем, если ClickHouse ещё не готов: сервис поднимется, /healthz покажет проблему,
	// а запросы начнут проходить, как только база станет доступна.
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	if err := store.Ping(pingCtx); err != nil {
		log.Warn("clickhouse is not reachable yet", "addr", cfg.ClickHouseAddr, "error", err)
	} else {
		log.Info("clickhouse is reachable", "addr", cfg.ClickHouseAddr, "database", cfg.ClickHouseDB)
	}
	cancelPing()

	srv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           httpapi.New(store, cfg.RequestTimeout, log),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
