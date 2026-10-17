package smtpmail_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/providers/smtpmail"
)

// Catches: the adapter serving a channel it cannot, an invalid recipient
// reaching the relay, and a message id that is not a valid header value.
func TestMessengerServesEmailOnly(t *testing.T) {
	mailer, err := smtpmail.New(smtpmail.Config{Host: "127.0.0.1", Port: 1, From: "ADX <sales@reportage.ae>"})
	require.NoError(t, err)
	m, err := smtpmail.NewMessenger(mailer, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"email"}, m.Channels())

	_, err = m.Send(t.Context(), outreach.OutboundMessage{Channel: "whatsapp", To: "+971501234567", Body: "x"})
	require.Error(t, err)
	_, err = m.Send(t.Context(), outreach.OutboundMessage{Channel: "email", To: "not-an-address", Body: "x"})
	require.Error(t, err)
	// A valid address reaches the relay, which is not listening here: the
	// failure is a dial error, never a silent success.
	_, err = m.Send(t.Context(), outreach.OutboundMessage{Channel: "email", To: "ali@example.com", Body: "x", IdempotencyKey: "draft:7:v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "smtp dial")

	assert.Equal(t, "<draft-7-v1@reportage.ae>", smtpmail.MessageID("draft:7:v1", "ADX <sales@reportage.ae>"))
	assert.Equal(t, "<k@adx.invalid>", smtpmail.MessageID("k", "broken"))
	_, err = smtpmail.NewMessenger(nil, "")
	require.Error(t, err)
	_, err = smtpmail.NewMessenger(mailer, "two\nlines")
	require.Error(t, err)
}
