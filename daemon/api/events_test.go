package api_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// readUntilEvent reads SSE lines until it sees an `event: <name>` line,
// then returns. Returns the next data line (best effort). Bounds the
// total lines scanned so a broken stream doesn't hang the test.
func readUntilEvent(t *testing.T, sc *bufio.Scanner, name string, deadline time.Time) string {
	t.Helper()
	for sc.Scan() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for event %q", name)
		}
		line := sc.Text()
		if line == "event: "+name {
			// next non-empty data line is the payload
			for sc.Scan() {
				next := sc.Text()
				if strings.HasPrefix(next, "data: ") {
					return strings.TrimPrefix(next, "data: ")
				}
				if next == "" {
					continue
				}
			}
			return ""
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner: %v", err)
	}
	t.Fatalf("EOF before event %q", name)
	return ""
}

func TestStreamEvents_SnapshotThenLive(t *testing.T) {
	h := apitestServer(t)
	seedDrive(t, h)
	seedProfile(t, h)

	srv := httptest.NewServer(http.HandlerFunc(h.StreamEvents))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type %q", ct)
	}

	sc := bufio.NewScanner(resp.Body)
	deadline := time.Now().Add(2 * time.Second)

	snapshot := readUntilEvent(t, sc, "state.snapshot", deadline)
	if !strings.Contains(snapshot, "drives") || !strings.Contains(snapshot, "profiles") {
		t.Errorf("snapshot missing keys: %s", snapshot)
	}

	// Publish a live event after subscribe and verify it arrives. Run
	// the publish on a separate goroutine so the scanner blocks on it
	// rather than the publisher blocking on the broadcaster's mutex.
	go func() {
		// Small delay so the publish happens after the snapshot has
		// been fully drained on the reader side.
		time.Sleep(50 * time.Millisecond)
		h.Broadcaster.Publish(state.Event{
			Name:    "drive.changed",
			Payload: map[string]any{"drive_id": "x", "state": "ripping"},
		})
	}()

	got := readUntilEvent(t, sc, "drive.changed", deadline)
	if !strings.Contains(got, "ripping") {
		t.Errorf("payload: %s", got)
	}
}
