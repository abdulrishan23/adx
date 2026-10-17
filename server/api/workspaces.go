package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/workspaces"
)

type WorkspacesResponse struct {
	Workspaces []workspaces.Workspace `json:"workspaces"`
}

type MembersResponse struct {
	Members []workspaces.Member `json:"members"`
}

type SwitchWorkspaceInput struct {
	WorkspaceID int64 `json:"workspaceId"`
}

func (h *handlers) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	list, err := h.workspaces.Mine(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, WorkspacesResponse{Workspaces: list})
}

func (h *handlers) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var in workspaces.WorkspaceInput
	if !readJSON(w, r, &in) {
		return
	}
	ws, err := h.workspaces.Create(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, ws)
}

func (h *handlers) listMembers(w http.ResponseWriter, r *http.Request) {
	list, err := h.workspaces.Members(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MembersResponse{Members: list})
}

func (h *handlers) addMember(w http.ResponseWriter, r *http.Request) {
	var in workspaces.MemberInput
	if !readJSON(w, r, &in) {
		return
	}
	m, err := h.workspaces.AddMember(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *handlers) removeMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "user")
	if !ok {
		return
	}
	if err := h.workspaces.RemoveMember(r.Context(), actorFrom(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// switchWorkspace moves the session to another workspace. A real session
// gets a new cookie from identity.Switch; the dev admin, who has no session
// row, gets the dev workspace cookie that actor.go reads.
func (h *handlers) switchWorkspace(w http.ResponseWriter, r *http.Request) {
	var in SwitchWorkspaceInput
	if !readJSON(w, r, &in) {
		return
	}
	if in.WorkspaceID <= 0 {
		writeError(w, http.StatusBadRequest, "workspaceId is required")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if _, ok := sessionFrom(r); ok {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "sign in to continue")
			return
		}
		token, session, err := h.identity.Switch(r.Context(), core.ViaWeb, cookie.Value, in.WorkspaceID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		setSessionCookie(w, token, h.dev)
		expires := session.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		writeJSON(w, http.StatusOK, SessionResponse{
			User: &session.User, Role: session.Role, WorkspaceID: session.WorkspaceID, Workspace: session.Workspace, ExpiresAt: &expires,
		})
		return
	}
	if !h.dev {
		writeError(w, http.StatusUnauthorized, "sign in to continue")
		return
	}
	actor := actorFrom(r)
	mine, err := h.workspaces.Mine(r.Context(), actor)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for _, ws := range mine {
		if ws.ID == in.WorkspaceID {
			http.SetCookie(w, &http.Cookie{Name: devWorkspaceCookie, Value: formatID(ws.ID), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
			writeJSON(w, http.StatusOK, SessionResponse{Role: actor.Role, WorkspaceID: ws.ID, Workspace: ws.Name + " (dev)", Dev: true})
			return
		}
	}
	writeError(w, http.StatusNotFound, "no such workspace")
}

func (h *handlers) getIndustry(w http.ResponseWriter, r *http.Request) {
	state, err := h.workspaces.Industry(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *handlers) getIndustryVersion(w http.ResponseWriter, r *http.Request) {
	version, ok := pathID(w, r, "version", "version")
	if !ok {
		return
	}
	cfg, err := h.workspaces.IndustryVersion(r.Context(), actorFrom(r), version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// putIndustry reads the raw body first so permission-shaped keys are refused
// rather than silently dropped by the typed decode.
func (h *handlers) putIndustry(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "body must be JSON")
		return
	}
	if err := workspaces.RejectPermissionKeys(body); err != nil {
		h.fail(w, r, err)
		return
	}
	var doc workspaces.IndustryDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	cfg, err := h.workspaces.PutIndustry(r.Context(), actorFrom(r), doc)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}
