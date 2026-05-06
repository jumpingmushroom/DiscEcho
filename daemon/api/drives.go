package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// ListDrives returns every known drive.
func (h *Handlers) ListDrives(w http.ResponseWriter, r *http.Request) {
	drives, err := h.Store.ListDrives(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, drives)
}

// GetDrive returns a single drive by ID.
func (h *Handlers) GetDrive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, err := h.Store.GetDrive(r.Context(), id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "drive not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// EjectDrive transitions the drive to the ejecting state. Real eject
// happens via the eject Tool wired in Phase G; this M1.1 stub just
// records the intent so the UI gets feedback.
func (h *Handlers) EjectDrive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Store.UpdateDriveState(r.Context(), id, state.DriveStateEjecting); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "drive not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
