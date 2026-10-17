package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"log/slog"
	"strings"

	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/providers/gohighlevel"
	"github.com/WokeWorks/adx/server/providers/googleads"
	"github.com/WokeWorks/adx/server/providers/meta"
	"github.com/WokeWorks/adx/server/store"
)

// configuredAdPlatforms builds the Google Ads and Meta adapters that have
// settings. A platform without settings is left out, so connecting it fails
// with "not configured" instead of a half-working client. Outside dev a
// configured platform needs the https public URL for its callback.
func configuredAdPlatforms(cfg config, dev bool) (map[string]connections.AdsPlatform, error) {
	out := map[string]connections.AdsPlatform{}
	callback := func(provider string) (string, error) {
		if cfg.PublicURL == "" {
			return "", fmt.Errorf("ADX_PUBLIC_URL is required to connect %s", provider)
		}
		if !strings.HasPrefix(cfg.PublicURL, "https://") && !dev {
			return "", fmt.Errorf("ADX_PUBLIC_URL must use https to connect %s", provider)
		}
		return strings.TrimRight(cfg.PublicURL, "/") + "/api/connections/" + provider + "/callback", nil
	}
	switch {
	case cfg.GoogleOAuthClientID != "" && cfg.GoogleOAuthClientSecret != "":
		redirect, err := callback(connections.ProviderGoogleAds)
		if err != nil {
			return nil, err
		}
		id, err := googleads.NewOAuth(googleads.OAuthConfig{ClientID: cfg.GoogleOAuthClientID, ClientSecret: cfg.GoogleOAuthClientSecret, RedirectURL: redirect})
		if err != nil {
			return nil, err
		}
		out[connections.ProviderGoogleAds] = connections.AdsPlatform{Identity: id, Reader: googleads.New(googleads.Config{LoginCustomerID: cfg.GoogleAdsLoginCustomer})}
	case cfg.GoogleOAuthClientID != "" || cfg.GoogleOAuthClientSecret != "":
		return nil, errors.New("set both ADX_GOOGLE_OAUTH_CLIENT_ID and ADX_GOOGLE_OAUTH_CLIENT_SECRET, or neither")
	default:
		slog.Info("Google Ads is not configured; connecting it is disabled")
	}
	switch {
	case cfg.MetaAppID != "" && cfg.MetaAppSecret != "":
		redirect, err := callback(connections.ProviderMeta)
		if err != nil {
			return nil, err
		}
		id, err := meta.NewOAuth(meta.OAuthConfig{AppID: cfg.MetaAppID, AppSecret: cfg.MetaAppSecret, RedirectURL: redirect})
		if err != nil {
			return nil, err
		}
		out[connections.ProviderMeta] = connections.AdsPlatform{Identity: id, Reader: meta.New(meta.Config{})}
	case cfg.MetaAppID != "" || cfg.MetaAppSecret != "":
		return nil, errors.New("set both ADX_META_APP_ID and ADX_META_APP_SECRET, or neither")
	default:
		slog.Info("Meta is not configured; connecting it is disabled")
	}
	return out, nil
}

// ghlClient is kept so the same provider serves connection checks and lead
// imports.
var ghlClient *gohighlevel.Client

func configuredConnections(db *store.DB, cfg config) (*connections.Connections, error) {
	if cfg.GHLCredentialsFile == "" && cfg.ConnectionKeyFile == "" {
		return nil, nil
	}
	provider, err := configuredGHL(cfg.GHLCredentialsFile)
	if err != nil {
		return nil, err
	}
	key, err := readSecretFile(cfg.ConnectionKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read connection encryption key: %w", err)
	}
	ghlClient = provider
	return connections.New(db, provider, connections.Config{EncryptionKey: key})
}

// ghlReader is the lead importer's CRM, or nil when connections aren't
// configured, which makes imports fail with a clear message.
func ghlReader(c *connections.Connections) leads.CRM {
	if c == nil || ghlClient == nil {
		return nil
	}
	return ghlClient
}

func configuredGHL(credentialsFile string) (*gohighlevel.Client, error) {
	if credentialsFile == "" {
		return gohighlevel.NewReader(), nil
	}
	raw, err := readSecretFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read GoHighLevel credentials: %w", err)
	}
	var credentials struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RedirectURI  string `json:"redirect_uri"`
		VersionID    string `json:"version_id"`
	}
	if err := json.Unmarshal(raw, &credentials); err != nil {
		return nil, errors.New("GoHighLevel credential file is invalid")
	}
	return gohighlevel.New(gohighlevel.Config{
		ClientID: credentials.ClientID, ClientSecret: credentials.ClientSecret,
		RedirectURI: credentials.RedirectURI, VersionID: credentials.VersionID,
	})
}

func readSecretFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("secret file must be a regular file readable only by its owner")
	}
	return os.ReadFile(path)
}
