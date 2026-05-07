package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// ListProfiles returns every configured profile.
func (h *Handlers) ListProfiles(w http.ResponseWriter, r *http.Request) {
	ps, err := h.Store.ListProfiles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

// GetProfile returns a single profile by ID.
func (h *Handlers) GetProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := h.Store.GetProfile(r.Context(), id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// CreateProfile inserts a new profile. Daemon generates ID +
// timestamps; client-supplied values are ignored.
//
// 201 + Profile on success
// 400 on bad JSON
// 422 on validation failure (body: {field: msg, ...})
// 500 on store error
func (h *Handlers) CreateProfile(w http.ResponseWriter, r *http.Request) {
	var p state.Profile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	p.ID = ""
	p.CreatedAt = time.Time{}
	p.UpdatedAt = time.Time{}
	if errs := ValidateProfile(&p); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}
	if err := h.Store.CreateProfile(r.Context(), &p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Broadcaster != nil {
		h.Broadcaster.Publish(state.Event{
			Name:    "profile.changed",
			Payload: map[string]any{"profile": p},
		})
	}
	writeJSON(w, http.StatusCreated, p)
}

// UpdateProfile full-replaces a profile. ID comes from the URL; the
// body's id field is ignored. CreatedAt is preserved (the store
// doesn't touch it); UpdatedAt is refreshed.
func (h *Handlers) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var p state.Profile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	p.ID = id
	if errs := ValidateProfile(&p); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}
	if err := h.Store.UpdateProfile(r.Context(), &p); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fresh, err := h.Store.GetProfile(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Broadcaster != nil {
		h.Broadcaster.Publish(state.Event{
			Name:    "profile.changed",
			Payload: map[string]any{"profile": fresh},
		})
	}
	writeJSON(w, http.StatusOK, fresh)
}

// DeleteProfile removes a profile.
//
// 204 on success
// 404 unknown id
// 409 if the profile is referenced by any non-terminal job
//
//	(FK ON DELETE RESTRICT). User must cancel the job first.
//
// Seeded profiles aren't specially protected — daemon re-seeds on
// next start (existing idempotent seed pattern).
func (h *Handlers) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteProfile(r.Context(), id); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		if isFKConstraint(err) {
			writeError(w, http.StatusConflict, "profile is referenced by an active job; cancel the job first")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Broadcaster != nil {
		h.Broadcaster.Publish(state.Event{
			Name:    "profile.changed",
			Payload: map[string]any{"profile_id": id, "deleted": true},
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeValidationErrors serialises a []ValidationError as a flat
// {field: msg} JSON map and writes 422.
func writeValidationErrors(w http.ResponseWriter, errs []ValidationError) {
	out := make(map[string]string, len(errs))
	for _, e := range errs {
		out[e.Field] = e.Msg
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(out)
}

// isFKConstraint detects SQLite's foreign-key constraint failure.
// modernc.org/sqlite's exact error format has shifted across versions;
// cover both shapes that appear in the wild.
func isFKConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "FOREIGN KEY constraint") {
		return true
	}
	if strings.Contains(msg, "constraint failed") && strings.Contains(msg, "FOREIGN KEY") {
		return true
	}
	return false
}
