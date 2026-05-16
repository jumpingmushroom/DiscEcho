package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
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
// All fields are optional: an empty body re-reads the stored disc.
type identifyRequest struct {
	Query     string `json:"query,omitempty"`
	MediaType string `json:"media_type,omitempty"` // 'movie' | 'tv' | 'both' (default both)
	// Force re-runs the full classify + identify pipeline against the
	// drive the disc lives in. Used by the drive card's "Re-identify"
	// button when MusicBrainz / TMDB pick the wrong release.
	Force bool `json:"force,omitempty"`
}

// IdentifyDisc returns the disc plus its candidates. With Force=true it
// re-runs the full classify + identify pipeline against the drive the
// disc lives in (replaces candidates + metadata fields). With a non-
// empty Query it triggers a manual TMDB search. Empty body → returns
// the current stored disc + candidates.
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

	if req.Force {
		h.forceReidentify(w, r, disc)
		return
	}

	if req.Query == "" {
		writeJSON(w, http.StatusOK, map[string]any{"disc": disc, "candidates": disc.Candidates})
		return
	}

	// Dispatch manual search to the appropriate metadata source.
	// Audio CDs → MusicBrainz; game discs → IGDB; data discs have no
	// searchable metadata; everything else (DVD, BDMV, UHD) → TMDB.
	var cands []state.Candidate
	switch {
	case disc.Type == state.DiscTypeAudioCD:
		if h.MusicBrainz == nil {
			writeError(w, http.StatusServiceUnavailable, "MusicBrainz not configured")
			return
		}
		cands, err = h.MusicBrainz.SearchByName(r.Context(), req.Query)

	case isGameDisc(disc.Type):
		if h.IGDB == nil || !h.IGDB.Configured() {
			writeError(w, http.StatusServiceUnavailable, "IGDB not configured")
			return
		}
		cands, err = h.IGDB.SearchGames(r.Context(), req.Query, disc.Type)

	case disc.Type == state.DiscTypeData:
		writeError(w, http.StatusUnprocessableEntity,
			"data discs do not support metadata search")
		return

	default: // DVD, BDMV, UHD, VCD
		if h.TMDB == nil {
			writeError(w, http.StatusServiceUnavailable, "TMDB not configured")
			return
		}
		mediaType := req.MediaType
		if mediaType == "" {
			mediaType = "both"
		}
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
	var payload any
	switch {
	case c.MediaType == "movie" && c.TMDBID > 0 && h.TMDB != nil:
		m, err := h.TMDB.MovieDetails(ctx, c.TMDBID)
		if err != nil {
			return "", err
		}
		payload = m
	case c.MediaType == "tv" && c.TMDBID > 0 && h.TMDB != nil:
		m, err := h.TMDB.TVDetails(ctx, c.TMDBID)
		if err != nil {
			return "", err
		}
		payload = m
	case disc.Type == state.DiscTypeAudioCD && c.MBID != "" && h.MusicBrainz != nil:
		m, err := h.MusicBrainz.ReleaseDetails(ctx, c.MBID)
		if err != nil {
			return "", err
		}
		payload = m
	case isGameDisc(disc.Type) && c.Source == "Redump":
		// Game discs build their blob from already-stored candidate data
		// (Redump matched at identify time). No external fetch needed.
		payload = map[string]any{
			"system": gameSystemName(disc.Type),
			"serial": disc.MetadataID,
		}
	default:
		return "", nil
	}
	body, err := json.Marshal(payload)
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

// forceReidentify re-runs the classify + Identify pipeline for an
// existing disc and updates the row in place. Used by the drive card's
// Re-identify button when the prober/lookup landed on a wrong candidate
// (e.g. MusicBrainz picked the wrong release, TMDB grabbed the wrong
// title). Refuses 409 if the drive has an active job or another claim
// is already in flight.
func (h *Handlers) forceReidentify(w http.ResponseWriter, r *http.Request, disc *state.Disc) {
	ctx := r.Context()
	if disc.DriveID == "" {
		writeError(w, http.StatusUnprocessableEntity, "disc has no drive — cannot re-identify")
		return
	}
	drv, err := h.Store.GetDrive(ctx, disc.DriveID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if drv.DevPath == "" {
		writeError(w, http.StatusUnprocessableEntity, "drive has no dev_path")
		return
	}
	busy, err := h.Store.HasActiveJobOnDrive(ctx, drv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if busy {
		writeError(w, http.StatusConflict, "drive has an active job")
		return
	}
	claimed, err := h.Store.ClaimDriveForIdentify(ctx, drv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !claimed {
		writeError(w, http.StatusConflict, "drive already identifying")
		return
	}
	defer func() {
		if uerr := h.Store.UpdateDriveState(context.Background(), drv.ID, state.DriveStateIdle); uerr != nil {
			slog.Warn("force-reidentify: release drive state", "err", uerr, "drive_id", drv.ID)
		}
	}()
	if h.Broadcaster != nil {
		h.Broadcaster.Publish(state.Event{
			Name:    "drive.changed",
			Payload: map[string]any{"drive_id": drv.ID, "state": "identifying"},
		})
	}

	if h.Classifier == nil || h.Pipelines == nil {
		writeError(w, http.StatusServiceUnavailable, "identify not configured")
		return
	}
	dt, err := h.Classifier.Classify(ctx, drv.DevPath)
	if err != nil {
		writeError(w, http.StatusBadGateway, "classify: "+err.Error())
		return
	}
	handler, ok := h.Pipelines.Get(dt)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "no pipeline for disc type "+string(dt))
		return
	}
	fresh, cands, ierr := handler.Identify(ctx, drv)
	switch {
	case errors.Is(ierr, pipelines.ErrNoCandidates):
		// Persist whatever metadata fresh contains (often just the type)
		// and clear candidates so the UI can offer manual search.
		if cands == nil {
			cands = []state.Candidate{}
		}
	case ierr != nil:
		writeError(w, http.StatusBadGateway, "identify: "+ierr.Error())
		return
	}

	// Merge fresh fields into the existing disc row.
	if fresh != nil {
		disc.Type = fresh.Type
		disc.Title = fresh.Title
		disc.Year = fresh.Year
		disc.MetadataProvider = fresh.MetadataProvider
		disc.MetadataID = fresh.MetadataID
	}
	disc.Candidates = cands
	if err := h.Store.UpdateDiscMetadata(ctx, disc.ID, disc.Title, disc.Year, disc.MetadataProvider, disc.MetadataID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.Store.UpdateDiscCandidates(ctx, disc.ID, cands); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Broadcaster != nil {
		h.Broadcaster.Publish(state.Event{
			Name:    "disc.identified",
			Payload: map[string]any{"disc": disc, "candidates": cands},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"disc": disc, "candidates": cands})
}
