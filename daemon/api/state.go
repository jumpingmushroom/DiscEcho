package api

import (
	"context"
	"encoding/json"
	"net/http"
)

// GetState returns the full snapshot for /api/state. The payload mirrors
// the SSE state.snapshot bootstrap so the UI can render either a fresh
// SSE connection or a cold REST GET identically.
func (h *Handlers) GetState(w http.ResponseWriter, r *http.Request) {
	payload, err := h.buildSnapshot(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// buildSnapshot assembles the bootstrap payload shared by GET /api/state
// and the SSE state.snapshot event. Returns the first DB error so
// /api/state can surface a 500; SSE callers can swallow it to keep the
// stream open with a partially-empty snapshot.
func (h *Handlers) buildSnapshot(ctx context.Context) (map[string]any, error) {
	drives, err := h.Store.ListDrives(ctx)
	if err != nil {
		return nil, err
	}
	jobs, err := h.Store.ListActiveAndRecentJobs(ctx, 50)
	if err != nil {
		return nil, err
	}
	discs, err := h.Store.ListRecentDiscs(ctx, 50)
	if err != nil {
		return nil, err
	}
	profiles, err := h.Store.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := h.Store.GetAllSettings(ctx)
	if err != nil {
		return nil, err
	}
	currentByDrive, err := h.Store.CurrentDiscByDrive(ctx)
	if err != nil {
		return nil, err
	}
	for i := range drives {
		if id, ok := currentByDrive[drives[i].ID]; ok {
			drives[i].CurrentDiscID = id
		}
	}
	return map[string]any{
		"drives":   drives,
		"jobs":     jobs,
		"discs":    discs,
		"profiles": profiles,
		"settings": settings,
		"stats":    h.computeStats(ctx),
	}, nil
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
