package identify

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func saturnIPBIN(productNumber, version, areaCodes string) []byte {
	buf := make([]byte, 256)
	copy(buf[0:], "SEGA SEGASATURN ")
	copy(buf[0x10:], "SEGA ENTERPRISES")
	copy(buf[0x20:], padRight(productNumber, 10))
	copy(buf[0x2A:], padRight(version, 6))
	copy(buf[0x40:], padRight(areaCodes, 10))
	return buf
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}

func TestProbeSaturnReader_OK(t *testing.T) {
	buf := saturnIPBIN("MK-81088  ", "V1.000", "JTUBKAEL  ")
	info, err := ProbeSaturnReader(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if info.ProductNumber != "MK-81088" {
		t.Fatalf("product number: got %q", info.ProductNumber)
	}
	if info.Version != "V1.000" {
		t.Fatalf("version: got %q", info.Version)
	}
	if info.Region != "JTUBKAEL" {
		t.Fatalf("region: got %q", info.Region)
	}
}

func TestProbeSaturnReader_BadMagic(t *testing.T) {
	buf := make([]byte, 256)
	copy(buf, "SOMETHING ELSE")
	_, err := ProbeSaturnReader(bytes.NewReader(buf))
	if !errors.Is(err, ErrNotSaturn) {
		t.Fatalf("expected ErrNotSaturn, got %v", err)
	}
}

func TestProbeSaturnReader_Short(t *testing.T) {
	_, err := ProbeSaturnReader(bytes.NewReader([]byte("too short")))
	if err == nil {
		t.Fatalf("expected error on short read")
	}
}
