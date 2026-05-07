package identify_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

func TestLoadRedumpDB_Sample(t *testing.T) {
	db, err := identify.LoadRedumpDB(filepath.Join("testdata", "redump-psx-sample.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if db == nil {
		t.Fatal("nil db")
	}
}

func TestLoadRedumpDB_LookupByBootCode_FoundUSA(t *testing.T) {
	db, err := identify.LoadRedumpDB(filepath.Join("testdata", "redump-psx-sample.dat"))
	if err != nil {
		t.Fatal(err)
	}
	got := db.LookupByBootCode("SCUS_004.34")
	if got == nil {
		t.Fatal("expected entry, got nil")
	}
	if got.Region != "USA" {
		t.Errorf("Region = %q, want USA", got.Region)
	}
	if got.Title != "Final Fantasy VII" {
		t.Errorf("Title = %q, want Final Fantasy VII", got.Title)
	}
	if got.BootCode != "SCUS_004.34" {
		t.Errorf("BootCode = %q, want SCUS_004.34", got.BootCode)
	}
}

func TestLoadRedumpDB_LookupByBootCode_FoundEurope(t *testing.T) {
	db, _ := identify.LoadRedumpDB(filepath.Join("testdata", "redump-psx-sample.dat"))
	got := db.LookupByBootCode("SLES_028.42")
	if got == nil {
		t.Fatal("nil")
	}
	if got.Region != "Europe" {
		t.Errorf("Region = %q, want Europe", got.Region)
	}
}

func TestLoadRedumpDB_LookupByBootCode_NotFound(t *testing.T) {
	db, _ := identify.LoadRedumpDB(filepath.Join("testdata", "redump-psx-sample.dat"))
	if got := db.LookupByBootCode("NOT_AN_ENTRY"); got != nil {
		t.Errorf("want nil for unknown code, got %+v", got)
	}
}

func TestLoadRedumpDB_LookupByMD5_RoundTrip(t *testing.T) {
	db, _ := identify.LoadRedumpDB(filepath.Join("testdata", "redump-psx-sample.dat"))
	got := db.LookupByMD5("d41d8cd98f00b204e9800998ecf8427e")
	if got == nil {
		t.Fatal("nil")
	}
	if got.BootCode != "SCUS_004.34" {
		t.Errorf("BootCode = %q via MD5 lookup, want SCUS_004.34", got.BootCode)
	}
}

func TestLoadRedumpDB_MalformedXML(t *testing.T) {
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "bad.dat")
	if err := os.WriteFile(bad, []byte("<not-xml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := identify.LoadRedumpDB(bad); err == nil {
		t.Errorf("want error on malformed XML")
	}
}

func TestLoadRedumpDB_MissingFile(t *testing.T) {
	if _, err := identify.LoadRedumpDB("testdata/no-such-file.dat"); err == nil {
		t.Errorf("want error on missing file")
	}
}
