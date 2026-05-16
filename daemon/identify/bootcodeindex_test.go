package identify_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestBootCodeIndex_LoadAndLookup(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "bootcodes_test.json"))
	if err != nil {
		t.Fatal(err)
	}
	idx, err := identify.LoadBootCodeFile(state.DiscTypePS2, raw)
	if err != nil {
		t.Fatalf("LoadBootCodeFile: %v", err)
	}

	t.Run("hit returns entry", func(t *testing.T) {
		got := idx.Lookup(state.DiscTypePS2, "SCES_534.09")
		if got == nil {
			t.Fatal("nil result for known boot code")
		}
		if got.Title != "Sly 3: Honor Among Thieves" {
			t.Errorf("Title = %q, want Sly 3: Honor Among Thieves", got.Title)
		}
		if got.Region != "Europe" {
			t.Errorf("Region = %q, want Europe", got.Region)
		}
	})

	t.Run("miss returns nil", func(t *testing.T) {
		if got := idx.Lookup(state.DiscTypePS2, "SCES_999.99"); got != nil {
			t.Errorf("got non-nil for unknown code: %+v", got)
		}
	})

	t.Run("wrong system returns nil", func(t *testing.T) {
		if got := idx.Lookup(state.DiscTypePSX, "SCES_534.09"); got != nil {
			t.Errorf("got non-nil for wrong-system lookup: %+v", got)
		}
	})

	t.Run("counts reflects entry total", func(t *testing.T) {
		if c := idx.Counts()[state.DiscTypePS2]; c != 2 {
			t.Errorf("Counts[PS2] = %d, want 2", c)
		}
	})
}

func TestBootCodeIndex_MultiSystemMerge(t *testing.T) {
	ps2, err := os.ReadFile(filepath.Join("testdata", "bootcodes_test.json"))
	if err != nil {
		t.Fatal(err)
	}
	idx, err := identify.LoadBootCodeFile(state.DiscTypePS2, ps2)
	if err != nil {
		t.Fatal(err)
	}
	psxJSON := []byte(`{"system":"PSX","source":"test","entries":{"SCUS_944.61":{"title":"Final Fantasy VII (Disc 1)","region":"USA","year":1997,"cover_url":"https://example.com/ff7.jpg"}}}`)
	if err := idx.MergeFile(state.DiscTypePSX, psxJSON); err != nil {
		t.Fatalf("MergeFile: %v", err)
	}
	if got := idx.Lookup(state.DiscTypePSX, "SCUS_944.61"); got == nil || got.Title != "Final Fantasy VII (Disc 1)" {
		t.Errorf("PSX lookup after merge: got %+v", got)
	}
	if c := idx.Counts()[state.DiscTypePSX]; c != 1 {
		t.Errorf("Counts[PSX] = %d, want 1", c)
	}
}

func TestLoadAllEmbedded_MissingFilesAreNonFatal(t *testing.T) {
	// Before Phase 9 generates the real files, all 5 lookups fail.
	// LoadAllEmbedded must still return a non-nil (empty) index and
	// record the per-system errors rather than blowing up startup.
	idx, errs := identify.LoadAllEmbedded()
	if idx == nil {
		t.Fatal("LoadAllEmbedded returned nil index")
	}
	// At least one system is expected to fail until the real files land.
	// We don't assert specific systems because Phase 9 may have generated
	// some by the time this runs.
	if len(idx.Counts()) > 0 && len(errs) > 0 {
		t.Logf("partial load: %d systems loaded, %d errored", len(idx.Counts()), len(errs))
	}
}
