package identify_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

func TestParseSystemCNF_PSX(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "systemcnf-psx.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := identify.ParseSystemCNF(string(b))
	if got == nil {
		t.Fatal("nil result")
	}
	if got.BootCode != "SCUS_004.34" {
		t.Errorf("BootCode = %q, want SCUS_004.34", got.BootCode)
	}
	if got.IsPS2 {
		t.Errorf("IsPS2 = true, want false for PSX content")
	}
}

func TestParseSystemCNF_PS2(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "systemcnf-ps2.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := identify.ParseSystemCNF(string(b))
	if got == nil {
		t.Fatal("nil result")
	}
	if got.BootCode != "SCES_500.51" {
		t.Errorf("BootCode = %q, want SCES_500.51 (5-digit normalised to dotted)", got.BootCode)
	}
	if !got.IsPS2 {
		t.Errorf("IsPS2 = false, want true for PS2 content")
	}
}

func TestParseSystemCNF_Empty(t *testing.T) {
	if got := identify.ParseSystemCNF(""); got != nil {
		t.Errorf("want nil for empty content, got %+v", got)
	}
}

func TestParseSystemCNF_NoBootLine(t *testing.T) {
	if got := identify.ParseSystemCNF("VER = 1.00\nVMODE = NTSC\n"); got != nil {
		t.Errorf("want nil when no BOOT line, got %+v", got)
	}
}

func TestNewSystemCNFProber_Default(t *testing.T) {
	p := identify.NewSystemCNFProber("")
	if p == nil {
		t.Fatal("nil prober")
	}
	// Calling against /dev/null should error cleanly (not panic).
	_, err := p.Probe(context.Background(), "/dev/null")
	if err == nil {
		t.Errorf("want error from /dev/null")
	}
}

func TestParseSystemCNF_LowercaseBootLine(t *testing.T) {
	// A small minority of PSX titles ship SYSTEM.CNF with lowercase
	// `boot = cdrom:\...`. The regex must accept both cases.
	content := "boot = cdrom:\\SCUS_944.61;1\r\nver = 1.00\r\n"
	got := identify.ParseSystemCNF(content)
	if got == nil {
		t.Fatal("nil result for lowercase boot line")
	}
	if got.BootCode != "SCUS_944.61" {
		t.Errorf("BootCode = %q, want SCUS_944.61", got.BootCode)
	}
	if got.IsPS2 {
		t.Errorf("IsPS2 = true, want false")
	}
}

func TestParseSystemCNF_MixedCaseBootCode(t *testing.T) {
	// (?i) on the regex permits a mixed-case file stem; strings.ToUpper
	// canonicalises it before downstream Redump / BootCodeIndex lookups.
	content := "BOOT = cdrom:\\Scus_944.61;1\r\n"
	got := identify.ParseSystemCNF(content)
	if got == nil {
		t.Fatal("nil result for mixed-case boot code")
	}
	if got.BootCode != "SCUS_944.61" {
		t.Errorf("BootCode = %q, want SCUS_944.61", got.BootCode)
	}
	if got.IsPS2 {
		t.Errorf("IsPS2 = true, want false")
	}
}

func TestParseSystemCNF_LowercasePS2Normalises(t *testing.T) {
	// Lowercase BOOT2 with a 5-digit stem must uppercase AND normalise
	// to the dotted Redump key form (sces_50051 → SCES_500.51).
	content := "boot2 = cdrom0:\\sces_50051.elf;1\r\nVMODE = PAL\r\n"
	got := identify.ParseSystemCNF(content)
	if got == nil {
		t.Fatal("nil result for lowercase PS2 5-digit code")
	}
	if got.BootCode != "SCES_500.51" {
		t.Errorf("BootCode = %q, want SCES_500.51 (lowercased + dotted)", got.BootCode)
	}
	if !got.IsPS2 {
		t.Errorf("IsPS2 = false, want true")
	}
}
