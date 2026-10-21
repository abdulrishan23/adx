package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/store"
)

const replyTool = "reply"

// ReplyDecision is the model's answer for one turn: what to say, whether to
// hand to a person and why, the project the customer named, an objection
// heard, and whether the agent has what it came for.
type ReplyDecision struct {
	Reply       string `json:"reply"`
	Language    string `json:"language"`
	Handoff     string `json:"handoff"`
	HandoffNote string `json:"handoffNote"`
	Project     string `json:"project"`
	Objection   *struct {
		Kind string `json:"kind"`
		Text string `json:"text"`
	} `json:"objection"`
	Done bool `json:"done"`
}

// Instructions is the agent's brief for a thread: the person, the project,
// the facts it may state, the questions, the rules. It is shared with the
// screen so a person sees exactly what the agent was told.
func (c *Conversations) Instructions(ctx context.Context, actor core.Actor, id int64) (string, outreach.SalesBrief, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return "", outreach.SalesBrief{}, core.ErrForbidden
	}
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return "", outreach.SalesBrief{}, err
	}
	b, err := c.brief(ctx, actor, row)
	if err != nil {
		return "", outreach.SalesBrief{}, err
	}
	names, err := c.projectNames(ctx, actor.WorkspaceID)
	if err != nil {
		return "", outreach.SalesBrief{}, err
	}
	return instructions(b, row.Language, names), b, nil
}

func (c *Conversations) brief(ctx context.Context, actor core.Actor, row store.Thread) (outreach.SalesBrief, error) {
	if c.cfg.Outreach == nil || !row.EnquiryID.Valid {
		return outreach.SalesBrief{Claims: []brands.Fact{}, ClaimsNote: "The conversation has no enquiry, so there are no project facts to quote."}, nil
	}
	return c.cfg.Outreach.NextAction(ctx, actor, row.EnquiryID.Int64)
}

func (c *Conversations) projectNames(ctx context.Context, workspaceID int64) ([]string, error) {
	rows, err := c.db.Reader().ListProjects(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, p := range rows {
		names = append(names, p.Name)
	}
	return names, nil
}

func instructions(b outreach.SalesBrief, language string, projects []string) string {
	var sb strings.Builder
	name := firstName(b.Person.DisplayName)
	project := b.ProjectName
	if project == "" {
		project = "Reportage's projects"
	}
	fmt.Fprintf(&sb, "You are the Reportage assistant answering %s on WhatsApp about %s. You are an assistant, not a salesperson; say so if asked. Reply in the customer's language (%s unless they switch). Keep every reply short: two or three sentences, one question at most.\n", name, project, language)
	sb.WriteString("Goal: learn, politely, their purchase timeline, how they would pay, whether it is to live in or invest, the unit type they want and a budget range. Ask one thing at a time. Accept 'I'd rather not say'. When you have these, thank them, say the team will follow up, and set done.\n")
	sb.WriteString("You may state only these approved facts, exactly as written:\n")
	if len(b.Claims) == 0 {
		sb.WriteString("- (none approved; say the team will confirm figures)\n")
	}
	for _, f := range b.Claims {
		v := f.Value
		if f.Kind == brands.KindMoney {
			v = formatMoney(v)
		}
		if f.Currency != nil && *f.Currency != "" {
			v = *f.Currency + " " + v
		}
		fmt.Fprintf(&sb, "- %s: %s\n", f.Key, v)
	}
	if b.ClaimsNote != "" {
		sb.WriteString(b.ClaimsNote + "\n")
	}
	sb.WriteString("Never state any other price, discount, payment plan, date, size or percentage, and never promise one. If asked for something the facts do not carry, say the team will confirm it and set handoff to question with the question in handoffNote.\n")
	if b.ProjectName == "" && len(projects) > 0 {
		fmt.Fprintf(&sb, "The customer has not said which project they mean. Our projects are: %s. If they name one, set project to its exact name.\n", strings.Join(projects, ", "))
	}
	if len(b.Objections) > 0 {
		sb.WriteString("Known concerns from earlier conversations:\n")
		for _, o := range b.Objections {
			if o.ResolvedAt == nil {
				fmt.Fprintf(&sb, "- %s: %s\n", o.Kind, o.Text)
			}
		}
	}
	sb.WriteString("If they ask for a person, a call, or to be contacted another way, agree at once, say a colleague will message them, and set handoff to requested_human. If they push back on price, location, timing, payment, trust or the unit and no approved fact answers it, acknowledge it, record it as objection, and set handoff to objection. If they ask to stop or not be messaged, set handoff to stop. Never infer nationality, religion or anything personal beyond the questions. Never pretend to be a person.\n")
	return sb.String()
}

func firstName(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return "there"
	}
	return f[0]
}

