package api

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/studio"
)

type RendersRequest struct {
	Renders []studio.NewRender `json:"renders"`
}

type RendersResponse struct {
	Renders []studio.Render `json:"renders"`
}

type UploadsRequest struct {
	Ads []studio.UploadedAd `json:"ads"`
}

type ResolveRequest struct {
	Resolved bool `json:"resolved"`
}

func (h *handlers) studioAvailable(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if h.studio == nil {
		h.fail(w, r, core.Conflict("creative storage is not configured"))
		return false
	}
	return true
}

func idParam(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil
}

// studioRoute wraps a handler that needs the creative service and one id
// from the path. A malformed id is a 404, like a missing one.
func (h *handlers) studioRoute(param, what string, fn func(w http.ResponseWriter, r *http.Request, id int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.studioAvailable(w, r) {
			return
		}
		id, ok := idParam(r, param)
		if !ok {
			h.fail(w, r, core.NotFound("%s not found", what))
			return
		}
		fn(w, r, id)
	}
}

func respond[T any](h *handlers, w http.ResponseWriter, r *http.Request, status int, v T, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, status, v)
}

func (h *handlers) creativeOverview(w http.ResponseWriter, r *http.Request) {
	if !h.studioAvailable(w, r) {
		return
	}
	out, err := h.studio.Overview(r.Context(), actorFrom(r))
	respond(h, w, r, http.StatusOK, out, err)
}

func (h *handlers) setUpCreative(w http.ResponseWriter, r *http.Request) {
	if !h.studioAvailable(w, r) {
		return
	}
	var in studio.SetupInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.SetUp(r.Context(), actorFrom(r), in)
	respond(h, w, r, http.StatusCreated, out, err)
}

func (h *handlers) creativeProject(w http.ResponseWriter, r *http.Request, id int64) {
	out, err := h.studio.Project(r.Context(), actorFrom(r), id)
	respond(h, w, r, http.StatusOK, out, err)
}

func (h *handlers) updateCreativeSettings(w http.ResponseWriter, r *http.Request, id int64) {
	var in studio.SettingsInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.UpdateSettings(r.Context(), actorFrom(r), id, in)
	respond(h, w, r, http.StatusOK, out, err)
}

func (h *handlers) rotateReviewKey(w http.ResponseWriter, r *http.Request, id int64) {
	out, err := h.studio.RotateReviewKey(r.Context(), actorFrom(r), id)
	respond(h, w, r, http.StatusOK, out, err)
}

func (h *handlers) saveBrandKit(w http.ResponseWriter, r *http.Request, id int64) {
	var kit json.RawMessage
	if !readJSON(w, r, &kit) {
		return
	}
	out, err := h.studio.SaveBrandKit(r.Context(), actorFrom(r), id, kit)
	respond(h, w, r, http.StatusOK, out, err)
}

func (h *handlers) addRenders(w http.ResponseWriter, r *http.Request, id int64) {
	var in RendersRequest
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.AddRenders(r.Context(), actorFrom(r), id, in.Renders)
	respond(h, w, r, http.StatusCreated, RendersResponse{Renders: out}, err)
}

func (h *handlers) updateRender(w http.ResponseWriter, r *http.Request, id int64) {
	var in studio.RenderInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.UpdateRender(r.Context(), actorFrom(r), id, in)
	respond(h, w, r, http.StatusOK, out, err)
}

func (h *handlers) createConcept(w http.ResponseWriter, r *http.Request, id int64) {
	var in studio.NewConcept
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.CreateConcept(r.Context(), actorFrom(r), id, in)
	respond(h, w, r, http.StatusCreated, out, err)
}

func (h *handlers) generateConcepts(w http.ResponseWriter, r *http.Request, id int64) {
	var in studio.GenerateConceptsInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.GenerateConcepts(r.Context(), actorFrom(r), id, in)
	respond(h, w, r, http.StatusCreated, out, err)
}

func (h *handlers) saveConcept(w http.ResponseWriter, r *http.Request, id int64) {
	var in studio.ConceptInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.SaveConcept(r.Context(), actorFrom(r), id, in)
	respond(h, w, r, http.StatusOK, out, err)
}

func (h *handlers) addUploads(w http.ResponseWriter, r *http.Request, id int64) {
	var in UploadsRequest
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.AddUploads(r.Context(), actorFrom(r), id, in.Ads)
	respond(h, w, r, http.StatusCreated, out, err)
}

func (h *handlers) saveGenerated(w http.ResponseWriter, r *http.Request, id int64) {
	var in studio.GeneratedAd
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.SaveGenerated(r.Context(), actorFrom(r), id, in)
	respond(h, w, r, http.StatusOK, out, err)
}

func (h *handlers) creativeAd(w http.ResponseWriter, r *http.Request, id int64) {
	out, err := h.studio.Ad(r.Context(), actorFrom(r), id)
	respond(h, w, r, http.StatusOK, out, err)
}

