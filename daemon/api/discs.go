package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
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
	// Fast-path duplicate guard: skip the candidate-metadata fetch
	// below for an obvious duplicate. This check is NOT atomic with
	// Submit — the authoritative, race-safe guard is under startMu
	// just before Submit. See that block and the startMu doc comment.
	hasActive, err := h.Store.DiscHasActiveJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hasActive {
		writeError(w, http.StatusConflict, "disc already has an active job")
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
		// If the picked candidate is a TMDB movie, fetch its runtime
		// from `/movie/{id}` so the DVD pipeline can sanity-check the
		// scanned title duration. Best-effort: a failure here logs
		// (via the error path) but doesn't block the rip.
		if c.MediaType == "movie" && c.TMDBID > 0 && h.TMDB != nil {
			if rt, err := h.TMDB.MovieRuntime(r.Context(), c.TMDBID); err == nil && rt > 0 {
				_ = h.Store.UpdateDiscRuntime(r.Context(), disc.ID, rt)
			}
		}
		// Persist the extended pane metadata for the picked candidate so
		// disc.metadata_json is ready before the first SSE snapshot — the
		// pane renders rich data on first paint. Best-effort: a failure
		// here doesn't block the rip.
		if blob, err := h.fetchExtendedMetadata(r.Context(), disc, &c); err == nil && blob != "" {
			_ = h.Store.UpdateDiscMetadataBlob(r.Context(), disc.ID, blob)
		}
	}

	// Authoritative duplicate guard. The fast-path check above isn't
	// atomic with Submit, so two requests racing within that window
	// both pass it. Re-check under startMu — held across Submit — so
	// exactly one job is created no matter how many requests race;
	// the losers get the same 409 as a sequential duplicate.
	h.startMu.Lock()
	defer h.startMu.Unlock()
	hasActive, err = h.Store.DiscHasActiveJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hasActive {
		writeError(w, http.StatusConflict, "disc already has an active job")
		return
	}

	job, err := h.Orchestrator.Submit(r.Context(), disc.ID, req.ProfileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// DeleteDisc removes a disc row by id. Used by the dashboard's Skip
// affordance on awaiting-decision cards: a disc with no Job that the
// user wants to dismiss permanently (otherwise the same row is
// re-derived on every page load and the card returns).
//
// Refuses to delete a disc that has any job referencing it — the user
// can already see those discs' outcomes in History, and removing them
// would orphan job rows that still point at the disc_id.
func (h *Handlers) DeleteDisc(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	hasJob, err := h.Store.DiscHasAnyJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hasJob {
		writeError(w, http.StatusConflict, "disc has job history; cannot delete")
		return
	}

	if err := h.Store.DeleteDisc(r.Context(), id); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "disc not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.Broadcaster.Publish(state.Event{
		Name:    "disc.deleted",
		Payload: map[string]any{"disc_id": id},
	})

	w.WriteHeader(http.StatusNoContent)
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

// fetchExtendedMetadata calls the appropriate identify-source method to
// retrieve the pane payload for the picked candidate and marshals it as
// JSON. Returns "" + nil when the candidate type has no source mapping
// (data discs, unknown).
func (h *Handlers) fetchExtendedMetadata(ctx context.Context, disc *state.Disc, c *state.Candidate) (string, error) {
	switch {
	case c.MediaType == "movie" && c.TMDBID > 0 && h.TMDB != nil:
		m, err := h.TMDB.MovieDetails(ctx, c.TMDBID)
		if err != nil {
			return "", err
		}
		return marshalBlob(m)
	case c.MediaType == "tv" && c.TMDBID > 0 && h.TMDB != nil:
		m, err := h.TMDB.TVDetails(ctx, c.TMDBID)
		if err != nil {
			return "", err
		}
		return marshalBlob(m)
	case disc.Type == state.DiscTypeAudioCD && c.MBID != "" && h.MusicBrainz != nil:
		m, err := h.MusicBrainz.ReleaseDetails(ctx, c.MBID)
		if err != nil {
			return "", err
		}
		return marshalBlob(m)
	case isGameDisc(disc.Type) && c.Source == "Redump":
		// Game discs build their blob from already-stored candidate data
		// (Redump matched at identify time). No external fetch needed.
		return marshalBlob(map[string]any{
			"system": gameSystemName(disc.Type),
			"serial": disc.MetadataID,
		})
	default:
		return "", nil
	}
}

func marshalBlob(v any) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func isGameDisc(t state.DiscType) bool {
	switch t {
	case state.DiscTypePSX, state.DiscTypePS2, state.DiscTypeSAT, state.DiscTypeDC, state.DiscTypeXBOX:
		return true
	}
	return false
}

func gameSystemName(t state.DiscType) string {
	switch t {
	case state.DiscTypePSX:
		return "Sony PlayStation"
	case state.DiscTypePS2:
		return "Sony PlayStation 2"
	case state.DiscTypeSAT:
		return "Sega Saturn"
	case state.DiscTypeDC:
		return "Sega Dreamcast"
	case state.DiscTypeXBOX:
		return "Microsoft Xbox"
	}
	return string(t)
}

// Compile-time check: identify package needs to be available for the
// fetchExtendedMetadata switch logic to compile; the import is exercised
// elsewhere by the live MovieDetails / ReleaseDetails calls.
var _ = identify.DiscMetadata{}
