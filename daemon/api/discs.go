package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

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
		// MBID is set for MusicBrainz candidates; TMDBID for TMDB
		// (movie/TV) candidates. Pick whichever is present so the
		// chosen candidate's ID actually persists.
		var metaID string
		switch {
		case c.MBID != "":
			metaID = c.MBID
		case c.TMDBID > 0:
			metaID = strconv.Itoa(c.TMDBID)
		}
		// Persist the chosen identity. The orchestrator re-reads the
		// disc row inside Submit, so without this the user choice is
		// dropped and the pipeline runs on the original auto-identified
		// candidate.
		if err := h.Store.UpdateDiscMetadata(r.Context(), disc.ID, c.Title, c.Year, c.Source, metaID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	job, err := h.Orchestrator.Submit(r.Context(), disc.ID, req.ProfileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// identifyRequest is the optional body for POST /api/discs/:id/identify.
// Both fields are optional: an empty body re-reads the stored disc.
type identifyRequest struct {
	Query     string `json:"query,omitempty"`
	MediaType string `json:"media_type,omitempty"` // 'movie' | 'tv' | 'both' (default both)
}

// IdentifyDisc returns the disc plus its candidates. If the body
// contains a non-empty Query, it triggers a manual TMDB search for the
// chosen media type and persists the new candidates back onto the disc.
// Empty body → returns the current stored disc + candidates.
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

	if r.ContentLength == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"disc": disc, "candidates": disc.Candidates})
		return
	}

	var req identifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}

	if req.Query == "" {
		writeJSON(w, http.StatusOK, map[string]any{"disc": disc, "candidates": disc.Candidates})
		return
	}

	if h.TMDB == nil {
		writeError(w, http.StatusServiceUnavailable, "TMDB not configured")
		return
	}
	mediaType := req.MediaType
	if mediaType == "" {
		mediaType = "both"
	}
	var cands []state.Candidate
	switch mediaType {
	case "movie":
		cands, err = h.TMDB.SearchMovie(r.Context(), req.Query)
	case "tv":
		cands, err = h.TMDB.SearchTV(r.Context(), req.Query)
	case "both":
		cands, err = h.TMDB.SearchBoth(r.Context(), req.Query)
	default:
		writeError(w, http.StatusBadRequest, "media_type must be 'movie', 'tv', or 'both'")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if cands == nil {
		cands = []state.Candidate{}
	}

	disc.Candidates = cands
	if err := h.Store.UpdateDiscCandidates(r.Context(), disc.ID, cands); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"disc": disc, "candidates": cands})
}
