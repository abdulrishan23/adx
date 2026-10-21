package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/conversations"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/qualification"
	"github.com/WokeWorks/adx/server/store"
)

// demoThread is one WhatsApp conversation the demo shows in Convert ›
// Inbox, written the way the agent would have left it: the turns, their
// states and reasons, the handoff and the reading. The agent itself needs
// a model, which a dev laptop has not got; these threads show each state
// the screen handles.
type demoThread struct {
	name, phone, state, stateReason, handoff string
	language                                 string
	turns                                    []demoTurn
	reading                                  *qualification.Reading
	review                                   string
	minutesAgo                               int
}

type demoTurn struct {
	in      bool
	author  string
	body    string
	state   string
	reason  string
	handoff string
	minutes int // minutes after the thread started
}

var demoThreads = []demoThread{
	{
		name: "Fatima Al Mansoori", phone: "+971501234501", state: conversations.StateWaiting, handoff: conversations.HandoffOutsideFacts,
		stateReason: "the reply quotes 10% off, which is not an approved fact; not sent", minutesAgo: 25,
		turns: []demoTurn{
			{in: true, author: "customer", body: "Hi, I saw your ad for The Wadi. What does a 2 bedroom start at?", state: "received"},
			{author: "agent", body: "Hello Fatima, happy to help with The Wadi. Prices start at AED 1,500,000. Is it to live in or to invest?", state: "read", minutes: 1},
			{in: true, author: "customer", body: "To invest. Is there any discount if I pay cash?", state: "received", minutes: 6},
			{author: "agent", body: "For a cash buyer we can do 10% off the list price.", state: "held", reason: "the reply quotes 10% off, which is not an approved fact; not sent", handoff: conversations.HandoffOutsideFacts, minutes: 7},
		},
	},
	{
		name: "Omar Haddad", phone: "+971501234502", state: conversations.StateWaiting, handoff: conversations.HandoffRequestedHuman,
		stateReason: "the customer asked for a person: Can I speak to someone from sales?", minutesAgo: 70,
		turns: []demoTurn{
			{in: true, author: "customer", body: "Hello, I have a few questions about the payment plan at The Wadi.", state: "received"},
			{author: "agent", body: "Hello Omar. The payment plan is 60/40, with 60% during construction and 40% on handover. What would you like to know?", state: "read", minutes: 1},
			{in: true, author: "customer", body: "Can I speak to someone from sales?", state: "received", minutes: 4},
			{author: "agent", body: "Of course. A colleague from Reportage will message you here shortly.", state: "delivered", handoff: conversations.HandoffRequestedHuman, minutes: 4},
		},
		reading: &qualification.Reading{Summary: "Omar asked about the payment plan at The Wadi and then asked for a salesperson.", Outcome: "transfer", Confidence: "high", Answers: map[string]qualification.ReadingAnswer{"payment": {Value: "plan", Quote: "questions about the payment plan"}}},
		review:  "auto",
	},
	{
		name: "Layla Rahman", phone: "+971501234503", state: conversations.StateClosed, stateReason: "the agent learned what it came for", minutesAgo: 60 * 26, language: "en",
		turns: []demoTurn{
			{in: true, author: "customer", body: "Hi, interested in a 3BR at The Wadi for my family", state: "received"},
			{author: "agent", body: "Hello Layla, lovely. When are you hoping to buy?", state: "read", minutes: 1},
			{in: true, author: "customer", body: "Within the next 3 months, we're renting now", state: "received", minutes: 3},
			{author: "agent", body: "That works well with the handover. How would you pay, cash, mortgage or the payment plan?", state: "read", minutes: 4},
			{in: true, author: "customer", body: "Mortgage, budget around 2.4", state: "received", minutes: 9},
			{author: "agent", body: "Thank you, Layla. The team will follow up with the 3-bedroom options and the mortgage partners.", state: "read", minutes: 10},
		},
		reading: &qualification.Reading{Summary: "Layla wants a 3-bedroom at The Wadi to live in, buying within three months with a mortgage; her budget figure was unclear.", Outcome: "interested", Confidence: "high",
			Answers: map[string]qualification.ReadingAnswer{"purchase_timeline": {Value: "3_months", Quote: "Within the next 3 months"}, "payment": {Value: "mortgage", Quote: "Mortgage"}, "purpose": {Value: "live", Quote: "for my family"}, "unit_type": {Value: "3br", Quote: "a 3BR"}, "budget": {Value: "2.4M", Uncertain: true, Quote: "budget around 2.4"}}},
		review: "auto",
	},
	{
		name: "Khalid Noor", phone: "+971501234504", state: conversations.StateAgent, stateReason: "the agent replies", minutesAgo: 12, language: "ar",
		turns: []demoTurn{
			{in: true, author: "customer", body: "مرحبا، هل الوادي مناسب للاستثمار؟", state: "received"},
			{author: "agent", body: "أهلاً خالد. الوادي في دبي لاند والتسليم في الربع الرابع من 2027. هل تفكر في شقة أم فيلا؟", state: "sent", minutes: 1},
		},
	},
}

