package api

import (
	"encoding/json"
	"net/http"

	"github.com/jumpingmushroom/DiscEcho/daemon/version"
)

// VersionHandler returns the build info embedded at compile time.
func VersionHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(version.Info())
}
