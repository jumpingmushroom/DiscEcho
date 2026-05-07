package identify_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

type datEntry struct {
	name string
	roms []datROM
}

type datROM struct {
	name string
	md5  string
}

func writeDat(t *testing.T, path string, entries []datEntry) {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("<datafile>\n")
	for _, e := range entries {
		fmt.Fprintf(&sb, "  <game name=%q>\n", e.name)
		for _, r := range e.roms {
			fmt.Fprintf(&sb, "    <rom name=%q md5=%q/>\n", r.name, r.md5)
		}
		sb.WriteString("  </game>\n")
	}
	sb.WriteString("</datafile>\n")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

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

func TestLoadRedumpDir_MultipleSystems(t *testing.T) {
	root := t.TempDir()
	psxDir := filepath.Join(root, "psx")
	if err := os.MkdirAll(psxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDat(t, filepath.Join(psxDir, "psx.dat"), []datEntry{
		{name: "Final Fantasy VII (USA) (Disc 1)", roms: []datROM{
			{name: "Final Fantasy VII (USA) (Disc 1) [SCUS-94163].bin", md5: "abc"},
		}},
	})
	satDir := filepath.Join(root, "saturn")
	if err := os.MkdirAll(satDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDat(t, filepath.Join(satDir, "saturn.dat"), []datEntry{
		{name: "Nights into Dreams (USA)", roms: []datROM{
			{name: "Nights into Dreams (USA) [MK-81088].bin", md5: "def"},
		}},
	})
	xboxDir := filepath.Join(root, "xbox")
	if err := os.MkdirAll(xboxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDat(t, filepath.Join(xboxDir, "xbox.dat"), []datEntry{
		{name: "Halo - Combat Evolved (USA)", roms: []datROM{
			{name: "Halo - Combat Evolved (USA) [4D530002].iso", md5: "ghi"},
		}},
	})

	db, err := identify.LoadRedumpDir(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := db.LookupByBootCode("SCUS-94163"); got == nil || got.Title != "Final Fantasy VII" {
		t.Fatalf("psx miss: %#v", got)
	}
	if got := db.LookupByBootCode("MK-81088"); got == nil || got.Title != "Nights into Dreams" {
		t.Fatalf("saturn miss: %#v", got)
	}
	if got := db.LookupByXboxTitleID(0x4D530002); got == nil || got.Title != "Halo - Combat Evolved" {
		t.Fatalf("xbox miss: %#v", got)
	}
	if got := db.LookupByMD5("def"); got == nil {
		t.Fatalf("md5 lookup miss")
	}
}

func TestLoadRedumpDir_MissingDirOK(t *testing.T) {
	root := filepath.Join(t.TempDir(), "doesnotexist")
	db, err := identify.LoadRedumpDir(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if db == nil {
		t.Fatal("db is nil")
	}
}

func TestLoadRedumpDir_PartialPopulation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "psx"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeDat(t, filepath.Join(root, "psx", "psx.dat"), nil)
	db, err := identify.LoadRedumpDir(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if db == nil {
		t.Fatal("db is nil")
	}
}
