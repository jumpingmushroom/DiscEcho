package identify_test

import (
	"os"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseIsoInfoListing_DVD(t *testing.T) {
	got := identify.ParseIsoInfoListing(loadFixture(t, "isoinfo-list-dvd.txt"))
	if !contains(got, "/VIDEO_TS") {
		t.Errorf("DVD listing missing /VIDEO_TS, got %v", got)
	}
	if !contains(got, "/VIDEO_TS/VIDEO_TS.IFO") {
		t.Errorf("DVD listing missing VIDEO_TS.IFO, got %v", got)
	}
	if contains(got, "/BDMV") {
		t.Errorf("DVD listing should not contain /BDMV, got %v", got)
	}
}

func TestParseIsoInfoListing_BDMV(t *testing.T) {
	got := identify.ParseIsoInfoListing(loadFixture(t, "isoinfo-list-bdmv.txt"))
	if !contains(got, "/BDMV/index.bdmv") {
		t.Errorf("BDMV listing missing /BDMV/index.bdmv, got %v", got)
	}
}

func TestParseIsoInfoListing_UHD(t *testing.T) {
	got := identify.ParseIsoInfoListing(loadFixture(t, "isoinfo-list-uhd.txt"))
	if !contains(got, "/BDMV/index.bdmv") {
		t.Errorf("UHD listing missing /BDMV/index.bdmv, got %v", got)
	}
	if !contains(got, "/AACS") {
		t.Errorf("UHD listing missing /AACS dir, got %v", got)
	}
}

func TestParseIsoInfoListing_Data(t *testing.T) {
	got := identify.ParseIsoInfoListing(loadFixture(t, "isoinfo-list-data.txt"))
	if contains(got, "/VIDEO_TS") || contains(got, "/BDMV") {
		t.Errorf("DATA listing should have neither, got %v", got)
	}
}

func TestParseIsoInfoListing_StripsVersionSuffix(t *testing.T) {
	got := identify.ParseIsoInfoListing(loadFixture(t, "isoinfo-list-dvd.txt"))
	for _, p := range got {
		if strings.HasSuffix(p, ";1") {
			t.Errorf("path %q should have ;1 stripped", p)
		}
	}
}

func TestParseIsoInfoListing_SkipsDotEntries(t *testing.T) {
	// isoinfo lists the ISO9660 "." and ".." self/parent directory
	// entries for every directory. They are never useful paths, and
	// emitting them would make "did we read any real files?" — the
	// signal the classifier's spin-up retry keys on — unreliable.
	listing := "Directory listing of /\n" +
		"d---------   0    0    0      278 Sep 26 2005 [    261 02]  . \n" +
		"d---------   0    0    0      278 Sep 26 2005 [    261 02]  .. \n" +
		"----------   0    0    0       56 Sep 22 2005 [    269 00]  SYSTEM.CNF;1 \n"
	got := identify.ParseIsoInfoListing(listing)
	if contains(got, "/.") || contains(got, "/..") {
		t.Errorf("listing should skip . and .. entries, got %v", got)
	}
	if !contains(got, "/SYSTEM.CNF") {
		t.Errorf("listing missing /SYSTEM.CNF, got %v", got)
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
