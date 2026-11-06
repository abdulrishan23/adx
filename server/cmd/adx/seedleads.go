package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/store"
)

// demoLocation is the GoHighLevel workspace the demo leads belong to. Its
// credential is a placeholder that cannot read GoHighLevel: the leads are
// fed through the same ingest a real import uses, not read from the CRM.
const demoLocation = "demo-location"

// demoLead is one contact as GoHighLevel would hand it over: raw status,
// raw source and raw project text, which the funnel mapping turns into a
// stage, a source and a project. The set covers each state the Leads
// screens handle: every stage, a blank status, a status the table does not
// know, a status field missing on the last scan, a phone two leads share,
// and each match level.
type demoLead struct {
	id, first, last, phone, email, source, project, status string
	daysAgo                                                int
	attributions                                           []leads.Attribution
	// events are signal kinds recorded for the person, oldest first, a day
	// apart ending yesterday.
	events []string
	// noStatusField makes a second observation arrive without the status
	// field, the way a contact looks when the field is missing.
	noStatusField bool
}

var demoLeads = []demoLead{
	{id: "ghl-demo-01", first: "Rahul", last: "Mehta", phone: "+971501110001", email: "rahul.mehta@example.com", source: "Meta", project: "The Wadi", status: "Follow Up", daysAgo: 3,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "Meta", UTMMedium: "paid", UTMCampaign: "wadi-launch", FBCLID: "fb.1.demo01"}, {UTMSource: "Meta", UTMCampaign: "wadi-launch"}},
		events:       []string{"form_submit", "whatsapp_tap"}},
	{id: "ghl-demo-02", first: "Ayesha", last: "Siddiqui", phone: "+971501110002", email: "ayesha.s@example.com", source: "Website", project: "Verdana", status: "Meeting", daysAgo: 9,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "google", UTMMedium: "cpc", UTMCampaign: "verdana-search"}},
		events:       []string{"form_submit", "page_view", "stage_change"}},
	{id: "ghl-demo-03", first: "Omar", last: "Al Mansoori", phone: "+971501110003", source: "WhatsApp Camp.", project: "The Wadi", status: "New", daysAgo: 1,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "Meta", UTMCampaign: "wadi-launch", FBCLID: "fb.1.demo03"}},
		events:       []string{"whatsapp_tap"}},
	{id: "ghl-demo-04", first: "James", last: "Whitfield", phone: "+447700900004", email: "james.w@example.co.uk", source: "Google", project: "Sila", daysAgo: 2,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "google", UTMMedium: "cpc", UTMCampaign: "sila-search", GCLID: "Cj0demo04"}},
		events:       []string{"form_submit"}},
	{id: "ghl-demo-05", first: "Lucía", last: "Fernández", phone: "+34600000005", source: "Google", project: "Oceana", status: "Not Interested", daysAgo: 14,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "google", UTMCampaign: "oceana-search", GCLID: "Cj0demo05"}}},
	{id: "ghl-demo-06", first: "Dmitri", last: "Volkov", phone: "+971501110006", source: "Bayut", project: "Selina Bay", status: "Call Back", daysAgo: 6},
	{id: "ghl-demo-07", first: "Priya", last: "Nair", phone: "+919800000007", email: "priya.nair@example.in", source: "Meta", project: "Verdana", status: "Done-Deal", daysAgo: 40,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "Meta", UTMCampaign: "verdana-monthly", FBCLID: "fb.1.demo07"}},
		events:       []string{"form_submit", "message_reply", "stage_change"}},
	{id: "ghl-demo-08", first: "Bilal", last: "Qureshi", phone: "+923000000008", source: "Referral", project: "Sensi", status: "Follow Up", daysAgo: 12},
	{id: "ghl-demo-09", first: "Fatima", last: "Al Mansoori", phone: "+971501234501", source: "Meta", project: "The Wadi", status: "Potential", daysAgo: 4,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "Meta", UTMCampaign: "wadi-launch", FBCLID: "fb.1.demo09"}},
		events:       []string{"form_submit"}},
	{id: "ghl-demo-10", first: "Hassan", last: "Al Marzouqi", phone: "+971501110010", source: "Meta", project: "The Wadi", status: "No Answer", daysAgo: 5,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "Meta", UTMCampaign: "wadi-launch", FBCLID: "fb.1.demo10"}}},
	{id: "ghl-demo-11", first: "Saeed", last: "Al Marzouqi", phone: "+971501110010", source: "Website", project: "The Wadi", status: "Follow Up", daysAgo: 2,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "google", UTMCampaign: "wadi-search"}}},
	{id: "ghl-demo-12", first: "Noor", last: "Haddad", phone: "+971501110012", email: "noor.h@example.com", source: "Google", project: "Sila", status: "Meeting", daysAgo: 8, noStatusField: true,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "google", UTMMedium: "cpc", UTMCampaign: "sila-search", GCLID: "Cj0demo12"}},
		events:       []string{"form_submit", "stage_change"}},
	{id: "ghl-demo-13", first: "Mariam", last: "Khan", phone: "+971501110013", source: "Website", daysAgo: 1,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "google", UTMCampaign: "brand"}}},
	{id: "ghl-demo-14", first: "Yusuf", last: "Rahman", phone: "+971501110014", email: "yusuf.r@example.com", source: "Meta", project: "Oceana", status: "Sales Offer", daysAgo: 20,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "Meta", UTMCampaign: "oceana-sea-view", FBCLID: "fb.1.demo14"}},
		events:       []string{"form_submit", "whatsapp_tap", "message_reply", "stage_change"}},
	{id: "ghl-demo-15", first: "Khalid", last: "Saeed", phone: "+971501110015", source: "Property Finder", project: "Sensi", status: "Unreachable", daysAgo: 30},
	{id: "ghl-demo-16", first: "Sara", last: "Ibrahim", phone: "+971501110016", source: "Meta", project: "Selina Bay", status: "Broker", daysAgo: 7,
		attributions: []leads.Attribution{{IsFirst: true, UTMSource: "Meta", UTMCampaign: "selina-book5", FBCLID: "fb.1.demo16"}}},
}

