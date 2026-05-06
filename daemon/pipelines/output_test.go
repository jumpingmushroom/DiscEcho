package pipelines_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
)

func TestRenderOutputPath_Standard(t *testing.T) {
	got, err := pipelines.RenderOutputPath(
		`{{.Artist}}/{{.Album}} ({{.Year}})/{{printf "%02d" .TrackNumber}} - {{.Title}}.flac`,
		pipelines.OutputFields{
			Artist: "Miles Davis", Album: "Kind of Blue", Year: 1959,
			TrackNumber: 1, Title: "So What",
		})
	if err != nil {
		t.Fatal(err)
	}
	want := "Miles Davis/Kind of Blue (1959)/01 - So What.flac"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestRenderOutputPath_SanitizesPathSeparators(t *testing.T) {
	// A track title with "/" must not introduce a directory.
	got, err := pipelines.RenderOutputPath(
		`{{.Artist}}/{{.Title}}.flac`,
		pipelines.OutputFields{Artist: "X", Title: "A/B"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "X/A_B.flac" {
		t.Errorf("got %q", got)
	}
}

func TestRenderOutputPath_TrimsControlChars(t *testing.T) {
	got, err := pipelines.RenderOutputPath(`{{.Title}}.flac`,
		pipelines.OutputFields{Title: "Bad\x01Char"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "BadChar.flac" {
		t.Errorf("got %q", got)
	}
}

func TestRenderOutputPath_RejectsAbsoluteEscape(t *testing.T) {
	// Template tries to escape via .. — sanitized
	got, err := pipelines.RenderOutputPath(`{{.Album}}/file.flac`,
		pipelines.OutputFields{Album: "../../etc"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(got) {
		t.Errorf("absolute path leaked: %q", got)
	}
	if got != "etc/file.flac" {
		t.Errorf("got %q", got)
	}
}

func TestRenderOutputPath_BadTemplate(t *testing.T) {
	_, err := pipelines.RenderOutputPath(`{{.NoSuchField}}`,
		pipelines.OutputFields{})
	if err != nil {
		t.Errorf("missing fields should not error: %v", err)
	}

	_, err = pipelines.RenderOutputPath(`{{.Artist`, pipelines.OutputFields{})
	if err == nil {
		t.Errorf("malformed template should error")
	}
}

func TestProbeWritable_OK(t *testing.T) {
	dir := t.TempDir()
	if err := pipelines.ProbeWritable(dir); err != nil {
		t.Errorf("ok dir: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("probe should clean up, found %d entries", len(entries))
	}
}

func TestProbeWritable_ReadOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	err := pipelines.ProbeWritable(dir)
	if err == nil {
		t.Errorf("expected error on read-only dir")
	}
}

func TestProbeWritable_Missing(t *testing.T) {
	err := pipelines.ProbeWritable("/no/such/path")
	if err == nil {
		t.Fatalf("missing dir should error")
	}
	// Errors may wrap differently across OSes, so we accept either a
	// wrapped os.ErrNotExist or any non-empty error message.
	if !errors.Is(err, os.ErrNotExist) && err.Error() == "" {
		t.Errorf("error should at least carry a message")
	}
}

func TestAtomicMove_SameFS(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.flac")
	dst := filepath.Join(root, "sub", "dir", "dst.flac")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := pipelines.AtomicMove(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("src still exists")
	}
	body, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Errorf("content mismatch: %q", body)
	}
}

func TestAtomicMove_RefusesOverwrite(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "a")
	dst := filepath.Join(root, "b")
	if err := os.WriteFile(src, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pipelines.AtomicMove(src, dst); err == nil {
		t.Errorf("want error when dst exists")
	}
}
