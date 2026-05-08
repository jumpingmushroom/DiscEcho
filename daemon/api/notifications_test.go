package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/api"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// fakeApprise stubs the api.Apprise surface used by handlers.
type fakeApprise struct {
	dryRunErr   error
	dryRunCalls []string
}

func (f *fakeApprise) DryRun(_ context.Context, url string) error {
	f.dryRunCalls = append(f.dryRunCalls, url)
	return f.dryRunErr
}

func (f *fakeApprise) Send(_ context.Context, _ []string, _, _ string) error {
	return nil
}

// withURLParam injects a chi route param into the request context so
// chi.URLParam(r, key) returns value inside handler tests that call
// handlers directly (bypassing the chi router).
func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func authedReq(_ *testing.T, method, path string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNotifications_List(t *testing.T) {
	h := apitestServer(t)
	if err := h.Store.CreateNotification(context.Background(), &state.Notification{
		Name: "n1", URL: "ntfy://example/n1", Triggers: "done", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	req := authedReq(t, http.MethodGet, "/api/notifications", nil)
	rec := httptest.NewRecorder()
	h.ListNotifications(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
	var got []state.Notification
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "n1" {
		t.Fatalf("body: %s", rec.Body.String())
	}
}

func TestNotifications_Create_OK(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{
		"name": "ntfy-1", "url": "ntfy://example/topic",
		"triggers": "done,failed", "enabled": true,
	})
	req := authedReq(t, http.MethodPost, "/api/notifications", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.CreateNotification(rec, req)
	if rec.Code != 201 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
	fa := h.Apprise.(*fakeApprise)
	if len(fa.dryRunCalls) != 1 || fa.dryRunCalls[0] != "ntfy://example/topic" {
		t.Fatalf("dry-run not invoked correctly: %v", fa.dryRunCalls)
	}
	var got state.Notification
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == "" {
		t.Fatalf("returned row missing ID: %s", rec.Body.String())
	}
}

func TestNotifications_Create_BadURL_422(t *testing.T) {
	h := apitestServer(t)
	h.Apprise.(*fakeApprise).dryRunErr = errors.New("apprise dry-run: Could not load URL")
	body := mustMarshal(t, map[string]any{
		"name": "x", "url": "bogus://nope", "triggers": "done", "enabled": true,
	})
	req := authedReq(t, http.MethodPost, "/api/notifications", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.CreateNotification(rec, req)
	if rec.Code != 422 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
	var errs map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errs["url"], "Could not load URL") {
		t.Fatalf("body: %s", rec.Body.String())
	}
}

func TestNotifications_Create_BadTriggers_422(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{
		"name": "x", "url": "ntfy://x", "triggers": "done,nuke", "enabled": true,
	})
	req := authedReq(t, http.MethodPost, "/api/notifications", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.CreateNotification(rec, req)
	if rec.Code != 422 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestNotifications_Create_EmptyName_422(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{
		"name": "", "url": "ntfy://x", "triggers": "done", "enabled": true,
	})
	req := authedReq(t, http.MethodPost, "/api/notifications", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.CreateNotification(rec, req)
	if rec.Code != 422 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestNotifications_Update(t *testing.T) {
	h := apitestServer(t)
	n := &state.Notification{Name: "n", URL: "ntfy://x", Triggers: "done", Enabled: true}
	if err := h.Store.CreateNotification(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	body := mustMarshal(t, map[string]any{
		"name": "renamed", "url": "ntfy://y",
		"triggers": "done,failed", "enabled": false,
	})
	req := authedReq(t, http.MethodPut, "/api/notifications/"+n.ID, bytes.NewReader(body))
	req = withURLParam(req, "id", n.ID)
	rec := httptest.NewRecorder()
	h.UpdateNotification(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
	var got state.Notification
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "renamed" {
		t.Fatalf("name not updated: %s", got.Name)
	}
}

func TestNotifications_Update_NotFound(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{
		"name": "x", "url": "ntfy://x", "triggers": "done", "enabled": true,
	})
	req := authedReq(t, http.MethodPut, "/api/notifications/missing", bytes.NewReader(body))
	req = withURLParam(req, "id", "missing")
	rec := httptest.NewRecorder()
	h.UpdateNotification(rec, req)
	if rec.Code != 404 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestNotifications_Delete(t *testing.T) {
	h := apitestServer(t)
	n := &state.Notification{Name: "n", URL: "ntfy://x", Triggers: "done", Enabled: true}
	if err := h.Store.CreateNotification(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	req := authedReq(t, http.MethodDelete, "/api/notifications/"+n.ID, nil)
	req = withURLParam(req, "id", n.ID)
	rec := httptest.NewRecorder()
	h.DeleteNotification(rec, req)
	if rec.Code != 204 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
	if _, err := h.Store.GetNotification(context.Background(), n.ID); err == nil {
		t.Fatal("notification should be deleted")
	}
}

func TestNotifications_SSE_OnCreate(t *testing.T) {
	h := apitestServer(t)
	ch, cancel := h.Broadcaster.Subscribe(4)
	defer cancel()

	body := mustMarshal(t, map[string]any{
		"name": "x", "url": "ntfy://x", "triggers": "done", "enabled": true,
	})
	req := authedReq(t, http.MethodPost, "/api/notifications", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.CreateNotification(rec, req)
	if rec.Code != 201 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
	select {
	case ev := <-ch:
		if ev.Name != "notification.changed" {
			t.Fatalf("event name: %q", ev.Name)
		}
	default:
		t.Fatal("expected a notification.changed SSE event")
	}
}

func TestNotifications_SSE_OnUpdate(t *testing.T) {
	h := apitestServer(t)
	n := &state.Notification{Name: "n", URL: "ntfy://x", Triggers: "done", Enabled: true}
	if err := h.Store.CreateNotification(context.Background(), n); err != nil {
		t.Fatal(err)
	}

	ch, cancel := h.Broadcaster.Subscribe(4)
	defer cancel()

	body := mustMarshal(t, map[string]any{
		"name": "renamed", "url": "ntfy://x", "triggers": "done", "enabled": true,
	})
	req := authedReq(t, http.MethodPut, "/api/notifications/"+n.ID, bytes.NewReader(body))
	req = withURLParam(req, "id", n.ID)
	rec := httptest.NewRecorder()
	h.UpdateNotification(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: %d", rec.Code)
	}
	select {
	case ev := <-ch:
		if ev.Name != "notification.changed" {
			t.Fatalf("event name: %q", ev.Name)
		}
	default:
		t.Fatal("expected a notification.changed SSE event")
	}
}

func TestNotifications_SSE_OnDelete(t *testing.T) {
	h := apitestServer(t)
	n := &state.Notification{Name: "n", URL: "ntfy://x", Triggers: "done", Enabled: true}
	if err := h.Store.CreateNotification(context.Background(), n); err != nil {
		t.Fatal(err)
	}

	ch, cancel := h.Broadcaster.Subscribe(4)
	defer cancel()

	req := authedReq(t, http.MethodDelete, "/api/notifications/"+n.ID, nil)
	req = withURLParam(req, "id", n.ID)
	rec := httptest.NewRecorder()
	h.DeleteNotification(rec, req)
	if rec.Code != 204 {
		t.Fatalf("status: %d", rec.Code)
	}
	select {
	case ev := <-ch:
		if ev.Name != "notification.changed" {
			t.Fatalf("event name: %q", ev.Name)
		}
	default:
		t.Fatal("expected a notification.changed SSE event")
	}
}

// Ensure api.Apprise is satisfied by *fakeApprise at compile time.
var _ api.Apprise = (*fakeApprise)(nil)
