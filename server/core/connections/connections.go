// Package connections owns what ADX uses to reach outside providers. A
// credential is what grants access, such as a GoHighLevel private token or an
// OAuth grant. An account is something ADX reads through a credential, such as
// a GoHighLevel location. One Google or Meta sign-in reaches many accounts, so
// the two are stored apart, and each account has one row whichever credential
// reaches it. Connections belong to the workspace, and only admins change
// them. Provider API calls stay in providers.
package connections

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// keyVersion marks secrets sealed with Config.EncryptionKey. Rotating the key
// means adding a version and resealing every credential.
const keyVersion = 1

const (
	kindOAuth        = "oauth"
	kindPrivateToken = "private_token"
)

type Config struct {
	EncryptionKey []byte
	Now           func() time.Time
}

type Connections struct {
	db   *store.DB
	ghl  GHL
	aead cipher.AEAD
	now  func() time.Time
	// Serializes reconnects and rotating refresh tokens in this single-server app.
	credentials sync.Mutex
}

func New(db *store.DB, ghl GHL, cfg Config) (*Connections, error) {
	if len(cfg.EncryptionKey) != 32 {
		return nil, errors.New("connection encryption key must contain 32 bytes")
	}
	if ghl == nil {
		return nil, errors.New("GHL provider is required")
	}
	block, err := aes.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Connections{db: db, ghl: ghl, aead: aead, now: cfg.Now}, nil
}

// save writes a credential and the account it reaches together. Reconnecting
// replaces the credential's secret and keeps the account's first connection
// time.
func (c *Connections) save(ctx context.Context, actor core.Actor,
	cred store.UpsertCredentialParams, account store.UpsertAccountParams,
) (store.Account, store.Credential, error) {
	now := c.now().UTC()
	cred.KeyVersion = keyVersion
	cred.ConnectedBy = sql.NullInt64{Int64: actor.UserID, Valid: actor.UserID != 0}
	cred.ConnectedAt = now
	account.ConnectedAt, account.VerifiedAt = now, now
	var savedAccount store.Account
	var savedCred store.Credential
	err := c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		if savedCred, err = q.UpsertCredential(ctx, cred); err != nil {
			return err
		}
		account.CredentialID = savedCred.ID
		savedAccount, err = q.UpsertAccount(ctx, account)
		return err
	})
	return savedAccount, savedCred, err
}

// seal binds a secret to its credential's provider and subject, so a secret
// copied onto another row won't decrypt.
func (c *Connections) seal(provider, subject string, secret any) ([]byte, error) {
	raw, err := json.Marshal(secret)
	if err != nil {
		return nil, err
	}
	return c.aead.Seal(nil, nil, raw, []byte(provider+":"+subject)), nil
}

func (c *Connections) open(cred store.Credential, secret any) error {
	if cred.KeyVersion != keyVersion {
		return fmt.Errorf("credential %d was sealed with unknown key version %d", cred.ID, cred.KeyVersion)
	}
	raw, err := c.aead.Open(nil, nil, cred.EncryptedSecret, []byte(cred.Provider+":"+cred.Subject))
	if err != nil {
		return errors.New("cannot decrypt connection credentials")
	}
	if err := json.Unmarshal(raw, secret); err != nil {
		return fmt.Errorf("decode connection credentials: %w", err)
	}
	return nil
}

func validID(id string) bool {
	if id == "" || len(id) > 100 {
		return false
	}
	for _, ch := range id {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' {
			continue
		}
		return false
	}
	return true
}

func hash(s string) string {
	digest := sha256.Sum256([]byte(s))
	return hex.EncodeToString(digest[:])
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
