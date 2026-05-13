package api

import (
	"encoding/json"
	"net/http"
)

// GetState returns the full snapshot for /api/state. The payload mirrors
// the SSE state.snapshot bootstrap so the UI can render either a fresh
// SSE connection or a cold REST GET identically.
func (h *Handlers) GetState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	drives, err := h.Store.ListDrives(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jobs, err := h.Store.ListActiveAndRecentJobs(ctx, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	discs, err := h.Store.ListRecentDiscs(ctx, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	profiles, err := h.Store.ListProfiles(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings, err := h.Store.GetAllSettings(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"drives":   drives,
		"jobs":     jobs,
		"discs":    discs,
		"profiles": profiles,
		"settings": settings,
		"stats":    h.computeStats(ctx),
	})
}

// writeJSON serializes body as JSON with the given status. JSON encode
// failures are logged via the response itself only when the header has
// not been written yet; once status is set we just swallow the error.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError emits a {"error": "..."} JSON document at the given status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
