package api

import (
	"encoding/json"
	"net/http"
)

// HealthHandler reports liveness with a fixed JSON payload.
// Liveness here means "the process is up and routing requests" — it
// deliberately does not poke external dependencies, so a healthy
// response does not imply working udev / SQLite / drives.
func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(struct {
		OK bool `json:"ok"`
	}{OK: true})
}
