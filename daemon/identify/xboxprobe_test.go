package identify

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeXBE(t *testing.T, dir string, baseAddr uint32, certVA uint32, titleID uint32, regions uint32) string {
	t.Helper()
	// Lay out: header (0x180 bytes) + cert at file offset (certVA - baseAddr).
	// For the test, set certVA = baseAddr + 0x180 so cert immediately follows header.
	certFileOff := certVA - baseAddr
	totalLen := certFileOff + 0x200
	buf := make([]byte, totalLen)
	copy(buf, []byte("XBEH"))
	binary.LittleEndian.PutUint32(buf[0x104:], baseAddr)
	binary.LittleEndian.PutUint32(buf[0x118:], certVA)
	// Cert size at cert+0x0; titleID at cert+0x8; region at cert+0xC8.
	binary.LittleEndian.PutUint32(buf[certFileOff+0x0:], 0x1EC)
	binary.LittleEndian.PutUint32(buf[certFileOff+0x8:], titleID)
	binary.LittleEndian.PutUint32(buf[certFileOff+0xC8:], regions)
	path := filepath.Join(dir, "default.xbe")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write xbe: %v", err)
	}
	return path
}

func TestProbeXbox_OK(t *testing.T) {
	dir := t.TempDir()
	writeXBE(t, dir, 0x10000, 0x10180, 0x4D530002, 0x07) // all three regions
	info, err := ProbeXbox(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if info.TitleID != 0x4D530002 {
		t.Fatalf("title id: got %#x", info.TitleID)
	}
	if info.Region != "World" {
		t.Fatalf("region: got %q", info.Region)
	}
}

func TestProbeXbox_USAOnly(t *testing.T) {
	dir := t.TempDir()
	writeXBE(t, dir, 0x10000, 0x10180, 0xDEADBEEF, 0x01)
	info, err := ProbeXbox(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if info.Region != "USA" {
		t.Fatalf("region: got %q", info.Region)
	}
}

func TestProbeXbox_NoFile(t *testing.T) {
	dir := t.TempDir()
	_, err := ProbeXbox(dir)
	if !errors.Is(err, ErrNotXbox) {
		t.Fatalf("expected ErrNotXbox, got %v", err)
	}
}

func TestProbeXbox_BadMagic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "default.xbe"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ProbeXbox(dir)
	if !errors.Is(err, ErrNotXbox) {
		t.Fatalf("expected ErrNotXbox, got %v", err)
	}
}

// TestProbeXBE_Bytes exercises the bytes-only parser used by the xbox pipeline
// via the isoinfo prober path.
func TestProbeXBE_Bytes(t *testing.T) {
	dir := t.TempDir()
	path := writeXBE(t, dir, 0x10000, 0x10180, 0x4D530002, 0x01) // USA
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	info, err := ProbeXBE(data)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if info.TitleID != 0x4D530002 {
		t.Fatalf("title id: got %#x", info.TitleID)
	}
	if info.Region != "USA" {
		t.Fatalf("region: got %q", info.Region)
	}
}

func TestProbeXBE_BadMagic(t *testing.T) {
	_, err := ProbeXBE([]byte("nope"))
	if !errors.Is(err, ErrNotXbox) {
		t.Fatalf("expected ErrNotXbox, got %v", err)
	}
}

func TestProbeXBE_TooShort(t *testing.T) {
	_, err := ProbeXBE([]byte("XBEH"))
	if !errors.Is(err, ErrNotXbox) {
		t.Fatalf("expected ErrNotXbox, got %v", err)
	}
}
