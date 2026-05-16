package identify_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

// writeSyntheticIPBin builds a 256-byte IP.BIN matching Sonic Adventure
// (MK-51000) field-by-field per the official Dreamcast specification at
// https://mc.pp.se/dc/ip.bin.html. Used by the parser tests and by the
// real-device reader integration test.
func writeSyntheticIPBin(t *testing.T) string {
	t.Helper()
	var buf [256]byte
	for i := range buf {
		buf[i] = ' '
	}
	copy(buf[0x000:], "SEGA SEGAKATANA ")
	copy(buf[0x010:], "SEGA ENTERPRISES")
	copy(buf[0x020:], "1100B0011 1.000 ")
	copy(buf[0x030:], "JUE     ")
	copy(buf[0x038:], "E0000F10")
	copy(buf[0x040:], "MK-51000  ")
	copy(buf[0x04A:], "V1.000")
	copy(buf[0x050:], "19981109        ")
	copy(buf[0x060:], "1ST_READ.BIN    ")
	copy(buf[0x070:], "SEGA ENTERPRISES,LTD.   ")
	copy(buf[0x080:], "SONIC ADVENTURE                                                                                                                 ")

	path := filepath.Join(t.TempDir(), "ipbin.bin")
	if err := os.WriteFile(path, buf[:], 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseDCIPBin_SonicAdventure(t *testing.T) {
	path := writeSyntheticIPBin(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := identify.ParseDCIPBin(raw)
	if got == nil {
		t.Fatal("nil result for valid IP.BIN")
	}
	if got.ProductNumber != "MK-51000" {
		t.Errorf("ProductNumber = %q, want MK-51000", got.ProductNumber)
	}
	if got.Version != "V1.000" {
		t.Errorf("Version = %q", got.Version)
	}
	if got.SoftwareName != "SONIC ADVENTURE" {
		t.Errorf("SoftwareName = %q, want SONIC ADVENTURE (trimmed)", got.SoftwareName)
	}
}

func TestParseDCIPBin_InvalidHardwareID(t *testing.T) {
	// First 16 bytes don't match "SEGA SEGAKATANA " — not a Dreamcast disc.
	buf := bytes.Repeat([]byte{0xFF}, 256)
	got := identify.ParseDCIPBin(buf)
	if got != nil {
		t.Errorf("ParseDCIPBin returned non-nil for garbage input: %+v", got)
	}
}

func TestParseDCIPBin_TruncatedInput(t *testing.T) {
	// Less than 256 bytes — defensive rejection.
	short := bytes.Repeat([]byte{0x00}, 128)
	if got := identify.ParseDCIPBin(short); got != nil {
		t.Errorf("ParseDCIPBin returned non-nil for truncated input: %+v", got)
	}
}
