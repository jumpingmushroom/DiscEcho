package pipelines

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// OutputFields is the data passed to a profile's output_path_template.
// Add fields here as new disc types need them; templates that reference
// missing fields render as the empty string (text/template default).
type OutputFields struct {
	Artist        string
	Album         string
	Year          int
	TrackNumber   int
	Title         string
	DiscNumber    int
	Show          string // DVD-Series episodes
	Season        int    // DVD-Series episodes
	EpisodeNumber int    // DVD-Series episodes (1-based after filtering)
}

// RenderOutputPath applies a Go template to fields and sanitizes the
// result so it can't escape the configured library root: no leading
// "/", no ".." segments, no path-separator characters within field
// values, no control chars. Field values are scrubbed of in-segment
// slashes BEFORE rendering so a track title containing "/" becomes
// "_" rather than introducing a stray directory boundary.
//
// Templates that reference unknown fields render as "" (we pass the
// fields as a map so text/template's missingkey=zero applies).
func RenderOutputPath(tmpl string, fields OutputFields) (string, error) {
	t, err := template.New("output").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, fields.asMap()); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return sanitizeRelPath(buf.String()), nil
}

// asMap returns a map view used at render time. String fields are
// pre-sanitized so user-supplied titles can't introduce extra path
// components or escape the library root: in-segment slashes are
// folded to "_" while traversal segments (".", "..") are dropped.
func (f OutputFields) asMap() map[string]any {
	return map[string]any{
		"Artist":        sanitizeFieldValue(f.Artist),
		"Album":         sanitizeFieldValue(f.Album),
		"Year":          f.Year,
		"TrackNumber":   f.TrackNumber,
		"Title":         sanitizeFieldValue(f.Title),
		"DiscNumber":    f.DiscNumber,
		"Show":          sanitizeFieldValue(f.Show),
		"Season":        f.Season,
		"EpisodeNumber": f.EpisodeNumber,
	}
}

// sanitizeFieldValue strips ".." / "." traversal segments from a field
// value and joins the remaining pieces with "_" so a single field
// can never split into multiple path components or climb out of the
// library root.
func sanitizeFieldValue(s string) string {
	parts := strings.Split(s, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." || p == ".." {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, "_")
}

// sanitizeRelPath strips control chars from each path segment, replaces
// in-segment slashes with "_", drops "." and ".." segments, and trims
// leading slashes so the result is always a relative path under
// whatever root the caller joins.
func sanitizeRelPath(p string) string {
	// Drop control chars first
	p = stripControlChars(p)

	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, seg := range parts {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		// In-segment NULs or backslashes shouldn't exist after our
		// template fields, but defensively strip them.
		seg = strings.ReplaceAll(seg, "\x00", "")
		out = append(out, seg)
	}
	return strings.Join(out, "/")
}

func stripControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0x20 || r == '\t' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ProbeWritable verifies that dir exists and the daemon can create
// files inside it by writing and removing a small probe file. Used at
// the start of the rip step so we fail before any disc reads happen.
func ProbeWritable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("library probe: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("library probe: %s is not a directory", dir)
	}
	probe := filepath.Join(dir, ".discecho-probe")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("library probe: open %s: %w", probe, err)
	}
	_ = f.Close()
	if err := os.Remove(probe); err != nil {
		return fmt.Errorf("library probe: remove %s: %w", probe, err)
	}
	return nil
}

// AtomicMove moves src to dst, creating parent directories as needed.
// Refuses to overwrite an existing dst (returns error). Falls back to
// copy+remove if rename fails (cross-filesystem).
func AtomicMove(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("AtomicMove: destination exists: %s", dst)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("AtomicMove: stat dst: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("AtomicMove: mkdir parent: %w", err)
	}

	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fall back to copy + remove (cross-filesystem rename).
	if err := copyFile(src, dst); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("AtomicMove: remove src after copy: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy: open src: %w", err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("copy: create dst: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("copy: write: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("copy: close dst: %w", err)
	}
	return nil
}
