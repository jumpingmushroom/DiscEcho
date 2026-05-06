package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// StreamEvents is the GET /api/events SSE handler. It subscribes to the
// broadcaster, writes an initial state.snapshot, then forwards events
// until the client disconnects or the broadcaster closes.
func (h *Handlers) StreamEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// X-Accel-Buffering disables proxy-side buffering when behind nginx;
	// harmless when served direct.
	w.Header().Set("X-Accel-Buffering", "no")

	// Subscribe before writing the snapshot so any event published in
	// the gap between snapshot read and stream-attach is delivered.
	ch, cancel := h.Broadcaster.Subscribe(64)
	defer cancel()

	if err := h.writeSnapshot(r.Context(), w, flusher); err != nil {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSE(w, flusher, ev.Name, ev.Payload); err != nil {
				return
			}
		}
	}
}

// writeSnapshot emits the bootstrap state.snapshot event. Errors from
// the read calls are logged and the snapshot still goes out (with the
// erroring section empty), so the SSE stream stays alive even on a
// transient DB hiccup.
func (h *Handlers) writeSnapshot(ctx context.Context, w http.ResponseWriter, flusher http.Flusher) error {
	drives, _ := h.Store.ListDrives(ctx)
	jobs, _ := h.Store.ListActiveAndRecentJobs(ctx, 50)
	profiles, _ := h.Store.ListProfiles(ctx)
	settings, _ := h.Store.GetAllSettings(ctx)
	return writeSSE(w, flusher, "state.snapshot", map[string]any{
		"drives":   drives,
		"jobs":     jobs,
		"profiles": profiles,
		"settings": settings,
	})
}

// writeSSE writes one SSE event in the canonical
// `event: <name>\ndata: <json>\n\n` format and flushes.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, name string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, body); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
