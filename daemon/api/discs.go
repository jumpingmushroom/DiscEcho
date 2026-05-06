package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// startDiscRequest is the wire format for POST /api/discs/:id/start.
type startDiscRequest struct {
	ProfileID      string `json:"profile_id"`
	CandidateIndex int    `json:"candidate_index"`
}

// StartDisc creates a job for the given disc + profile and queues it on
// the orchestrator.
func (h *Handlers) StartDisc(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req startDiscRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if req.ProfileID == "" {
		writeError(w, http.StatusBadRequest, "profile_id required")
		return
	}

	disc, err := h.Store.GetDisc(r.Context(), id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "disc not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Promote chosen candidate by mutating the in-memory disc; the
	// orchestrator only re-reads disc metadata, not candidate index, so
	// this carries the user choice into the pipeline. M1.2 may persist
	// the choice back to the DB; for M1.1 the orchestrator's job row is
	// the source of truth once submitted.
	if req.CandidateIndex >= 0 && req.CandidateIndex < len(disc.Candidates) {
		c := disc.Candidates[req.CandidateIndex]
		disc.MetadataID = c.MBID
		disc.MetadataProvider = c.Source
		disc.Title = c.Title
		disc.Year = c.Year
	}

	job, err := h.Orchestrator.Submit(r.Context(), disc.ID, req.ProfileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// IdentifyDisc is a stub in M1.1 — real classification happens
// automatically when a disc is inserted (M1.2 wires the udev →
// classifier flow). This endpoint just returns the disc's current
// candidates so the UI has a way to refresh.
func (h *Handlers) IdentifyDisc(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	disc, err := h.Store.GetDisc(r.Context(), id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "disc not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"disc":       disc,
		"candidates": disc.Candidates,
	})
}
