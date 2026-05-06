package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/api"
)

func TestRouterRoutes(t *testing.T) {
	r := api.NewRouter(nil)
	srv := httptest.NewServer(r)
	defer srv.Close()

	for _, path := range []string{"/api/health", "/api/version"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("GET %s: want 200, got %d", path, res.StatusCode)
		}
	}
}

func TestRouterUnknownReturns404(t *testing.T) {
	r := api.NewRouter(nil)
	srv := httptest.NewServer(r)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/nope")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", res.StatusCode)
	}
}
