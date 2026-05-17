package tools

import "testing"

func TestParseDDRescueLine_PctRescued(t *testing.T) {
	ev, ok := ParseDDRescueLine("pct rescued:   52.66%, read errors:        0,  remaining time:      2m 53s", 0)
	if !ok {
		t.Fatal("expected ok")
	}
	if ev.pct < 52.6 || ev.pct > 52.7 {
		t.Errorf("pct: got %v, want ~52.66", ev.pct)
	}
	if ev.eta != 2*60+53 {
		t.Errorf("eta: got %d, want 173", ev.eta)
	}
}

func TestParseDDRescueLine_CurrentRateLower(t *testing.T) {
	ev, ok := ParseDDRescueLine("     ipos:    9043 MB, non-trimmed:        0 B,  current rate:  44826 kB/s", 0)
	if !ok {
		t.Fatal("expected ok")
	}
	if ev.speed != "44826 kB/s" {
		t.Errorf("speed: got %q, want 44826 kB/s", ev.speed)
	}
}

func TestParseDDRescueLine_CurrentRateUpper(t *testing.T) {
	ev, ok := ParseDDRescueLine("     ipos:    1 GB,  current rate:  10.5 MB/s", 0)
	if !ok {
		t.Fatal("expected ok")
	}
	if ev.speed != "10.5 MB/s" {
		t.Errorf("speed: got %q", ev.speed)
	}
}

func TestParseDDRescueLine_RemainingTimeNA(t *testing.T) {
	// `remaining time: n/a` is what ddrescue prints during the trim phase
	// when it can't estimate. ETA must stay 0 (not parse "n/a" as bytes).
	ev, ok := ParseDDRescueLine("pct rescued:    0.10%, read errors:        0,  remaining time:        n/a", 0)
	if !ok {
		t.Fatal("expected ok for pct match")
	}
	if ev.eta != 0 {
		t.Errorf("eta: got %d, want 0 for n/a", ev.eta)
	}
}

func TestParseDDRescueLine_RescuedFallbackPct(t *testing.T) {
	// When the pct-rescued line hasn't landed yet, derive percentage
	// from the rescued-bytes line + total disc size.
	ev, ok := ParseDDRescueLine("  rescued:    343 MB,   bad areas:        0,        run time:     1m 50s", 686_000_000)
	if !ok {
		t.Fatal("expected ok")
	}
	if ev.pct < 49 || ev.pct > 51 {
		t.Errorf("pct: got %v, want ~50", ev.pct)
	}
}

func TestParseDDRescueLine_Unrelated(t *testing.T) {
	if _, ok := ParseDDRescueLine("Copying non-tried blocks... Pass 1 (forwards)", 0); ok {
		t.Error("expected !ok for phase banner")
	}
	if _, ok := ParseDDRescueLine("", 0); ok {
		t.Error("expected !ok for empty")
	}
}

func TestParseDDRescueDuration(t *testing.T) {
	cases := map[string]int{
		"15s":     15,
		"2m 30s":  150,
		"1h 5m":   3900,
		"1h":      3600,
		"":        0,
		"garbage": 0,
	}
	for in, want := range cases {
		if got := parseDDRescueDuration(in); got != want {
			t.Errorf("parseDDRescueDuration(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseDDRescueBytes(t *testing.T) {
	cases := []struct {
		num, unit string
		want      int64
	}{
		{"100", "B", 100},
		{"1.5", "kB", 1500},
		{"343", "MB", 343_000_000},
		{"1", "GB", 1_000_000_000},
		{"bad", "MB", 0},
	}
	for _, c := range cases {
		if got := parseDDRescueBytes(c.num, c.unit); got != c.want {
			t.Errorf("parseDDRescueBytes(%q, %q) = %d, want %d", c.num, c.unit, got, c.want)
		}
	}
}
