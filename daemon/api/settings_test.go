package api_test

import (
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
