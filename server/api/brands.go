package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/plans"
)

type BrandsResponse struct {
	Brands     []brands.BrandSummary `json:"brands"`
	Unassigned []brands.BrandProject `json:"unassigned"`
}

type AssignProjectInput struct {
	ProjectID int64 `json:"projectId"`
	// BrandID 0 takes the project out of its brand.
	BrandID int64 `json:"brandId"`
}

type OfferStateInput struct {
	State brands.OfferState `json:"state"`
}

type OfferHistoryResponse struct {
	Offer    brands.Offer              `json:"offer"`
	Versions []brands.OfferVersion     `json:"versions"`
	Affected []brands.AffectedCreative `json:"affected"`
	// AffectedPlans are plan versions citing the offer; empty when plans
	// aren't wired.
	AffectedPlans []plans.AffectedPlan `json:"affectedPlans"`
}

type ProjectOffersResponse struct {
	BrandID int64          `json:"brandId"`
	Offers  []brands.Offer `json:"offers"`
}

type FactsResponse struct {
	Facts []brands.Fact `json:"facts"`
}

type RetireFactInput struct {
	Reason string `json:"reason"`
}

type ResolveConflictInput struct {
	WinnerID   int64  `json:"winnerId"`
	Resolution string `json:"resolution"`
}

func (h *handlers) listBrands(w http.ResponseWriter, r *http.Request) {
	actor := actorFrom(r)
	list, err := h.brands.List(r.Context(), actor)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	unassigned, err := h.brands.UnassignedProjects(r.Context(), actor)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, BrandsResponse{Brands: list, Unassigned: unassigned})
}

func (h *handlers) createBrand(w http.ResponseWriter, r *http.Request) {
	var in brands.BrandInput
	if !readJSON(w, r, &in) {
		return
	}
	b, err := h.brands.Create(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (h *handlers) getBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "brand")
	if !ok {
		return
	}
	d, err := h.brands.Get(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) assignProjectBrand(w http.ResponseWriter, r *http.Request) {
	var in AssignProjectInput
	if !readJSON(w, r, &in) {
		return
	}
	if err := h.brands.AssignProject(r.Context(), actorFrom(r), in.ProjectID, in.BrandID); err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *handlers) createOffer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "brand")
	if !ok {
		return
	}
	var in brands.OfferInput
	if !readJSON(w, r, &in) {
		return
	}
	o, err := h.brands.CreateOffer(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (h *handlers) projectOffers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "projectID", "project")
	if !ok {
		return
	}
	brandID, offers, err := h.brands.OffersForProject(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ProjectOffersResponse{BrandID: brandID, Offers: offers})
}

func (h *handlers) getOffer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "offer")
	if !ok {
		return
	}
	actor := actorFrom(r)
	o, versions, err := h.brands.OfferVersions(r.Context(), actor, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	affected, err := h.brands.OfferImpact(r.Context(), actor, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	affectedPlans := []plans.AffectedPlan{}
	if h.plans != nil {
		affectedPlans, err = h.plans.PlansUsingOffer(r.Context(), actor, id)
		if err != nil {
			h.fail(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, OfferHistoryResponse{Offer: o, Versions: versions, Affected: affected, AffectedPlans: affectedPlans})
}

func (h *handlers) reviseOffer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "offer")
	if !ok {
		return
	}
	var in brands.OfferInput
	if !readJSON(w, r, &in) {
		return
	}
	change, err := h.brands.ReviseOffer(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, change)
}

func (h *handlers) setOfferState(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "offer")
	if !ok {
		return
	}
	var in OfferStateInput
	if !readJSON(w, r, &in) {
		return
	}
	o, err := h.brands.SetOfferState(r.Context(), actorFrom(r), id, in.State)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *handlers) listFacts(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "brand")
	if !ok {
		return
	}
	q := r.URL.Query()
	projectID, _ := strconv.ParseInt(q.Get("project_id"), 10, 64)
	var (
		facts []brands.Fact
		err   error
	)
	if key := q.Get("key"); key != "" {
		facts, err = h.brands.FactHistory(r.Context(), actorFrom(r), id, key)
	} else {
		facts, err = h.brands.ListFacts(r.Context(), actorFrom(r), id, projectID)
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, FactsResponse{Facts: facts})
}

func (h *handlers) addFact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "brand")
	if !ok {
		return
	}
	var in brands.FactInput
	if !readJSON(w, r, &in) {
		return
	}
	change, err := h.brands.AddFact(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, change)
}

// claims answers what a draft may say about a project today or on ?at=.
func (h *handlers) claims(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "brand")
	if !ok {
		return
	}
	q := r.URL.Query()
	projectID, _ := strconv.ParseInt(q.Get("project_id"), 10, 64)
	at := time.Now()
	if s := q.Get("at"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "at must be a date like 2026-10-17")
			return
		}
		at = t
	}
	set, err := h.brands.Claims(r.Context(), actorFrom(r), id, projectID, at)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (h *handlers) approveFact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "fact")
	if !ok {
		return
	}
	f, err := h.brands.ApproveFact(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *handlers) retireFact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "fact")
	if !ok {
		return
	}
	var in RetireFactInput
	if !readJSON(w, r, &in) {
		return
	}
	f, err := h.brands.RetireFact(r.Context(), actorFrom(r), id, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *handlers) resolveConflict(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conflict")
	if !ok {
		return
	}
	var in ResolveConflictInput
	if !readJSON(w, r, &in) {
		return
	}
	c, err := h.brands.ResolveConflict(r.Context(), actorFrom(r), id, in.WinnerID, in.Resolution)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func pathID(w http.ResponseWriter, r *http.Request, param, what string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, param), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+what+" id")
		return 0, false
	}
	return id, true
}
