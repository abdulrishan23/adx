package api

import (
	"net/http"

	"github.com/WokeWorks/adx/server/core/connections"
)

// CapabilityMatrixResponse is GET /api/connections/capabilities: what each
// connector can and cannot do for ADX, and what this server has wired.
type CapabilityMatrixResponse struct {
	Capabilities []connections.ConnectorCapability `json:"capabilities"`
	LiveWrites   bool                              `json:"liveWrites"`
}

func (h *handlers) capabilityMatrix(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, CapabilityMatrixResponse{Capabilities: connections.Catalog(h.wiring), LiveWrites: h.wiring.LiveWrites})
}