// formatMoney writes minor units as a whole-unit figure with thousands
// separators, the way a price is quoted to a customer (AED 1,500,000).
func formatMoney(minor string) string {
	n := strings.TrimSpace(minor)
	if len(n) <= 2 {
		return n
	}
	whole := n[:len(n)-2]
	var out []byte
	for i, ch := range []byte(whole) {
		if i > 0 && (len(whole)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, ch)
	}
	return string(out)
}

var replySchema = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"reply", "language", "handoff", "handoffNote", "project", "done"},
	"properties": map[string]any{
		"reply":       map[string]any{"type": "string", "description": "What to send the customer. Empty only when nothing should be sent."},
		"language":    map[string]any{"type": "string", "enum": []string{"en", "ar"}},
		"handoff":     map[string]any{"type": "string", "enum": []string{"", HandoffRequestedHuman, HandoffObjection, HandoffQuestion, HandoffStop}},
		"handoffNote": map[string]any{"type": "string"},
		"project":     map[string]any{"type": "string", "description": "The exact name of one of our projects the customer said they mean, or empty."},
		"objection":   map[string]any{"type": "object", "required": []string{"kind", "text"}, "properties": map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"price", "location", "timing", "payment", "trust", "unit", "other"}}, "text": map[string]any{"type": "string"}}},
		"done":        map[string]any{"type": "boolean"},
	}}

