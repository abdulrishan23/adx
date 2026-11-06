package conversations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/qualification"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/store"
)

// OutcomeKind is the scoring signal a conversation's reading produces.
const OutcomeKind = "conversation_outcome"

// Read takes the conversation's reading now and applies it. The agent does
// this on its own at a handoff, when it is done, and when a person closes
// the thread; a person may ask for it any time. Marketers may.
func (c *Conversations) Read(ctx context.Context, actor core.Actor, id int64) (Thread, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return Thread{}, core.ErrForbidden
	}
	if _, err := c.row(ctx, actor.WorkspaceID, id); err != nil {
		return Thread{}, err
	}
	if err := c.read(ctx, actor, id); err != nil {
		return Thread{}, err
	}
	return c.get(ctx, actor.WorkspaceID, id)
}

// read turns the thread into a transcript, asks the model for the reading
// with the call reading prompt, and applies it. Without a model, or when
// the model is unsure, the reading waits for a person. A reading taken
// over the same turns as the last one is not taken again.
func (c *Conversations) read(ctx context.Context, actor core.Actor, id int64) error {
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return err
	}
	turns, err := c.db.Reader().ListThreadTurns(ctx, id)
	if err != nil {
		return err
	}
	segs := transcript(turns)
	if len(segs) == 0 {
		return nil
	}
	if int64(len(segs)) == row.ReadTurns && row.ReviewStatus != "" && row.ReviewStatus != "pending" {
		return nil
	}
	now := c.clock.Now().UTC()
	// A corrected reading stands over a re-read: the person's word is the
	// last one unless the customer has said more since.
	if row.ReviewStatus == "corrected" && int64(len(segs)) == row.ReadTurns {
		return nil
	}
	pending := func(reason string) error {
		return c.db.Writer().SetThreadReading(ctx, store.SetThreadReadingParams{Summary: row.Summary, Answers: row.Answers, Outcome: row.Outcome, Confidence: row.Confidence, RequestedHuman: row.RequestedHuman, ReviewStatus: "pending", ReviewReason: reason, ReadAt: sql.NullTime{Time: now, Valid: true}, ReadTurns: int64(len(segs)), UpdatedAt: now, ID: id})
	}
	if c.cfg.Model == nil {
		return pending("no model is configured to read the conversation")
	}
	r, err := qualification.ReadTranscript(ctx, c.cfg.Model, segs)
	if err != nil {
		var re *qualification.ReadError
		if errors.As(err, &re) {
			return pending(re.Reason)
		}
		return pending("the model did not answer: " + safe(err))
	}
	review, reason := "auto", ""
	if r.Confidence == "low" {
		review, reason = "pending", "the model was not sure of its reading; a person confirms or corrects it"
	}
	answers, _ := json.Marshal(r.Answers)
	if err := c.db.Writer().SetThreadReading(ctx, store.SetThreadReadingParams{Summary: r.Summary, Answers: string(answers), Outcome: r.Outcome, Confidence: r.Confidence, RequestedHuman: boolInt(r.RequestedHuman), ReviewStatus: review, ReviewReason: reason, ReadAt: sql.NullTime{Time: now, Valid: true}, ReadTurns: int64(len(segs)), UpdatedAt: now, ID: id}); err != nil {
		return err
	}
	if c.cfg.Outreach != nil {
		var enq int64
		if row.EnquiryID.Valid {
			enq = row.EnquiryID.Int64
		}
		for _, o := range r.Objections {
			_, _ = c.cfg.Outreach.RecordObjection(ctx, actor, outreach.ObjectionInput{PersonID: row.PersonID, EnquiryID: enq, Kind: o.Kind, Text: o.Text, Source: fmt.Sprintf("conversation:%d", id)})
		}
	}
	_, err = c.apply(ctx, actor, id)
	return err
}

