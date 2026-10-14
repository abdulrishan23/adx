// Command adx runs the ADX server: the HTTP API and the background workers in
// one process. `adx users ...` manages people from the command line instead.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/projects"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/jobs"
	"github.com/WokeWorks/adx/server/providers/authentik"
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
	// BootstrapAdmin becomes an admin at startup if ADX has no enabled admin.
	BootstrapAdmin string `env:"ADX_BOOTSTRAP_ADMIN"`

	AuthentikIssuer       string `env:"ADX_AUTHENTIK_ISSUER"`
	AuthentikAPIToken     string `env:"ADX_AUTHENTIK_API_TOKEN"`
	AuthentikGroupID      string `env:"ADX_AUTHENTIK_GROUP_ID"`
	AuthentikEmailStageID string `env:"ADX_AUTHENTIK_EMAIL_STAGE_ID"`
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

	db, err := openDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	dir, err := directory(cfg, dev)
	if err != nil {
		return err
	}
	people := users.New(db, dir)

	if len(os.Args) > 1 && os.Args[1] == "users" {
		return runUsers(ctx, people, os.Args[2:], os.Stdout)
	}

	connections, err := configuredConnections(db, cfg)
	if err != nil {
		return err
	}
	if err := jobs.Migrate(ctx, db); err != nil {
		return err
	}
	if cfg.BootstrapAdmin != "" {
		// A failure here is logged, not fatal, so ADX still starts while
		// authentik is down. The next start tries again.
		admin, changed, err := people.Bootstrap(ctx, core.System(core.ViaCLI), cfg.BootstrapAdmin)
		switch {
		case err != nil:
			log.Error("bootstrap admin failed", "email", cfg.BootstrapAdmin, "error", err)
		case changed:
			log.Info("made bootstrap admin", "email", admin.Email, "authentik_uuid", admin.AuthentikUUID)
		}
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

func openDB(ctx context.Context, cfg config) (*store.DB, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// directory connects to authentik. Outside dev all of its settings are
// required. In dev, missing settings give a directory that explains what's
// missing whenever someone is added or changed.
func directory(cfg config, dev bool) (users.Directory, error) {
	missing := []string{}
	for name, v := range map[string]string{
		"ADX_AUTHENTIK_ISSUER":         cfg.AuthentikIssuer,
		"ADX_AUTHENTIK_API_TOKEN":      cfg.AuthentikAPIToken,
		"ADX_AUTHENTIK_GROUP_ID":       cfg.AuthentikGroupID,
		"ADX_AUTHENTIK_EMAIL_STAGE_ID": cfg.AuthentikEmailStageID,
	} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		msg := "authentik isn't configured, so set " + strings.Join(missing, ", ")
		if !dev {
			return nil, errors.New(msg)
		}
		return unconfigured{core.Unavailable(nil, "%s", msg)}, nil
	}
	issuer, err := url.Parse(cfg.AuthentikIssuer)
	if err != nil || issuer.Scheme == "" || issuer.Host == "" {
		return nil, fmt.Errorf("ADX_AUTHENTIK_ISSUER %q isn't a URL", cfg.AuthentikIssuer)
	}
	// The API token goes to this address, so it must be encrypted.
	if issuer.Scheme != "https" && !dev {
		return nil, fmt.Errorf("ADX_AUTHENTIK_ISSUER %q must use https", cfg.AuthentikIssuer)
	}
	return authentik.NewDirectory(authentik.DirectoryConfig{
		BaseURL:      issuer.Scheme + "://" + issuer.Host,
		Token:        cfg.AuthentikAPIToken,
		GroupID:      cfg.AuthentikGroupID,
		EmailStageID: cfg.AuthentikEmailStageID,
	}), nil
}

type unconfigured struct{ err error }

func (u unconfigured) FindByEmail(context.Context, string) ([]users.Account, error) {
	return nil, u.err
}
func (u unconfigured) Create(context.Context, string) (users.Account, error) {
	return users.Account{}, u.err
}
func (u unconfigured) AddToGroup(context.Context, int64) error           { return u.err }
func (u unconfigured) RemoveFromGroup(context.Context, int64) error      { return u.err }
func (u unconfigured) SendSetPasswordEmail(context.Context, int64) error { return u.err }
