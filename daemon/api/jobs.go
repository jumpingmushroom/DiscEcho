package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// JobDetail bundles the data the /jobs/[id] webui page needs in a
// single response: the job (with its steps) and the disc it ripped.
// The disc carries the title, year, cover/poster URL, and disc-type
// badge data that the running and terminal views both render.
type JobDetail struct {
	Job  state.Job  `json:"job"`
	Disc state.Disc `json:"disc"`
}

// JobLogsResponse is the wire shape of GET /api/jobs/:id/logs.
type JobLogsResponse struct {
	Lines  []state.LogLine `json:"lines"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

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

// GetJob returns a single job (with its steps) plus the disc it
// ripped. Used by the webui's /jobs/[id] detail page for terminal
// jobs that have aged out of the live `$jobs` snapshot.
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
	disc, err := h.Store.GetDisc(r.Context(), job.DiscID)
	if err != nil {
		// A job referencing a missing disc is an FK violation we
		// shouldn't see; surface it as 500 rather than hiding it.
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, JobDetail{Job: *job, Disc: *disc})
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

// ListJobLogs returns persisted log lines for a job, oldest-first,
// optionally filtered by step. Powers the LogPhaseViewer's terminal
// (and page-reload) data path; live jobs append-stream over SSE.
func (h *Handlers) ListJobLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.Store.GetJob(r.Context(), id); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	q := r.URL.Query()
	f := state.LogFilter{
		Step:   state.StepID(q.Get("step")),
		Limit:  parseIntOr(q.Get("limit"), 500),
		Offset: parseIntOr(q.Get("offset"), 0),
	}

	lines, total, err := h.Store.ListLogLines(r.Context(), id, f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if lines == nil {
		lines = []state.LogLine{}
	}
	writeJSON(w, http.StatusOK, JobLogsResponse{
		Lines: lines, Total: total, Limit: f.Limit, Offset: f.Offset,
	})
}

// DeleteJob removes a single terminal (done/failed/cancelled) job, its
// step rows, and its log lines. Running jobs reject with 409 — use
// CancelJob instead. Orphaned disc rows are pruned in the same
// transaction so the /history page doesn't grow phantom entries.
func (h *Handlers) DeleteJob(w http.ResponseWriter, r *http.Request) {
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
	switch job.State {
	case state.JobStateDone, state.JobStateFailed,
		state.JobStateCancelled, state.JobStateInterrupted:
	default:
		writeError(w, http.StatusConflict, "job is not in a terminal state")
		return
	}
	if err := h.Store.DeleteJobAndOrphans(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