// transcript is the thread as the reading prompt wants it: what was sent
// and received, in order, without drafts that never went.
func transcript(turns []store.ThreadTurn) []voice.Segment {
	var segs []voice.Segment
	for _, t := range turns {
		switch {
		case t.Direction == "in" && t.Kind == "text":
			segs = append(segs, voice.Segment{Speaker: "customer", Text: t.Body})
		case t.Direction == "out" && (t.State == "sent" || t.State == "delivered" || t.State == "read"):
			speaker := "agent"
			if strings.HasPrefix(t.Author, "user:") {
				speaker = "reportage"
			}
			segs = append(segs, voice.Segment{Speaker: speaker, Text: t.Body})
		}
	}
	return segs
}

// apply writes the reading to the enquiry once per version, the way a call
// reading is applied: the scoring signal under the conversation's
// occurrence key, stated attributes the model was sure of, a stage for a
// clear outcome. A pending reading applies nothing; an uncertain figure is
// held and named.
func (c *Conversations) apply(ctx context.Context, actor core.Actor, id int64) (qualification.AppliedDetail, error) {
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return qualification.AppliedDetail{}, err
	}
	d := qualification.AppliedDetail{Attributes: []string{}, Held: []string{}}
	var prev qualification.AppliedDetail
	_ = json.Unmarshal([]byte(row.AppliedDetail), &prev)
	switch {
	case prev.ModelOutcome != "":
		d.ModelOutcome = prev.ModelOutcome
	case row.ReviewStatus == "corrected":
		d.ModelOutcome = ""
	default:
		d.ModelOutcome = row.Outcome
	}
	if row.ReviewStatus == "pending" || row.ReviewStatus == "" {
		d.Note = "reading pending review; nothing applied"
		return d, nil
	}
	version := row.AppliedVersion + 1
	key := fmt.Sprintf("conversation:%d:v%d", id, version)
	var answers map[string]qualification.ReadingAnswer
	_ = json.Unmarshal([]byte(row.Answers), &answers)
	scoring := row.Outcome == "interested" || row.Outcome == "not_interested" || row.Outcome == "callback" || row.Outcome == "transfer"
	if scoring && c.cfg.Signals != nil {
		payload, _ := json.Marshal(map[string]any{"conversationId": id, "outcome": row.Outcome, "version": version})
		res, err := c.cfg.Signals.Ingest(ctx, actor, signals.SignalInput{Source: row.Channel, Kind: OutcomeKind, OccurrenceKey: key, OccurredAt: c.clock.Now().UTC(), Identity: signals.SignalIdentity{PersonID: row.PersonID}, Payload: payload})
		if err == nil {
			eid := res.Event.ID
			d.SignalEventID = &eid
		}
	}
	if !scoring {
		d.Note = fmt.Sprintf("outcome %s does not change the enquiry", row.Outcome)
	}
	for _, k := range sortedKeys(answers) {
		a := answers[k]
		if strings.TrimSpace(a.Value) == "" {
			continue
		}
		if a.Uncertain {
			d.Held = append(d.Held, fmt.Sprintf("%s=%q marked uncertain; not recorded", k, a.Value))
			continue
		}
		if c.cfg.CRM != nil {
			if _, err := c.cfg.CRM.SetStated(ctx, actor, row.PersonID, k, a.Value, fmt.Sprintf("conversation:%d", id)); err == nil {
				d.Attributes = append(d.Attributes, k)
			}
		}
	}
	switch row.Outcome {
	case "interested":
		d.Stage = "qualified"
	case "not_interested":
		d.Stage = "not_interested"
	case "callback":
		d.Stage = "callback"
	case "transfer":
		d.Stage = "handoff"
	}
	if d.Stage != "" && row.EnquiryID.Valid {
		if err := c.db.Writer().TouchEnquiry(ctx, store.TouchEnquiryParams{LastActivityAt: c.clock.Now().UTC(), Stage: sql.NullString{String: d.Stage, Valid: true}, ID: row.EnquiryID.Int64}); err != nil {
			return d, err
		}
	}
	detail, _ := json.Marshal(d)
	n, err := c.db.Writer().SetThreadApplied(ctx, store.SetThreadAppliedParams{AppliedKey: key, AppliedVersion: version, AppliedAt: sql.NullTime{Time: c.clock.Now().UTC(), Valid: true}, AppliedDetail: string(detail), UpdatedAt: c.clock.Now().UTC(), ID: id, AppliedVersion_2: row.AppliedVersion})
	if err != nil {
		return d, err
	}
	if n != 1 {
		return d, core.Conflict("conversation %d was applied by someone else meanwhile", id)
	}
	return d, nil
}

