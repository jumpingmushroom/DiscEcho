package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// driveReadOffsetMaxAbs caps the accepted PATCH offset value. Real-world
// drives span roughly ±1500 samples (Pioneer drives sit around -1164,
// some Plextor drives at +667); we accept a generous ±3000 to give
// users headroom without taking anything pathological.
const driveReadOffsetMaxAbs = 3000

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

// EjectDrive shells out to the eject binary for the drive's dev_path.
// Refuses with 409 if there is an active job on the drive — eject mid-
// rip kills the SCSI handle and corrupts the in-flight output. Flips
// the drive state to `ejecting` for the duration of the call, then back
// to `idle`. Returns 503 if the daemon was built without an Ejector
// (tests/edge configs).
func (h *Handlers) EjectDrive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	drv, err := h.Store.GetDrive(r.Context(), id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "drive not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	busy, err := h.Store.HasActiveJobOnDrive(r.Context(), drv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if busy {
		writeError(w, http.StatusConflict, "drive has an active job")
		return
	}
	if h.Ejector == nil {
		writeError(w, http.StatusServiceUnavailable, "eject not configured")
		return
	}
	if drv.DevPath == "" {
		writeError(w, http.StatusUnprocessableEntity, "drive has no dev_path")
		return
	}
	if err := h.Store.UpdateDriveState(r.Context(), drv.ID, state.DriveStateEjecting); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.Ejector(r.Context(), drv.DevPath); err != nil {
		// Restore idle so a failed eject doesn't leave the UI stuck.
		_ = h.Store.UpdateDriveState(r.Context(), drv.ID, state.DriveStateIdle)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.Store.UpdateDriveState(r.Context(), drv.ID, state.DriveStateIdle); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Broadcaster != nil {
		h.Broadcaster.Publish(state.Event{
			Name:    "drive.changed",
			Payload: map[string]any{"drive_id": drv.ID, "state": "idle"},
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReclassifyDrive reruns the disc-flow handler against the disc that
// is already sitting in the drive. Used to recover a drive stuck in
// `error` after the cold-disc spin-up race exhausted the classifier
// retry budget — without this the user has to eject and re-insert
// to get the kernel to fire DISK_MEDIA_CHANGE again. 503 when no
// Reclassify hook is wired (tests), 409 when a job is already
// running on the drive (the running pipeline holds the SCSI handle).
func (h *Handlers) ReclassifyDrive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	drv, err := h.Store.GetDrive(r.Context(), id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "drive not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Reclassify == nil {
		writeError(w, http.StatusServiceUnavailable, "reclassify not configured")
		return
	}
	if drv.Bus == "" {
		writeError(w, http.StatusUnprocessableEntity, "drive has no bus identifier")
		return
	}
	busy, err := h.Store.HasActiveJobOnDrive(r.Context(), drv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if busy {
		writeError(w, http.StatusConflict, "drive has an active job")
		return
	}
	h.Reclassify(drv.Bus)
	w.WriteHeader(http.StatusAccepted)
}

// PatchDriveOffset persists a manually-entered read-offset for a drive.
// Body: {"read_offset": <int>}. Offset is in CDDA samples (-3000..+3000).
// 409 when an active job holds the drive (stale calibration changing
// mid-rip would be a logic bug we don't want to mask). Setting an
// offset always tags the source as "manual"; the auto-detect endpoint
// (future work) is the only writer for source="auto".
func (h *Handlers) PatchDriveOffset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	drv, err := h.Store.GetDrive(r.Context(), id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "drive not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var body struct {
		ReadOffset *int `json:"read_offset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.ReadOffset == nil {
		writeError(w, http.StatusUnprocessableEntity, "read_offset is required")
		return
	}
	offset := *body.ReadOffset
	if offset < -driveReadOffsetMaxAbs || offset > driveReadOffsetMaxAbs {
		writeError(w, http.StatusUnprocessableEntity,
			"read_offset out of range (must be within ±3000 samples)")
		return
	}

	busy, err := h.Store.HasActiveJobOnDrive(r.Context(), drv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if busy {
		writeError(w, http.StatusConflict, "drive has an active job")
		return
	}

	if err := h.Store.UpdateDriveReadOffset(r.Context(), drv.ID, offset, "manual"); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "drive not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out, err := h.Store.GetDrive(r.Context(), drv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.Broadcaster != nil {
		// The webui SSE handler treats every drive.changed payload as
		// `{drive_id, state, ...}` and writes payload.state straight into
		// the local drive's state field. Omitting state here turned the
		// drive's status pill into the literal string "undefined" until
		// the next snapshot fetch. Always include state so the SSE
		// patch is a no-op for fields the broadcast didn't change.
		h.Broadcaster.Publish(state.Event{
			Name: "drive.changed",
			Payload: map[string]any{
				"drive_id":           drv.ID,
				"state":              string(out.State),
				"read_offset":        out.ReadOffset,
				"read_offset_source": out.ReadOffsetSource,
			},
		})
	}
	writeJSON(w, http.StatusOK, out)
}
