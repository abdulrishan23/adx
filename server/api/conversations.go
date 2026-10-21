package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/conversations"
	"github.com/WokeWorks/adx/server/core/qualification"
	"github.com/WokeWorks/adx/server/providers/whatsapp"
)

type ThreadsResponse struct {
	Conversations []conversations.ThreadSummary `json:"conversations"`
}

type ConversationMessageInput struct {
	Body string `json:"body"`
}

type NoteInput struct {
	Note string `json:"note"`
}

// InstructionsResponse is the agent's brief for a thread, for the screen.
type InstructionsResponse struct {
	Instructions string `json:"instructions"`
}

// WhatsAppWebhookResponse says what a delivery did.
type WhatsAppWebhookResponse struct {
	Messages  int      `json:"messages"`
	Replies   int      `json:"replies"`
	Statuses  int      `json:"statuses"`
	Duplicate int      `json:"duplicate"`
	Ignored   []string `json:"ignored"`
}

func (h *handlers) listThreads(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.conversations.List(r.Context(), actorFrom(r), r.URL.Query().Get("state"), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ThreadsResponse{Conversations: list})
}

func (h *handlers) conversationReadiness(w http.ResponseWriter, r *http.Request) {
	st, err := h.conversations.Readiness(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *handlers) conversationsPending(w http.ResponseWriter, r *http.Request) {
	wt, err := h.conversations.Pending(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wt)
}

func (h *handlers) getConversation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conversation")
	if !ok {
		return
	}
	d, err := h.conversations.Get(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) conversationInstructions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conversation")
	if !ok {
		return
	}
	text, _, err := h.conversations.Instructions(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, InstructionsResponse{Instructions: text})
}

func (h *handlers) conversationReply(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conversation")
	if !ok {
		return
	}
	t, err := h.conversations.Reply(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *handlers) conversationSend(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conversation")
	if !ok {
		return
	}
	var in ConversationMessageInput
	if !readJSON(w, r, &in) {
		return
	}
	t, err := h.conversations.Send(r.Context(), actorFrom(r), id, in.Body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *handlers) conversationTake(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conversation")
	if !ok {
		return
	}
	var in NoteInput
	if !readJSON(w, r, &in) {
		return
	}
	c, err := h.conversations.Take(r.Context(), actorFrom(r), id, in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) conversationHandBack(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conversation")
	if !ok {
		return
	}
	c, err := h.conversations.HandBack(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) conversationClose(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conversation")
	if !ok {
		return
	}
	var in NoteInput
	if !readJSON(w, r, &in) {
		return
	}
	c, err := h.conversations.Close(r.Context(), actorFrom(r), id, in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) conversationRead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conversation")
	if !ok {
		return
	}
	c, err := h.conversations.Read(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) conversationConfirm(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conversation")
	if !ok {
		return
	}
	c, err := h.conversations.Confirm(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) conversationCorrect(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conversation")
	if !ok {
		return
	}
	var in qualification.CorrectionInput
	if !readJSON(w, r, &in) {
		return
	}
	c, err := h.conversations.Correct(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) conversationTurnApprove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "turn")
	if !ok {
		return
	}
	t, err := h.conversations.Approve(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *handlers) conversationTurnDiscard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "turn")
	if !ok {
		return
	}
	var in NoteInput
	if !readJSON(w, r, &in) {
		return
	}
	t, err := h.conversations.Discard(r.Context(), actorFrom(r), id, in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// simulateInbound feeds a message in as the webhook would, without Meta.
// Dev only: it is how the thread is walked on a laptop with no Business
// account, and it never exists on a server with ADX_ENV unset.
func (h *handlers) simulateInbound(w http.ResponseWriter, r *http.Request) {
	var in conversations.InboundMessage
	if !readJSON(w, r, &in) {
		return
	}
	actor := actorFrom(r)
	out, err := h.conversations.Receive(r.Context(), core.SystemIn(core.ViaJob, actor.WorkspaceID), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// whatsappVerify answers Meta's subscription check with the challenge
// when the verify token matches.
func (h *handlers) whatsappVerify(w http.ResponseWriter, r *http.Request) {
	q := map[string]string{}
	for _, k := range []string{"hub.mode", "hub.verify_token", "hub.challenge"} {
		q[k] = r.URL.Query().Get(k)
	}
	challenge, ok := whatsapp.VerifyChallenge(h.whatsappWebhook.VerifyToken, q)
	if !ok {
		writeError(w, http.StatusForbidden, "verify token does not match")
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(challenge))
}

// whatsappWebhook is Meta's delivery of customer messages and statuses.
// It is public, so every delivery must be signed with the app secret; a
// bad signature is 403 and nothing is recorded. A delivery for a Business
// number this server does not send from is acknowledged and ignored. The
// workspace is Reportage's while one workspace exists; a second client on
// WhatsApp needs its own number and a mapping that does not exist yet.
func (h *handlers) whatsappWebhookPost(w http.ResponseWriter, r *http.Request) {
	ev, err := whatsapp.ReadWebhook(r, h.whatsappWebhook.AppSecret)
	switch {
	case errors.Is(err, whatsapp.ErrBadSignature):
		writeError(w, http.StatusForbidden, "webhook signature is invalid")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out := WhatsAppWebhookResponse{Ignored: ev.Ignored}
	if h.whatsappWebhook.PhoneNumberID != "" && ev.PhoneNumberID != "" && ev.PhoneNumberID != h.whatsappWebhook.PhoneNumberID {
		out.Ignored = append(out.Ignored, "phone number id "+ev.PhoneNumberID+" is not this server's")
		writeJSON(w, http.StatusAccepted, out)
		return
	}
	workspaceID, err := h.singleWorkspace(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	actor := core.SystemIn(core.ViaJob, workspaceID)
	for _, m := range ev.Messages {
		res, err := h.conversations.Receive(r.Context(), actor, m)
		if err != nil {
			h.log.WarnContext(r.Context(), "whatsapp message not recorded", "ref", m.ProviderRef, "error", err)
			h.fail(w, r, err)
			return
		}
		out.Messages++
		if res.Duplicate {
			out.Duplicate++
		}
		if res.Reply != nil {
			out.Replies++
		}
	}
	for _, s := range ev.Statuses {
		if ok, err := h.conversations.UpdateStatus(r.Context(), actor, s); err != nil {
			h.fail(w, r, err)
			return
		} else if ok {
			out.Statuses++
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// singleWorkspace is the workspace a WhatsApp delivery lands in: the only
// one, while there is only one.
func (h *handlers) singleWorkspace(r *http.Request) (int64, error) {
	if h.connections == nil {
		return core.DefaultWorkspace, nil
	}
	ids, err := h.connections.WorkspaceIDs(r.Context())
	if err != nil {
		return 0, err
	}
	if len(ids) != 1 {
		return 0, core.Invalid("the WhatsApp number is not mapped to a workspace, and ADX serves %d clients", len(ids))
	}
	return ids[0], nil
}
