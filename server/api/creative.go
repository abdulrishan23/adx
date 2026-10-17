package api

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assets"
	"github.com/WokeWorks/adx/server/core/creative"
)

type AssetsResponse struct {
	Assets []creative.Asset `json:"assets"`
}

type CreativesResponse struct {
	Creatives []creative.Creative `json:"creatives"`
}

type CreativeResponse struct {
	Creative creative.Creative  `json:"creative"`
	Versions []creative.Version `json:"versions"`
}

type BrandKitResponse struct {
	Kit   creative.BrandKitVersion `json:"kit"`
	Saved bool                     `json:"saved"`
}

type PutBrandKitInput struct {
	Kit assets.BrandKit `json:"kit"`
}

type SaveDocumentInput struct {
	Document assets.Document `json:"document"`
}

type CreativeQueueResponse struct {
	Items        []creative.QueueItem  `json:"items"`
	Capabilities creative.Capabilities `json:"capabilities"`
}

type ReviewInput struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

// uploadAsset takes a multipart form with a "file" part and optional
// project_id and kind fields. 25 MB cap.
func (h *handlers) uploadAsset(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 25<<20)
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		h.fail(w, r, core.Invalid("upload must be a multipart form under 25 MB"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		h.fail(w, r, core.Invalid("a file part is required"))
		return
	}
	defer file.Close()
	projectID, _ := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	kind := r.FormValue("kind")
	if kind == "" {
		kind = "render"
	}
	asset, err := h.creative.UploadAsset(r.Context(), actorFrom(r), projectID, kind, header.Filename, header.Header.Get("Content-Type"), file)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, asset)
}

func (h *handlers) listAssets(w http.ResponseWriter, r *http.Request) {
	projectID, _ := strconv.ParseInt(r.URL.Query().Get("project_id"), 10, 64)
	list, err := h.creative.ListAssets(r.Context(), actorFrom(r), projectID, r.URL.Query().Get("kind"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, AssetsResponse{Assets: list})
}

// serveAsset streams a file by key with its stored content type. SVG is
// served with a strict content security policy because it can carry script.
func (h *handlers) serveAsset(w http.ResponseWriter, r *http.Request) {
	rc, ref, err := h.creative.Open(r.Context(), actorFrom(r), chi.URLParam(r, "key"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", ref.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(ref.Size, 10))
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data:; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", "attachment")
	}
	_, _ = io.Copy(w, rc)
}

func (h *handlers) getBrandKit(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(chi.URLParam(r, "projectID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	kit, saved, err := h.creative.GetBrandKit(r.Context(), actorFrom(r), projectID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, BrandKitResponse{Kit: kit, Saved: saved})
}

func (h *handlers) putBrandKit(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(chi.URLParam(r, "projectID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	var in PutBrandKitInput
	if !readJSON(w, r, &in) {
		return
	}
	kit, err := h.creative.PutBrandKit(r.Context(), actorFrom(r), projectID, in.Kit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, kit)
}

func (h *handlers) listCreatives(w http.ResponseWriter, r *http.Request) {
	projectID, _ := strconv.ParseInt(r.URL.Query().Get("project_id"), 10, 64)
	list, err := h.creative.List(r.Context(), actorFrom(r), projectID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, CreativesResponse{Creatives: list})
}

func (h *handlers) createCreative(w http.ResponseWriter, r *http.Request) {
	var in creative.CreativeInput
	if !readJSON(w, r, &in) {
		return
	}
	c, v, err := h.creative.Create(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, CreativeResponse{Creative: c, Versions: []creative.Version{v}})
}

func (h *handlers) getCreative(w http.ResponseWriter, r *http.Request) {
	id, ok := h.creativeID(w, r)
	if !ok {
		return
	}
	c, versions, err := h.creative.Get(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, CreativeResponse{Creative: c, Versions: versions})
}

func (h *handlers) saveCreativeDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := h.creativeID(w, r)
	if !ok {
		return
	}
	var in SaveDocumentInput
	if !readJSON(w, r, &in) {
		return
	}
	v, err := h.creative.SaveDocument(r.Context(), actorFrom(r), id, in.Document)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *handlers) creativeAction(action func(*http.Request, int64) (creative.Version, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := h.creativeID(w, r)
		if !ok {
			return
		}
		v, err := action(r, id)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func (h *handlers) reviewCreative(w http.ResponseWriter, r *http.Request) {
	id, ok := h.creativeID(w, r)
	if !ok {
		return
	}
	var in ReviewInput
	if !readJSON(w, r, &in) {
		return
	}
	v, err := h.creative.ReviewVersion(r.Context(), actorFrom(r), id, in.Decision, in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *handlers) exportCreative(w http.ResponseWriter, r *http.Request) {
	id, ok := h.creativeID(w, r)
	if !ok {
		return
	}
	version, err := strconv.ParseInt(chi.URLParam(r, "version"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid version")
		return
	}
	rc, v, err := h.creative.Export(r.Context(), actorFrom(r), id, version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", v.Output.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\"creative-"+strconv.FormatInt(id, 10)+"-v"+strconv.FormatInt(version, 10)+".svg\"")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data:; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, rc)
}

func (h *handlers) creativeID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative id")
		return 0, false
	}
	return id, true
}

func (h *handlers) creativeQueue(w http.ResponseWriter, r *http.Request) {
	items, err := h.creative.Queue(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, CreativeQueueResponse{Items: items, Capabilities: h.creative.Capabilities()})
}

// startRender queues a render run. Without a job runner it renders inline,
// which small servers and tests use.
func (h *handlers) startRender(w http.ResponseWriter, r *http.Request) {
	id, ok := h.creativeID(w, r)
	if !ok {
		return
	}
	if h.runs == nil {
		v, err := h.creative.Render(r.Context(), actorFrom(r), id)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
		return
	}
	run, created, err := h.creative.StartRender(r.Context(), actorFrom(r), h.runs, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, StartRunResponse{Run: run, Created: created})
}

func (h *handlers) creativeReadiness(w http.ResponseWriter, r *http.Request) {
	id, ok := h.creativeID(w, r)
	if !ok {
		return
	}
	rd, err := h.creative.Readiness(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rd)
}
