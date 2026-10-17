// Package smtpmail sends report emails over SMTP with STARTTLS. It
// implements reports.Mailer. D-24 notes a shared sender such as
// noreply@wokeworks.ai is still needed; until then the From address is
// whatever the operator configures.
package smtpmail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core/reports"
)

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	// InsecureSkipVerify is for a local test relay only.
	InsecureSkipVerify bool
}

type Mailer struct{ cfg Config }

func New(cfg Config) (*Mailer, error) {
	if cfg.Host == "" || cfg.Port == 0 || cfg.From == "" {
		return nil, errors.New("smtp host, port and from address are required")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, fmt.Errorf("smtp from address %q is invalid", cfg.From)
	}
	return &Mailer{cfg: cfg}, nil
}

// Build renders the RFC 5322 message. Exported so tests can check headers
// without a server.
func Build(from string, m reports.Message) ([]byte, error) {
	if len(m.To) == 0 {
		return nil, errors.New("message has no recipients")
	}
	for _, to := range m.To {
		if _, err := mail.ParseAddress(to); err != nil {
			return nil, fmt.Errorf("recipient %q is invalid", to)
		}
	}
	if strings.ContainsAny(m.Subject, "\r\n") {
		return nil, errors.New("subject contains a line break")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(m.To, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", m.Subject)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(strings.ReplaceAll(m.Body, "\n", "\r\n"))
	return []byte(b.String()), nil
}

func (s *Mailer) Send(ctx context.Context, m reports.Message) error {
	msg, err := Build(s.cfg.From, m)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer conn.Close()
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp: %w", err)
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, InsecureSkipVerify: s.cfg.InsecureSkipVerify}); err != nil { //nolint:gosec // operator opt-in for a local relay
			return fmt.Errorf("smtp starttls: %w", err)
		}
	} else if s.cfg.Password != "" {
		return errors.New("smtp server offers no STARTTLS; refusing to send the password in clear")
	}
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("smtp from: %w", err)
	}
	for _, to := range m.To {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("smtp rcpt: %w", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

var _ reports.Mailer = (*Mailer)(nil)