// demoFunnel is a proposal of the status-to-stage table in the shape the
// owner saves (reference.md: the table is the owner's, keyed by what
// GoHighLevel spells). "Call Back" is left out on purpose so the screens
// show a status waiting for a row, and a blank status is left unmapped
// because what a lead with no status counts as is still the owner's call.
func demoFunnel() leads.FunnelDocument {
	return leads.FunnelDocument{
		StatusField:  leads.FieldLocation{Kind: "custom_field", Ref: "lead_status"},
		ProjectField: leads.FieldLocation{Kind: "custom_field", Ref: "project"},
		Statuses: map[string]leads.Stage{
			"New": leads.StageOpen, "No Answer": leads.StageAttempted, "Follow Up": leads.StageEngaged, "Potential": leads.StageEngaged,
			"Meeting": leads.StageMeeting, "Sales Offer": leads.StageOffer, "Done-Deal": leads.StageWon,
			"Not Interested": leads.StageDisqualified, "Low Budget": leads.StageDisqualified, "Ready To Move": leads.StageDisqualified,
			"No availability": leads.StageDisqualified, "Duplicated": leads.StageExcluded, "Broker": leads.StageExcluded,
			"Unreachable": leads.StageDead, "Wrong number": leads.StageDead,
		},
		Sources: map[string]string{"Meta": "meta", "Google": "google", "Website": "website", "WhatsApp Camp.": "handling_channel",
			"Bayut": "portal:bayut", "Property Finder": "portal:propertyfinder"},
		Note: "Demo proposal from adx seed-demo. Replace with Reportage's table.",
	}
}

