package smtpmail

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/reports"
)

// Messenger sends outreach email drafts through the same SMTP relay the
// reports use. It serves the "email" channel only. The provider reference is
// the Message-ID ADX put on the mail, derived from the draft's idempotency
// key, so a retried send is recognisable at the receiving end too.
type Messenger struct {
	mailer *Mailer
	// Subject is the fixed subject line for outreach mail; the body is the
	// approved draft. Empty uses a plain default.
	Subject string
}

func NewMessenger(m *Mailer, subject string) (*Messenger, error) {
	if m == nil {
		return nil, errors.New("smtp messenger needs a configured mailer")
	}
	if strings.TrimSpace(subject) == "" {
		subject = "A note from your sales team"
	}
	if strings.ContainsAny(subject, "\r\n") {
		return nil, errors.New("subject contains a line break")
	}
	return &Messenger{mailer: m, Subject: subject}, nil
}

func (m *Messenger) Name() string       { return "smtp" }
func (m *Messenger) Channels() []string { return []string{"email"} }

// MessageID is the Message-ID header for a draft key at a sender domain.
func MessageID(key, from string) string {
	domain := "adx.invalid"
	if addr, err := mail.ParseAddress(from); err == nil {
		if at := strings.LastIndex(addr.Address, "@"); at >= 0 {
			domain = addr.Address[at+1:]
		}
	}
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			return r
		}
		return '-'
	}, key)
	return fmt.Sprintf("<%s@%s>", safe, domain)
}

func (m *Messenger) Send(ctx context.Context, msg outreach.OutboundMessage) (string, error) {
	if msg.Channel != "email" {
		return "", fmt.Errorf("smtp adapter does not serve channel %q", msg.Channel)
	}
	if _, err := mail.ParseAddress(msg.To); err != nil {
		return "", fmt.Errorf("recipient %q is invalid", msg.To)
	}
	id := MessageID(msg.IdempotencyKey, m.mailer.cfg.From)
	if err := m.mailer.send(ctx, reports.Message{To: []string{msg.To}, Subject: m.Subject, Body: msg.Body}, id); err != nil {
		return "", err
	}
	return id, nil
}

var _ outreach.Messenger = (*Messenger)(nil)
