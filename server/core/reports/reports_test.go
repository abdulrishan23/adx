package reports_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/reports"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type fakeMailer struct {
	sent []reports.Message
	fail error
}

func (m *fakeMailer) Send(_ context.Context, msg reports.Message) error {
	if m.fail != nil {
		return m.fail
	}
	m.sent = append(m.sent, msg)
	return nil
}

type env struct {
	db      *store.DB
	clock   *coretest.Clock
	reports *reports.Reports
}

func setup(t *testing.T, mailer reports.Mailer) *env {
	t.Helper()
	// Friday 17 Oct 2026, 09:00 Dubai.
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 5, 0, 0, 0, time.UTC))}
	coretest.SeedUsers(t, e.db)
	r := reporting.New(e.db, nil, nil, evidence.New(e.db, e.clock), reporting.Config{Clock: e.clock})
	e.reports = reports.New(e.db, r, mailer, reports.Config{Clock: e.clock})
	// One Google campaign with spend yesterday.
	cred, err := e.db.Writer().UpsertCredential(t.Context(), store.UpsertCredentialParams{Provider: "google_ads", Kind: "oauth", Subject: "u", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(t.Context(), store.UpsertAccountParams{Type: "google_ads", ExternalID: "1", CredentialID: cred.ID, Currency: sql.NullString{String: "AED", Valid: true},
		TimeZone: sql.NullString{String: "Asia/Dubai", Valid: true}, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	c, err := e.db.Writer().UpsertAdCampaign(t.Context(), store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: acct.ID, Platform: "google_ads", ExternalID: "g1", Name: "Wadi Search", Status: "ENABLED",
		Currency: sql.NullString{String: "AED", Valid: true}, FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	require.NoError(t, e.db.Writer().UpsertCampaignDailyMetrics(t.Context(), store.UpsertCampaignDailyMetricsParams{WorkspaceID: 1, AccountID: acct.ID, CampaignID: c.ID, Platform: "google_ads",
		Day: "2026-10-16", Currency: "AED", CostMinor: 123456, Impressions: 10, Clicks: 2, ObservedAt: e.clock.Now()}))
	return e
}

// Catches: a schedule ticking twice building or sending the report twice,
// a report sent with no recipients, and the stored snapshot drifting from
// what was computed.
func TestScheduleOccurrenceIsBuiltAndDeliveredOnce(t *testing.T) {
	mailer := &fakeMailer{}
	e := setup(t, mailer)
	sched, err := e.reports.CreateSchedule(t.Context(), coretest.Admin, reports.ReportScheduleInput{Name: "Daily brief", Cadence: reports.Daily, Enabled: true,
		Recipients: []string{"CMO@reportage.ae"}, Params: reports.ReportParams{GroupBy: "campaign", Compare: "previous"}})
	require.NoError(t, err)
	assert.True(t, sched.DeliveryConfigured)
	assert.Equal(t, []string{"cmo@reportage.ae"}, sched.Recipients)

	first, created, err := e.reports.Generate(t.Context(), coretest.Marketer, sched.ID)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, reporting.Day("2026-10-16"), first.PeriodStart, "daily covers yesterday in Dubai")
	assert.Equal(t, "sent", first.DeliveryStatus)
	require.NotNil(t, first.DeliveredAt)
	require.Len(t, mailer.sent, 1)
	assert.Equal(t, []string{"cmo@reportage.ae"}, mailer.sent[0].To)
	assert.Contains(t, mailer.sent[0].Body, "Wadi Search: spend 1234.56 AED")
	assert.Contains(t, mailer.sent[0].Body, "revenue not available")
	assert.Contains(t, mailer.sent[0].Body, "Formula v1")
	require.NotNil(t, first.Result)
	assert.Equal(t, int64(123456), *first.Result.Summary.Metrics.Spend.Minor)

	// The job runs again the same day: nothing new, nothing re-sent.
	again, created, err := e.reports.Generate(t.Context(), coretest.Marketer, sched.ID)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, again.ID)
	assert.Len(t, mailer.sent, 1)
	generated, errs := e.reports.RunDue(t.Context())
	assert.Empty(t, errs)
	assert.Equal(t, 0, generated)
	assert.Len(t, mailer.sent, 1)

	// Next day: a new occurrence.
	e.clock.Advance(24 * time.Hour)
	generated, errs = e.reports.RunDue(t.Context())
	assert.Empty(t, errs)
	assert.Equal(t, 1, generated)
	assert.Len(t, mailer.sent, 2)
	list, err := e.reports.List(t.Context(), coretest.Viewer, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Nil(t, list[0].Result, "the list is light")
	got, err := e.reports.Get(t.Context(), coretest.Viewer, list[1].ID)
	require.NoError(t, err)
	assert.NotNil(t, got.Result)
}

// Catches: a report delivered with nobody configured to receive it, or
// treated as sent when no transport exists.
func TestUnconfiguredRecipientsOrTransportNeverDeliver(t *testing.T) {
	mailer := &fakeMailer{}
	e := setup(t, mailer)
	noRecipients, err := e.reports.CreateSchedule(t.Context(), coretest.Admin, reports.ReportScheduleInput{Name: "Weekly", Cadence: reports.Weekly, Enabled: true})
	require.NoError(t, err)
	assert.False(t, noRecipients.DeliveryConfigured)
	rep, created, err := e.reports.Generate(t.Context(), coretest.Marketer, noRecipients.ID)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "unconfigured", rep.DeliveryStatus)
	assert.Equal(t, "no recipients configured", *rep.DeliveryDetail)
	assert.Equal(t, "generated", rep.Status)
	assert.Equal(t, reporting.Day("2026-10-05"), rep.PeriodStart, "last full week Mon 5 to Sun 11 Oct")
	assert.Equal(t, reporting.Day("2026-10-11"), rep.PeriodEnd)
	assert.Empty(t, mailer.sent)

	noTransport := setup(t, nil)
	sched, err := noTransport.reports.CreateSchedule(t.Context(), coretest.Admin, reports.ReportScheduleInput{Name: "Monthly", Cadence: reports.Monthly, Enabled: true, Recipients: []string{"ceo@reportage.ae"}})
	require.NoError(t, err)
	assert.False(t, sched.DeliveryConfigured)
	rep, _, err = noTransport.reports.Generate(t.Context(), coretest.Marketer, sched.ID)
	require.NoError(t, err)
	assert.Equal(t, "unconfigured", rep.DeliveryStatus)
	assert.Equal(t, "no mail transport configured", *rep.DeliveryDetail)
	assert.Equal(t, reporting.Day("2026-09-01"), rep.PeriodStart)
	assert.Equal(t, reporting.Day("2026-09-30"), rep.PeriodEnd)

	// A failing transport records failed, not sent, and the occurrence stays.
	mailer.fail = errors.New("smtp: connection refused to 10.0.0.5")
	failing, err := e.reports.CreateSchedule(t.Context(), coretest.Admin, reports.ReportScheduleInput{Name: "Quarterly", Cadence: reports.Quarterly, Enabled: true, Recipients: []string{"ceo@reportage.ae"}})
	require.NoError(t, err)
	rep, _, err = e.reports.Generate(t.Context(), coretest.Marketer, failing.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", rep.DeliveryStatus)
	assert.NotContains(t, *rep.DeliveryDetail, "10.0.0.5")
	assert.Equal(t, reporting.Day("2026-07-01"), rep.PeriodStart)
	assert.Equal(t, reporting.Day("2026-09-30"), rep.PeriodEnd)
}

func TestScheduleValidationAndRoles(t *testing.T) {
	e := setup(t, &fakeMailer{})
	_, err := e.reports.CreateSchedule(t.Context(), coretest.Marketer, reports.ReportScheduleInput{Name: "x", Cadence: reports.Daily})
	assert.ErrorIs(t, err, core.ErrForbidden)
	var ce *core.Error
	_, err = e.reports.CreateSchedule(t.Context(), coretest.Admin, reports.ReportScheduleInput{Name: "x", Cadence: "hourly"})
	require.ErrorAs(t, err, &ce)
	_, err = e.reports.CreateSchedule(t.Context(), coretest.Admin, reports.ReportScheduleInput{Name: "x", Cadence: reports.Daily, Recipients: []string{"not an email"}})
	require.ErrorAs(t, err, &ce)
	_, err = e.reports.CreateSchedule(t.Context(), coretest.Admin, reports.ReportScheduleInput{Name: "x", Cadence: reports.Daily, Params: reports.ReportParams{GroupBy: "colour"}})
	require.ErrorAs(t, err, &ce)
	_, _, err = e.reports.Generate(t.Context(), coretest.Viewer, 1)
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, _, err = e.reports.Generate(t.Context(), coretest.Marketer, 99)
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindNotFound, ce.Kind)

	adhoc, created, err := e.reports.GenerateAdHoc(t.Context(), coretest.Marketer, "2026-10-10", "2026-10-16", reports.ReportParams{GroupBy: "platform"})
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "unconfigured", adhoc.DeliveryStatus, "ad hoc reports are shown, not sent")
	_, created, err = e.reports.GenerateAdHoc(t.Context(), coretest.Marketer, "2026-10-10", "2026-10-16", reports.ReportParams{GroupBy: "platform"})
	require.NoError(t, err)
	assert.False(t, created)
}

func TestPeriods(t *testing.T) {
	// Monday 2 Nov 2026 03:00 Dubai (Sunday 23:00 UTC).
	now := time.Date(2026, 11, 1, 23, 0, 0, 0, time.UTC)
	from, to := reports.Period(reports.Daily, now)
	assert.Equal(t, reporting.Day("2026-11-01"), from)
	assert.Equal(t, reporting.Day("2026-11-01"), to)
	from, to = reports.Period(reports.Weekly, now)
	assert.Equal(t, reporting.Day("2026-10-26"), from)
	assert.Equal(t, reporting.Day("2026-11-01"), to)
	from, to = reports.Period(reports.Monthly, now)
	assert.Equal(t, reporting.Day("2026-10-01"), from)
	assert.Equal(t, reporting.Day("2026-10-31"), to)
	from, to = reports.Period(reports.Quarterly, now)
	assert.Equal(t, reporting.Day("2026-07-01"), from)
	assert.Equal(t, reporting.Day("2026-09-30"), to)
}
