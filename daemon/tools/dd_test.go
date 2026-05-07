package tools

import (
	"testing"
)

func TestParseDDProgress_PartialLine(t *testing.T) {
	pct, speed, ok := ParseDDProgress("123456789 bytes (123 MB, 117 MiB) copied, 12.3 s, 10.0 MB/s", 1234567890)
	if !ok {
		t.Fatal("expected ok")
	}
	if pct < 9 || pct > 11 {
		t.Fatalf("pct: got %v, want ~10", pct)
	}
	if speed != "10.0 MB/s" {
		t.Fatalf("speed: got %q", speed)
	}
}

func TestParseDDProgress_TotalZero(t *testing.T) {
	pct, speed, ok := ParseDDProgress("987654 bytes copied, 1 s, 1.0 MB/s", 0)
	if !ok {
		t.Fatal("expected ok")
	}
	if pct != 0 {
		t.Fatalf("pct: got %v, want 0", pct)
	}
	if speed != "1.0 MB/s" {
		t.Fatalf("speed: got %q", speed)
	}
}

func TestParseDDProgress_FinalSummaryIgnored(t *testing.T) {
	if _, _, ok := ParseDDProgress("1+0 records in", 100); ok {
		t.Fatal("expected !ok for records line")
	}
	if _, _, ok := ParseDDProgress("", 100); ok {
		t.Fatal("expected !ok for empty line")
	}
}

func TestParseDDProgress_Malformed(t *testing.T) {
	if _, _, ok := ParseDDProgress("totally unrelated stderr line", 100); ok {
		t.Fatal("expected !ok for malformed")
	}
}
