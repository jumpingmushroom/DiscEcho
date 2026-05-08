package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSettings_ReturnsKV(t *testing.T) {
	h := apitestServer(t)
	if err := h.Store.SetSetting(context.Background(), "library_path", "/srv/lib"); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.SetSetting(context.Background(), "default_audiocd_profile", "p1"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	w := httptest.NewRecorder()
	h.GetSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["library_path"] != "/srv/lib" || got["default_audiocd_profile"] != "p1" {
		t.Errorf("got %+v", got)
	}
}

func TestSettings_Put_PrefsValid(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{
		"prefs.accent": "amber", "prefs.mood": "carbon", "prefs.density": "compact",
	})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
	got, _ := h.Store.GetSetting(context.Background(), "prefs.accent")
	if got != "amber" {
		t.Fatalf("accent: %q", got)
	}
}

func TestSettings_Put_PrefsInvalidAccent_422(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{"prefs.accent": "neon"})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 422 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
	var errs map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &errs)
	if errs["prefs.accent"] == "" {
		t.Fatalf("expected accent error: %s", rec.Body.String())
	}
}

func TestSettings_Put_RetentionValid(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{
		"retention.forever": false, "retention.days": 30,
	})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
	v, _ := h.Store.GetSetting(context.Background(), "retention.days")
	if v != "30" {
		t.Fatalf("days: %q", v)
	}
	v2, _ := h.Store.GetSetting(context.Background(), "retention.forever")
	if v2 != "false" {
		t.Fatalf("forever: %q", v2)
	}
}

func TestSettings_Put_RetentionForeverFalse_DaysZero_422(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{
		"retention.forever": false, "retention.days": 0,
	})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 422 {
		t.Fatalf("status: %d", rec.Code)
	}
}

func TestSettings_Put_RetentionForeverFalse_NoDaysInPatch_StoredZero_422(t *testing.T) {
	h := apitestServer(t)
	// No prior days value set; PATCH sets forever=false but doesn't include days.
	body := mustMarshal(t, map[string]any{"retention.forever": false})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 422 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestSettings_Put_RetentionForeverFalse_NoDaysInPatch_StoredValid_OK(t *testing.T) {
	h := apitestServer(t)
	_ = h.Store.SetSetting(context.Background(), "retention.days", "60")
	body := mustMarshal(t, map[string]any{"retention.forever": false})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestSettings_Put_RetentionForeverTrue_DaysZero_OK(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{
		"retention.forever": true, "retention.days": 0,
	})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestSettings_Put_LibraryPathValid(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{"library.path": "/srv/media"})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
	v, _ := h.Store.GetSetting(context.Background(), "library.path")
	if v != "/srv/media" {
		t.Fatalf("library.path: %q", v)
	}
}

func TestSettings_Put_LibraryPathRelative_422(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{"library.path": "media"})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 422 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestSettings_Put_LibraryPathEmpty_422(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{"library.path": "   "})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 422 {
		t.Fatalf("status: %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestSettings_Put_UnknownKey_422(t *testing.T) {
	h := apitestServer(t)
	body := mustMarshal(t, map[string]any{"unknown.key": "value"})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 422 {
		t.Fatalf("status: %d", rec.Code)
	}
}

func TestSettings_Put_BroadcastsSettingsChanged(t *testing.T) {
	h := apitestServer(t)
	ch, cancel := h.Broadcaster.Subscribe(4)
	defer cancel()
	body := mustMarshal(t, map[string]any{"prefs.accent": "cobalt"})
	req := authedReq(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.PutSettings(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: %d", rec.Code)
	}
	select {
	case ev := <-ch:
		if ev.Name != "settings.changed" {
			t.Fatalf("event: %q", ev.Name)
		}
	default:
		t.Fatal("expected settings.changed SSE event")
	}
}
