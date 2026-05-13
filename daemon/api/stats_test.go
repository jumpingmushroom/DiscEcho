package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestStats_EmptyDB(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Get("/api/stats", h.Stats)

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var got state.Stats
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ActiveJobs.Value != 0 {
		t.Errorf("active.value: %d", got.ActiveJobs.Value)
	}
	if len(got.ActiveJobs.Spark24h) != 24 {
		t.Errorf("spark_24h len: %d", len(got.ActiveJobs.Spark24h))
	}
	if len(got.TodayRipped.Spark7dBytes) != 7 {
		t.Errorf("spark_7d_bytes len: %d", len(got.TodayRipped.Spark7dBytes))
	}
	if len(got.Library.Spark30dUsed) != 30 {
		t.Errorf("spark_30d_used len: %d", len(got.Library.Spark30dUsed))
	}
	if len(got.Failures7d.Spark30d) != 30 {
		t.Errorf("failures spark_30d len: %d", len(got.Failures7d.Spark30d))
	}
}
