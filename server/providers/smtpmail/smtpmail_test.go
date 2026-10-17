package smtpmail_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/reports"
	"github.com/WokeWorks/adx/server/providers/smtpmail"
)

// Catches: a subject line injecting headers, an invalid recipient, and a
// body with bare line feeds.
func TestBuildMessage(t *testing.T) {
	msg, err := smtpmail.Build("ADX <noreply@example.com>", reports.Message{To: []string{"cmo@reportage.ae"}, Subject: "ADX daily", Body: "line one\nline two"})
	require.NoError(t, err)
	s := string(msg)
	assert.Contains(t, s, "From: ADX <noreply@example.com>\r\n")
	assert.Contains(t, s, "To: cmo@reportage.ae\r\n")
	assert.Contains(t, s, "Subject: ADX daily\r\n")
	assert.Contains(t, s, "\r\n\r\nline one\r\nline two")

	_, err = smtpmail.Build("a@b.c", reports.Message{To: []string{"cmo@reportage.ae"}, Subject: "x\r\nBcc: evil@example.com"})
	require.Error(t, err)
	_, err = smtpmail.Build("a@b.c", reports.Message{To: []string{"not an address"}, Subject: "x"})
	require.Error(t, err)
	_, err = smtpmail.Build("a@b.c", reports.Message{Subject: "x"})
	require.Error(t, err)
	_, err = smtpmail.New(smtpmail.Config{Host: "mail.example", Port: 587, From: "nope"})
	require.Error(t, err)
	_, err = smtpmail.New(smtpmail.Config{})
	require.Error(t, err)
}