func (h *handlers) saveDraft(w http.ResponseWriter, r *http.Request, id int64) {
	var in studio.AdDraft
	if !readJSON(w, r, &in) {
		return
	}
	if err := h.studio.SaveDraft(r.Context(), actorFrom(r), id, in); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) publishAd(w http.ResponseWriter, r *http.Request, id int64) {
	var in studio.PublishInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.Publish(r.Context(), actorFrom(r), id, in)
	respond(h, w, r, http.StatusCreated, out, err)
}

func (h *handlers) createVariation(w http.ResponseWriter, r *http.Request, id int64) {
	var in studio.VariationInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.CreateVariation(r.Context(), actorFrom(r), id, in)
	respond(h, w, r, http.StatusCreated, out, err)
}

func (h *handlers) resolveComment(w http.ResponseWriter, r *http.Request, id int64) {
	var in ResolveRequest
	if !readJSON(w, r, &in) {
		return
	}
	if err := h.studio.ResolveComment(r.Context(), actorFrom(r), id, in.Resolved); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// downloadCreatives streams a zip of current pictures. JPGs don't compress,
// so they're stored as they are.
func (h *handlers) downloadCreatives(w http.ResponseWriter, r *http.Request, id int64) {
	name, entries, err := h.studio.Download(r.Context(), actorFrom(r), id, r.URL.Query().Get("set") != "all")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	zw := zip.NewWriter(w)
	for _, e := range entries {
		if err := h.copyToZip(r, zw, e); err != nil {
			// Headers are gone, so the broken zip is the only signal left.
			h.log.ErrorContext(r.Context(), "download failed", "path", e.Path, "error", err)
			return
		}
	}
	if err := zw.Close(); err != nil {
		h.log.ErrorContext(r.Context(), "download failed", "error", err)
	}
}

func (h *handlers) copyToZip(r *http.Request, zw *zip.Writer, e studio.DownloadEntry) error {
	f, _, err := h.studio.OpenFile(r.Context(), actorFrom(r), e.SHA256)
	if err != nil {
		return err
	}
	defer f.Close()
	dst, err := zw.CreateHeader(&zip.FileHeader{Name: e.Path, Method: zip.Store})
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, f)
	return err
}

// uploadFile takes one multipart field named "file".
func (h *handlers) uploadFile(w http.ResponseWriter, r *http.Request) {
	if !h.studioAvailable(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 26<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		h.fail(w, r, core.Invalid("send the file as multipart form data"))
		return
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			h.fail(w, r, core.Invalid("no file was sent"))
			return
		}
		if err != nil {
			h.fail(w, r, core.Invalid("files can be up to 25 MB"))
			return
		}
		if part.FormName() != "file" {
			continue
		}
		ref, err := h.studio.Upload(r.Context(), actorFrom(r), part)
		respond(h, w, r, http.StatusCreated, ref, err)
		return
	}
}

// serveFile serves an uploaded file by its SHA-256. Its bytes never change,
// so browsers may keep it for good. Nothing it contains may run: an SVG
// opened directly is sandboxed.
func (h *handlers) serveFile(w http.ResponseWriter, r *http.Request) {
	if h.studio == nil {
		http.NotFound(w, r)
		return
	}
	f, ref, err := h.studio.OpenFile(r.Context(), core.Anonymous(core.ViaWeb), chi.URLParam(r, "sha"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", ref.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	// Fonts and pictures are drawn onto the editor's canvas, which needs CORS
	// to export it.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeContent(w, r, "", time.Time{}, f)
}

func (h *handlers) reviewBoard(w http.ResponseWriter, r *http.Request) {
	if !h.studioAvailable(w, r) {
		return
	}
	out, err := h.studio.Board(r.Context(), core.Anonymous(core.ViaWeb), chi.URLParam(r, "key"))
	respond(h, w, r, http.StatusOK, out, err)
}

func (h *handlers) reviewDecide(w http.ResponseWriter, r *http.Request) {
	if !h.studioAvailable(w, r) {
		return
	}
	var in studio.BoardDecision
	if !readJSON(w, r, &in) {
		return
	}
	if err := h.studio.Decide(r.Context(), core.Anonymous(core.ViaWeb), chi.URLParam(r, "key"), in); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) reviewComment(w http.ResponseWriter, r *http.Request) {
	if !h.studioAvailable(w, r) {
		return
	}
	var in studio.CommentInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.studio.Comment(r.Context(), core.Anonymous(core.ViaWeb), chi.URLParam(r, "key"), in)
	respond(h, w, r, http.StatusCreated, out, err)
}

func (h *handlers) reviewPick(w http.ResponseWriter, r *http.Request) {
	if !h.studioAvailable(w, r) {
		return
	}
	var in studio.PickInput
	if !readJSON(w, r, &in) {
		return
	}
	if err := h.studio.PickRender(r.Context(), core.Anonymous(core.ViaWeb), chi.URLParam(r, "key"), in); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
