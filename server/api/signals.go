package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/signals"
)

type SignalEventsResponse struct {
	Events []signals.SignalEvent `json:"events"`
}

type CorrectSignalInput struct {
	Action   string `json:"action"`
	PersonID int64  `json:"personId"`
	Reason   string `json:"reason"`
}

type DeriveResponse struct {
	Derived int `json:"derived"`
}

type WebhookResponse struct {
	Accepted  bool   `json:"accepted"`
	Ignored   string `json:"ignored"`
	Duplicate bool   `json:"duplicate"`
	EventID   int64  `json:"eventId"`
}

func (h *handlers) getScoringRules(w http.ResponseWriter, r *http.Request) {
	st, err := h.signals.GetRules(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *handlers) putScoringRules(w http.ResponseWriter, r *http.Request) {
	var doc signals.ScoringDocument
	if !readJSON(w, r, &doc) {
		return
	}
	rules, err := h.signals.PutRules(r.Context(), actorFrom(r), doc)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, rules)
}

func (h *handlers) ingestEvent(w http.ResponseWriter, r *http.Request) {
	var in signals.SignalInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.signals.Ingest(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if out.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, out)
}

func (h *handlers) personScore(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "person")
	if !ok {
		return
	}
	v, err := h.signals.Score(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *handlers) unknownEvents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.signals.Unknown(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, SignalEventsResponse{Events: list})
}

func (h *handlers) correctEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "event")
	if !ok {
		return
	}
	var in CorrectSignalInput
	if !readJSON(w, r, &in) {
		return
	}
	c, err := h.signals.Correct(r.Context(), actorFrom(r), id, in.Action, in.PersonID, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) deriveSignals(w http.ResponseWriter, r *http.Request) {
	n, err := h.signals.Derive(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, DeriveResponse{Derived: n})
}

// ghlWebhook is the GoHighLevel webhook adapter. It is public (GoHighLevel
// calls it), so it needs the shared secret configured in ADX_WEBHOOK_SECRET
// in the X-ADX-Webhook-Secret header; without a configured secret it
// answers 503 and records nothing. Only the event types ADX can score are
// ingested; the rest are acknowledged and named as ignored.
func (h *handlers) ghlWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhookSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "webhooks are not configured: set ADX_WEBHOOK_SECRET and the same value in GoHighLevel")
		return
	}
	given := r.Header.Get("X-ADX-Webhook-Secret")
	if subtle.ConstantTimeCompare([]byte(given), []byte(h.webhookSecret)) != 1 {
		writeError(w, http.StatusUnauthorized, "webhook secret does not match")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || !json.Valid(body) {
		writeError(w, http.StatusBadRequest, "body must be JSON")
		return
	}
	in, ignored := parseGHLWebhook(body, r.Header.Get("X-Webhook-Id"))
	if ignored != "" {
		writeJSON(w, http.StatusAccepted, WebhookResponse{Ignored: ignored})
		return
	}
	workspaceID, err := h.webhookWorkspace(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	actor := core.SystemIn(core.ViaJob, workspaceID)
	out, err := h.signals.Ingest(r.Context(), actor, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, WebhookResponse{Accepted: true, Duplicate: out.Duplicate, EventID: out.Event.ID})
}

// webhookWorkspace finds the client a GoHighLevel event belongs to from its
// location. An event that names a location nobody connected is refused, so a
// stray event never lands in another client's records. Without a location,
// the event is accepted only while one workspace exists.
func (h *handlers) webhookWorkspace(ctx context.Context, body []byte) (int64, error) {
	var b struct {
		LocationID string `json:"locationId"`
	}
	_ = json.Unmarshal(body, &b)
	if b.LocationID != "" {
		acct, err := h.connections.AccountForLocation(ctx, b.LocationID)
		if err != nil {
			return 0, err
		}
		return acct.WorkspaceID, nil
	}
	ids, err := h.connections.WorkspaceIDs(ctx)
	if err != nil {
		return 0, err
	}
	if len(ids) != 1 {
		return 0, core.Invalid("the event names no locationId, and ADX serves more than one client")
	}
	return ids[0], nil
}

type ghlWebhookBody struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	ContactID string `json:"contactId"`
	Contact   string `json:"contact_id"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Timestamp string `json:"timestamp"`
	Stage     string `json:"pipelineStageId"`
	StageName string `json:"pipleineStage"`
	Status    string `json:"status"`
	Direction string `json:"direction"`
	Message   string `json:"messageType"`
}

// parseGHLWebhook maps a GoHighLevel event to a signal. The occurrence key
// is the provider's webhook id when it sends one, else a hash of the body,
// so a redelivery is the same event.
func parseGHLWebhook(body []byte, webhookID string) (signals.SignalInput, string) {
	var b ghlWebhookBody
	_ = json.Unmarshal(body, &b)
	contact := b.ContactID
	if contact == "" {
		contact = b.Contact
	}
	if contact == "" && b.Type != "" && strings.HasPrefix(b.Type, "Contact") {
		contact = b.ID
	}
	kind := ""
	payload := map[string]any{"type": b.Type}
	switch b.Type {
	case "OpportunityStageUpdate", "OpportunityStatusUpdate":
		kind = "stage_change"
		payload["stage"] = b.StageName
		payload["stageId"] = b.Stage
		payload["status"] = b.Status
	case "InboundMessage":
		if b.Direction != "" && b.Direction != "inbound" {
			return signals.SignalInput{}, "outbound message"
		}
		kind = "message_reply"
		payload["messageType"] = b.Message
	case "":
		return signals.SignalInput{}, "no event type"
	default:
		return signals.SignalInput{}, "event type " + b.Type + " is not scored"
	}
	key := webhookID
	if key == "" {
		sum := sha256.Sum256(body)
		key = "sha256:" + hex.EncodeToString(sum[:])
	}
	at := time.Now().UTC()
	if t, err := time.Parse(time.RFC3339, b.Timestamp); err == nil {
		at = t.UTC()
	}
	raw, _ := json.Marshal(payload)
	id := signals.SignalIdentity{Email: b.Email, Phone: b.Phone}
	if contact != "" {
		id.External = "ghl:" + contact
	}
	return signals.SignalInput{Source: "ghl.webhook", Kind: kind, OccurrenceKey: key, OccurredAt: at, Identity: id, Payload: raw}, ""
}
