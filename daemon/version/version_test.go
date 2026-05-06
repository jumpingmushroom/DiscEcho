package version_test

import (
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/version"
)

func TestInfoDefaults(t *testing.T) {
	got := version.Info()
	if got.Version == "" {
		t.Errorf("Version should not be empty, got %q", got.Version)
	}
	if got.Commit == "" {
		t.Errorf("Commit should not be empty, got %q", got.Commit)
	}
	if got.BuildDate == "" {
		t.Errorf("BuildDate should not be empty, got %q", got.BuildDate)
	}
}
