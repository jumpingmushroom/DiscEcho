package api

import "net/http"

// GetSettings returns the full key/value map. Read-only in M1.1; PATCH
// support lands when the UI grows a settings editor.
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	all, err := h.Store.GetAllSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, all)
}
