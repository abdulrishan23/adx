// Command adx runs the ADX server: the HTTP API and the background workers in
// one process.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core/projects"
	"github.com/WokeWorks/adx/server/jobs"
	"github.com/WokeWorks/adx/server/store"

	// Containers often ship without time zone data, and reports need Asia/Dubai.
	_ "time/tzdata"
)

type config struct {
	Env                string `env:"ADX_ENV" envDefault:"dev"`
	Addr               string `env:"ADX_ADDR" envDefault:"localhost:8080"`
	DBPath             string `env:"ADX_DB_PATH" envDefault:"data/adx.db"`
	GHLCredentialsFile string `env:"ADX_GHL_CREDENTIALS_FILE"`
	ConnectionKeyFile  string `env:"ADX_CONNECTION_KEY_FILE"`
}

func main() {
	if err := run(); err != nil {
		slog.Error("adx stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := env.ParseAs[config]()
	if err != nil {
		return err
	}
	dev := cfg.Env == "dev"

	var log *slog.Logger
	if dev {
		log = slog.New(slog.NewTextHandler(os.Stdout, nil))
	} else {
		log = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}
	connections, err := configuredConnections(db, cfg)
	if err != nil {
		return err
	}
	if err := jobs.Migrate(ctx, db); err != nil {
		return err
	}

	// Claim the port before starting workers, so a busy port fails fast
	// instead of leaving River running against a database about to close.
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}

	worker, err := jobs.New(db, log)
	if err != nil {
		return err
	}
	if err := worker.Start(ctx); err != nil {
		return err
	}

	srv := &http.Server{
		Handler: api.NewRouter(api.Deps{
			Log:         log,
			Dev:         dev,
			Projects:    projects.New(db),
			Connections: connections,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr, "env", cfg.Env)
		serveErr <- srv.Serve(ln)
	}()

	var failed error
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			failed = err
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return errors.Join(failed, srv.Shutdown(shutdownCtx), worker.Stop(shutdownCtx))
}
