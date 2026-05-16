package pipelines

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMD5File_HashesContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := MD5File(path)
	if err != nil {
		t.Fatalf("MD5File: %v", err)
	}
	const wantHelloMD5 = "5d41402abc4b2a76b9719d911017c592"
	if got != wantHelloMD5 {
		t.Errorf("md5: want %q, got %q", wantHelloMD5, got)
	}
}

func TestMD5File_MissingPathReturnsError(t *testing.T) {
	if _, err := MD5File(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Errorf("expected error for missing file")
	}
}
