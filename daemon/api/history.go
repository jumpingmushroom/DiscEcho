package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// ListHistory returns finished jobs (done/failed/cancelled) joined with
// their disc, ordered by finished_at DESC. Supports filtering by disc
// type and finished-at date range, plus standard limit/offset pagination
// (limit clamps to [1, 200], default 50).
func (h *Handlers) ListHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := state.HistoryFilter{
		Type:   state.DiscType(q.Get("type")),
		Limit:  parseIntOr(q.Get("limit"), 50),
		Offset: parseIntOr(q.Get("offset"), 0),
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if f.Limit < 1 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.From = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.To = t
		}
	}

	rows, err := h.Store.ListHistory(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := h.Store.CountHistory(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []state.HistoryRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rows":   rows,
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}

// ClearHistory deletes every finished-rip record — done/failed/cancelled
// jobs, their logs and steps, and the disc rows left orphaned by that
// deletion. In-progress rips are untouched, and the ripped files on
// disk are not touched. Responds with {"deleted": N}.
func (h *Handlers) ClearHistory(w http.ResponseWriter, r *http.Request) {
	n, err := h.Store.ClearHistory(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
}

func parseIntOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
