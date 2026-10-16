// Package userstest gives tests an in-memory authentik.
package userstest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/WokeWorks/adx/server/core/users"
)

// ErrDown is what a failing Authentik call returns.
var ErrDown = errors.New("authentik is down")

type Account struct {
	users.Account
	Email   string
	InGroup bool
	// Invites counts set-password emails sent.
	Invites int
}

// Authentik is a fake authentik. Set Fail to make one method fail, by name,
// such as "SendSetPasswordEmail".
type Authentik struct {
	mu       sync.Mutex
	Accounts []*Account
	Fail     map[string]bool
	Created  int
}

func NewAuthentik() *Authentik {
	return &Authentik{Fail: map[string]bool{}}
}

// Existing adds an account that was there before ADX, so ADX didn't create
// it, and returns its UUID.
func (d *Authentik) Existing(email string, active bool) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	pk := int64(len(d.Accounts) + 1)
	a := &Account{Account: users.Account{PK: pk, UUID: fmt.Sprintf("uuid-%d", pk), Username: email, Email: email, Active: active}, Email: email}
	d.Accounts = append(d.Accounts, a)
	return a.UUID
}

// ByEmail returns the first account with this email, or nil.
func (d *Authentik) ByEmail(email string) *Account {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, a := range d.Accounts {
		if strings.EqualFold(a.Email, email) {
			return a
		}
	}
	return nil
}

func (d *Authentik) failing(method string) error {
	if d.Fail[method] {
		return ErrDown
	}
	return nil
}

func (d *Authentik) byPK(pk int64) (*Account, error) {
	for _, a := range d.Accounts {
		if a.PK == pk {
			return a, nil
		}
	}
	return nil, fmt.Errorf("no account %d", pk)
}

func (d *Authentik) FindByEmail(_ context.Context, email string) ([]users.Account, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.failing("FindByEmail"); err != nil {
		return nil, err
	}
	var out []users.Account
	for _, a := range d.Accounts {
		if strings.EqualFold(a.Email, email) {
			out = append(out, a.Account)
		}
	}
	return out, nil
}

func (d *Authentik) Create(_ context.Context, email string) (users.Account, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.failing("Create"); err != nil {
		return users.Account{}, err
	}
	pk := int64(len(d.Accounts) + 1)
	a := &Account{Account: users.Account{PK: pk, UUID: fmt.Sprintf("uuid-%d", pk), Username: email, Email: email, Active: true, CreatedByADX: true}, Email: email}
	d.Accounts = append(d.Accounts, a)
	d.Created++
	return a.Account, nil
}

func (d *Authentik) AddToGroup(_ context.Context, pk int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.failing("AddToGroup"); err != nil {
		return err
	}
	a, err := d.byPK(pk)
	if err != nil {
		return err
	}
	a.InGroup = true
	return nil
}

func (d *Authentik) RemoveFromGroup(_ context.Context, pk int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.failing("RemoveFromGroup"); err != nil {
		return err
	}
	a, err := d.byPK(pk)
	if err != nil {
		return err
	}
	a.InGroup = false
	return nil
}

func (d *Authentik) SendSetPasswordEmail(_ context.Context, pk int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.failing("SendSetPasswordEmail"); err != nil {
		return err
	}
	a, err := d.byPK(pk)
	if err != nil {
		return err
	}
	a.Invites++
	return nil
}
