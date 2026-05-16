package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// sseKeepaliveInterval is how often we emit an SSE comment line. A
// comment (`: ping\n\n`) keeps reverse-proxy and load-balancer idle
// timeouts from killing the stream during long quiet periods.
const sseKeepaliveInterval = 15 * time.Second

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

	keepalive := time.NewTicker(sseKeepaliveInterval)
	defer keepalive.Stop()

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
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeSnapshot emits the bootstrap state.snapshot event. A DB error
// is swallowed so the SSE stream stays alive on a transient hiccup;
// the client receives an empty payload and the next event will
// refresh the relevant slice.
func (h *Handlers) writeSnapshot(ctx context.Context, w http.ResponseWriter, flusher http.Flusher) error {
	payload, err := h.buildSnapshot(ctx)
	if err != nil {
		payload = map[string]any{}
	}
	return writeSSE(w, flusher, "state.snapshot", payload)
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