// seedLeads gives the demo a GoHighLevel workspace with leads in every
// state the Convert screens show, fed through the engine's own ingest,
// resolved to people and scored, the way a real import ends up. It saves
// the funnel mapping and the suggested scoring rules only when none exist,
// and adds nothing when the demo location already has leads.
func seedLeads(ctx context.Context, db *store.DB, admin, marketer core.Actor) (int, error) {
	now := time.Now().UTC()
	ev := evidence.New(db, core.SystemClock{})
	l := leads.New(db, nil, nil, ev, leads.Config{})
	people := crm.New(db, ev, crm.Config{})
	sig := signals.New(db, ev, people, signals.Config{})

	var acct store.Account
	row, err := db.Reader().GetAccount(ctx, store.GetAccountParams{Type: "gohighlevel", ExternalID: demoLocation})
	acct = row.Account
	if errors.Is(err, sql.ErrNoRows) {
		cred, err := db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "gohighlevel", Kind: "private_token", Subject: demoLocation,
			EncryptedSecret: []byte("demo: not a token"), KeyVersion: 1, Scopes: "contacts.readonly", ConnectedAt: now})
		if err != nil {
			return 0, err
		}
		acct, err = db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{WorkspaceID: core.DefaultWorkspace, Type: "gohighlevel", ExternalID: demoLocation, CredentialID: cred.ID,
			Name: sql.NullString{String: "Reportage demo workspace", Valid: true}, TimeZone: sql.NullString{String: "Asia/Dubai", Valid: true},
			Details: "{}", ConnectedAt: now, VerifiedAt: now})
		if err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}
	have, err := db.Reader().ListLeadsByAccount(ctx, acct.ID)
	if err != nil {
		return 0, err
	}
	if len(have) > 0 {
		return 0, nil
	}

	// The contacts carry their status and project under whatever custom
	// fields the saved table reads, so a workspace with its own table still
	// gets leads with statuses.
	statusRef, projectRef := "lead_status", "project"
	funnel, err := l.GetFunnel(ctx, admin)
	if err != nil {
		return 0, err
	}
	if !funnel.Configured {
		if _, err := l.PutFunnel(ctx, admin, demoFunnel()); err != nil {
			return 0, fmt.Errorf("funnel: %w", err)
		}
		slog.Info("seed-demo: funnel mapping saved as a proposal")
	} else {
		doc := funnel.Current.Document
		if doc.StatusField.Kind == "custom_field" {
			statusRef = doc.StatusField.Ref
		} else {
			slog.Warn("seed-demo: the saved table reads the status from somewhere other than a custom field, so the demo leads will have no status", "kind", doc.StatusField.Kind)
		}
		if doc.ProjectField.Kind == "custom_field" {
			projectRef = doc.ProjectField.Ref
		} else {
			slog.Warn("seed-demo: the saved table reads the project from somewhere other than a custom field, so the demo leads will have no project", "kind", doc.ProjectField.Kind)
		}
	}
	rules, err := sig.GetRules(ctx, admin)
	if err != nil {
		return 0, err
	}
	if !rules.Configured {
		if _, err := sig.PutRules(ctx, admin, signals.SuggestedRules()); err != nil {
			return 0, fmt.Errorf("scoring rules: %w", err)
		}
		slog.Info("seed-demo: suggested scoring rules saved, thresholds unset")
	}

	for _, d := range demoLeads {
		added := now.Add(-time.Duration(d.daysAgo) * 24 * time.Hour)
		c := leads.Contact{ID: d.id, FirstName: d.first, LastName: d.last, Email: d.email, Phone: d.phone, Source: d.source,
			DateAdded: &added, DateUpdated: &added, CustomFields: map[string]string{}, Attributions: d.attributions}
		if d.project != "" {
			c.CustomFields[projectRef] = d.project
		}
		if d.status != "" {
			c.CustomFields[statusRef] = d.status
		}
		if _, err := l.Ingest(ctx, marketer, acct.ID, c); err != nil {
			return 0, fmt.Errorf("%s: %w", d.id, err)
		}
		if d.noStatusField {
			delete(c.CustomFields, statusRef)
			if _, err := l.Ingest(ctx, marketer, acct.ID, c); err != nil {
				return 0, fmt.Errorf("%s again: %w", d.id, err)
			}
		}
	}
	if _, err := people.Sync(ctx, marketer); err != nil {
		return 0, fmt.Errorf("resolving people: %w", err)
	}

	for _, d := range demoLeads {
		var projectID int64
		if d.project != "" {
			if p, err := db.Reader().GetProjectByName(ctx, store.GetProjectByNameParams{WorkspaceID: core.DefaultWorkspace, Name: d.project}); err == nil {
				projectID = p.ID
			}
		}
		for i, kind := range d.events {
			at := now.Add(-time.Duration(len(d.events)-i) * 24 * time.Hour)
			if _, err := sig.Ingest(ctx, marketer, signals.SignalInput{Source: "demo", Kind: kind, OccurrenceKey: fmt.Sprintf("%s/%d/%s", d.id, i, kind),
				OccurredAt: at, Identity: signals.SignalIdentity{Phone: d.phone, External: "ghl:" + d.id}, ProjectID: projectID, Payload: []byte(`{"demo":true}`)}); err != nil {
				return 0, fmt.Errorf("%s %s: %w", d.id, kind, err)
			}
		}
	}
	return len(demoLeads), nil
}