// Confirm accepts a pending reading as read and applies it.
func (c *Conversations) Confirm(ctx context.Context, actor core.Actor, id int64) (Thread, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return Thread{}, core.ErrForbidden
	}
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return Thread{}, err
	}
	if row.ReviewStatus != "pending" {
		return Thread{}, core.Conflict("conversation %d's reading is %s, not pending", id, firstNonEmpty(row.ReviewStatus, "not taken"))
	}
	if row.Outcome == "" {
		return Thread{}, core.Conflict("conversation %d has no reading to confirm; correct it with an outcome instead", id)
	}
	now := c.clock.Now().UTC()
	if err := c.db.Writer().SetThreadReading(ctx, store.SetThreadReadingParams{Summary: row.Summary, Answers: row.Answers, Outcome: row.Outcome, Confidence: "high", RequestedHuman: row.RequestedHuman, ReviewStatus: "confirmed", ReviewReason: fmt.Sprintf("confirmed by user %d", actor.UserID), ReadAt: row.ReadAt, ReadTurns: row.ReadTurns, UpdatedAt: now, ID: id}); err != nil {
		return Thread{}, err
	}
	if _, err := c.apply(ctx, actor, id); err != nil {
		return Thread{}, err
	}
	return c.get(ctx, actor.WorkspaceID, id)
}

// Correct replaces the reading with a person's, keeps the model's in the
// correction history, discards the earlier scoring event and applies
// again under a new version.
func (c *Conversations) Correct(ctx context.Context, actor core.Actor, id int64, in qualification.CorrectionInput) (Thread, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return Thread{}, core.ErrForbidden
	}
	known := false
	for _, o := range qualification.Outcomes {
		if o == in.Outcome {
			known = true
		}
	}
	if !known {
		return Thread{}, core.Invalid("outcome must be one of %s", strings.Join(qualification.Outcomes, ", "))
	}
	in.Note = strings.TrimSpace(in.Note)
	if in.Note == "" {
		return Thread{}, core.Invalid("say why the reading was wrong")
	}
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return Thread{}, err
	}
	if in.Answers == nil {
		in.Answers = map[string]qualification.ReadingAnswer{}
	}
	answers, _ := json.Marshal(in.Answers)
	now := c.clock.Now().UTC()
	if _, err := c.db.Writer().CreateThreadCorrection(ctx, store.CreateThreadCorrectionParams{ThreadID: id, Outcome: in.Outcome, Answers: string(answers), Note: in.Note, CreatedBy: optionalID(actor.UserID), CreatedAt: now}); err != nil {
		return Thread{}, err
	}
	var prev qualification.AppliedDetail
	_ = json.Unmarshal([]byte(row.AppliedDetail), &prev)
	if prev.SignalEventID != nil && c.cfg.Signals != nil {
		_, _ = c.cfg.Signals.Correct(ctx, actor, *prev.SignalEventID, "discard", 0, "conversation reading corrected: "+in.Note)
	}
	turns, err := c.db.Reader().ListThreadTurns(ctx, id)
	if err != nil {
		return Thread{}, err
	}
	readTurns := int64(len(transcript(turns)))
	if err := c.db.Writer().SetThreadReading(ctx, store.SetThreadReadingParams{Summary: row.Summary, Answers: string(answers), Outcome: in.Outcome, Confidence: "high", RequestedHuman: row.RequestedHuman, ReviewStatus: "corrected", ReviewReason: in.Note, ReadAt: sql.NullTime{Time: now, Valid: true}, ReadTurns: readTurns, UpdatedAt: now, ID: id}); err != nil {
		return Thread{}, err
	}
	if _, err := c.apply(ctx, actor, id); err != nil {
		return Thread{}, err
	}
	return c.get(ctx, actor.WorkspaceID, id)
}
