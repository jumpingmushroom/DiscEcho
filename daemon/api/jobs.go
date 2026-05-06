package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// ListJobs returns jobs filtered by ?state=, ?drive=, ?limit=, ?offset=.
func (h *Handlers) ListJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := state.JobFilter{
		State:   state.JobState(q.Get("state")),
		DriveID: q.Get("drive"),
	}
	if l, err := strconv.Atoi(q.Get("limit")); err == nil {
		f.Limit = l
	}
	if o, err := strconv.Atoi(q.Get("offset")); err == nil {
		f.Offset = o
	}
	jobs, err := h.Store.ListJobs(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

// GetJob returns a single job (with its steps) by ID.
func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.Store.GetJob(r.Context(), id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// CancelJob signals an active job to stop. Queued jobs are flipped to
// cancelled directly by the orchestrator.
func (h *Handlers) CancelJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Orchestrator.Cancel(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PauseJob is a 501 placeholder per M1.1 decision #6 — pause/resume is
// out of scope for this milestone.
func (h *Handlers) PauseJob(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "pause not implemented in M1.1")
}