// seedConversations writes the demo threads for The Wadi. Each thread is
// keyed by its number, so running it again adds only what is missing.
func seedConversations(ctx context.Context, db *store.DB, c *crm.CRM, marketer core.Actor) (int, error) {
	project, err := db.Reader().GetProjectByName(ctx, store.GetProjectByNameParams{WorkspaceID: core.DefaultWorkspace, Name: demoProjects[0].name})
	if err != nil {
		return 0, err
	}
	added := 0
	for _, d := range demoThreads {
		if _, err := db.Reader().GetThreadByAddress(ctx, store.GetThreadByAddressParams{WorkspaceID: core.DefaultWorkspace, Channel: conversations.Channel, Address: d.phone}); err == nil {
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			return added, err
		}
		started := time.Now().UTC().Add(-time.Duration(d.minutesAgo) * time.Minute)
		contact, err := c.ResolveContact(ctx, marketer, crm.ContactInput{Phone: d.phone, Name: d.name, Source: conversations.Channel, At: started, ProjectID: project.ID})
		if err != nil {
			return added, err
		}
		lang := d.language
		if lang == "" {
			lang = "en"
		}
		row, err := db.Writer().CreateThread(ctx, store.CreateThreadParams{WorkspaceID: core.DefaultWorkspace, PersonID: contact.Person.ID, EnquiryID: sql.NullInt64{Int64: contact.Enquiry.ID, Valid: true}, Channel: conversations.Channel, Address: d.phone, State: d.state, StateReason: d.stateReason, Language: lang, CreatedAt: started, UpdatedAt: started})
		if err != nil {
			return added, err
		}
		var lastIn, lastOut, last time.Time
		for i, t := range d.turns {
			at := started.Add(time.Duration(t.minutes) * time.Minute)
			dir, key, ref := "out", fmt.Sprintf("out:%d:%d", row.ID, i), ""
			if t.in {
				dir, key, ref = "in", fmt.Sprintf("in:wamid.demo.%d.%d", row.ID, i), fmt.Sprintf("wamid.demo.%d.%d", row.ID, i)
				lastIn = at
			} else if t.state == "sent" || t.state == "delivered" || t.state == "read" {
				lastOut, ref = at, fmt.Sprintf("wamid.demo.out.%d.%d", row.ID, i)
			}
			var sent sql.NullTime
			if ref != "" && !t.in {
				sent = sql.NullTime{Time: at, Valid: true}
			}
			claims := "{}"
			if t.handoff == conversations.HandoffOutsideFacts {
				claims = `{"brandId":0,"offerId":null,"offerVersion":null,"offerState":"","offerCurrent":null,"factIds":[],"claims":[{"text":"10%","kind":"percent","status":"unapproved","factId":null,"detail":"no approved fact or approved offer states this percentage"}]}`
			}
			turn, err := db.Writer().CreateThreadTurn(ctx, store.CreateThreadTurnParams{ThreadID: row.ID, Direction: dir, Author: t.author, Kind: "text", Body: t.body, State: t.state, Reason: t.reason, Handoff: t.handoff, ProviderRef: ref, IdempotencyKey: key, Claims: claims, OccurredAt: at, CreatedAt: at, UpdatedAt: at})
			if err != nil {
				return added, err
			}
			if sent.Valid {
				if _, err := db.Writer().MarkThreadTurnSent(ctx, store.MarkThreadTurnSentParams{ProviderRef: ref, SentAt: sent, UpdatedAt: at, ID: turn.ID, FromState: t.state}); err != nil {
					return added, err
				}
				// MarkThreadTurnSent sets 'sent'; a delivered or read turn
				// goes back to what the provider reported.
				if t.state != "sent" {
					if _, err := db.Writer().SetThreadTurnState(ctx, store.SetThreadTurnStateParams{State: t.state, Reason: "", UpdatedAt: at, ID: turn.ID, FromState: "sent"}); err != nil {
						return added, err
					}
				}
			}
			last = at
		}
		if !lastIn.IsZero() {
			if err := db.Writer().TouchThreadInbound(ctx, store.TouchThreadInboundParams{LastInboundAt: sql.NullTime{Time: lastIn, Valid: true}, Language: lang, UpdatedAt: last, ID: row.ID}); err != nil {
				return added, err
			}
		}
		if !lastOut.IsZero() {
			if err := db.Writer().TouchThreadOutbound(ctx, store.TouchThreadOutboundParams{LastOutboundAt: sql.NullTime{Time: lastOut, Valid: true}, UpdatedAt: last, ID: row.ID}); err != nil {
				return added, err
			}
		}
		var handoffAt sql.NullTime
		if d.handoff != "" {
			handoffAt = sql.NullTime{Time: last, Valid: true}
		}
		if err := db.Writer().SetThreadStateAny(ctx, store.SetThreadStateAnyParams{State: d.state, StateReason: d.stateReason, HandoffKind: d.handoff, HandoffAt: handoffAt, UpdatedAt: last, ID: row.ID}); err != nil {
			return added, err
		}
		if d.reading != nil {
			answers, _ := json.Marshal(d.reading.Answers)
			if err := db.Writer().SetThreadReading(ctx, store.SetThreadReadingParams{Summary: d.reading.Summary, Answers: string(answers), Outcome: d.reading.Outcome, Confidence: d.reading.Confidence, ReviewStatus: d.review, ReadAt: sql.NullTime{Time: last, Valid: true}, ReadTurns: int64(len(d.turns)), UpdatedAt: last, ID: row.ID}); err != nil {
				return added, err
			}
			detail := qualification.AppliedDetail{ModelOutcome: d.reading.Outcome, Attributes: []string{}, Held: []string{}}
			for k, a := range d.reading.Answers {
				if a.Uncertain {
					detail.Held = append(detail.Held, fmt.Sprintf("%s=%q marked uncertain; not recorded", k, a.Value))
					continue
				}
				if _, err := c.SetStated(ctx, marketer, contact.Person.ID, k, a.Value, fmt.Sprintf("conversation:%d", row.ID)); err == nil {
					detail.Attributes = append(detail.Attributes, k)
				}
			}
			switch d.reading.Outcome {
			case "interested":
				detail.Stage = "qualified"
			case "transfer":
				detail.Stage = "handoff"
			}
			if detail.Stage != "" {
				_ = db.Writer().TouchEnquiry(ctx, store.TouchEnquiryParams{LastActivityAt: last, Stage: sql.NullString{String: detail.Stage, Valid: true}, ID: contact.Enquiry.ID})
			}
			raw, _ := json.Marshal(detail)
			if _, err := db.Writer().SetThreadApplied(ctx, store.SetThreadAppliedParams{AppliedKey: fmt.Sprintf("conversation:%d:v1", row.ID), AppliedVersion: 1, AppliedAt: sql.NullTime{Time: last, Valid: true}, AppliedDetail: string(raw), UpdatedAt: last, ID: row.ID, AppliedVersion_2: 0}); err != nil {
				return added, err
			}
		}
		added++
		slog.Info("seed-demo: conversation added", "with", d.name)
	}
	return added, nil
}
