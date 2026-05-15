package pipelines_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
)

// fakeSettings stubs SettingsReader for the truth-table tests below.
type fakeSettings map[string]string

func (f fakeSettings) GetSetting(_ context.Context, key string) (string, error) {
	v, ok := f[key]
	if !ok {
		return "", errors.New("missing")
	}
	return v, nil
}

func TestShouldEjectOnFinish_TruthTable(t *testing.T) {
	cases := []struct {
		name     string
		settings fakeSettings
		want     bool
	}{
		{"batch + true", fakeSettings{"operation.mode": "batch", "rip.eject_on_finish": "true"}, true},
		{"batch + false", fakeSettings{"operation.mode": "batch", "rip.eject_on_finish": "false"}, false},
		{"manual + true", fakeSettings{"operation.mode": "manual", "rip.eject_on_finish": "true"}, false},
		{"manual + false", fakeSettings{"operation.mode": "manual", "rip.eject_on_finish": "false"}, false},
		{"unset mode + true", fakeSettings{"rip.eject_on_finish": "true"}, true},
		{"unset mode + false", fakeSettings{"rip.eject_on_finish": "false"}, false},
		{"unset both → default true", fakeSettings{}, true},
		{"garbage value → default true", fakeSettings{"rip.eject_on_finish": "yes please"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pipelines.ShouldEjectOnFinish(context.Background(), tc.settings)
			if got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestResolveShouldEject_NilDefaultsTrue(t *testing.T) {
	if !pipelines.ResolveShouldEject(context.Background(), nil) {
		t.Error("nil ShouldEject should default to true")
	}
	if pipelines.ResolveShouldEject(context.Background(), func(context.Context) bool { return false }) {
		t.Error("explicit false ignored")
	}
}

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

// TestProbeWritable_AutoCreates verifies a fresh-install path where
// the library root (and intermediate parents) don't exist yet — the
// probe should create them rather than fail. Prevents the "first rip
// dies because /library/music doesn't exist" surprise.
func TestProbeWritable_AutoCreates(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "library", "music")
	if err := pipelines.ProbeWritable(dir); err != nil {
		t.Fatalf("ProbeWritable: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("expected %s to be created as a dir: err=%v", dir, err)
	}
	// And it should be usable on the next call (idempotent).
	if err := pipelines.ProbeWritable(dir); err != nil {
		t.Errorf("second call: %v", err)
	}
}

// TestProbeWritable_UncreatableErrors covers the case where the dir
// doesn't exist AND we can't create it (parent is read-only). The
// probe should still surface a clear error.
func TestProbeWritable_UncreatableErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses chmod restrictions; can't simulate uncreatable dir")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(parent, 0o755) }()

	err := pipelines.ProbeWritable(filepath.Join(parent, "sub"))
	if err == nil {
		t.Fatalf("expected error on uncreatable dir")
	}
	if err.Error() == "" {
		t.Errorf("error should carry a message")
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
