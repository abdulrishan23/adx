package api

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/pages"
)

type PagesResponse struct {
	Pages      []pages.PageSummary  `json:"pages"`
	Publishing pages.PublishingGate `json:"publishing"`
}

type PublishPageInput struct {
	Version int64 `json:"version"`
}

type SubmissionsResponse struct {
	Submissions []pages.Submission `json:"submissions"`
}

// pageCSP allows no script at all: a page is data the server rendered.
const pageCSP = "default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'self'"

func (h *handlers) listPages(w http.ResponseWriter, r *http.Request) {
	list, err := h.pages.ListSummaries(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, PagesResponse{Pages: list, Publishing: h.pages.Publishing()})
}

func (h *handlers) createPage(w http.ResponseWriter, r *http.Request) {
	var in pages.PageInput
	if !readJSON(w, r, &in) {
		return
	}
	d, err := h.pages.Create(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *handlers) draftPage(w http.ResponseWriter, r *http.Request) {
	var in pages.PageDraftInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.pages.Draft(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *handlers) getPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "page")
	if !ok {
		return
	}
	d, err := h.pages.Get(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) savePageVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "page")
	if !ok {
		return
	}
	var in pages.PageVersionInput
	if !readJSON(w, r, &in) {
		return
	}
	v, err := h.pages.SaveVersion(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

// previewPage returns a version's HTML for the editor's sandboxed frame.
func (h *handlers) previewPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "page")
	if !ok {
		return
	}
	version, err := strconv.ParseInt(chi.URLParam(r, "version"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid version")
		return
	}
	html, err := h.pages.Render(r.Context(), actorFrom(r), id, version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeHTML(w, html, true)
}

func (h *handlers) publishPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "page")
	if !ok {
		return
	}
	var in PublishPageInput
	if !readJSON(w, r, &in) {
		return
	}
	d, err := h.pages.Publish(r.Context(), actorFrom(r), id, in.Version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) rollbackPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "page")
	if !ok {
		return
	}
	d, err := h.pages.Rollback(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) pageReadiness(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "page")
	if !ok {
		return
	}
	rd, err := h.pages.Readiness(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rd)
}

func (h *handlers) pageSubmissions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "page")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.pages.Submissions(r.Context(), actorFrom(r), id, limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, SubmissionsResponse{Submissions: list})
}

// Public routes. No session: these are what a visitor's browser calls.

func (h *handlers) publicPage(w http.ResponseWriter, r *http.Request) {
	html, err := h.pages.ServePublished(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		h.publicFail(w, err)
		return
	}
	writeHTML(w, html, false)
}

func (h *handlers) publicThanks(w http.ResponseWriter, r *http.Request) {
	html, err := h.pages.ThankYou(r.Context(), chi.URLParam(r, "slug"), r.URL.Query().Get("test") == "1")
	if err != nil {
		h.publicFail(w, err)
		return
	}
	writeHTML(w, html, false)
}

// publicForm takes a visitor's post. Fields come from the form body; the
// project and version are the hidden fields the page carried; attribution
// is whatever utm, click id or referrer the page's URL had, passed through
// as query parameters on the action or as hidden fields. The response is a
// thank-you page, or JSON for callers that ask for it.
func (h *handlers) publicForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "could not read the form")
		return
	}
	fields := map[string]string{}
	attribution := map[string]string{}
	for k, v := range r.Form {
		if len(v) == 0 {
			continue
		}
		switch {
		case strings.HasPrefix(k, "_"), k == "website":
		case strings.HasPrefix(k, "utm_"), k == "gclid", k == "gbraid", k == "wbraid", k == "fbclid", k == "referrer", k == "landing":
			attribution[k] = v[0]
		default:
			fields[k] = v[0]
		}
	}
	if ref := r.Referer(); ref != "" && attribution["landing"] == "" {
		attribution["landing"] = ref
	}
	project, _ := strconv.ParseInt(r.Form.Get("_project"), 10, 64)
	version, _ := strconv.ParseInt(r.Form.Get("_version"), 10, 64)
	sub, err := h.pages.Submit(r.Context(), pages.SubmissionInput{
		Slug: chi.URLParam(r, "slug"), Fields: fields, ProjectID: project, VersionID: version, TestToken: r.Form.Get("_test"), Honeypot: r.Form.Get("website"),
		RemoteIP: clientIP(r), UserAgent: r.UserAgent(), Attribution: attribution,
	})
	wantsJSON := strings.Contains(r.Header.Get("Accept"), "application/json")
	if err != nil {
		var se *pages.SubmitError
		if ok := asSubmitError(err, &se); ok {
			if wantsJSON {
				writeJSON(w, se.Status, map[string]any{"error": se.Message, "reason": se.Reason, "submission": sub})
				return
			}
			if se.Status == http.StatusOK {
				// A honeypot hit gets the same thank-you a person would.
				http.Redirect(w, r, "/p/"+chi.URLParam(r, "slug")+"/thanks", http.StatusSeeOther)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(se.Status)
			_, _ = w.Write([]byte(se.Message))
			return
		}
		h.log.ErrorContext(r.Context(), "form submission failed", "error", err)
		writeError(w, http.StatusInternalServerError, "something went wrong")
		return
	}
	if wantsJSON {
		writeJSON(w, http.StatusCreated, sub)
		return
	}
	target := "/p/" + chi.URLParam(r, "slug") + "/thanks"
	if sub.Test {
		target += "?test=1"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func asSubmitError(err error, target **pages.SubmitError) bool {
	se, ok := err.(*pages.SubmitError)
	if ok {
		*target = se
	}
	return ok
}

func (h *handlers) publicFail(w http.ResponseWriter, err error) {
	var ce *core.Error
	if errorsAs(err, &ce) && ce.Kind == core.KindNotFound {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not found"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte("Something went wrong"))
}

func writeHTML(w http.ResponseWriter, html []byte, sandbox bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	csp := pageCSP
	if sandbox {
		csp += "; sandbox allow-forms"
	}
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(html)
}

// clientIP trusts the proxy's X-Forwarded-For first hop; the deploy runbook
// puts ADX behind Caddy.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func errorsAs(err error, target **core.Error) bool { return errors.As(err, target) }
