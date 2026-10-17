package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/providers/gohighlevel"
	"github.com/WokeWorks/adx/server/store"
)

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