// reply drafts the agent's next turn and tries to send it. It returns nil
// when there is nothing to answer: the last word is already the agent's,
// or the thread was handed off without a reply. Every path that leaves the
// customer unanswered puts the thread in front of a person.
func (c *Conversations) reply(ctx context.Context, actor core.Actor, id int64) (*ThreadTurn, error) {
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return nil, err
	}
	turns, err := c.db.Reader().ListThreadTurns(ctx, id)
	if err != nil {
		return nil, err
	}
	var last *store.ThreadTurn
	var lastIn *store.ThreadTurn
	for i := range turns {
		t := turns[i]
		if t.State == "discarded" {
			continue
		}
		if t.Direction == "in" {
			lastIn = &turns[i]
		}
		last = &turns[i]
	}
	if last == nil || last.Direction == "out" || lastIn == nil {
		return nil, nil
	}
	now := c.clock.Now().UTC()
	// A request for a person is honoured without asking the model: the
	// customer said it, and a model that misses it must not get the chance.
	if lastIn.Kind == "text" && asksForHuman(lastIn.Body) {
		body := "Of course. A colleague from Reportage will message you here shortly."
		if strings.HasPrefix(row.Language, "ar") {
			body = "بالتأكيد. سيراسلك زميل من ريبورتاج هنا قريباً."
		}
		t, err := c.db.Writer().CreateThreadTurn(ctx, store.CreateThreadTurnParams{ThreadID: id, Direction: "out", Author: "agent", Kind: "text", Body: body, State: "draft", Handoff: HandoffRequestedHuman, IdempotencyKey: outKey(id, now), Claims: "{}", OccurredAt: now, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return nil, err
		}
		if err := c.handoff(ctx, actor, id, HandoffRequestedHuman, "the customer asked for a person: "+truncate(lastIn.Body, 120)); err != nil {
			return nil, err
		}
		sent, err := c.dispatch(ctx, actor, row, t, false)
		if err != nil {
			return nil, err
		}
		return &sent, nil
	}
	if c.cfg.Model == nil {
		return nil, c.handoff(ctx, actor, id, HandoffNoModel, "no model is configured; a person replies")
	}
	var d ReplyDecision
	for attempt := 0; attempt < 2; attempt++ {
		b, err := c.brief(ctx, actor, row)
		if err != nil {
			return nil, err
		}
		names, err := c.projectNames(ctx, actor.WorkspaceID)
		if err != nil {
			return nil, err
		}
		d, err = c.decide(ctx, instructions(b, row.Language, names), turns)
		if err != nil {
			var me *assistant.ModelError
			reason := "the model did not answer"
			if errors.As(err, &me) {
				reason += ": " + me.Safe
			}
			return nil, c.handoff(ctx, actor, id, HandoffNoModel, reason)
		}
		// The customer named a project the enquiry did not have: record it
		// and ask again with that project's facts in the brief.
		if d.Project != "" && b.ProjectName == "" && row.EnquiryID.Valid && c.cfg.CRM != nil {
			if pid := matchProject(d.Project, names, c.projectIDs(ctx, actor.WorkspaceID)); pid != 0 {
				if _, err := c.cfg.CRM.SetEnquiryProject(ctx, actor, row.EnquiryID.Int64, pid); err == nil {
					continue
				}
			}
		}
		break
	}
	if d.Objection != nil && c.cfg.Outreach != nil && strings.TrimSpace(d.Objection.Text) != "" {
		var enq int64
		if row.EnquiryID.Valid {
			enq = row.EnquiryID.Int64
		}
		_, _ = c.cfg.Outreach.RecordObjection(ctx, actor, outreach.ObjectionInput{PersonID: row.PersonID, EnquiryID: enq, Kind: d.Objection.Kind, Text: d.Objection.Text, Source: fmt.Sprintf("conversation:%d", id)})
	}
	if d.Handoff == HandoffStop {
		return nil, c.stop(ctx, actor, id, row.PersonID, lastIn.Body)
	}
	body := strings.TrimSpace(d.Reply)
	if body == "" {
		return nil, c.handoff(ctx, actor, id, HandoffQuestion, "the agent had no reply: "+firstNonEmpty(d.HandoffNote, "a person answers"))
	}
	// The reply is checked against the approved facts before it goes
	// anywhere. A figure no fact carries holds it; the agent never sends a
	// price, a discount or a percentage that nobody approved.
	state, reason, handoff := "draft", "", d.Handoff
	var report *brands.ClaimReport
	if c.cfg.Brands != nil {
		var brandID, projectID int64
		if row.EnquiryID.Valid {
			if enq, err := c.db.Reader().GetEnquiry(ctx, store.GetEnquiryParams{ID: row.EnquiryID.Int64, WorkspaceID: actor.WorkspaceID}); err == nil && enq.ProjectID.Valid {
				projectID = enq.ProjectID.Int64
				if bid, err := c.cfg.Brands.BrandOfProject(ctx, actor, projectID); err == nil {
					brandID = bid
				}
			}
		}
		r, err := c.cfg.Brands.CheckCopy(ctx, actor, brandID, projectID, 0, 0, body)
		if err != nil {
			return nil, err
		}
		report = &r
		for _, cl := range r.Claims {
			if cl.Status != "approved" {
				state, handoff = "held", HandoffOutsideFacts
				reason = "the reply quotes " + cl.Text + ", which is not an approved fact; not sent"
				break
			}
		}
	}
	claims, _ := json.Marshal(report)
	if report == nil {
		claims = []byte("{}")
	}
	t, err := c.db.Writer().CreateThreadTurn(ctx, store.CreateThreadTurnParams{ThreadID: id, Direction: "out", Author: "agent", Kind: "text", Body: body, State: state, Reason: reason, Handoff: handoff, IdempotencyKey: outKey(id, now), Claims: string(claims), OccurredAt: now, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return nil, err
	}
	switch handoff {
	case HandoffOutsideFacts:
		if err := c.handoff(ctx, actor, id, handoff, reason); err != nil {
			return nil, err
		}
		out, err := c.getTurn(ctx, actor.WorkspaceID, t.ID)
		return &out, err
	case HandoffRequestedHuman, HandoffObjection, HandoffQuestion:
		if err := c.handoff(ctx, actor, id, handoff, firstNonEmpty(d.HandoffNote, "the agent asked for a person")); err != nil {
			return nil, err
		}
	}
	out, err := c.dispatch(ctx, actor, row, t, false)
	if err != nil {
		return nil, err
	}
	if d.Done && handoff == "" {
		if err := c.read(ctx, actor, id); err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// decide asks the model for the next turn with a forced tool call.
func (c *Conversations) decide(ctx context.Context, system string, turns []store.ThreadTurn) (ReplyDecision, error) {
	var msgs []assistant.Message
	for _, t := range turns {
		switch {
		case t.State == "discarded", t.State == "held", t.State == "failed":
			continue
		case t.Direction == "in":
			body := t.Body
			if t.Kind == "unsupported" {
				body = "[" + t.Body + "]"
			}
			msgs = appendMessage(msgs, assistant.RoleUser, body)
		case t.State == "sent", t.State == "delivered", t.State == "read":
			msgs = appendMessage(msgs, assistant.RoleAssistant, t.Body)
		}
	}
	if len(msgs) == 0 || msgs[0].Role != assistant.RoleUser {
		msgs = append([]assistant.Message{{Role: assistant.RoleUser, Content: []assistant.Block{{Type: "text", Text: "(the customer wrote in)"}}}}, msgs...)
	}
	req := assistant.Request{System: system, Messages: msgs, Tools: []assistant.ToolSpec{{Name: replyTool, Description: "Decide the next turn of the conversation.", InputSchema: replySchema}}, ToolChoice: replyTool, MaxTokens: 600}
	resp, err := c.cfg.Model.Complete(ctx, req)
	if err != nil {
		return ReplyDecision{}, err
	}
	for _, b := range resp.Content {
		if b.Type == "tool_use" && b.ToolName == replyTool {
			var d ReplyDecision
			if err := json.Unmarshal(b.Input, &d); err != nil {
				return ReplyDecision{}, &assistant.ModelError{Safe: "the model's answer was not valid"}
			}
			switch d.Handoff {
			case "", HandoffRequestedHuman, HandoffObjection, HandoffQuestion, HandoffStop:
			default:
				d.Handoff = HandoffQuestion
			}
			return d, nil
		}
	}
	return ReplyDecision{}, &assistant.ModelError{Safe: "the model returned no reply"}
}

// appendMessage adds a turn, merging consecutive turns from one side: the
// model API wants the roles to alternate.
func appendMessage(msgs []assistant.Message, role assistant.Role, text string) []assistant.Message {
	if n := len(msgs); n > 0 && msgs[n-1].Role == role {
		msgs[n-1].Content = append(msgs[n-1].Content, assistant.Block{Type: "text", Text: text})
		return msgs
	}
	return append(msgs, assistant.Message{Role: role, Content: []assistant.Block{{Type: "text", Text: text}}})
}

func (c *Conversations) projectIDs(ctx context.Context, workspaceID int64) map[string]int64 {
	out := map[string]int64{}
	rows, err := c.db.Reader().ListProjects(ctx, workspaceID)
	if err != nil {
		return out
	}
	for _, p := range rows {
		out[strings.ToLower(strings.TrimSpace(p.Name))] = p.ID
	}
	return out
}

func matchProject(named string, names []string, ids map[string]int64) int64 {
	n := strings.ToLower(strings.TrimSpace(named))
	if id, ok := ids[n]; ok {
		return id
	}
	for _, name := range names {
		if strings.Contains(n, strings.ToLower(name)) || strings.Contains(strings.ToLower(name), n) {
			return ids[strings.ToLower(name)]
		}
	}
	return 0
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return b
}
