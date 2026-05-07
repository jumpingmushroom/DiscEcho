package identify

import "testing"

func TestLooksLikeDreamcast_Match(t *testing.T) {
	out := "Sessions: 2\nSession 2 first LBA: 45000\n"
	if !looksLikeDreamcast(out) {
		t.Fatal("expected DC match")
	}
}

func TestLooksLikeDreamcast_OneSession(t *testing.T) {
	out := "Sessions: 1\nSession 2 first LBA: 0\n"
	if looksLikeDreamcast(out) {
		t.Fatal("expected no DC match for single-session")
	}
}

func TestLooksLikeDreamcast_LowSession2(t *testing.T) {
	out := "Sessions: 2\nSession 2 first LBA: 12000\n"
	if looksLikeDreamcast(out) {
		t.Fatal("expected no DC match — session 2 too early for HD-area")
	}
}
