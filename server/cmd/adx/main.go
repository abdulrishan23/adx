// Command adx runs the ADX server: the HTTP API and the background workers in
// one process. `adx backup DIR` copies the database and assets instead of
// serving; see docs/runbooks/backup-restore.md.
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
	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/creative"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/ops"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/projects"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/reports"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/jobs"
	anthropicprovider "github.com/WokeWorks/adx/server/providers/anthropic"
	"github.com/WokeWorks/adx/server/providers/authentik"
	"github.com/WokeWorks/adx/server/providers/filestore"
	"github.com/WokeWorks/adx/server/providers/smtpmail"
	"github.com/WokeWorks/adx/server/providers/svgrender"
	"github.com/WokeWorks/adx/server/store"

	// Containers often ship without time zone data, and reports need Asia/Dubai.
	_ "time/tzdata"
)

type config struct {
	// Env is "dev" only when set so explicitly. Anything else runs as
	// production, because dev hands every request an admin without sign-in.
	Env  string `env:"ADX_ENV" envDefault:"production"`
	Addr string `env:"ADX_ADDR" envDefault:"localhost:8080"`
	// DBPath and AssetsDir are the persistent state. Outside dev their
	// directories must already exist, so a missing mount stops the server
	// instead of starting it empty on a disk that goes away.
	DBPath             string `env:"ADX_DB_PATH" envDefault:"data/adx.db"`
	AssetsDir          string `env:"ADX_ASSETS_DIR" envDefault:"data/assets"`
	GHLCredentialsFile string `env:"ADX_GHL_CREDENTIALS_FILE"`
	ConnectionKeyFile  string `env:"ADX_CONNECTION_KEY_FILE"`
	// BootstrapAdmin becomes an admin at startup if ADX has no enabled admin.
	BootstrapAdmin string `env:"ADX_BOOTSTRAP_ADMIN"`

	AuthentikIssuer       string `env:"ADX_AUTHENTIK_ISSUER"`
	AuthentikAPIToken     string `env:"ADX_AUTHENTIK_API_TOKEN"`
	AuthentikGroupID      string `env:"ADX_AUTHENTIK_GROUP_ID"`
	AuthentikEmailStageID string `env:"ADX_AUTHENTIK_EMAIL_STAGE_ID"`

	// Sign-in (D-24, B0.3). PublicURL is where browsers reach ADX, so the
	// OIDC callback is PublicURL + /api/auth/callback. The group name is the
	// one authentik puts in the groups claim; GroupID above is the same group
	// by UUID for the users API. All required outside dev.
	PublicURL          string `env:"ADX_PUBLIC_URL"`
	OIDCClientID       string `env:"ADX_OIDC_CLIENT_ID"`
	OIDCClientSecret   string `env:"ADX_OIDC_CLIENT_SECRET"`
	AuthentikGroupName string `env:"ADX_AUTHENTIK_GROUP_NAME" envDefault:"adx-users"`

	// Ad platforms (B1.1). Each is optional: without its settings the
	// platform can't be connected and the screen says so. The callback is
	// ADX_PUBLIC_URL + /api/connections/{provider}/callback.
	GoogleOAuthClientID     string `env:"ADX_GOOGLE_OAUTH_CLIENT_ID"`
	GoogleOAuthClientSecret string `env:"ADX_GOOGLE_OAUTH_CLIENT_SECRET"`
	GoogleAdsLoginCustomer  string `env:"ADX_GOOGLE_ADS_LOGIN_CUSTOMER_ID"`
	MetaAppID               string `env:"ADX_META_APP_ID"`
	MetaAppSecret           string `env:"ADX_META_APP_SECRET"`
	// AdsRefreshTrailingDays is how far back the daily refresh re-reads,
	// because both platforms revise recent days (28 for Meta's windows).
	AdsRefreshTrailingDays int `env:"ADX_ADS_REFRESH_TRAILING_DAYS" envDefault:"30"`

	// Report delivery (B1.4). Optional: without a host, reports are
	// generated and shown but never sent.
	SMTPHost     string `env:"ADX_SMTP_HOST"`
	SMTPPort     int    `env:"ADX_SMTP_PORT" envDefault:"587"`
	SMTPUsername string `env:"ADX_SMTP_USERNAME"`
	SMTPPassword string `env:"ADX_SMTP_PASSWORD"`
	SMTPFrom     string `env:"ADX_SMTP_FROM"`

	// Ask ADX (B1.5). The model is configuration: provider, name, key,
	// effort, timeout, daily token budget and prices for cost accounting.
	// Without a key the assistant answers "not configured" and invents nothing.
	ModelProvider          string `env:"ADX_MODEL_PROVIDER" envDefault:"anthropic"`
	ModelAPIKey            string `env:"ADX_MODEL_API_KEY"`
	ModelName              string `env:"ADX_MODEL_NAME" envDefault:"claude-opus-5-5"`
	ModelEffort            string `env:"ADX_MODEL_EFFORT" envDefault:"medium"`
	ModelBaseURL           string `env:"ADX_MODEL_BASE_URL"`
	ModelTimeoutSeconds    int    `env:"ADX_MODEL_TIMEOUT_SECONDS" envDefault:"120"`
	AssistantDailyTokens   int64  `env:"ADX_ASSISTANT_DAILY_TOKEN_BUDGET" envDefault:"2000000"`
	ModelInputPriceMicros  int64  `env:"ADX_MODEL_INPUT_PRICE_MICROS"`
	ModelOutputPriceMicros int64  `env:"ADX_MODEL_OUTPUT_PRICE_MICROS"`
}

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "backup" {
		err = runBackup(os.Args[2:])
	} else {
		err = run()
	}
	if err != nil {
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
	if err := checkStorage(cfg, dev); err != nil {
		return err
	}

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
	log.Info("storage", "db", cfg.DBPath, "assets", cfg.AssetsDir, "env", cfg.Env)

	accounts, err := authentikAccounts(cfg, dev)
	if err != nil {
		return err
	}
	people := users.New(db, accounts)

	oidc, err := configuredOIDC(ctx, cfg, dev)
	if err != nil {
		return err
	}
	sessions := identity.New(db, people, oidc, identity.Config{AllowedGroup: cfg.AuthentikGroupName})

	connections, err := configuredConnections(db, cfg)
	if err != nil {
		return err
	}
	adPlatforms, err := configuredAdPlatforms(cfg, dev)
	if err != nil {
		return err
	}
	if connections != nil {
		connections.WithAds(adPlatforms)
	}
	if err := jobs.Migrate(ctx, db); err != nil {
		return err
	}
	if cfg.BootstrapAdmin != "" {
		// A failure here is logged, not fatal, so ADX still starts while
		// authentik is down. The next start tries again.
		admin, changed, err := people.Bootstrap(ctx, core.System(core.ViaStartup), cfg.BootstrapAdmin)
		switch {
		case err != nil:
			log.Error("bootstrap admin failed", "email", cfg.BootstrapAdmin, "error", err)
		case changed:
			log.Info("made bootstrap admin", "email", admin.Email, "authentik_uuid", admin.AuthentikUUID)
		}
	}

	// Claim the port before starting workers, so a busy port fails fast
	// instead of leaving River running against a database about to close.
	if dev {
		if err := devAddrIsLocal(cfg.Addr); err != nil {
			return err
		}
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}

	// Durable work: core/runs records it, River works it. The queue is bound
	// after the client exists because each needs the other.
	queue := &jobs.Queue{}
	runsService := runs.New(db, queue, runs.Config{})
	evidenceStore := evidence.New(db, core.SystemClock{})
	var tokens leads.Tokens
	if connections != nil {
		tokens = connections
	}
	leadsService := leads.New(db, ghlReader(connections), tokens, evidenceStore, leads.Config{})
	readers := map[string]reporting.Provider{}
	for name, p := range adPlatforms {
		readers[name] = p.Reader
	}
	var adTokens reporting.Tokens
	if connections != nil {
		adTokens = connections
	}
	reportingService := reporting.New(db, readers, adTokens, evidenceStore, reporting.Config{})
	mailer, err := configuredMailer(cfg)
	if err != nil {
		return err
	}
	reportsService := reports.New(db, reportingService, mailer, reports.Config{})
	alertsService := alerts.New(db, reportingService, alerts.Config{})
	model, err := configuredModel(cfg)
	if err != nil {
		return err
	}
	assetStore, err := filestore.New(cfg.AssetsDir)
	if err != nil {
		return err
	}
	brandService := brands.New(db, brands.Config{})
	creativeService := creative.New(db, assetStore, svgrender.New(), creative.Config{Brands: brandService})
	assistantService := assistant.New(db, model, assistant.Services{Reporting: reportingService, Leads: leadsService, Alerts: alertsService, Connections: connections},
		assistant.Config{Limits: assistant.Limits{DailyTokenBudget: cfg.AssistantDailyTokens, InputPriceMicros: cfg.ModelInputPriceMicros, OutputPriceMicros: cfg.ModelOutputPriceMicros}})
	worker, err := jobs.New(db, log, jobs.Deps{
		Runs:      runsService,
		Executors: runExecutors(leadsService, reportingService),
		Periodic: []jobs.Periodic{
			{Kind: "ads.refresh_all", Every: 24 * time.Hour, Run: func(ctx context.Context) error {
				started, errs := reportingService.RefreshAll(ctx, runsService, cfg.AdsRefreshTrailingDays)
				for _, e := range errs {
					log.WarnContext(ctx, "daily ads refresh could not start", "error", e)
				}
				log.InfoContext(ctx, "daily ads refresh", "started", started, "failed", len(errs))
				return nil
			}},
			{Kind: "reports.run_due", Every: 6 * time.Hour, Run: func(ctx context.Context) error {
				generated, errs := reportsService.RunDue(ctx)
				for _, e := range errs {
					log.WarnContext(ctx, "scheduled report failed", "error", e)
				}
				log.InfoContext(ctx, "scheduled reports", "generated", generated, "failed", len(errs))
				return nil
			}},
			{Kind: "alerts.evaluate", Every: 6 * time.Hour, Run: func(ctx context.Context) error {
				ev, err := alertsService.EvaluateAll(ctx)
				if err != nil {
					return err
				}
				log.InfoContext(ctx, "alert checks", "day", ev.Day, "evaluated", ev.Evaluated, "fired", ev.Fired, "repeated", ev.Repeated, "skipped", len(ev.Skipped))
				return nil
			}},
		},
	})
	if err != nil {
		return err
	}
	queue.Bind(worker)
	if err := worker.Client.Start(ctx); err != nil {
		return err
	}

	readiness := ops.New(log, db, ops.Check{Name: "jobs", Run: func(context.Context) error {
		select {
		case <-worker.Client.Stopped():
			return errors.New("job worker stopped")
		default:
			return nil
		}
	}})

	srv := &http.Server{
		Handler: api.NewRouter(api.Deps{
			Log:         log,
			Dev:         dev,
			Projects:    projects.New(db),
			Connections: connections,
			Ops:         readiness,
			Identity:    sessions,
			Policies:    policies.New(db),
			Runs:        runsService,
			Leads:       leadsService,
			Reporting:   reportingService,
			Reports:     reportsService,
			Alerts:      alertsService,
			Assistant:   assistantService,
			Creative:    creativeService,
			Brands:      brandService,
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
	return errors.Join(failed, srv.Shutdown(shutdownCtx), worker.Client.Stop(shutdownCtx))
}

// configuredModel builds the assistant's model, or returns nil when no key
// is set. Only the Anthropic provider exists; the name is checked so a typo
// fails at startup rather than at the first question.
func configuredModel(cfg config) (assistant.Model, error) {
	if cfg.ModelAPIKey == "" {
		slog.Info("no model API key; Ask ADX answers that it is not configured")
		return nil, nil
	}
	if cfg.ModelProvider != "anthropic" {
		return nil, fmt.Errorf("ADX_MODEL_PROVIDER %q is not supported; only anthropic is", cfg.ModelProvider)
	}
	m, err := anthropicprovider.New(anthropicprovider.Config{APIKey: cfg.ModelAPIKey, Model: cfg.ModelName, BaseURL: cfg.ModelBaseURL,
		Timeout: time.Duration(cfg.ModelTimeoutSeconds) * time.Second, Effort: cfg.ModelEffort})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// configuredMailer builds the SMTP transport, or returns nil when no host is
// set, which leaves report delivery unconfigured.
func configuredMailer(cfg config) (reports.Mailer, error) {
	if cfg.SMTPHost == "" {
		slog.Info("SMTP is not configured; reports are generated but not delivered")
		return nil, nil
	}
	if cfg.SMTPFrom == "" {
		return nil, errors.New("ADX_SMTP_FROM is required when ADX_SMTP_HOST is set")
	}
	return smtpmail.New(smtpmail.Config{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.SMTPFrom})
}

// runExecutors maps run kinds to the code that works them.
func runExecutors(l *leads.Leads, r *reporting.Reporting) map[string]jobs.Executor {
	return map[string]jobs.Executor{leads.RunKind: l, reporting.RefreshKind: r}
}

// checkStorage makes sure the persistent directories are there. Dev creates
// them, because a developer's data/ directory is throwaway. Production refuses
// to, because a missing directory means the volume isn't mounted and a fresh
// database would quietly start from nothing.
func checkStorage(cfg config, dev bool) error {
	for name, dir := range map[string]string{
		"ADX_DB_PATH": filepath.Dir(cfg.DBPath), "ADX_ASSETS_DIR": cfg.AssetsDir,
	} {
		info, err := os.Stat(dir)
		switch {
		case errors.Is(err, os.ErrNotExist) && dev:
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		case errors.Is(err, os.ErrNotExist):
			return fmt.Errorf("%s: directory %s does not exist; mount the persistent storage before starting", name, dir)
		case err != nil:
			return fmt.Errorf("%s: %w", name, err)
		case !info.IsDir():
			return fmt.Errorf("%s: %s is not a directory", name, dir)
		}
	}
	return nil
}

func openDB(ctx context.Context, cfg config) (*store.DB, error) {
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

// authentikAccounts connects to authentik. Outside dev all of its settings
// are required. In dev, missing settings give accounts that explain what's
// missing whenever someone is added or changed.
func authentikAccounts(cfg config, dev bool) (users.Accounts, error) {
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
	return authentik.NewAccounts(authentik.AccountsConfig{
		BaseURL:      issuer.Scheme + "://" + issuer.Host,
		Token:        cfg.AuthentikAPIToken,
		GroupID:      cfg.AuthentikGroupID,
		EmailStageID: cfg.AuthentikEmailStageID,
	}), nil
}

// configuredOIDC prepares sign-in. Outside dev every setting is required and
// discovery must succeed at startup. In dev, missing settings return a nil
// provider: existing sessions still resolve, StartSignIn says what's missing,
// and requests without a cookie get the dev admin.
func configuredOIDC(ctx context.Context, cfg config, dev bool) (identity.OIDC, error) {
	missing := []string{}
	for name, v := range map[string]string{
		"ADX_AUTHENTIK_ISSUER": cfg.AuthentikIssuer, "ADX_PUBLIC_URL": cfg.PublicURL,
		"ADX_OIDC_CLIENT_ID": cfg.OIDCClientID, "ADX_OIDC_CLIENT_SECRET": cfg.OIDCClientSecret,
	} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		msg := "sign-in isn't configured, so set " + strings.Join(missing, ", ")
		if !dev {
			return nil, errors.New(msg)
		}
		slog.Warn(msg + "; dev requests without a session get the dev admin")
		return nil, nil
	}
	public, err := url.Parse(cfg.PublicURL)
	if err != nil || public.Scheme == "" || public.Host == "" || public.RawQuery != "" || public.Fragment != "" {
		return nil, fmt.Errorf("ADX_PUBLIC_URL %q isn't an origin URL", cfg.PublicURL)
	}
	if public.Scheme != "https" && !dev {
		return nil, fmt.Errorf("ADX_PUBLIC_URL %q must use https", cfg.PublicURL)
	}
	provider, err := authentik.NewOIDC(ctx, authentik.OIDCConfig{
		Issuer: cfg.AuthentikIssuer, ClientID: cfg.OIDCClientID, ClientSecret: cfg.OIDCClientSecret,
		RedirectURL: strings.TrimRight(cfg.PublicURL, "/") + "/api/auth/callback",
	})
	if err != nil {
		return nil, err
	}
	return provider, nil
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

// devAddrIsLocal refuses to serve dev mode beyond this machine, since dev
// mode has no sign-in.
func devAddrIsLocal(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("ADX_ADDR %q: %w", addr, err)
	}
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return nil
	}
	return fmt.Errorf("ADX_ADDR %q: dev mode only listens on localhost, because it has no sign-in", addr)
}
