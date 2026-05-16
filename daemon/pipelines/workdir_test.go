package pipelines

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateWorkDir_WithKind(t *testing.T) {
	root := t.TempDir()
	dir, err := CreateWorkDir(root, "psx", "abc123")
	if err != nil {
		t.Fatalf("CreateWorkDir: %v", err)
	}
	if filepath.Dir(dir) != root {
		t.Errorf("parent: want %q, got %q", root, filepath.Dir(dir))
	}
	base := filepath.Base(dir)
	if !strings.HasPrefix(base, "discecho-psx-abc123-") {
		t.Errorf("base prefix: got %q", base)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("dir not created: stat=%v err=%v", fi, err)
	}
}

func TestCreateWorkDir_EmptyKindOmitsSegment(t *testing.T) {
	root := t.TempDir()
	dir, err := CreateWorkDir(root, "", "xyz")
	if err != nil {
		t.Fatalf("CreateWorkDir: %v", err)
	}
	base := filepath.Base(dir)
	if !strings.HasPrefix(base, "discecho-xyz-") {
		t.Errorf("base prefix: got %q", base)
	}
	if strings.HasPrefix(base, "discecho--") {
		t.Errorf("empty kind leaked a double dash: %q", base)
	}
}

func TestCreateWorkDir_EmptyRootUsesTempDir(t *testing.T) {
	dir, err := CreateWorkDir("", "data", "abc")
	if err != nil {
		t.Fatalf("CreateWorkDir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if filepath.Dir(dir) != os.TempDir() {
		t.Errorf("parent: want %q, got %q", os.TempDir(), filepath.Dir(dir))
	}
}
