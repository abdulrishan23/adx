package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/WokeWorks/adx/server/core"
)

// ErrorResponse is the body of every error the API returns.
type ErrorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

// readJSON decodes a request body into dst. It rejects unknown fields so a
// typo in the frontend fails loudly instead of being ignored.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// fail writes err as a response. A core.Error keeps its message. Anything
// else is logged and shown as a generic 500, so internals never leak.
func (h *handlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ce *core.Error
	if errors.As(err, &ce) {
		writeError(w, statusFor(ce.Kind), ce.Message)
		return
	}
	h.log.ErrorContext(r.Context(), "request failed", "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "something went wrong")
}

func statusFor(k core.Kind) int {
	switch k {
	case core.KindInvalid:
		return http.StatusBadRequest
	case core.KindForbidden:
		return http.StatusForbidden
	case core.KindNotFound:
		return http.StatusNotFound
	case core.KindConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
